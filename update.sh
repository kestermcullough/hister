#!/usr/bin/env bash
# [fork] Sync this fork onto the latest upstream master, then rebuild & redeploy.
# Our patches live as commits on top of upstream/master and are replayed by rebase.
set -euo pipefail
cd "$(dirname "$0")"

echo "==> Fetching upstream..."
git fetch upstream

echo "==> Rebasing fork patches onto upstream/master..."
git rebase upstream/master   # stops here on conflict; resolve, then: git rebase --continue

echo "==> Pushing updated master to the fork..."
git push --force-with-lease origin master   # rebase rewrites history, so force (safely)

echo "==> Rebuilding patched image and redeploying (data volume is preserved)..."
docker compose up -d --build

echo "==> Done. Now at upstream $(git rev-parse --short upstream/master) + these fork patches:"
git log --oneline upstream/master..HEAD
