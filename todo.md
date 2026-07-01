# Hister fork — TODO

Working roadmap for the kestermcullough/hister fork. Bugs are upstream issues we
can carry patches for (see FORK.md for the patch/rebase workflow).

## Upstream bugs (fork-patch candidates)

1. **`--min-visit` flag is dead for `import-browser`.**
   Registered on the JSON `import` command (`cmd/root.go:271`,
   `importCmd.Flags().IntP("min-visit", …)`) but read by `import-browser` from its
   *own* flag set (`cmd/browser.go:207`, `cmd.Flags().GetInt("min-visit")`) → always
   errors → the visit-count filter is silently ignored. Fix: register the flag on
   `browserImportCmd` (or make it persistent). ~1 line.

2. **`crawler.delay` is a no-op on the `http` backend (no server-side rate limiting).**
   `CrawlerConfig.Delay` exists and is documented, but only the BiDi backend
   implements a wait (`server/crawler/bidi.go` captureDelay). `http.go`/`crawler.go`
   never sleep between requests, and the import/index loops don't either — so
   `crawler.delay: N` does nothing over http. (We had to throttle the history import
   externally.) Fix: honor `Delay` in the http crawler and/or the import loop.

3. **Extension drops captures when the server is unreachable — no retry, no offline queue.**
   `webui/ext/src/background/background.ts:513-538`: `sendPageData(…).catch(…)` just
   sets an error badge. No retry, no `storage.local`/IndexedDB queue, no replay on
   reconnect. Anything browsed while Hister is down (e.g. the WSL crash on
   2026-07-01) is lost. Fix: queue failed captures locally and flush on next
   successful add / reconnect. (High value — this is our data-loss gap.)

4. **`import-browser` has no date filter.**
   Only a commented-out TODO (`cmd/browser.go:124-129`). Can't scope by recency
   natively — worked around by pre-filtering the staged SQLite DB. Fix: add
   `--after`/`--since` (and wire up `--min-visit` at the same time, see #1).

5. *(minor)* **`import-browser` silently skips URLs on transient errors.**
   `cmd/browser.go:278-282`: a failed `DocumentExists` just increments `skipped`;
   `indexURL` failures only warn. Transient server/network blips drop URLs with no retry.

## Features / enhancements

6. **Import Edge's ~200+ open tabs from SNSS session files.** (task tracker #1)
   Edge history DB has only 402 URLs (2yr span, lightly used); the open tabs live in
   `…/Edge/User Data/Default/Sessions/Tabs_*` & `Session_*` (~1,054 URLs), which
   `import-browser` does not read. Needs an SNSS parser → feed URLs to `hister index`.

7. **Native YouTube transcript extractor** — replace ytdlp for captions (faster, drops
   the yt-dlp dependency). Fetch YouTube's transcript/innertube endpoint directly in Go.

8. **Obsidian Web Clipper / Defuddle content engine** — better extraction on hard pages
   than readability. Realistic route: a small Node `defuddle` sidecar the extractor
   shells out to (same external-tool pattern as ytdlp), not a Go reimplementation.

9. **Reclaim pre-`strip_images` snapshots.** Pages indexed before the strip patch still
   hold base64 images. `hister reindex` (re-crawls under strip_images) + `hister cleanup`
   (drop orphaned blobs).

10. **Imports lose original visit timestamps.** `hister index`/`import-browser` date
    imported pages to *import time*, not the browser's original `last_visit_time`, so a
    bulk import all clusters at the crawl moment instead of slotting chronologically into
    the History timeline (imports end up buried below newer live captures and are hard to
    spot as "new"). Fix: thread the visit timestamp through import/`indexURL` and set the
    document's `added` accordingly.

## In progress

- **Helium history import — 2-week window, filtered.** ~420 of 886 URLs indexed before
  the WSL crash (472 total indexed now, incl. ~52 prior live captures). Loop + `/tmp`
  staging lost in the crash; **resumable** — re-stage from the Windows History DB and
  re-run (skips already-indexed). Filters: dropped search engines, auth/SSO,
  internal/MS-cloud, shopping (amazon/redfin/zillow), and localhost. Next: finish the
  batch, then more weeks of Helium; then Edge/Chrome/Chromium.

## Config / deployment notes

- Fork: github.com/kestermcullough/hister · built locally (`docker compose up -d --build`)
  · data in Docker volume `hister_hister_data` · project name pinned `hister`. See FORK.md.
- `/hister/data/config.yml` (in volume): `disable_previews: false`, `strip_images: true`,
  `display_extractor_config: true`, `crawler.user_agent` = Chrome UA (server-side crawls
  only; extension capture is unaffected).
- Browser history locations (Windows): Helium `…/imput/Helium/User Data/Default/History`;
  Edge `…/Microsoft/Edge/…`; Chrome `…/Google/Chrome/…`; Chromium `…/Chromium/…`.
