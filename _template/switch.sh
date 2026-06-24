#!/usr/bin/env bash
set -euo pipefail

task_id="$(basename "$(dirname "$0")")"
action="${1:-toggle}"
shift || true

exec python3 /shumengya/project/agent/sproutclaw-cron/cronctl.py "$action" "$task_id" "$@"
