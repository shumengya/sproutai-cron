"""sproutclaw-cron 任务管理 API，供 cronctl 与 WebUI 共用。"""

from __future__ import annotations

import fcntl
import importlib.util
import re
import shutil
from dataclasses import asdict, dataclass
from datetime import datetime
from pathlib import Path

from shumengya_cron.runner import (
    DISABLED_DIR_NAME,
    LogMode,
    TaskContext,
    set_task_enabled,
    task_is_enabled,
)

CRON_D = Path("/etc/cron.d")
_CRON_LINE = re.compile(
    r"^(\S+\s+\S+\s+\S+\s+\S+\s+\S+)\s+\S+\s+.+$",
)


def cron_root() -> Path:
    return Path(__file__).resolve().parents[2]


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
        if (path / "run.py").is_file():
            task_ids.add(path.name)

    disabled_root = base / DISABLED_DIR_NAME
    if disabled_root.is_dir():
        for path in sorted(disabled_root.iterdir()):
            if path.is_dir() and (path / "run.py").is_file():
                task_ids.add(path.name)

    return sorted(task_ids)


def get_context(task_id: str, root: Path | None = None) -> TaskContext:
    ctx = TaskContext.from_task_id(task_id, cron_root=root)
    if not ctx.task_dir.is_dir():
        raise FileNotFoundError(f"任务不存在: {task_id}")
    if not (ctx.task_dir / "run.py").is_file():
        raise FileNotFoundError(f"任务目录缺少 run.py: {task_id}")
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
                if text and not text.startswith("开关") and "cronctl" not in text:
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
    fd = lock_file.open("r")
    try:
        try:
            fcntl.flock(fd.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
            return False
        except BlockingIOError:
            return True
    finally:
        fd.close()


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
    return TaskInfo(
        task_id=task_id,
        enabled=task_is_enabled(ctx),
        running=task_is_running(ctx),
        schedule=schedule,
        description=description,
        log_size=log_size,
        log_updated=log_updated,
        task_dir=str(ctx.task_dir),
    )


def list_tasks(root: Path | None = None) -> list[TaskInfo]:
    migrate_legacy_disabled_layout(root)
    return [build_task_info(task_id, root) for task_id in iter_task_ids(root)]


def sync_cron_d(ctx: TaskContext, enabled: bool) -> str | None:
    """将 schedule.cron 安装到 /etc/cron.d/。返回提示信息或 None。"""
    cron_d_file = CRON_D / ctx.task_id
    schedule = ctx.task_dir / "schedule.cron"
    if not schedule.is_file():
        return "未找到 schedule.cron，跳过安装"
    shutil.copy2(schedule, cron_d_file)
    state = "开启" if enabled else "关闭（cron 仍触发，run.py 自动跳过）"
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
    """动态加载任务 run.py，调用其 run(ctx) 并返回退出码。"""
    ctx = get_context(task_id, root)
    run_py = ctx.task_dir / "run.py"

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
