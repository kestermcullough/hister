# Hister fork — TODO

Working roadmap for this fork of asciimoo/hister. Bugs are upstream issues we can
carry patches for; see FORK.md for the patch/rebase workflow.

## Upstream bugs (fork-patch candidates)

1. **`--min-visit` flag is dead for `import-browser`.**
   Registered on the JSON `import` command (`cmd/root.go:271`,
   `importCmd.Flags().IntP("min-visit", …)`) but read by `import-browser` from its
   *own* flag set (`cmd/browser.go:207`, `cmd.Flags().GetInt("min-visit")`) → always
   errors → the visit-count filter is silently ignored. Fix: register the flag on
   `browserImportCmd` (or make it persistent). ~1 line. **[patched in fork]**

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

## Deployment notes

- Public fork is deployment-agnostic. Per-machine settings that would leak
  hostnames or IPs (e.g. `HISTER__SERVER__BASE_URL`) live in a gitignored `.env`
  file — see `compose.override.yml` for the interpolation. Build + run per
  FORK.md.
