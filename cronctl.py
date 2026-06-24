#!/usr/bin/env python3
"""shumengya cron 任务开关与运行管理。"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent / "lib"))

from shumengya_cron.manager import (
    cron_root,
    get_context,
    iter_task_ids,
    migrate_legacy_disabled_layout,
    run_task,
    set_task_state,
    sync_cron_d,
)
from shumengya_cron.runner import task_enabled_text, task_is_enabled


CRON_ROOT = cron_root()


def _resolve_task_ids(values: list[str]) -> list[str]:
    if not values:
        return iter_task_ids(CRON_ROOT)
    if len(values) == 1 and values[0] == "all":
        return iter_task_ids(CRON_ROOT)
    return values


def _print_status(task_id: str) -> None:
    ctx = get_context(task_id, CRON_ROOT)
    print(f"{ctx.task_id}: {task_enabled_text(ctx)}")


def _set_state(task_id: str, enabled: bool) -> None:
    _info, message = set_task_state(task_id, enabled, CRON_ROOT)
    print(f"{task_id}: {'开启' if enabled else '关闭'}")
    if message:
        print(f"  → {message}")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="cronctl.py",
        description="管理 /shumengya/project/agent/sproutclaw-cron 下每个定时任务的开关状态，或手动执行任务。",
    )
    parser.add_argument(
        "action",
        nargs="?",
        default="status",
        choices=["status", "enable", "disable", "toggle", "on", "off", "run", "sync-cron"],
        help="status / enable / disable / toggle / run / sync-cron",
    )
    parser.add_argument("tasks", nargs="*", help="任务名，status 时默认列出全部；也可传 all")
    args = parser.parse_args(argv)

    migrate_legacy_disabled_layout(CRON_ROOT)

    action = args.action
    if action == "on":
        action = "enable"
    elif action == "off":
        action = "disable"

    task_ids = _resolve_task_ids(list(args.tasks))
    if not task_ids:
        print("未找到可管理的任务。", file=sys.stderr)
        return 1

    if action in {"enable", "disable", "toggle", "run", "sync-cron"} and not args.tasks:
        print("请至少指定一个任务名，或传 all。", file=sys.stderr)
        return 1

    rc = 0
    for task_id in task_ids:
        try:
            if action == "run":
                task_rc = run_task(task_id, CRON_ROOT)
                print(f"▶ {task_id} (exit={task_rc})")
                if task_rc != 0:
                    rc = task_rc
                continue

            if action == "status":
                _print_status(task_id)
            elif action == "sync-cron":
                ctx = get_context(task_id, CRON_ROOT)
                message = sync_cron_d(ctx, task_is_enabled(ctx))
                if message:
                    print(f"  → {message}")
            elif action == "enable":
                _set_state(task_id, True)
            elif action == "disable":
                _set_state(task_id, False)
            else:
                _set_state(task_id, not task_is_enabled(get_context(task_id, CRON_ROOT)))
        except FileNotFoundError as e:
            print(str(e), file=sys.stderr)
            rc = 1
    return rc


if __name__ == "__main__":
    raise SystemExit(main())
