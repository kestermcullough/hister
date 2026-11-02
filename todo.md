# Hister fork — TODO

Working roadmap for this fork of asciimoo/hister. Bugs are upstream issues we can
carry patches for; see FORK.md for the patch/rebase workflow.

## Current priority (November 2, 2026)

- [x] **Restore the October 28 index failure.** The service started after reboot,
  then began failing at 20:55 UTC, about four hours later. Five English index
  segment files were missing. A consistent full-volume backup was made on the
  Extreme SSD before testing removal of their snapshot references on a copy.
  Applied the tested repair with approval and retained 120,026 documents and
  19 rules. The cause of the missing files remains unknown.
- [x] **Recover browser history after the outage.** Used a separate persistent
  job, `browser-import-recovery-20261102`, so earlier completion records could
  not suppress recovery. Scanned laptop Brave and Safari with a 14-day overlap,
  laptop Chrome, two inactive Brave profiles, and local Chrome. Existing URLs
  were skipped. The index increased to 120,783 documents. The job finished with
  664 done, 1,359 skipped, 148 failed, and no pending URLs. Failed URLs remain
  in the job, mostly because of access restrictions, missing pages, or non-HTML
  responses. Another laptop refused SSH, so its history was not read. Added
  laptop Chrome to the regular import configuration. SQLite integrity, search,
  stored text, and a cold restart passed. Hister is healthy with zero crash
  restarts. All 12 recurring-import tests passed. Deleted the temporary SSD
  backup and recovery files after verification, as requested.
- [x] **Check origin and upstream without changing the deployed release.**
  Fetched both remotes. `origin/master` is `102495de`; local HEAD has 35 commits
  absent from origin, while origin has 103 absent locally. A plain pull would
  combine different fork histories and could discard local behavior if resolved
  incorrectly. Upstream has released v0.19.0, v0.20.0, and v0.21.0. The latest
  stable tag is `161bd84e` (v0.21.0). No merge, rebase, push, or upgrade was done.
- [ ] **Test a v0.21.0 upgrade on a disposable data copy.** Prioritize the security
  fixes, browser capture improvements, native Safari imports, diagnostics, and
  correct reporting of failed document submissions. Preserve the recurring
  importer, Docs/Sheets extractor, yt-dlp process and temporary-directory fixes,
  cookie support, and snapshot settings. Review changed CLI output and exit
  codes against the import script. The Bleve index version remains 8, but the
  SQLite timestamp migration still needs testing. No reviewed change is a
  confirmed fix for the missing segment files.

## Previous deployment work (September 12, 2026)

- [x] **Back up and deploy the yt-dlp private-runtime-temp fix on v0.17.0.**
  **Completed August 31:** created a 6.8 GiB checksum-verified full-volume
  backup on the Extreme SSD and preserved the previous image as
  `hister:fork-pre-ytdlp-tmpfix`. The initial backup attempt exposed Docker's
  default log driver duplicating the tar stream into `Docker.raw`; the failed
  partial archive was removed and the successful retry used
  `--log-driver=none`, leaving internal free space stable at 27 GiB. The
  deployed extractor gives each metadata/subtitle invocation a private runtime
  directory and removes it after success or timeout. Success and forced-timeout
  regression tests pass. A live captioned-video refresh stored 24,531
  characters, a transcript-only phrase returned that video first in CLI search,
  `/tmp` remained empty, and Hister stayed healthy at 104,804 documents / 19
  rules with zero restarts.
- [x] **Stage the v0.18.0 upgrade in an isolated worktree and disposable data
  copy.** **Completed September 2:** rebased all 27 fork commits onto the v0.18.0
  extractor SDK in an Extreme SSD worktree. The UI build, all Go packages, 11
  recurring-import tests, arm64 image build, Google Docs/Sheets/Defuddle focused
  tests, and yt-dlp timeout/temp cleanup tests pass. A verified backup restored
  into a disposable SSD corpus migrated to database version 2 / index version 8,
  retained all 104,804 indexed URLs, passed SQLite integrity, survived a cold
  restart with zero restart-loop count, and returned search results. A YouTube
  `text:Transcript` query returned saved videos with transcript text. Testing
  also found and fixed v0.18 CLI JSON's trailing comma and made `-F text` request
  stored text. The live v0.17 volume/image remain untouched. Current upstream
  master also has native Safari history import, but it is newer than v0.18.0;
  evaluate it separately instead of deploying an unreleased branch.
- [x] **Back up and deploy the tested v0.18.0 integration.** **Completed
  September 12:** captured fresh Brave/Safari/Chrome history first, then made a
  consistent 9.72 GiB full-volume archive (6.85 GiB compressed) on the Extreme
  SSD and verified SHA-256
  `18b4d392de6f7580a16b251bb8a231a6e7a35cfdff77757917d1013091fed975`.
  Preserved v0.17 as `hister:fork-pre-v0.18-20260912`, deployed the non-root
  v0.18 release image, and verified database version 2 / index version 8,
  SQLite integrity, search JSON and stored transcript text. All 106,701
  pre-cutover documents and 19 rules survived migration and a controlled cold
  restart with zero crash restarts. Post-upgrade imports added 28 more
  documents, ending at 106,729. A production `--force` refresh of an available
  captioned YouTube video stored 15,400 characters including its transcript,
  left no yt-dlp runtime files under `/tmp`, and kept the service healthy with
  zero restarts.
- [ ] **Reconcile the public fork remote.** Local `master` now contains the
  deployed v0.18 integration. The November 2 fetch found another fork history
  on origin, with 35 local-only commits and 103 remote-only commits. See the
  current update assessment above. Review the exact push target and obtain
  explicit approval before replacing remote history.
- [x] **Monitor the next scheduled recurring-browser run.** Two manual September
  12 runs completed Brave, Safari, and local Chrome under v0.18 with one-second
  request pacing, deduplication, and zero pending URLs. The live run also exposed
  slow remote transfers: remote SQLite creation and rsync now have separate
  bounded timeouts, a ten-minute total allowance, and cleanup for rsync's hidden
  partial files. **Completed September 12:** the unattended 21:00 run completed
  all three sources successfully. Brave queued and completed one new URL;
  Safari deduplicated all 300 eligible URLs; local Chrome was a clean no-op;
  every source ended with zero failed and zero pending URLs.
- [ ] **Plan extension offline capture recovery.** The live extension still has
  no durable queue when Hister is unavailable; this is distinct from recurring
  browser-history recovery.

## Completed recovery and integration work (August 1–31, 2026)

- [x] **Restore Hister from the post-power-blip crash loop without losing the
  indexed corpus.** Back up the full `hister_data` volume first, isolate the
  corrupted Bleve index, restore service, and verify search/document counts.
  **Completed August 1:** isolated one corrupt one-document segment in the
  English shard, removed only its snapshot reference, restored 87,091 English /
  97,577 total pages, and retained a verified full backup on the Extreme SSD.
- [x] **Finish and deploy the yt-dlp transcript fix.** The fork is now rebased on
  upstream v0.17.0 and its checksum-verified standalone yt-dlp packaging and
  two-job concurrency limiter. The narrow writable-cookie-copy compatibility
  patch has passed the extractor tests. A disposable extension-style API test
  indexed a real captioned YouTube video with a 3,669-character transcript.
  That test also found and fixed the standalone binary's need for an executable
  256 MiB `/tmp`, plus missing extractor initialization in `hister index`.
  **Completed August 1:** deployed v0.17.0 and reindexed an existing live
  YouTube document through the fixed CLI path; its stored text now contains the
  transcript plus duration/uploader metadata. Hister remained healthy with zero
  restarts. The old pip packaging patch and `--ignore-no-formats-error` were not
  retained. **Timeout hardening completed August 2:** both metadata and subtitle
  yt-dlp commands now run in an isolated process group and kill the entire tree
  at deadline, with a one-second final pipe-drain bound. A regression test
  reproduces a descendant retaining stdout and proves that it neither blocks nor
  survives cancellation. Bare media-site homepages are also rejected even when
  they carry query parameters. The release is deployed with a dedicated rollback
  image; a live forced refresh returned within 4.46 seconds and left no yt-dlp
  descendants. The refreshed document contains 3,006 characters including its
  transcript and uploader metadata, and a CLI search for a transcript-only
  phrase returns the video. (`search -F text` currently displays an empty field
  because that v0.17 CLI path does not request `include_text`; the indexed text
  and full-text search are working. The staged v0.18 fork fixes this CLI display
  bug and its invalid trailing-comma JSON output.)
- [x] **Reconcile the fork with upstream.** The complete 13-commit fork has been
  rebased in an isolated worktree onto upstream v0.17.0. Review storage
  migrations and fork-patch overlap, run the complete test/build checks, and
  deploy only after validation against the backed-up data model. A disposable
  migration of all 97,577 recovered documents and its restart both passed with
  the count unchanged. **Completed August 1:** deployed the full v0.17.0 update;
  the live one-time migration completed across every shard, search returned
  results in the browser, and normal capture continued at 97,588 documents.
- [x] **Verify boot recovery.** Confirm Docker and Hister start after boot and add
  health monitoring/alerting. See operational item 13 below. **August 2:** Docker
  Desktop `AutoStart` is enabled; Hister and Defuddle both use
  `restart: unless-stopped`. Installed a read-only launchd health monitor that
  checks Docker, both container health states, restart counts, and the Hister
  HTTP endpoint every minute. It alerts after three consecutive failures and on
  recovery; simulated failure/recovery and macOS notification tests passed.
  **Completed August 2:** after a controlled full Mac reboot, Docker restored
  both Hister containers automatically on the first verification attempt. Both
  were healthy with zero restart failures; the corpus remained at 97,610
  documents with all 19 rules intact. The permanent monitor subsequently
  reported healthy with zero consecutive failures.
- [x] **Design robust recurring browser imports.** Restore the existing jobs,
  then reconsider scheduling, retries, source discovery, incremental scope, and
  last-success/error reporting as a distinct ingestion system. See operational
  item 14 below. **Completed August 2:** replaced the two brittle full-history
  jobs with one locked incremental orchestrator using an upstream-style named
  persistent crawl job. It tracks a high-water mark per source with a three-day
  overlap, retries an unavailable remote host on a bounded schedule, records
  per-source success/error/count state, alerts after 48 hours stale, rotates its
  log, and leaves only one small staging database per source. A launchd job runs
  at 03:00, 09:00, 15:00, and 21:00 so the remote MacBook can be caught while
  awake. Recurring-only filters exclude Google Search results, non-video
  YouTube pages, and account/cart/login/order utility pages without changing
  live-extension rules. The controlled catch-up completed at 97,788 documents;
  a repeat run added zero duplicates, and a launchd test correctly reported the
  offline MacBook while still processing local Chrome with exit code 0.
- [x] **Add public Google Sheets capture and fix rule precedence.**
  **Completed August 1:** the Google extractor now fetches public Sheets through
  the static visualization endpoint, stores tab-separated rows plus an offline
  table preview, and respects the selected `gid`. Narrowed the broad Google skip
  rule so `docs.google.com` is allowed while Search, account, Gmail, and Calendar
  URLs remain excluded. A previously skipped public Sheet from browser history
  indexed as 363 rows / 49,966 characters and appeared first in a live search.
  Slides remain unsupported.
- [x] **Audit live skip rules separately from recurring-import filters.**
  **Completed August 1:** confirmed global rules are absolute exclusions used by
  both the extension/server and browser imports; priority rules do not override
  skips. Removed Amazon, Walmart, Target, eBay, Best Buy, Costco, Home Depot, and
  Lowe's from the global set. Retained mail, financial/payment, tracking redirect,
  Google account/search noise, and private-network protections. Import-only
  Google/query normalization belongs in the recurring-ingestion workflow and is
  intentionally not represented as a global rule.
  **Follow-up completed August 31:** split the combined Google Search/redirect
  rule. Live Google Search pages are now eligible for capture, while Google
  redirect URLs, accounts, Gmail, Calendar, financial services, and private
  networks remain blocked. Recurring imports continue to suppress Google and
  AI-search noise independently.

## Upstream bugs (fork-patch candidates)

1. **`--min-visit` flag is dead for `import-browser`.**
   Registered on the JSON `import` command (`cmd/root.go:271`,
   `importCmd.Flags().IntP("min-visit", …)`) but read by `import-browser` from its
   *own* flag set (`cmd/browser.go:207`, `cmd.Flags().GetInt("min-visit")`) → always
   errors → the visit-count filter is silently ignored. Fix: register the flag on
   `browserImportCmd` (or make it persistent). **[fixed upstream in v0.17.0; the
   obsolete fork patch was dropped during rebase]**

2. **`crawler.delay` rate-limiting audit.** **[closed August 2: existing code works]**
   The earlier audit missed the delay in both ordinary BFS
   (`server/crawler/crawler.go`) and persistent BFS
   (`server/crawler/persistent.go`). It runs after robots, validator, and
   exact-document skip checks but before the network fetch, so known URLs do not
   incur the delay. Recurring browser imports now set a one-second delay only on
   their `docker exec` process; live extension captures keep their current latency.

3. **Extension drops captures when the server is unreachable — no retry, no offline queue.**
   `webui/ext/src/background/background.ts:513-538`: `sendPageData(…).catch(…)` just
   sets an error badge. No retry, no `storage.local`/IndexedDB queue, no replay on
   reconnect. Anything browsed while Hister is down is silently lost. Fix: queue
   failed captures locally and flush on next successful add / reconnect.

4. **`import-browser` has no date filter.** **[fixed upstream in v0.18.0]**
   Only a commented-out TODO at `cmd/browser.go:124-129`. Can't scope by recency
   natively — has to be worked around by pre-filtering the staged SQLite DB with
   `sqlite3 <db> "DELETE FROM urls WHERE last_visit_time < <cutoff>"`. Fix: add
   `--after`/`--since` (Chromium `last_visit_time` is WebKit epoch µs; Firefox
   `last_visit_date` is Unix epoch µs). Upstream added `--start-date`; adopt it
   when the fork moves to v0.18.0, then reassess how much staging SQL remains
   necessary.

5. *(minor)* **`import-browser` silently skips URLs on transient errors.**
   `cmd/browser.go:278-282`: a failed `DocumentExists` just increments `skipped`;
   `indexURL` failures only warn. Transient server/network blips drop URLs with no
   retry.

6. **Public Google Docs are not captured usefully.** **[patched in fork — `googledocs`
   handles Docs HTML exports and Sheets visualization tables; Slides still open]**
   `docs.google.com/document/d/…/edit` renders content entirely via JS. The http
   crawler backend gets an empty shell → Readability fails → Basic saves a stub
   with almost no text. Two workable fixes:
   - **Export-URL rewrite in a new `google-docs` extractor.** Before crawl, rewrite
     `/document/d/<id>/edit*` → `/document/d/<id>/export?format=html` (or `txt`).
     If the doc is public, the export URL returns static HTML/text — no JS needed.
     Runs before defuddle in the chain. Sheets/Slides have similar `export?format=…`
     paths.
   - **Fall back to a JS-capable backend** (`chromedp` or `bidi`) for a hard-coded
     list of JS-heavy hosts. Bigger change; needs per-URL backend selection which
     the crawler doesn't have today (`crawler.backend` is single-value in config).
   Prefer the extractor route — cheap, targeted, works for the shared-link case.

## Features / enhancements

7. **YouTube transcripts — enable ytdlp server-side + a cookie source.**
   Server-side ytdlp is the correct architecture (the community-maintained tool
   that tracks YouTube's client-spoof/pot dance). Verified this is not a hot-path
   tax: `YtdlpExtractor.Match()` at `server/extractor/extractors/ytdlp/ytdlp.go:116`
   is a URL host check against a small fixed list (youtube.com, youtu.be, vimeo,
   dailymotion, twitch, bilibili, nicovideo, soundcloud, bandcamp, mixcloud, ted,
   archive.org, media.ccc.de, tiktok, +peertube substring). Non-YouTube URLs bail
   in nanoseconds — `Extract()` is never called.
   - **Cookies plumbing is already there** — `cookieArgs()` at `ytdlp.go:105`
     supports two config options and passes them straight to yt-dlp:
       - `extractors.ytdlp.options.cookies_file: <path>` → `--cookies <file>`
         (Netscape/Mozilla format).
       - `extractors.ytdlp.options.cookies_from_browser: <name>` →
         `--cookies-from-browser <name>`. Container-side this only works if the
         relevant browser profile is present in the container FS; the file route
         is more portable for a containerised deployment.
   - **Server-side ytdlp is the primary path going forward.** The extension-side
     YouTube patch (`webui/ext/src/modules/youtube.ts`) is expected to be dropped
     when a store-shipped extension replaces the custom capture side.
   - **Timeout follow-up found and fixed August 2.** During the recurring-import test,
     yt-dlp was incorrectly invoked for a YouTube homepage carrying query
     parameters. `exec.CommandContext` timed out its parent, but a PyInstaller
     descendant retained the output pipe and kept the call blocked until it was
     terminated manually. The recurring importer admits only actual YouTube
     video paths; the extractor now rejects queried homepages and applies the
     configured deadline to the complete yt-dlp process tree on Unix and Windows.
   The deterministic child-pipe regression test and live deployment both pass.
   - **Runtime-temp follow-up August 26:** repeated live imports eventually made
     the standalone PyInstaller binary fail while decompressing native modules.
     The deployed binary had previously worked, pointing to abandoned `_MEI`
     extraction trees filling the 256 MiB tmpfs after forced timeouts. The
     prepared fix gives every metadata/subtitle command its own `TMPDIR` and
     removes it after success, failure, or process-tree timeout. Unit and live
     transcript tests plus a fresh data-volume backup are required before
     deployment.

8. **Import Edge's ~200+ open tabs from SNSS session files.** Edge history DB has
   few rows (400 URLs over 2yr, lightly used); the open tabs live in
   `…/Edge/User Data/Default/Sessions/Tabs_*` & `Session_*` (~1,054 URLs), which
   `import-browser` does not read. Needs an SNSS parser → feed URLs to
   `hister index`.

9. **Defuddle content engine.** Already patched in as a Node sidecar in
   `defuddle-svc/`; extractor chain is `defuddle → readability → basic`. Item
   kept for tracking any follow-up bugs or refinements.

10. **Reclaim pre-`strip_images` snapshots.** Pages indexed before the strip patch
    still hold base64 images. `hister reindex` (re-crawls under strip_images) +
    `hister cleanup` (drop orphaned blobs).

11. **Imports lose original visit timestamps.** `hister index`/`import-browser`
    date imported pages to *import time*, not the browser's original
    `last_visit_time`, so a bulk import all clusters at the crawl moment instead
    of slotting chronologically into the History timeline (imports end up buried
    below newer live captures and are hard to spot as "new"). Fix: thread the
    visit timestamp through import/`indexURL` and set the document's `added`
    accordingly.

## Operational reliability

12. **Recover safely from the August 2026 hard-reboot index corruption.**
    **[completed August 1, 2026]**
    Docker did restart the `hister` service after the power blip, but Hister
    crash-looped while opening one damaged final segment in the English Bleve
    index (`fieldDvLoc too small`). SQLite passed `PRAGMA integrity_check`.
    Preserved a verified full data-volume backup plus the original index root on
    the Extreme SSD, removed only the bad one-document segment reference, and
    reconciled the recovered 87,091 English documents with 97,577 total pages
    across all language shards.

13. **Verify boot recovery and add a visible health check/alert.**
    `restart: unless-stopped` starts the container once Docker is running, but it
    cannot recover from an application crash loop. Docker Desktop `AutoStart`
    and both container restart policies are confirmed. The installed
    `com.kester.hister-health-monitor` LaunchAgent runs every 60 seconds and
    checks Docker, Hister, Defuddle, restart counts, and
    `http://localhost:4433/`; it waits for three consecutive failures before a
    desktop alert and notifies on recovery. Script:
    `~/bin/hister-health-monitor.sh`; log:
    `~/Library/Logs/HisterHealthMonitor.log`. Simulated failure/recovery and
    notification tests passed August 2. A controlled full reboot then passed on
    the first post-login verification attempt: both containers were healthy with
    zero restart failures, HTTP responded, and all 97,610 documents / 19 rules
    remained intact. The one-shot verifier was removed after the test; its small
    before/after reports remain under `~/Library/Application Support/HisterMonitor/`.

14. **Replace the ad-hoc browser refreshes with robust recurring ingestion.**
    **[completed August 2, 2026]** Installed the version-controlled
    `contrib/hister-browser-import.py` via `~/bin/hister-browser-import.py`, with
    machine-local configuration/state in
    `~/Library/Application Support/HisterBrowserImport/`. One launchd job,
    `com.kester.hister-browser-import`, runs at 03:00/09:00/15:00/21:00 and
    processes remote Brave mobile/Profile 2, remote Safari, and local Chrome.
    It uses a lock, atomic SQLite snapshots, per-source high-water marks plus a
    three-day overlap, three bounded SSH attempts shared by remote sources, a
    stable `browser-import-recurring` persistent Hister crawl job, JSON
    last-run/source state, 48-hour stale alerts, and a 10 MiB rotating log.
    The fork adds `hister import browser --job-id` so automation selects that
    upstream datastore job explicitly instead of relying on prompts.
    - Google/AI Search result pages are excluded only from recurring staging;
      no global skip rule was added, and public Docs/Sheets remain eligible.
      Actual video URLs remain eligible, while YouTube home/search/account pages
      are excluded. Account, login, cart, checkout, and order-history paths are
      likewise import-only exclusions; retail product pages remain allowed.
    - The initial 30-day catch-up staged 2,200 useful URLs and excluded 1,318
      noisy/utility URLs. The completed persistent job recorded 680 done, 1,510
      already-indexed skips, 151 fetch failures, and zero pending rows. Corpus
      count rose from 97,610 to 97,788; a second overlapping run added zero.
    - The old Brave/Safari LaunchAgents and scripts were disabled and removed.
      Their 334 MiB host staging tree was archived as a checksum-verified 53.7
      MiB zstd file on the Extreme SSD, and 388 MiB of duplicate container
      staging was removed. The new cache is under 1 MiB and container staging is
      empty after each run. The remote MacBook was offline during the final
      launchd test; this is now visible as per-source failure state while local
      ingestion continues normally.
    - **Hardening pass August 2:** recurring staging now canonicalizes X
      status/media/profile variants and YouTube video variants; strips fragments,
      analytics, Cloudflare-challenge, and commerce-attribution parameters; and
      excludes Google/X/retail search pages, OAuth/login/2FA/token/session URLs,
      configured private dashboards, and retail cart actions. These remain
      import-only decisions. The locked Brave database is captured by copying
      `History` plus WAL/SHM into remote temporary storage; all remote snapshots
      are now acquired before any long crawl begins. The importer records
      candidate/normalized/collapsed/excluded/new/done/failed/skipped/pending
      counts, alerts once after three consecutive failures, and announces
      recovery. Nine focused tests cover the policy, accounting, quoted remote
      paths, and successful no-op behavior.
    - The first paced live pass completed Brave: 1,925 candidates became 874
      distinct staged URLs after 917 Google searches and other noise were
      excluded; 631 were newly queued, 243 were already known/rule-skipped, 450
      fetched successfully, 76 failed at the site level, 105 skipped, and zero
      remain pending. Safari's live snapshot was deferred because the MacBook
      slept after the Brave crawl; the revised snapshot-first ordering will catch
      it on the next reachable run. Local Chrome's sole remaining private URL was
      correctly rejected by global rules and is now treated as a successful no-op.
    - **Follow-up August 26–31:** a local Chrome online backup remained blocked
      for more than ten hours because Python's SQLite backup API has no effective
      outer deadline while Chrome holds an exclusive lock. The first correction
      bounded it at 30 seconds, preventing another scheduler wedge, but its five
      futile online retries consumed the deadline before fallback on August 31.
      Local Chrome now goes directly to a private database/WAL/SHM/journal copy,
      then creates and integrity-checks a clean snapshot. Eleven focused tests
      pass; an actual locked-Chrome snapshot containing 141 URLs completed with
      `PRAGMA quick_check=ok` in under one second. The old process is gone and
      launchd is idle after normal exit. The 15:00 scheduled run then completed
      all three sources successfully: Safari had no new URLs, Brave indexed 8
      with one site-level failure, and local Chrome indexed 21 with 19 site-level
      failures (primarily non-HTML GitHub API/raw-file responses, 404s, and one
      429). All queues reached zero pending, one-second request pacing was active,
      and no importer/source failure was recorded.

## Deployment notes

- Public fork is deployment-agnostic. Per-machine settings that would leak
  hostnames or IPs (e.g. `HISTER__SERVER__BASE_URL`) live in a gitignored `.env`
  file — see `compose.override.yml` for the interpolation. Build + run per
  FORK.md.
