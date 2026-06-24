"""sproutclaw-cron 任务管理 API，供 cronctl 与 WebUI 共用。"""

from __future__ import annotations

import importlib.util
import os
import re
import shutil
from dataclasses import asdict, dataclass
from datetime import datetime
from pathlib import Path

from external_runner import run_external_task
from cron_platform import is_windows, lock_ex_nb, sync_windows_task, unlock
from runner import (
    DISABLED_DIR_NAME,
    LogMode,
    TaskContext,
    set_task_enabled,
    task_is_enabled,
)
from task_manifest import (
    effective_task_tags,
    is_valid_task_dir,
    load_task_manifest,
    load_task_tags,
    normalize_tags,
    save_task_tags,
)

CRON_D = Path("/etc/cron.d")
_CRON_LINE = re.compile(
    r"^(\S+\s+\S+\s+\S+\s+\S+\s+\S+)\s+\S+\s+.+$",
)
_DESC_SKIP_PREFIX = ("开关",)


def _is_description_comment(text: str) -> bool:
    if not text:
        return False
    if text.startswith(_DESC_SKIP_PREFIX):
        return False
    if "cronctl" in text:
        return False
    return True


def _validate_cron_schedule(schedule: str) -> str:
    parts = schedule.strip().split()
    if len(parts) != 5:
        raise ValueError("cron 表达式须为 5 段：分 时 日 月 周")
    return " ".join(parts)


def _replace_cron_expression(line: str, new_schedule: str) -> str:
    stripped = line.strip()
    if not _CRON_LINE.match(stripped):
        return line
    parts = stripped.split(None, 6)
    if len(parts) < 7:
        raise ValueError("schedule.cron 中的 cron 行格式无法识别")
    return f"{new_schedule} {parts[5]} {parts[6]}"


def cron_root() -> Path:
    return Path(__file__).resolve().parents[1]


def migrate_legacy_disabled_layout(root: Path | None = None) -> None:
    """将旧的 <task-id>.disabled/ 目录迁入 .disabled/<task-id>/。"""
    base = root or cron_root()
    disabled_root = base / DISABLED_DIR_NAME
    disabled_root.mkdir(exist_ok=True)
    for path in sorted(base.iterdir()):
        if not path.is_dir() or not path.name.endswith(".disabled"):
            continue
        task_id = path.name.removesuffix(".disabled")
        target = disabled_root / task_id
        if target.exists():
            continue
        path.rename(target)


def iter_task_ids(root: Path | None = None) -> list[str]:
    """遍历根目录与 .disabled/ 下的任务，返回 task_id 列表。"""
    base = root or cron_root()
    skip = {"lib", "__pycache__", DISABLED_DIR_NAME, ".claude", "webui"}
    task_ids: set[str] = set()

    for path in sorted(base.iterdir()):
        if not path.is_dir() or path.name in skip or path.name.startswith("."):
            continue
        if path.name.endswith(".disabled"):
            task_ids.add(path.name.removesuffix(".disabled"))
            continue
        if is_valid_task_dir(path):
            task_ids.add(path.name)

    disabled_root = base / DISABLED_DIR_NAME
    if disabled_root.is_dir():
        for path in sorted(disabled_root.iterdir()):
            if path.is_dir() and is_valid_task_dir(path):
                task_ids.add(path.name)

    return sorted(task_ids)


def get_context(task_id: str, root: Path | None = None) -> TaskContext:
    ctx = TaskContext.from_task_id(task_id, cron_root=root)
    if not ctx.task_dir.is_dir():
        raise FileNotFoundError(f"任务不存在: {task_id}")
    if not is_valid_task_dir(ctx.task_dir):
        raise FileNotFoundError(f"任务目录缺少 schedule.cron 或入口脚本: {task_id}")
    return ctx


def _parse_schedule(task_dir: Path) -> tuple[str | None, str | None]:
    schedule = task_dir / "schedule.cron"
    if not schedule.is_file():
        return None, None
    description: str | None = None
    expression: str | None = None
    for raw in schedule.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line:
            continue
        if line.startswith("#"):
            if description is None:
                text = line.lstrip("#").strip()
                if _is_description_comment(text):
                    description = text
            continue
        match = _CRON_LINE.match(line)
        if match:
            expression = match.group(1)
            break
    return expression, description


def task_is_running(ctx: TaskContext) -> bool:
    lock_file = ctx.lock_file
    if not lock_file.is_file():
        return False
    fd = os.open(str(lock_file), os.O_RDWR | os.O_CREAT, 0o644)
    try:
        try:
            lock_ex_nb(fd)
            return False
        except BlockingIOError:
            return True
    finally:
        unlock(fd)
        os.close(fd)


@dataclass
class TaskInfo:
    task_id: str
    enabled: bool
    running: bool
    schedule: str | None
    description: str | None
    log_size: int
    log_updated: str | None
    task_dir: str
    runtime: str
    tags: list[str]

    def to_dict(self) -> dict[str, object]:
        return asdict(self)


def build_task_info(task_id: str, root: Path | None = None) -> TaskInfo:
    ctx = get_context(task_id, root)
    schedule, description = _parse_schedule(ctx.task_dir)
    log_size = 0
    log_updated: str | None = None
    if ctx.log_file.is_file():
        stat = ctx.log_file.stat()
        log_size = stat.st_size
        log_updated = datetime.fromtimestamp(stat.st_mtime).strftime("%Y-%m-%d %H:%M:%S")
    manifest = load_task_manifest(ctx.task_dir)
    return TaskInfo(
        task_id=task_id,
        enabled=task_is_enabled(ctx),
        running=task_is_running(ctx),
        schedule=schedule,
        description=description,
        log_size=log_size,
        log_updated=log_updated,
        task_dir=str(ctx.task_dir),
        runtime=manifest.runtime,
        tags=effective_task_tags(ctx.task_dir, manifest),
    )


def list_tasks(root: Path | None = None) -> list[TaskInfo]:
    migrate_legacy_disabled_layout(root)
    return [build_task_info(task_id, root) for task_id in iter_task_ids(root)]


def sync_cron_d(ctx: TaskContext, enabled: bool) -> str | None:
    """安装定时调度：Linux 写入 /etc/cron.d/，Windows 注册任务计划程序。"""
    schedule = ctx.task_dir / "schedule.cron"
    if not schedule.is_file():
        return "未找到 schedule.cron，跳过安装"

    expression, _ = _parse_schedule(ctx.task_dir)

    if is_windows():
        return sync_windows_task(
            ctx.task_id,
            ctx.cron_root,
            expression,
            enabled=enabled,
        )

    cron_d_file = CRON_D / ctx.task_id
    shutil.copy2(schedule, cron_d_file)
    state = "开启" if enabled else "关闭（cron 仍触发，任务自动跳过）"
    return f"已同步到 {cron_d_file}（{state}）"


def set_task_state(task_id: str, enabled: bool, root: Path | None = None) -> tuple[TaskInfo, str | None]:
    ctx = get_context(task_id, root)
    set_task_enabled(ctx, enabled)
    ctx = TaskContext.from_task_id(task_id, cron_root=root)
    message = sync_cron_d(ctx, enabled)
    return build_task_info(task_id, root), message


def toggle_task(task_id: str, root: Path | None = None) -> tuple[TaskInfo, str | None]:
    ctx = get_context(task_id, root)
    return set_task_state(task_id, not task_is_enabled(ctx), root)


def run_task(task_id: str, root: Path | None = None) -> int:
    """执行任务：Python 动态加载 run.py，其余 runtime 由 external_runner 启动。"""
    ctx = get_context(task_id, root)
    manifest = load_task_manifest(ctx.task_dir)

    if manifest.runtime != "python":
        return run_external_task(ctx, manifest)

    run_py = ctx.task_dir / manifest.entry
    spec = importlib.util.spec_from_file_location(f"cron_run_{task_id}", run_py)
    mod = importlib.util.module_from_spec(spec)  # type: ignore[arg-type]
    spec.loader.exec_module(mod)  # type: ignore[union-attr]

    log_mode = getattr(mod, "LOG_MODE", LogMode.REDIRECT_STD)
    ctx = TaskContext.from_task_id(task_id, log_mode=log_mode, cron_root=root)
    return mod.run(ctx)


def read_log_tail(task_id: str, lines: int = 200, root: Path | None = None) -> str:
    ctx = get_context(task_id, root)
    if not ctx.log_file.is_file():
        return ""
    content = ctx.log_file.read_text(encoding="utf-8", errors="replace")
    parts = content.splitlines()
    if lines <= 0 or len(parts) <= lines:
        return content
    return "\n".join(parts[-lines:]) + "\n"


def update_task_schedule(
    task_id: str,
    *,
    description: str,
    schedule: str,
    tags: list[str] | None = None,
    root: Path | None = None,
    sync: bool = True,
) -> tuple[TaskInfo, str | None]:
    """更新 schedule.cron 中的描述注释、cron 表达式前五段，以及 task.json 标签。"""
    ctx = get_context(task_id, root)
    schedule_file = ctx.task_dir / "schedule.cron"
    if not schedule_file.is_file():
        raise FileNotFoundError(f"未找到 schedule.cron: {task_id}")

    new_schedule = _validate_cron_schedule(schedule)
    desc_text = description.strip()

    raw_lines = schedule_file.read_text(encoding="utf-8").splitlines()
    desc_updated = False
    cron_updated = False
    new_lines: list[str] = []

    for line in raw_lines:
        stripped = line.strip()
        if not desc_updated and stripped.startswith("#"):
            text = stripped.lstrip("#").strip()
            if _is_description_comment(text):
                new_lines.append(f"# {desc_text}" if desc_text else "#")
                desc_updated = True
                continue
        if _CRON_LINE.match(stripped):
            new_lines.append(_replace_cron_expression(line, new_schedule))
            cron_updated = True
            continue
        new_lines.append(line)

    if not cron_updated:
        raise ValueError("schedule.cron 中未找到 cron 调度行")

    if not desc_updated and desc_text:
        inserted: list[str] = []
        for line in new_lines:
            if _CRON_LINE.match(line.strip()) and (not inserted or inserted[-1] != ""):
                inserted.append(f"# {desc_text}")
                inserted.append("")
            inserted.append(line)
        new_lines = inserted

    schedule_file.write_text("\n".join(new_lines) + "\n", encoding="utf-8")

    if tags is not None:
        save_task_tags(ctx.task_dir, normalize_tags(tags))

    message = None
    if sync:
        ctx = get_context(task_id, root)
        message = sync_cron_d(ctx, task_is_enabled(ctx))

    return build_task_info(task_id, root), message
