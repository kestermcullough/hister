# Fork notes (kestermcullough/hister)

A thin fork of [asciimoo/hister](https://github.com/asciimoo/hister) carrying a few
local patches on top of upstream `master`. We do **not** open PRs upstream; we just
track `master` and replay our patches with `git rebase`.

## Patches on top of upstream

- **Keep the preview UI usable when `disable_previews` stores no snapshot.**
  With `app.disable_previews: true` the server skips storing page HTML snapshots
  (no inlined images = small storage), but stock upstream then hides the preview
  button/panel entirely. The preview endpoint already falls back to the saved
  extracted text, so we (a) stop the web UI from consuming the server's
  `disablePreviews` flag and (b) wrap that text fallback in `<pre>` so it reads
  cleanly. Files: `server/server.go`, `webui/app/src/routes/+page.svelte`,
  `webui/app/src/routes/history/+page.svelte`.

- **`app.strip_images`: drop all images from stored snapshots, keep formatting.**
  With previews enabled (`disable_previews: false`), `prepareForStorage` runs the
  snapshot through a surgical goquery pass (`sanitizer.StripImages`) that removes
  every image element (img/picture/source/svg/noscript/template + inline `data:`
  URIs) and image-bearing attrs/styles, while preserving all other markup — rich
  previews without the base64 image payload that dominates storage. Files:
  `config/config.go`, `server/sanitizer/sanitizer.go`, `server/indexer/indexer.go`.
  Enable in config: `disable_previews: false` + `strip_images: true`.

- **Defuddle extractor + Node sidecar.** Server-side extractor
  (`server/extractor/extractors/defuddle/`) POSTs rendered HTML to a Node `defuddle`
  sidecar (Obsidian Web Clipper's engine); runs before Readability, so it keeps
  discussion/comments (Reddit/HN) that Readability drops — in both searchable text and
  previews. Sidecar in `defuddle-svc/`; service + memory caps in `compose.override.yml`.
  Enable: `extractors.defuddle.enable: true`.

- **`--min-visit` fix.** Registered the flag on `import-browser` too (upstream only put
  it on the JSON `import` command, so it silently no-op'd). File: `cmd/root.go`.

## Deploy (build on each machine)

```bash
git clone https://github.com/kestermcullough/hister
cd hister
docker compose up -d --build      # builds hister + the defuddle sidecar, runs both
```

Open it at <http://localhost:4433>. In WSL, Windows reaches the same URL via localhost
forwarding. That's the whole install — identical settings on every machine.

- **Config is code:** `config.yml` in this repo is bind-mounted read-only into the
  container (`compose.override.yml` points `HISTER_CONFIG` at it), so every clone gets
  the same settings (strip_images, defuddle+ytdlp enabled, Chrome UA…). To change config,
  edit `config.yml` and `docker compose up -d` again. **Data** (index, DB, secret key,
  skip rules) stays per-machine in the `hister_data` Docker volume.
- `compose.override.yml` (committed) renames the image to `hister:fork` (so a pulled
  upstream image can't shadow the patched build), adds the `defuddle` sidecar, and caps
  container memory (hister 3g, defuddle 1g).

### WSL2 note (important if deploying under WSL)
Heavy builds + long import crawls can OOM WSL2 (defaults: 16 GB RAM / 4 GB swap). Give it
a swap cushion in `%USERPROFILE%\.wslconfig`, then run `wsl --shutdown` from Windows:

```ini
[wsl2]
memory=16GB
swap=24GB
[experimental]
autoMemoryReclaim=gradual
```

## Update to the latest upstream master

```bash
./update.sh        # fetch upstream, rebase our patches, push, rebuild + redeploy
```

If `git rebase upstream/master` reports a conflict (upstream changed a line we
patched), resolve it, `git rebase --continue`, then re-run the build step.
