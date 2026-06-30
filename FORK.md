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

## Deploy (build on each machine)

```bash
git clone https://github.com/kestermcullough/hister
cd hister
docker compose up -d --build      # builds the patched image locally and runs it
```

Open it at <http://localhost:4433>. In WSL, Windows reaches the same URL via
localhost forwarding.

- App config (e.g. `disable_previews`) lives in the persistent Docker volume at
  `/hister/data/config.yml`, **not** in this repo — so it survives rebuilds and
  is set per-machine.
- `compose.override.yml` (committed) renames the local image to `hister:fork` so a
  previously-pulled upstream image can't shadow the patched build.

## Update to the latest upstream master

```bash
./update.sh        # fetch upstream, rebase our patches, push, rebuild + redeploy
```

If `git rebase upstream/master` reports a conflict (upstream changed a line we
patched), resolve it, `git rebase --continue`, then re-run the build step.
