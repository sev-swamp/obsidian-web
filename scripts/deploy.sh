#!/usr/bin/env bash
# Deploy to the VPS: pull + docker compose build, detached from SSH so
# a dropped connection cannot kill the build, then poll until the new
# container is up.
#
# Usage:
#   OBSIDIANWEB_DEPLOY_HOST=root@1.2.3.4 scripts/deploy.sh
#   (or put the export into your shell profile / .env)
set -euo pipefail

HOST="${OBSIDIANWEB_DEPLOY_HOST:?set OBSIDIANWEB_DEPLOY_HOST, e.g. root@1.2.3.4}"
DIR="${OBSIDIANWEB_DEPLOY_DIR:-obsidian-web}"

echo "==> starting detached build on $HOST"
ssh "$HOST" "cd ~/$DIR && git pull -q && (nohup docker compose up -d --build > /tmp/owdeploy.log 2>&1 &) && git log --oneline -1"

echo "==> waiting for the container to restart (builds take up to ~10 min)"
for _ in $(seq 1 60); do
  sleep 20
  status=$(ssh -o ConnectTimeout=10 "$HOST" "cd ~/$DIR && docker compose ps --format '{{.Status}}'" 2>/dev/null || true)
  echo "   $status"
  if grep -qE 'Up (Less than|About a minute|[0-9]+ seconds?|1 minute)' <<<"$status"; then
    echo "==> health:"
    ssh "$HOST" 'curl -s localhost:8787/api/health'
    echo
    echo "==> deployed"
    exit 0
  fi
done

echo "!! timed out — inspect: ssh $HOST tail -50 /tmp/owdeploy.log" >&2
exit 1
