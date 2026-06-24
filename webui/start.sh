#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BACKEND="$ROOT/webui/backend"
VENV="$BACKEND/.venv"

if [[ ! -d "$VENV" ]]; then
  python3 -m venv "$VENV"
  "$VENV/bin/pip" install -r "$BACKEND/requirements.txt"
fi

HOST="${SPROUTCLAW_CRON_WEB_HOST:-0.0.0.0}"
PORT="${SPROUTCLAW_CRON_WEB_PORT:-8765}"

exec "$VENV/bin/uvicorn" main:app --app-dir "$BACKEND" --host "$HOST" --port "$PORT" \
  --reload --reload-dir "$BACKEND" --reload-dir "$ROOT/lib"
