#!/usr/bin/env python3
"""Incrementally stage and import browser history into Hister.

The source databases are never modified. Each run creates a small Chromium-
shaped staging database, excludes Google Search result pages, and feeds it to a
stable Hister v0.17 persistent browser-import job. That job provides exact-URL
deduplication before crawling. Machine-specific sources live in an external
JSON config file so this script can remain safe to commit and share.
"""

import argparse
import fcntl
import json
import os
import re
import shutil
import sqlite3
import subprocess
import sys
import time
from pathlib import Path
from typing import Dict, Iterable, List, Optional, Tuple
from urllib.parse import urlsplit


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

WEBKIT_EPOCH_OFFSET = 11644473600
COCOA_EPOCH_OFFSET = 978307200

REMOTE_BACKUP_SCRIPT = r"""
set -eu
src=$1
dest=$2
rm -f "$dest"
i=1
while [ "$i" -le 5 ]; do
    if /usr/bin/sqlite3 "$src" ".backup $dest" 2>/dev/null; then
        test -s "$dest"
        exit 0
    fi
    /bin/sleep 1
    i=$((i + 1))
done
exit 1
"""


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
    if host.startswith("accounts.") or host.startswith("login.") or host.startswith("auth."):
        return True
    components = [component.lower() for component in parsed.path.split("/") if component]
    utility_components = {
        "account", "accounts", "auth", "authorize", "cart", "checkout", "login",
        "logout", "oauth", "order", "orders", "order-history", "signin", "sign-in",
    }
    return any(component in utility_components for component in components)


def source_cutoff(source_state: dict, config: dict, now_epoch: int) -> int:
    high_watermark = int(source_state.get("high_watermark_epoch", 0) or 0)
    if high_watermark:
        return max(0, high_watermark - int(config.get("overlap_days", 3)) * 86400)
    return now_epoch - int(config.get("initial_lookback_days", 30)) * 86400


def sqlite_backup(source: Path, destination: Path) -> None:
    if destination.exists():
        destination.unlink()
    src = sqlite3.connect("file:{}?mode=ro".format(source), uri=True, timeout=30)
    dst = sqlite3.connect(str(destination))
    try:
        src.backup(dst)
    finally:
        dst.close()
        src.close()


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


def fetch_remote(source: dict, destination: Path, logger: Logger) -> None:
    host = source["host"]
    remote_tmp = "/tmp/hister-browser-import-{}-{}.sqlite".format(os.getuid(), safe_name(source["name"]))
    command = [
        SSH, "-o", "BatchMode=yes", "-o", "ConnectTimeout=15", host,
        "/bin/sh", "-s", "--", source["path"], remote_tmp,
    ]
    result = subprocess.run(command, input=REMOTE_BACKUP_SCRIPT, text=True, capture_output=True)
    if result.returncode != 0:
        raise RuntimeError("remote sqlite backup failed: {}".format(result.stderr.strip() or result.returncode))
    incoming = destination.with_suffix(destination.suffix + ".incoming")
    try:
        if incoming.exists():
            incoming.unlink()
        result = subprocess.run(
            [RSYNC, "-a", "{}:{}".format(host, remote_tmp), str(incoming)],
            capture_output=True,
            text=True,
        )
        if result.returncode != 0 or not incoming.exists() or incoming.stat().st_size == 0:
            raise RuntimeError("rsync failed: {}".format(result.stderr.strip() or result.returncode))
        os.replace(str(incoming), str(destination))
    finally:
        if incoming.exists():
            incoming.unlink()
        subprocess.run(
            [SSH, "-o", "BatchMode=yes", "-o", "ConnectTimeout=8", host, "rm", "-f", remote_tmp],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )


def create_chromium_stage(source: Path, destination: Path, cutoff_epoch: int) -> Tuple[int, int, int, int, int]:
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
    return write_chromium_stage(rows, destination, "webkit", src)


def create_safari_stage(source: Path, destination: Path, cutoff_epoch: int) -> Tuple[int, int, int, int, int]:
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
    return write_chromium_stage(rows, destination, "cocoa", src)


def write_chromium_stage(
    rows: Iterable[Tuple[str, str, int, int]],
    destination: Path,
    timestamp_kind: str,
    src: sqlite3.Connection,
) -> Tuple[int, int, int, int, int]:
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
    included = 0
    excluded_google = 0
    excluded_nonvideo_youtube = 0
    excluded_session_utility = 0
    high_watermark = 0
    batch: List[Tuple[str, str, int, int]] = []
    try:
        for raw_url, title, visit_count, raw_timestamp in rows:
            raw_timestamp = int(raw_timestamp or 0)
            if timestamp_kind == "cocoa":
                unix_epoch = int(raw_timestamp + COCOA_EPOCH_OFFSET)
                webkit_timestamp = (unix_epoch + WEBKIT_EPOCH_OFFSET) * 1_000_000
            else:
                unix_epoch = int(raw_timestamp / 1_000_000 - WEBKIT_EPOCH_OFFSET)
                webkit_timestamp = raw_timestamp
            high_watermark = max(high_watermark, unix_epoch)
            if is_google_search(raw_url):
                excluded_google += 1
                continue
            if is_nonvideo_youtube(raw_url):
                excluded_nonvideo_youtube += 1
                continue
            if is_session_utility_url(raw_url):
                excluded_session_utility += 1
                continue
            batch.append((raw_url, title or "", int(visit_count or 0), webkit_timestamp))
            if len(batch) >= 1000:
                dst.executemany(
                    "INSERT OR REPLACE INTO urls (url, title, visit_count, last_visit_time) VALUES (?, ?, ?, ?)",
                    batch,
                )
                included += len(batch)
                batch = []
        if batch:
            dst.executemany(
                "INSERT OR REPLACE INTO urls (url, title, visit_count, last_visit_time) VALUES (?, ?, ?, ?)",
                batch,
            )
            included += len(batch)
        dst.commit()
    finally:
        dst.close()
        src.close()
    return included, excluded_google, excluded_nonvideo_youtube, excluded_session_utility, high_watermark


def docker_import(source: dict, stage: Path, job_id: str, logger: Logger) -> None:
    container_path = "/hister/data/imports/recurring/{}-History".format(safe_name(source["name"]))
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
            DOCKER, "exec", "-i", "hister", "/hister/hister", "import", "browser",
            "--job-id", job_id, source.get("browser", "chromium"), container_path,
        ]
        result = subprocess.run(command, stdin=subprocess.DEVNULL, stdout=logger.handle, stderr=subprocess.STDOUT)
        if result.returncode != 0:
            raise RuntimeError("Hister import failed with status {}".format(result.returncode))
    finally:
        subprocess.run(
            [DOCKER, "exec", "hister", "rm", "-f", container_path],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )


def notify(title: str, message: str) -> None:
    script = 'display notification {} with title {}'.format(json.dumps(message), json.dumps(title))
    subprocess.run(["/usr/bin/osascript", "-e", script], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


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
        successes = 0
        failures = 0
        logger.log("browser import start{}".format(" (dry run)" if args.dry_run else ""))

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
                    host = source["host"]
                    if host not in remote_status:
                        remote_status[host] = probe_remote(host, logger)
                    if not remote_status[host]:
                        raise RuntimeError("remote host unavailable")
                    fetch_remote(source, raw, logger)
                    source_path = raw
                else:
                    local_path = Path(source["path"]).expanduser()
                    if not local_path.is_file():
                        raise RuntimeError("source database does not exist: {}".format(local_path))
                    sqlite_backup(local_path, raw)
                    source_path = raw

                cutoff = source_cutoff(source_state, config, run_started)
                if source["kind"].endswith("safari"):
                    included, excluded_google, excluded_nonvideo_youtube, excluded_session_utility, high_watermark = create_safari_stage(source_path, stage, cutoff)
                else:
                    included, excluded_google, excluded_nonvideo_youtube, excluded_session_utility, high_watermark = create_chromium_stage(source_path, stage, cutoff)
                source_report.update({
                    "status": "staged" if args.dry_run else "importing",
                    "cutoff_epoch": cutoff,
                    "included_urls": included,
                    "excluded_google_search_urls": excluded_google,
                    "excluded_nonvideo_youtube_urls": excluded_nonvideo_youtube,
                    "excluded_session_utility_urls": excluded_session_utility,
                    "data_high_watermark_epoch": high_watermark,
                    "stage_bytes": stage.stat().st_size,
                })
                logger.log("{}: staged {} URLs (excluded {} Google Search, {} non-video YouTube, {} session/utility), {} bytes".format(
                    name, included, excluded_google, excluded_nonvideo_youtube, excluded_session_utility,
                    stage.stat().st_size
                ))
                if not args.dry_run and included:
                    docker_import(source, stage, job_id, logger)
                if not args.dry_run:
                    source_report["status"] = "success"
                    source_state.update({
                        "last_success_at": now_iso(),
                        "last_success_epoch": run_started,
                        "high_watermark_epoch": max(
                            int(source_state.get("high_watermark_epoch", 0) or 0), high_watermark
                        ),
                        "last_included_urls": included,
                        "last_excluded_google_search_urls": excluded_google,
                        "last_excluded_nonvideo_youtube_urls": excluded_nonvideo_youtube,
                        "last_excluded_session_utility_urls": excluded_session_utility,
                        "last_error": "",
                        "consecutive_failures": 0,
                    })
                successes += 1
            except Exception as exc:
                failures += 1
                source_report.update({"status": "failed", "error": str(exc)})
                logger.log("{}: FAILED: {}".format(name, exc))
                if not args.dry_run:
                    source_state.update({
                        "last_error": str(exc),
                        "last_error_at": now_iso(),
                        "consecutive_failures": int(source_state.get("consecutive_failures", 0) or 0) + 1,
                    })
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
