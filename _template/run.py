"""
Hello World 示例任务：每天 08:00 输出一句 hello world。

新建任务时复制整个 _template/ 目录为 <task-id>/，更新 schedule.cron 中的
任务名，再替换下方业务逻辑。命名约定：<主机名>-<功能描述>。
"""

from __future__ import annotations

import sys as _sys, pathlib as _pl
_sys.path.insert(0, str(_pl.Path(__file__).resolve().parents[1] / "lib"))
del _sys, _pl

from shumengya_cron.runner import (
    TaskContext,
    acquire_cron_lock,
    task_is_disabled,
    task_logging,
)


def run(ctx: TaskContext) -> int:
    if task_is_disabled(ctx):
        return 0

    with task_logging(ctx) as log:
        with acquire_cron_lock(ctx.lock_file, log=log) as locked:
            if not locked:
                return 0

            log("start")
            log("hello world")
            log("end")

    return 0


def main() -> int:
    import pathlib
    task_id = pathlib.Path(__file__).parent.name
    ctx = TaskContext.from_task_id(task_id)
    return run(ctx)


if __name__ == "__main__":
    raise SystemExit(main())
