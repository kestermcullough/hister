#!/usr/bin/env python3
"""Incrementally stage and import browser history into Hister.

The source databases are never modified. Each run creates a small Chromium-
shaped staging database, excludes Google Search result pages, and feeds it to a
stable Hister persistent browser-import job. That job provides exact-URL
deduplication before crawling. Machine-specific sources live in an external
JSON config file so this script can remain safe to commit and share.
"""

import argparse
import fcntl
import json
import os
import re
import shlex
import sqlite3
import subprocess
import sys
import time
from pathlib import Path
from typing import Dict, Iterable, List, Optional, Tuple
from urllib.parse import parse_qs, parse_qsl, urlencode, urlsplit


HOME = Path.home()
STATE_DIR = HOME / "Library/Application Support/HisterBrowserImport"
CACHE_DIR = HOME / "Library/Caches/HisterBrowserImport"
LOG_PATH = HOME / "Library/Logs/HisterBrowserImport.log"
CONFIG_PATH = STATE_DIR / "config.json"
STATE_PATH = STATE_DIR / "state.json"
LOCK_PATH = STATE_DIR / "run.lock"
DOCKER = "/usr/local/bin/docker"
SSH = "/usr/bin/ssh"
RSYNC = "/usr/bin/rsync"
MAX_LOG_BYTES = 10 * 1024 * 1024
REMOTE_BACKUP_TIMEOUT_SECONDS = 45
RSYNC_TIMEOUT_SECONDS = 60

WEBKIT_EPOCH_OFFSET = 11644473600
COCOA_EPOCH_OFFSET = 978307200

REMOTE_BACKUP_SCRIPT = r"""
set -eu
src=$1
dest=$2
rm -f "$dest"
i=1
while [ "$i" -le 5 ]; do
    if /usr/bin/sqlite3 "$src" ".timeout 2000" ".backup $dest" 2>/dev/null; then
        test -s "$dest"
        exit 0
    fi
    /bin/sleep 1
    i=$((i + 1))
done

# Chromium may hold History under an exclusive lock for its entire lifetime.
# A private copy of the database together with its WAL/SHM sidecars can be
# opened consistently without closing the browser.
copy_dir=$(/usr/bin/mktemp -d /tmp/hister-browser-copy.XXXXXX)
trap '/bin/rm -rf "$copy_dir"' EXIT
/bin/cp "$src" "$copy_dir/History"
for suffix in -wal -shm -journal; do
    if test -f "$src$suffix"; then
        /bin/cp "$src$suffix" "$copy_dir/History$suffix"
    fi
done
rm -f "$dest"
/usr/bin/sqlite3 "$copy_dir/History" ".timeout 5000" ".backup $dest"
test -s "$dest"
"""

# Local Chrome is known to retain an exclusive lock for its whole lifetime.
# Do not spend most of the outer deadline retrying an online backup that cannot
# succeed: copy the database and recovery sidecars immediately, then have SQLite
# produce and integrity-check a clean snapshot from that stable private copy.
LOCAL_BACKUP_SCRIPT = r"""
set -eu
src=$1
dest=$2
copy_dir=$(/usr/bin/mktemp -d /tmp/hister-browser-copy.XXXXXX)
trap '/bin/rm -rf "$copy_dir"' EXIT
/bin/cp "$src" "$copy_dir/History"
for suffix in -wal -shm -journal; do
    if test -f "$src$suffix"; then
        /bin/cp "$src$suffix" "$copy_dir/History$suffix"
    fi
done
rm -f "$dest"
/usr/bin/sqlite3 "$copy_dir/History" ".timeout 5000" ".backup $dest"
test -s "$dest"
test "$(/usr/bin/sqlite3 "$dest" 'PRAGMA quick_check;')" = "ok"
"""

X_STATUS_RE = re.compile(r"^/([A-Za-z0-9_]+)/status/([0-9]+)(?:/.*)?$")
X_PROFILE_RE = re.compile(r"^/([A-Za-z0-9_]+)(?:/(?:photo|with_replies))?/?$")
X_UTILITY_ROOTS = {
    "compose", "explore", "home", "i", "intent", "login", "messages",
    "notifications", "search", "settings", "share", "signup",
}
YOUTUBE_ID_RE = re.compile(r"^[A-Za-z0-9_-]+$")
JWT_PATH_RE = re.compile(r"/(?:[A-Za-z0-9_-]{12,}\.){2}[A-Za-z0-9_-]{12,}(?:[/?#]|$)")
TRACKING_QUERY_KEYS = {
    "bes", "cns", "content_source", "dclid", "fbclid", "force-legacy-sct",
    "frs", "ga_order", "ga_search_query", "ga_search_type", "ga_view_type",
    "gclid", "igshid", "logging_key", "ls", "mc_cid", "mc_eid", "msclkid",
    "organic_search_click", "plkey", "pro", "ref", "ref_", "sts",
}
TRACKING_QUERY_PREFIXES = ("utm_", "__cf_chl_")
SENSITIVE_QUERY_KEYS = {
    "access_token", "auth_token", "authorization", "code", "encrypted_context",
    "f3_request_id", "flow", "oauth_token", "session", "sessionid", "session_id",
    "state", "token", "wtsid",
}


def now_iso() -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def atomic_json(path: Path, value: dict) -> None:
    tmp = path.with_suffix(path.suffix + ".tmp")
    tmp.write_text(json.dumps(value, indent=2, sort_keys=True) + "\n")
    os.replace(str(tmp), str(path))


def load_json(path: Path, default: dict) -> dict:
    try:
        return json.loads(path.read_text())
    except FileNotFoundError:
        return default


def rotate_log() -> None:
    try:
        if LOG_PATH.stat().st_size < MAX_LOG_BYTES:
            return
    except FileNotFoundError:
        return
    previous = LOG_PATH.with_suffix(LOG_PATH.suffix + ".1")
    if previous.exists():
        previous.unlink()
    LOG_PATH.rename(previous)


class Logger:
    def __init__(self) -> None:
        rotate_log()
        self.handle = LOG_PATH.open("a", buffering=1)

    def log(self, message: str) -> None:
        line = "[{}] {}".format(now_iso(), message)
        print(line)
        self.handle.write(line + "\n")

    def close(self) -> None:
        self.handle.close()


def safe_name(value: str) -> str:
    return re.sub(r"[^A-Za-z0-9_.-]+", "-", value).strip("-")


def is_google_search(raw_url: str) -> bool:
    try:
        parsed = urlsplit(raw_url)
    except ValueError:
        return False
    host = (parsed.hostname or "").lower().rstrip(".")
    google_host = host == "google.com" or host.startswith("google.") or host.startswith("www.google.")
    return google_host and (parsed.path == "/search" or parsed.path.startswith("/search/"))


def is_nonvideo_youtube(raw_url: str) -> bool:
    try:
        parsed = urlsplit(raw_url)
    except ValueError:
        return False
    host = (parsed.hostname or "").lower().rstrip(".")
    if host == "youtu.be":
        return parsed.path.strip("/") == ""
    if host != "youtube.com" and not host.endswith(".youtube.com"):
        return False
    path = parsed.path.rstrip("/")
    return not (path == "/watch" or path.startswith("/shorts/") or path.startswith("/live/"))


def is_session_utility_url(raw_url: str) -> bool:
    try:
        parsed = urlsplit(raw_url)
    except ValueError:
        return False
    host = (parsed.hostname or "").lower().rstrip(".")
    if (host.startswith("accounts.") or host.startswith("login.") or
            host.startswith("auth.") or host.startswith("order.")):
        return True
    components = [component.lower() for component in parsed.path.split("/") if component]
    utility_components = {
        "account", "accounts", "auth", "authorize", "cart", "checkout", "login",
        "logout", "oauth", "oauth2", "order", "orders", "order-history", "register",
        "signin", "sign-in", "two_factor", "two-factor", "two_step_verification",
        "verification", "verify",
    }
    utility_prefixes = ("login", "oauth", "signin", "sign-in", "two_factor", "two-factor")
    if any(component in utility_components or component.startswith(utility_prefixes)
           for component in components):
        return True
    query_keys = {key.lower() for key, _ in parse_qsl(parsed.query, keep_blank_values=True)}
    return bool(query_keys & SENSITIVE_QUERY_KEYS) or bool(JWT_PATH_RE.search(parsed.path))


def is_retail_search(raw_url: str) -> bool:
    try:
        parsed = urlsplit(raw_url)
    except ValueError:
        return False
    host = (parsed.hostname or "").lower().rstrip(".")
    path = parsed.path.rstrip("/").lower()
    return any((
        (host == "amazon.com" or host.endswith(".amazon.com")) and path in ("/s", "/gp/search"),
        (host == "homedepot.com" or host.endswith(".homedepot.com")) and path.startswith("/s/"),
        (host == "lowes.com" or host.endswith(".lowes.com")) and path.startswith("/search"),
        (host == "walmart.com" or host.endswith(".walmart.com")) and path.startswith("/search"),
        (host == "target.com" or host.endswith(".target.com")) and path == "/s",
        (host == "ebay.com" or host.endswith(".ebay.com")) and path.startswith("/sch"),
        (host == "bestbuy.com" or host.endswith(".bestbuy.com")) and path.startswith("/site/searchpage"),
        (host == "costco.com" or host.endswith(".costco.com")) and path.startswith("/catalogsearch"),
    ))


def host_matches(host: str, configured_host: str) -> bool:
    configured_host = configured_host.lower().strip().lstrip("*.").rstrip(".")
    return bool(configured_host) and (host == configured_host or host.endswith("." + configured_host))


def normalize_recurring_url(raw_url: str, config: dict) -> Tuple[Optional[str], str, bool]:
    """Return (canonical URL, exclusion reason, changed).

    Exclusions and canonicalization apply only to recurring browser imports;
    live extension captures and Hister's global rules are unchanged.
    """
    try:
        parsed = urlsplit(raw_url)
    except ValueError:
        return None, "invalid", False
    if parsed.scheme not in ("http", "https") or not parsed.hostname:
        return None, "invalid", False
    host = parsed.hostname.lower().rstrip(".")

    if any(host_matches(host, value) for value in config.get("excluded_hosts", [])):
        return None, "configured", False
    if any(raw_url.startswith(prefix) for prefix in config.get("excluded_url_prefixes", [])):
        return None, "configured", False
    if is_google_search(raw_url):
        return None, "google_search", False
    if is_retail_search(raw_url):
        return None, "retail_search", False

    if host == "youtu.be":
        video_id = parsed.path.strip("/").split("/", 1)[0]
        if not video_id or not YOUTUBE_ID_RE.fullmatch(video_id):
            return None, "nonvideo_youtube", False
        canonical = "https://www.youtube.com/watch?v=" + video_id
        return canonical, "", canonical != raw_url

    if host == "youtube.com" or host.endswith(".youtube.com"):
        path = parsed.path.rstrip("/")
        if path == "/watch":
            video_id = parse_qs(parsed.query).get("v", [""])[0]
            if not video_id or not YOUTUBE_ID_RE.fullmatch(video_id):
                return None, "nonvideo_youtube", False
            canonical = "https://www.youtube.com/watch?v=" + video_id
            return canonical, "", canonical != raw_url
        for prefix in ("/shorts/", "/live/"):
            if path.startswith(prefix):
                video_id = path[len(prefix):].split("/", 1)[0]
                if video_id and YOUTUBE_ID_RE.fullmatch(video_id):
                    canonical = "https://www.youtube.com" + prefix + video_id
                    return canonical, "", canonical != raw_url
        return None, "nonvideo_youtube", False

    if host in ("x.com", "www.x.com", "mobile.twitter.com", "twitter.com", "www.twitter.com"):
        status_match = X_STATUS_RE.fullmatch(parsed.path)
        if status_match:
            canonical = "https://x.com/{}/status/{}".format(status_match.group(1), status_match.group(2))
            return canonical, "", canonical != raw_url
        profile_match = X_PROFILE_RE.fullmatch(parsed.path)
        if profile_match and profile_match.group(1).lower() not in X_UTILITY_ROOTS:
            canonical = "https://x.com/{}".format(profile_match.group(1))
            return canonical, "", canonical != raw_url
        return None, "x_utility", False

    if is_session_utility_url(raw_url):
        return None, "sensitive_session", False

    query_pairs = parse_qsl(parsed.query, keep_blank_values=True)
    filtered_pairs = [
        (key, value) for key, value in query_pairs
        if not key.lower().startswith(TRACKING_QUERY_PREFIXES) and key.lower() not in TRACKING_QUERY_KEYS
    ]
    changed = bool(parsed.fragment) or len(filtered_pairs) != len(query_pairs)
    if not changed:
        return raw_url, "", False
    canonical = parsed._replace(query=urlencode(filtered_pairs), fragment="").geturl()
    return canonical, "", canonical != raw_url


def source_cutoff(source_state: dict, config: dict, now_epoch: int) -> int:
    high_watermark = int(source_state.get("high_watermark_epoch", 0) or 0)
    if high_watermark:
        return max(0, high_watermark - int(config.get("overlap_days", 3)) * 86400)
    return now_epoch - int(config.get("initial_lookback_days", 30)) * 86400


def sqlite_backup(source: Path, destination: Path) -> None:
    """Create a bounded, lock-aware snapshot of a live browser database.

    Python's sqlite3.Connection.backup() can wait forever while Chrome holds an
    exclusive lock. Snapshot the database and recovery sidecars to a private
    directory first, then make and integrity-check a clean SQLite backup there.
    The outer timeout guarantees that a browser can never wedge the scheduler.
    """
    incoming = destination.with_suffix(destination.suffix + ".incoming")
    try:
        for path in (destination, incoming):
            if path.exists():
                path.unlink()
        result = subprocess.run(
            ["/bin/sh", "-s", "--", str(source), str(incoming)],
            input=LOCAL_BACKUP_SCRIPT,
            text=True,
            capture_output=True,
            timeout=30,
        )
        if result.returncode != 0 or not incoming.exists() or incoming.stat().st_size == 0:
            raise RuntimeError(
                "local sqlite backup failed: {}".format(
                    result.stderr.strip() or "status {}".format(result.returncode)
                )
            )
        os.replace(str(incoming), str(destination))
    except subprocess.TimeoutExpired as exc:
        raise RuntimeError("local sqlite backup timed out after 30 seconds") from exc
    finally:
        if incoming.exists():
            incoming.unlink()


def probe_remote(host: str, logger: Logger) -> bool:
    delays = (0, 5, 15)
    for attempt, delay in enumerate(delays, 1):
        if delay:
            time.sleep(delay)
        result = subprocess.run(
            [SSH, "-o", "BatchMode=yes", "-o", "ConnectTimeout=8", host, "/usr/bin/true"],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.PIPE,
            text=True,
        )
        if result.returncode == 0:
            return True
        logger.log("remote {} unavailable (attempt {}/{}): {}".format(
            host, attempt, len(delays), result.stderr.strip() or "ssh failed"
        ))
    return False


def remote_backup_command(source_path: str, remote_tmp: str) -> str:
    return "/bin/sh -s -- {} {}".format(shlex.quote(source_path), shlex.quote(remote_tmp))


def fetch_remote(source: dict, destination: Path, logger: Logger) -> None:
    host = source["host"]
    remote_tmp = "/tmp/hister-browser-import-{}-{}.sqlite".format(os.getuid(), safe_name(source["name"]))
    remote_command = remote_backup_command(source["path"], remote_tmp)
    command = [
        SSH, "-o", "BatchMode=yes", "-o", "ConnectTimeout=15", host,
        remote_command,
    ]
    try:
        result = subprocess.run(
            command,
            input=REMOTE_BACKUP_SCRIPT,
            text=True,
            capture_output=True,
            timeout=REMOTE_BACKUP_TIMEOUT_SECONDS,
        )
    except subprocess.TimeoutExpired as exc:
        raise RuntimeError(
            "remote sqlite backup timed out after {} seconds".format(
                REMOTE_BACKUP_TIMEOUT_SECONDS
            )
        ) from exc
    if result.returncode != 0:
        raise RuntimeError("remote sqlite backup failed: {}".format(result.stderr.strip() or result.returncode))
    incoming = destination.with_suffix(destination.suffix + ".incoming")
    try:
        if incoming.exists():
            incoming.unlink()
        try:
            result = subprocess.run(
                [
                    RSYNC,
                    "-a",
                    "--timeout=30",
                    "-e",
                    "{} -o BatchMode=yes -o ConnectTimeout=8 "
                    "-o ServerAliveInterval=5 -o ServerAliveCountMax=2".format(SSH),
                    "{}:{}".format(host, remote_tmp),
                    str(incoming),
                ],
                capture_output=True,
                text=True,
                timeout=RSYNC_TIMEOUT_SECONDS,
            )
        except subprocess.TimeoutExpired as exc:
            raise RuntimeError(
                "rsync timed out after {} seconds".format(RSYNC_TIMEOUT_SECONDS)
            ) from exc
        if result.returncode != 0 or not incoming.exists() or incoming.stat().st_size == 0:
            raise RuntimeError("rsync failed: {}".format(result.stderr.strip() or result.returncode))
        os.replace(str(incoming), str(destination))
    finally:
        if incoming.exists():
            incoming.unlink()
        try:
            subprocess.run(
                [SSH, "-o", "BatchMode=yes", "-o", "ConnectTimeout=8", host, "rm", "-f", remote_tmp],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
                timeout=15,
            )
        except (OSError, subprocess.TimeoutExpired):
            # The local snapshot is already safe; a stale remote temp file is
            # preferable to wedging or masking the result of the import run.
            pass


def create_chromium_stage(source: Path, destination: Path, cutoff_epoch: int, config: dict) -> Tuple[dict, int]:
    cutoff_webkit = (cutoff_epoch + WEBKIT_EPOCH_OFFSET) * 1_000_000
    src = sqlite3.connect("file:{}?immutable=1&mode=ro".format(source), uri=True)
    rows = src.execute(
        """SELECT url, COALESCE(title, ''), COALESCE(visit_count, 0),
                  COALESCE(last_visit_time, 0)
           FROM urls
           WHERE (url LIKE 'http://%' OR url LIKE 'https://%')
             AND last_visit_time >= ?
           ORDER BY last_visit_time""",
        (cutoff_webkit,),
    )
    return write_chromium_stage(rows, destination, "webkit", src, config)


def create_safari_stage(source: Path, destination: Path, cutoff_epoch: int, config: dict) -> Tuple[dict, int]:
    cutoff_cocoa = cutoff_epoch - COCOA_EPOCH_OFFSET
    src = sqlite3.connect("file:{}?immutable=1&mode=ro".format(source), uri=True)
    rows = src.execute(
        """SELECT hi.url,
                  COALESCE((SELECT title FROM history_visits
                            WHERE history_item = hi.id AND title IS NOT NULL
                            ORDER BY visit_time DESC LIMIT 1), ''),
                  COALESCE(hi.visit_count, 0),
                  MAX(hv.visit_time)
           FROM history_items hi
           JOIN history_visits hv ON hv.history_item = hi.id
           WHERE (hi.url LIKE 'http://%' OR hi.url LIKE 'https://%')
           GROUP BY hi.id
           HAVING MAX(hv.visit_time) >= ?
           ORDER BY MAX(hv.visit_time)""",
        (cutoff_cocoa,),
    )
    return write_chromium_stage(rows, destination, "cocoa", src, config)


def write_chromium_stage(
    rows: Iterable[Tuple[str, str, int, int]],
    destination: Path,
    timestamp_kind: str,
    src: sqlite3.Connection,
    config: dict,
) -> Tuple[dict, int]:
    if destination.exists():
        destination.unlink()
    dst = sqlite3.connect(str(destination))
    dst.execute("PRAGMA journal_mode=OFF")
    dst.execute("PRAGMA synchronous=OFF")
    dst.execute(
        """CREATE TABLE urls (
               id INTEGER PRIMARY KEY AUTOINCREMENT,
               url TEXT NOT NULL UNIQUE,
               title TEXT DEFAULT '',
               visit_count INTEGER DEFAULT 0,
               typed_count INTEGER DEFAULT 0,
               last_visit_time INTEGER DEFAULT 0,
               hidden INTEGER DEFAULT 0
           )"""
    )
    stats = {
        "candidate_urls": 0,
        "admitted_urls": 0,
        "included_urls": 0,
        "normalized_urls": 0,
        "collapsed_urls": 0,
        "excluded_invalid_urls": 0,
        "excluded_google_search_urls": 0,
        "excluded_nonvideo_youtube_urls": 0,
        "excluded_x_utility_urls": 0,
        "excluded_retail_search_urls": 0,
        "excluded_sensitive_session_urls": 0,
        "excluded_configured_urls": 0,
    }
    high_watermark = 0
    batch: List[Tuple[str, str, int, int]] = []
    try:
        for raw_url, title, visit_count, raw_timestamp in rows:
            stats["candidate_urls"] += 1
            raw_timestamp = int(raw_timestamp or 0)
            if timestamp_kind == "cocoa":
                unix_epoch = int(raw_timestamp + COCOA_EPOCH_OFFSET)
                webkit_timestamp = (unix_epoch + WEBKIT_EPOCH_OFFSET) * 1_000_000
            else:
                unix_epoch = int(raw_timestamp / 1_000_000 - WEBKIT_EPOCH_OFFSET)
                webkit_timestamp = raw_timestamp
            high_watermark = max(high_watermark, unix_epoch)
            normalized_url, exclusion, changed = normalize_recurring_url(raw_url, config)
            if normalized_url is None:
                stats["excluded_{}_urls".format(exclusion)] += 1
                continue
            stats["admitted_urls"] += 1
            if changed:
                stats["normalized_urls"] += 1
            batch.append((normalized_url, title or "", int(visit_count or 0), webkit_timestamp))
            if len(batch) >= 1000:
                dst.executemany(
                    "INSERT OR REPLACE INTO urls (url, title, visit_count, last_visit_time) VALUES (?, ?, ?, ?)",
                    batch,
                )
                batch = []
        if batch:
            dst.executemany(
                "INSERT OR REPLACE INTO urls (url, title, visit_count, last_visit_time) VALUES (?, ?, ?, ?)",
                batch,
            )
        stats["included_urls"] = int(dst.execute("SELECT count(*) FROM urls").fetchone()[0])
        stats["collapsed_urls"] = stats["admitted_urls"] - stats["included_urls"]
        dst.commit()
    finally:
        dst.close()
        src.close()
    return stats, high_watermark


def parse_crawl_stats(output: str) -> dict:
    clean = re.sub(r"\x1b\[[0-9;]*m", "", output)
    stats = {}
    for key in ("pending", "in_progress", "done", "failed", "skipped"):
        match = re.search(r"^{}:\s*([0-9]+)\s*$".format(key), clean, re.MULTILINE)
        if match:
            stats[key] = int(match.group(1))
    return stats


def crawl_job_stats(job_id: str) -> dict:
    result = subprocess.run(
        [DOCKER, "exec", "-e", "NO_COLOR=1", "hister", "/hister/hister", "crawl", "show", job_id],
        stdin=subprocess.DEVNULL,
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        return {}
    return parse_crawl_stats(result.stdout)


def import_command_succeeded(returncode: int, no_urls_found: bool) -> bool:
    return returncode == 0 or (returncode == 1 and no_urls_found)


def docker_import(source: dict, stage: Path, job_id: str, request_delay: int, logger: Logger) -> dict:
    container_path = "/hister/data/imports/recurring/{}-History".format(safe_name(source["name"]))
    before = crawl_job_stats(job_id)
    try:
        with stage.open("rb") as stage_input:
            result = subprocess.run(
                [DOCKER, "exec", "-i", "hister", "/bin/sh", "-c", 'cat > "$1"', "sh", container_path],
                stdin=stage_input,
                stdout=logger.handle,
                stderr=subprocess.STDOUT,
            )
        if result.returncode != 0:
            raise RuntimeError("container staging failed with status {}".format(result.returncode))
        command = [
            DOCKER, "exec", "-i", "-e", "HISTER__CRAWLER__DELAY={}".format(request_delay),
            "hister", "/hister/hister", "import", "browser",
            "--job-id", job_id, source.get("browser", "chromium"), container_path,
        ]
        process = subprocess.Popen(
            command,
            stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
        )
        no_urls = False
        assert process.stdout is not None
        for line in process.stdout:
            logger.handle.write(line)
            if "No URLs found to import" in line:
                no_urls = True
        returncode = process.wait()
        if not import_command_succeeded(returncode, no_urls):
            raise RuntimeError("Hister import failed with status {}".format(returncode))
        after = crawl_job_stats(job_id)
        outcome = {"request_delay_seconds": request_delay}
        if before and after:
            before_total = sum(before.values())
            after_total = sum(after.values())
            outcome.update({
                "newly_queued_urls": max(0, after_total - before_total),
                "completed_urls": max(0, after.get("done", 0) - before.get("done", 0)),
                "failed_urls": max(0, after.get("failed", 0) - before.get("failed", 0)),
                "skipped_urls": max(0, after.get("skipped", 0) - before.get("skipped", 0)),
                "remaining_pending_urls": after.get("pending", 0) + after.get("in_progress", 0),
            })
        return outcome
    finally:
        subprocess.run(
            [DOCKER, "exec", "hister", "rm", "-f", container_path],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )


def notify(title: str, message: str) -> None:
    script = 'display notification {} with title {}'.format(json.dumps(message), json.dumps(title))
    subprocess.run(["/usr/bin/osascript", "-e", script], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


def mark_source_success(source_state: dict, run_started: int, high_watermark: int, stats: dict) -> bool:
    recovered = int(source_state.get("consecutive_failures", 0) or 0) > 0
    source_state.update({
        "last_success_at": now_iso(),
        "last_success_epoch": run_started,
        "high_watermark_epoch": max(
            int(source_state.get("high_watermark_epoch", 0) or 0), high_watermark
        ),
        "last_error": "",
        "consecutive_failures": 0,
        "failure_alerted": False,
    })
    for key, value in stats.items():
        source_state["last_" + key] = value
    return recovered


def mark_source_failure(source_state: dict, error: str, alert_after: int) -> bool:
    failures = int(source_state.get("consecutive_failures", 0) or 0) + 1
    should_alert = failures >= alert_after and not bool(source_state.get("failure_alerted", False))
    source_state.update({
        "last_error": error,
        "last_error_at": now_iso(),
        "consecutive_failures": failures,
        "failure_alerted": bool(source_state.get("failure_alerted", False)) or should_alert,
    })
    return should_alert


def parse_overrides(values: List[str]) -> Dict[str, Path]:
    result: Dict[str, Path] = {}
    for value in values:
        if "=" not in value:
            raise ValueError("--source-db must be NAME=PATH")
        name, path = value.split("=", 1)
        result[name] = Path(path).expanduser()
    return result


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--config", default=str(CONFIG_PATH))
    parser.add_argument("--dry-run", action="store_true", help="stage and report without Docker or state updates")
    parser.add_argument("--source-db", action="append", default=[], metavar="NAME=PATH",
                        help="use an existing snapshot for a named source")
    args = parser.parse_args()

    STATE_DIR.mkdir(parents=True, exist_ok=True)
    CACHE_DIR.mkdir(parents=True, exist_ok=True)
    LOG_PATH.parent.mkdir(parents=True, exist_ok=True)
    logger = Logger()
    lock_handle = LOCK_PATH.open("w")
    try:
        try:
            fcntl.flock(lock_handle, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            logger.log("another import run is active; exiting")
            return 0

        config = load_json(Path(args.config), {})
        if not config.get("sources"):
            raise RuntimeError("no sources configured in {}".format(args.config))
        job_id = config.get("job_id", "browser-import-recurring")
        if not job_id.startswith("browser-import-"):
            raise RuntimeError("job_id must start with browser-import-")
        request_delay = int(config.get("request_delay_seconds", 1))
        if request_delay < 0:
            raise RuntimeError("request_delay_seconds must not be negative")
        failure_alert_after = max(1, int(config.get("failure_alert_after", 3)))
        overrides = parse_overrides(args.source_db)
        state = load_json(STATE_PATH, {"sources": {}})
        state.setdefault("sources", {})
        run_started = int(time.time())
        report = {
            "started_at": now_iso(),
            "dry_run": args.dry_run,
            "job_id": job_id,
            "sources": {},
        }
        remote_status: Dict[str, bool] = {}
        remote_snapshots: Dict[str, Path] = {}
        remote_snapshot_errors: Dict[str, str] = {}
        successes = 0
        failures = 0
        logger.log("browser import start{}".format(" (dry run)" if args.dry_run else ""))

        # Capture every remote source before any potentially long crawl starts.
        # This keeps a laptop that is awake briefly from disappearing between
        # the first and second source.
        for source in config["sources"]:
            name = source["name"]
            if name in overrides or not source["kind"].startswith("remote-"):
                continue
            raw = CACHE_DIR / (safe_name(name) + "-raw.sqlite")
            host = source["host"]
            try:
                if host not in remote_status:
                    remote_status[host] = probe_remote(host, logger)
                if not remote_status[host]:
                    raise RuntimeError("remote host unavailable")
                fetch_remote(source, raw, logger)
                remote_snapshots[name] = raw
            except Exception as exc:
                remote_snapshot_errors[name] = str(exc)
                if raw.exists():
                    raw.unlink()

        for source in config["sources"]:
            name = source["name"]
            source_report = {"status": "starting"}
            report["sources"][name] = source_report
            source_state = state["sources"].setdefault(name, {})
            raw = CACHE_DIR / (safe_name(name) + "-raw.sqlite")
            stage = CACHE_DIR / (safe_name(name) + "-History")
            try:
                if name in overrides:
                    source_path = overrides[name]
                    if not source_path.is_file():
                        raise RuntimeError("override snapshot does not exist: {}".format(source_path))
                    logger.log("{}: using snapshot override {}".format(name, source_path))
                elif source["kind"].startswith("remote-"):
                    if name in remote_snapshot_errors:
                        raise RuntimeError(remote_snapshot_errors[name])
                    if name not in remote_snapshots:
                        raise RuntimeError("remote snapshot unavailable")
                    source_path = remote_snapshots[name]
                else:
                    local_path = Path(source["path"]).expanduser()
                    if not local_path.is_file():
                        raise RuntimeError("source database does not exist: {}".format(local_path))
                    sqlite_backup(local_path, raw)
                    source_path = raw

                cutoff = source_cutoff(source_state, config, run_started)
                if source["kind"].endswith("safari"):
                    stage_stats, high_watermark = create_safari_stage(source_path, stage, cutoff, config)
                else:
                    stage_stats, high_watermark = create_chromium_stage(source_path, stage, cutoff, config)
                source_report.update({
                    "status": "staged" if args.dry_run else "importing",
                    "cutoff_epoch": cutoff,
                    "data_high_watermark_epoch": high_watermark,
                    "stage_bytes": stage.stat().st_size,
                })
                source_report.update(stage_stats)
                logger.log("{}: staged {included_urls} distinct URLs from {candidate_urls} candidates "
                           "({normalized_urls} normalized, {collapsed_urls} collapsed; excluded "
                           "Google {excluded_google_search_urls}, YouTube {excluded_nonvideo_youtube_urls}, "
                           "X utility {excluded_x_utility_urls}, retail search {excluded_retail_search_urls}, "
                           "sensitive/session {excluded_sensitive_session_urls}, configured "
                           "{excluded_configured_urls}), {bytes} bytes".format(
                    name, bytes=stage.stat().st_size, **stage_stats
                ))
                import_stats = {}
                if not args.dry_run and stage_stats["included_urls"]:
                    import_stats = docker_import(source, stage, job_id, request_delay, logger)
                    import_stats["not_newly_queued_urls"] = max(
                        0, stage_stats["included_urls"] - import_stats.get("newly_queued_urls", 0)
                    )
                    source_report.update(import_stats)
                    logger.log("{}: queued {newly} new, {not_new} already known/rule-skipped; "
                               "crawl done {done}, failed {failed}, skipped {skipped}, pending {pending}".format(
                        name,
                        newly=import_stats.get("newly_queued_urls", "unknown"),
                        not_new=import_stats.get("not_newly_queued_urls", "unknown"),
                        done=import_stats.get("completed_urls", "unknown"),
                        failed=import_stats.get("failed_urls", "unknown"),
                        skipped=import_stats.get("skipped_urls", "unknown"),
                        pending=import_stats.get("remaining_pending_urls", "unknown"),
                    ))
                if not args.dry_run:
                    source_report["status"] = "success"
                    recovered = mark_source_success(
                        source_state, run_started, high_watermark, {**stage_stats, **import_stats}
                    )
                    if recovered:
                        notify("Hister browser import recovered", name)
                successes += 1
            except Exception as exc:
                failures += 1
                source_report.update({"status": "failed", "error": str(exc)})
                logger.log("{}: FAILED: {}".format(name, exc))
                if not args.dry_run:
                    if mark_source_failure(source_state, str(exc), failure_alert_after):
                        notify("Hister browser import repeatedly failing", "{}: {}".format(name, exc))
            finally:
                if raw.exists():
                    raw.unlink()

        report.update({
            "finished_at": now_iso(),
            "successes": successes,
            "failures": failures,
            "result": "success" if failures == 0 else ("partial" if successes else "failed"),
        })
        atomic_json(STATE_DIR / ("dry-run-report.json" if args.dry_run else "last-run.json"), report)
        if not args.dry_run:
            state["last_run"] = {
                "finished_at": report["finished_at"],
                "result": report["result"],
                "successes": successes,
                "failures": failures,
            }
            atomic_json(STATE_PATH, state)
            stale_hours = int(config.get("stale_warning_hours", 48))
            stale = []
            for source in config["sources"]:
                source_state = state["sources"].get(source["name"], {})
                last_success = int(source_state.get("last_success_epoch", 0) or 0)
                if not last_success or run_started - last_success >= stale_hours * 3600:
                    stale.append(source["name"])
            if stale:
                notify("Hister browser imports are stale", ", ".join(stale))
        logger.log("browser import {}: {} succeeded, {} failed".format(report["result"], successes, failures))
        return 0 if successes else 1
    except Exception as exc:
        logger.log("browser import fatal error: {}".format(exc))
        if not args.dry_run:
            notify("Hister browser import failed", str(exc))
        return 1
    finally:
        lock_handle.close()
        logger.close()


if __name__ == "__main__":
    sys.exit(main())
