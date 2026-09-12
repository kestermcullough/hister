import importlib.util
import shlex
import sqlite3
import tempfile
import unittest
from pathlib import Path
from unittest import mock


MODULE_PATH = Path(__file__).with_name("hister-browser-import.py")
SPEC = importlib.util.spec_from_file_location("hister_browser_import", MODULE_PATH)
IMPORTER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(IMPORTER)


class NormalizeRecurringURLTest(unittest.TestCase):
    def normalize(self, value, config=None):
        return IMPORTER.normalize_recurring_url(value, config or {})

    def test_x_status_and_media_variants_collapse(self):
        base = "https://x.com/example/status/12345"
        self.assertEqual(self.normalize(base), (base, "", False))
        self.assertEqual(
            self.normalize(base + "/mediaViewer?currentTweet=12345&currentTweetUser=example"),
            (base, "", True),
        )
        self.assertEqual(self.normalize("https://x.com/search?q=test")[1], "x_utility")

    def test_youtube_video_variants_collapse_and_nonvideo_is_excluded(self):
        canonical = "https://www.youtube.com/watch?v=abc_123"
        self.assertEqual(
            self.normalize("https://m.youtube.com/watch?app=desktop&v=abc_123&pp=tracking")[0],
            canonical,
        )
        self.assertEqual(self.normalize("https://youtu.be/abc_123?si=tracking")[0], canonical)
        self.assertEqual(self.normalize("https://www.youtube.com/?app=desktop")[1], "nonvideo_youtube")

    def test_sensitive_retail_and_configured_urls_are_excluded(self):
        self.assertEqual(
            self.normalize("https://m.facebook.com/oauth2/redirect/?state=secret&code=value")[1],
            "sensitive_session",
        )
        self.assertEqual(self.normalize("https://www.amazon.com/s?k=shoes")[1], "retail_search")
        self.assertIsNotNone(self.normalize("https://www.amazon.com/dp/B000000001")[0])
        config = {"excluded_hosts": ["dash.example.com"]}
        self.assertEqual(self.normalize("https://dash.example.com/account", config)[1], "configured")

    def test_tracking_parameters_and_fragments_are_removed(self):
        normalized, reason, changed = self.normalize(
            "https://example.com/article?id=7&utm_source=newsletter&__cf_chl_tk=challenge#comments"
        )
        self.assertEqual(normalized, "https://example.com/article?id=7")
        self.assertEqual(reason, "")
        self.assertTrue(changed)


class StageAccountingTest(unittest.TestCase):
    def test_stage_reports_exclusions_normalization_and_collapsed_duplicates(self):
        rows = [
            ("https://x.com/example/status/12345", "post", 1, 1),
            ("https://x.com/example/status/12345/mediaViewer?currentTweet=12345", "post", 2, 2),
            ("https://m.youtube.com/watch?v=abc_123&app=desktop", "video", 1, 3),
            ("https://youtu.be/abc_123?si=tracking", "video", 2, 4),
            ("https://www.google.com/search?q=private", "search", 1, 5),
            ("https://www.amazon.com/s?k=shoes", "search", 1, 6),
            ("https://m.facebook.com/oauth2/redirect/?state=secret", "oauth", 1, 7),
            ("https://www.amazon.com/dp/B000000001", "product", 1, 8),
            ("https://example.com/article?utm_source=test#part", "article", 1, 9),
            ("https://dash.example.com/account", "dashboard", 1, 10),
        ]
        source = sqlite3.connect(":memory:")
        with tempfile.TemporaryDirectory() as directory:
            destination = Path(directory) / "History"
            stats, _ = IMPORTER.write_chromium_stage(
                rows,
                destination,
                "webkit",
                source,
                {"excluded_hosts": ["dash.example.com"]},
            )
            self.assertEqual(stats["candidate_urls"], 10)
            self.assertEqual(stats["admitted_urls"], 6)
            self.assertEqual(stats["included_urls"], 4)
            self.assertEqual(stats["collapsed_urls"], 2)
            self.assertEqual(stats["normalized_urls"], 4)
            self.assertEqual(stats["excluded_google_search_urls"], 1)
            self.assertEqual(stats["excluded_retail_search_urls"], 1)
            self.assertEqual(stats["excluded_sensitive_session_urls"], 1)
            self.assertEqual(stats["excluded_configured_urls"], 1)
            staged = sqlite3.connect("file:{}?mode=ro".format(destination), uri=True)
            try:
                urls = {row[0] for row in staged.execute("SELECT url FROM urls")}
            finally:
                staged.close()
            self.assertIn("https://x.com/example/status/12345", urls)
            self.assertIn("https://www.youtube.com/watch?v=abc_123", urls)
            self.assertIn("https://example.com/article", urls)


class MonitoringAndStatsTest(unittest.TestCase):
    def test_local_sqlite_backup_is_valid(self):
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory) / "source.sqlite"
            destination = Path(directory) / "snapshot.sqlite"
            database = sqlite3.connect(str(source))
            database.execute("CREATE TABLE pages (url TEXT)")
            database.execute("INSERT INTO pages VALUES ('https://example.com')")
            database.commit()
            try:
                IMPORTER.sqlite_backup(source, destination)
            finally:
                database.close()
            snapshot = sqlite3.connect("file:{}?mode=ro".format(destination), uri=True)
            try:
                self.assertEqual(
                    snapshot.execute("SELECT url FROM pages").fetchone()[0],
                    "https://example.com",
                )
                self.assertEqual(snapshot.execute("PRAGMA quick_check").fetchone()[0], "ok")
            finally:
                snapshot.close()

    def test_local_sqlite_backup_timeout_is_bounded_and_cleans_partial_file(self):
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory) / "source.sqlite"
            destination = Path(directory) / "snapshot.sqlite"
            source.touch()
            incoming = destination.with_suffix(destination.suffix + ".incoming")

            def time_out(*args, **kwargs):
                incoming.write_bytes(b"partial")
                raise IMPORTER.subprocess.TimeoutExpired(args[0], kwargs.get("timeout", 30))

            with mock.patch.object(IMPORTER.subprocess, "run", side_effect=time_out):
                with self.assertRaisesRegex(RuntimeError, "timed out after 30 seconds"):
                    IMPORTER.sqlite_backup(source, destination)
            self.assertFalse(destination.exists())
            self.assertFalse(incoming.exists())

    def test_remote_paths_with_spaces_are_shell_quoted(self):
        source = "/Users/example/Library/Application Support/Browser/History"
        remote_tmp = "/tmp/hister.sqlite"
        command = IMPORTER.remote_backup_command(source, remote_tmp)
        self.assertEqual(
            shlex.split(command),
            ["/bin/sh", "-s", "--", source, remote_tmp],
        )

    def test_remote_rsync_timeout_is_bounded_and_cleans_partial_file(self):
        with tempfile.TemporaryDirectory() as directory:
            destination = Path(directory) / "remote.sqlite"
            incoming = destination.with_suffix(destination.suffix + ".incoming")
            source = {
                "name": "remote-brave",
                "host": "example.invalid",
                "path": "/remote/History",
            }

            def run(command, *args, **kwargs):
                if command[0] == IMPORTER.RSYNC:
                    incoming.write_bytes(b"partial")
                    raise IMPORTER.subprocess.TimeoutExpired(
                        command, kwargs.get("timeout", IMPORTER.RSYNC_TIMEOUT_SECONDS)
                    )
                return IMPORTER.subprocess.CompletedProcess(command, 0, "", "")

            with mock.patch.object(IMPORTER.subprocess, "run", side_effect=run):
                with self.assertRaisesRegex(RuntimeError, "rsync timed out after 600 seconds"):
                    IMPORTER.fetch_remote(source, destination, mock.Mock())
            self.assertFalse(destination.exists())
            self.assertFalse(incoming.exists())

    def test_parse_crawl_stats(self):
        output = "STATE\npending: 1\nin_progress: 2\ndone: 3\nfailed: 4\nskipped: 5\n"
        self.assertEqual(
            IMPORTER.parse_crawl_stats(output),
            {"pending": 1, "in_progress": 2, "done": 3, "failed": 4, "skipped": 5},
        )

    def test_no_eligible_urls_is_a_successful_noop(self):
        self.assertTrue(IMPORTER.import_command_succeeded(0, False))
        self.assertTrue(IMPORTER.import_command_succeeded(1, True))
        self.assertFalse(IMPORTER.import_command_succeeded(1, False))
        self.assertFalse(IMPORTER.import_command_succeeded(2, True))

    def test_failure_alerts_once_and_success_reports_recovery(self):
        state = {}
        self.assertFalse(IMPORTER.mark_source_failure(state, "offline", 3))
        self.assertFalse(IMPORTER.mark_source_failure(state, "offline", 3))
        self.assertTrue(IMPORTER.mark_source_failure(state, "offline", 3))
        self.assertFalse(IMPORTER.mark_source_failure(state, "offline", 3))
        self.assertTrue(IMPORTER.mark_source_success(state, 100, 90, {"included_urls": 5}))
        self.assertEqual(state["consecutive_failures"], 0)
        self.assertFalse(state["failure_alerted"])
        self.assertEqual(state["last_included_urls"], 5)


if __name__ == "__main__":
    unittest.main()
