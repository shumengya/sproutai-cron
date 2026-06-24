#!/usr/bin/env python3
"""SproutClaw Cron MCP 服务器 — 通过 stdio 暴露定时任务管理工具。"""

from __future__ import annotations

import json
import os
import re
import shutil
import sys
from pathlib import Path
from typing import Literal

from fastmcp import FastMCP

MCP_DIR = Path(__file__).resolve().parent
CRON_ROOT = Path(os.environ.get("SPROUTCLAW_CRON_ROOT", MCP_DIR.parent)).resolve()
LIB_DIR = CRON_ROOT / "lib"

if str(LIB_DIR) not in sys.path:
    sys.path.insert(0, str(LIB_DIR))

from manager import (  # noqa: E402
    build_task_info,
    get_context,
    iter_task_ids,
    list_tasks,
    read_log_tail,
    run_task,
    set_task_state,
    sync_cron_d,
    toggle_task,
    update_task_schedule,
)
from runner import task_is_enabled  # noqa: E402

TEMPLATE_DIRS: dict[str, str] = {
    "python": "_template",
    "javascript": "_template-javascript",
    "bash": "_template-bash",
    "powershell": "_template-powershell",
}

TASK_ID_RE = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$")
DEFAULT_CRON_ROOTS = (
    "/shumengya/project/agent/sproutclaw-cron",
    str(CRON_ROOT),
    str(CRON_ROOT).replace("\\", "/"),
)

mcp = FastMCP(
    name="sproutclaw-cron",
    instructions=(
        "SproutClaw 定时任务系统。先 cron_list_tasks 了解任务；"
        "新建任务用 cron_create_task（默认禁用）；"
        "改调度用 cron_update_schedule；"
        "试跑用 cron_run_task 后 cron_get_log 查日志。"
        f" 项目根目录: {CRON_ROOT}"
    ),
)


def _root() -> Path:
    return CRON_ROOT


def _task_dict(task_id: str) -> dict[str, object]:
    return build_task_info(task_id, _root()).to_dict()


def _json(data: object) -> str:
    return json.dumps(data, ensure_ascii=False, indent=2)


def _rewrite_schedule_cron(schedule_file: Path, template_name: str, task_id: str) -> None:
    text = schedule_file.read_text(encoding="utf-8")
    text = text.replace(template_name, task_id)
    cron_root_posix = _root().as_posix()
    for old in DEFAULT_CRON_ROOTS:
        if old and old != cron_root_posix:
            text = text.replace(old, cron_root_posix)
    text = re.sub(
        r"(/usr/bin/python3\s+)\S+/cronctl\.py",
        rf"\g<1>{cron_root_posix}/cronctl.py",
        text,
    )
    schedule_file.write_text(text, encoding="utf-8")


@mcp.tool(annotations={"readOnlyHint": True})
def cron_list_tasks() -> str:
    """列出所有定时任务（含 .disabled/ 下已关闭的任务）及状态、调度、runtime。"""
    tasks = [info.to_dict() for info in list_tasks(_root())]
    return _json({"count": len(tasks), "tasks": tasks})


@mcp.tool(annotations={"readOnlyHint": True})
def cron_get_task(task_id: str) -> str:
    """获取单个任务的详细信息（状态、cron 表达式、描述、日志大小、标签等）。"""
    return _json(_task_dict(task_id))


@mcp.tool
def cron_enable_task(task_id: str) -> str:
    """启用任务：移出 .disabled/ 并同步系统 cron / Windows 计划任务。"""
    info, message = set_task_state(task_id, True, _root())
    return _json({"task": info.to_dict(), "message": message})


@mcp.tool
def cron_disable_task(task_id: str) -> str:
    """禁用任务：移入 .disabled/（cron 条目可保留，到点自动跳过）。"""
    info, message = set_task_state(task_id, False, _root())
    return _json({"task": info.to_dict(), "message": message})


@mcp.tool
def cron_toggle_task(task_id: str) -> str:
    """切换任务启用/禁用状态。"""
    info, message = toggle_task(task_id, _root())
    return _json({"task": info.to_dict(), "message": message})


@mcp.tool
def cron_run_task(task_id: str) -> str:
    """立即执行一次任务（同步等待完成），返回 exit code。禁用的任务会直接跳过。"""
    exit_code = run_task(task_id, _root())
    return _json({"task_id": task_id, "exit_code": exit_code})


@mcp.tool(annotations={"readOnlyHint": True})
def cron_get_log(task_id: str, lines: int = 200) -> str:
    """读取任务日志末尾内容（默认 200 行，最多 2000）。"""
    lines = max(1, min(lines, 2000))
    content = read_log_tail(task_id, lines, _root())
    return _json({"task_id": task_id, "lines": lines, "content": content})


@mcp.tool
def cron_update_schedule(
    task_id: str,
    schedule: str,
    description: str = "",
    tags: list[str] | None = None,
) -> str:
    """更新 schedule.cron 中的 cron 五段表达式、描述注释与 task.json 标签，并同步 cron.d。"""
    info, message = update_task_schedule(
        task_id,
        description=description,
        schedule=schedule,
        tags=tags,
        root=_root(),
        sync=True,
    )
    return _json({"task": info.to_dict(), "message": message})


@mcp.tool
def cron_sync_cron(task_id: str) -> str:
    """将 schedule.cron 同步到 /etc/cron.d/（Linux）或 schtasks（Windows），不改开关状态。"""
    ctx = get_context(task_id, _root())
    message = sync_cron_d(ctx, task_is_enabled(ctx))
    return _json({"task_id": task_id, "message": message or "已同步"})


@mcp.tool
def cron_create_task(
    task_id: str,
    runtime: Literal["python", "javascript", "bash", "powershell"] = "python",
    enable: bool = False,
) -> str:
    """从语言模板复制创建新任务。命名约定 <主机名>-<功能描述>。默认创建后保持禁用。"""
    if not TASK_ID_RE.match(task_id):
        raise ValueError("task_id 仅允许字母数字、点、连字符与下划线，且不能以特殊符号开头")

    template_name = TEMPLATE_DIRS[runtime]
    template_dir = _root() / template_name
    if not template_dir.is_dir():
        raise FileNotFoundError(f"模板不存在: {template_name}")

    if task_id in iter_task_ids(_root()):
        raise ValueError(f"任务已存在: {task_id}")

    target = _root() / task_id
    shutil.copytree(template_dir, target)
    schedule_file = target / "schedule.cron"
    if schedule_file.is_file():
        _rewrite_schedule_cron(schedule_file, template_name, task_id)

    if runtime == "bash":
        run_sh = target / "run.sh"
        if run_sh.is_file():
            run_sh.chmod(run_sh.stat().st_mode | 0o111)

    message = "已创建"
    if not enable:
        info, msg = set_task_state(task_id, False, _root())
        message = msg or "已创建并禁用"
        return _json({"task": info.to_dict(), "message": message, "template": template_name})

    info, msg = set_task_state(task_id, True, _root())
    return _json({"task": info.to_dict(), "message": msg or message, "template": template_name})


@mcp.tool(annotations={"readOnlyHint": True})
def cron_list_templates() -> str:
    """列出可用于 cron_create_task 的语言模板。"""
    templates = []
    for runtime, dirname in TEMPLATE_DIRS.items():
        path = _root() / dirname
        templates.append(
            {
                "runtime": runtime,
                "template_dir": dirname,
                "exists": path.is_dir(),
                "entry": {
                    "python": "run.py",
                    "javascript": "run.js",
                    "bash": "run.sh",
                    "powershell": "run.ps1",
                }[runtime],
            }
        )
    return _json({"templates": templates})


if __name__ == "__main__":
    mcp.run()
