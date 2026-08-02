# Hister fork — TODO

Working roadmap for this fork of asciimoo/hister. Bugs are upstream issues we can
carry patches for; see FORK.md for the patch/rebase workflow.

## Today — priority (August 1, 2026)

- [x] **Restore Hister from the post-power-blip crash loop without losing the
  indexed corpus.** Back up the full `hister_data` volume first, isolate the
  corrupted Bleve index, restore service, and verify search/document counts.
  **Completed August 1:** isolated one corrupt one-document segment in the
  English shard, removed only its snapshot reference, restored 87,091 English /
  97,577 total pages, and retained a verified full backup on the Extreme SSD.
- [ ] **Finish and deploy the yt-dlp transcript fix.** The fork is now rebased on
  upstream v0.17.0 and its checksum-verified standalone yt-dlp packaging and
  two-job concurrency limiter. The narrow writable-cookie-copy compatibility
  patch has passed the extractor tests. Build the complete image, then prove a
  real YouTube transcript is present in the indexed document/preview before
  marking this complete. Do not retain the old pip packaging patch; only add
  `--ignore-no-formats-error` if a current-binary test demonstrates it is needed.
- [ ] **Reconcile the fork with upstream.** The complete 13-commit fork has been
  rebased in an isolated worktree onto upstream v0.17.0. Review storage
  migrations and fork-patch overlap, run the complete test/build checks, and
  deploy only after validation against the backed-up data model.
- [ ] **Verify boot recovery.** Confirm Docker and Hister start after boot and add
  health monitoring/alerting. See operational item 13 below.
- [ ] **Design robust recurring browser imports.** Restore the existing jobs,
  then reconsider scheduling, retries, source discovery, incremental scope, and
  last-success/error reporting as a distinct ingestion system. See operational
  item 14 below.
- [ ] **Add public Google Sheets capture and fix rule precedence.** The existing
  fork extractor handles Google Docs only; Sheets/Slides remain unsupported.
  The live broad `google.*` skip rule also matches `docs.google.com`. Narrow it
  without opening noisy Google app/search URLs, implement the public Sheets
  export rewrite, and verify a real Sheet is indexed usefully.
- [ ] **Audit live skip rules separately from recurring-import filters.** The
  server's global `rules.json` currently skips Amazon, Home Depot, and several
  other retailer domains; those rules affect ordinary extension captures as
  well as imports. Decide which exclusions are genuinely wanted for live
  browsing, and move import-only noise suppression into the recurring-ingestion
  workflow instead of keeping it in the global server rule set.

## Upstream bugs (fork-patch candidates)

1. **`--min-visit` flag is dead for `import-browser`.**
   Registered on the JSON `import` command (`cmd/root.go:271`,
   `importCmd.Flags().IntP("min-visit", …)`) but read by `import-browser` from its
   *own* flag set (`cmd/browser.go:207`, `cmd.Flags().GetInt("min-visit")`) → always
   errors → the visit-count filter is silently ignored. Fix: register the flag on
   `browserImportCmd` (or make it persistent). **[fixed upstream in v0.17.0; the
   obsolete fork patch was dropped during rebase]**

2. **`crawler.delay` is a no-op on the `http` backend (no server-side rate limiting).**
   `CrawlerConfig.Delay` exists and is documented, but only the BiDi backend
   implements a wait (`server/crawler/bidi.go` captureDelay). `http.go`/`crawler.go`
   never sleep between requests, and the import/index loops don't either — so
   `crawler.delay: N` does nothing over http. Fix: honor `Delay` in the http crawler
   and/or the import loop.

3. **Extension drops captures when the server is unreachable — no retry, no offline queue.**
   `webui/ext/src/background/background.ts:513-538`: `sendPageData(…).catch(…)` just
   sets an error badge. No retry, no `storage.local`/IndexedDB queue, no replay on
   reconnect. Anything browsed while Hister is down is silently lost. Fix: queue
   failed captures locally and flush on next successful add / reconnect.

4. **`import-browser` has no date filter.**
   Only a commented-out TODO at `cmd/browser.go:124-129`. Can't scope by recency
   natively — has to be worked around by pre-filtering the staged SQLite DB with
   `sqlite3 <db> "DELETE FROM urls WHERE last_visit_time < <cutoff>"`. Fix: add
   `--after`/`--since` (Chromium `last_visit_time` is WebKit epoch µs; Firefox
   `last_visit_date` is Unix epoch µs).

5. *(minor)* **`import-browser` silently skips URLs on transient errors.**
   `cmd/browser.go:278-282`: a failed `DocumentExists` just increments `skipped`;
   `indexURL` failures only warn. Transient server/network blips drop URLs with no
   retry.

6. **Public Google Docs are not captured usefully.** **[patched in fork — `googledocs`
   extractor implements the export-URL rewrite below; Sheets/Slides still open]**
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
    cannot recover from an application crash loop. Confirm Docker Desktop starts
    at login/boot, test Hister after a controlled reboot, and surface an alert
    when `http://localhost:4433/` is unavailable or the container is unhealthy.

14. **Replace the ad-hoc browser refreshes with robust recurring ingestion.**
    The loaded launchd jobs currently cover Brave mobile/Profile 2 at 03:00 and
    Safari at 03:30. Their last successful runs were July 22; every run from July
    23 through August 1 aborted because SSH to the source Mac timed out. Add
    retry/health gating and decide which local Brave/Chrome/Vivaldi/Helium
    profiles need recurring refreshes rather than the existing one-off import
    scripts. Evaluate a single orchestrated incremental job instead of separate
    profile scripts; record last-success timestamps and imported/skipped/error
    counts so stale ingestion is obvious.
    - Decide how recurring imports handle Google and AI-search result pages
      before enabling them. Exact-URL dedup does not collapse different search
      query strings, while some AI-search modes may repeatedly reuse one URL.
      Prefer a skip/normalization rule or lightweight query-history record that
      does not recrawl Google result pages; avoid rate-limit pressure and a noisy
      index full of near-duplicate search pages.

## Deployment notes

- Public fork is deployment-agnostic. Per-machine settings that would leak
  hostnames or IPs (e.g. `HISTER__SERVER__BASE_URL`) live in a gitignored `.env`
  file — see `compose.override.yml` for the interpolation. Build + run per
  FORK.md.
