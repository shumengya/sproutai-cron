"""任务清单 task.json 解析与 runtime 推断。"""

from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path
from typing import Literal

Runtime = Literal["python", "javascript", "bash", "powershell"]

RUNTIME_DEFAULT_ENTRY: dict[str, str] = {
    "python": "run.py",
    "javascript": "run.js",
    "bash": "run.sh",
    "powershell": "run.ps1",
}

RUNTIME_ALIASES: dict[str, Runtime] = {
    "python": "python",
    "py": "python",
    "javascript": "javascript",
    "js": "javascript",
    "node": "javascript",
    "bash": "bash",
    "sh": "bash",
    "shell": "bash",
    "powershell": "powershell",
    "ps1": "powershell",
    "pwsh": "powershell",
}

ENTRY_TO_RUNTIME: dict[str, Runtime] = {
    "run.py": "python",
    "run.js": "javascript",
    "run.sh": "bash",
    "run.ps1": "powershell",
}

RUNTIME_LABEL: dict[str, str] = {
    "python": "Python",
    "javascript": "JavaScript",
    "bash": "Bash",
    "powershell": "PowerShell",
}

MAX_TASK_TAGS = 4
MAX_TAG_LENGTH = 32


@dataclass(frozen=True)
class TaskManifest:
    runtime: Runtime
    entry: str

    @property
    def entry_path(self) -> str:
        return self.entry

    @property
    def label(self) -> str:
        return RUNTIME_LABEL.get(self.runtime, self.runtime)


def _normalize_runtime(raw: str) -> Runtime | None:
    key = raw.strip().lower()
    return RUNTIME_ALIASES.get(key)  # type: ignore[return-value]


def _infer_from_entry_files(task_dir: Path) -> TaskManifest | None:
    for entry, runtime in ENTRY_TO_RUNTIME.items():
        if (task_dir / entry).is_file():
            return TaskManifest(runtime=runtime, entry=entry)
    return None


def load_task_manifest(task_dir: Path) -> TaskManifest:
    """读取 task.json；缺失时按入口文件推断；默认 python + run.py。"""
    manifest_file = task_dir / "task.json"
    if manifest_file.is_file():
        data = json.loads(manifest_file.read_text(encoding="utf-8"))
        runtime_raw = str(data.get("runtime", "python"))
        runtime = _normalize_runtime(runtime_raw)
        if runtime is None:
            raise ValueError(f"不支持的 runtime: {runtime_raw}")
        entry = str(data.get("entry", RUNTIME_DEFAULT_ENTRY[runtime])).strip()
        if not entry:
            entry = RUNTIME_DEFAULT_ENTRY[runtime]
        entry_path = task_dir / entry
        if not entry_path.is_file():
            raise FileNotFoundError(f"task.json 指定的入口不存在: {entry}")
        return TaskManifest(runtime=runtime, entry=entry)

    inferred = _infer_from_entry_files(task_dir)
    if inferred is not None:
        return inferred

    if (task_dir / "run.py").is_file():
        return TaskManifest(runtime="python", entry="run.py")

    raise FileNotFoundError("未找到 task.json 或可识别的入口脚本")


def normalize_tags(raw: object) -> list[str]:
    """去重、去空白，最多 MAX_TASK_TAGS 个。"""
    if not isinstance(raw, (list, tuple)):
        return []
    tags: list[str] = []
    for item in raw:
        s = str(item).strip()
        if not s or len(s) > MAX_TAG_LENGTH:
            continue
        if s not in tags:
            tags.append(s)
        if len(tags) >= MAX_TASK_TAGS:
            break
    return tags


def load_task_tags(task_dir: Path) -> list[str]:
    manifest_file = task_dir / "task.json"
    if not manifest_file.is_file():
        return []
    try:
        data = json.loads(manifest_file.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        return []
    return normalize_tags(data.get("tags", []))


def effective_task_tags(task_dir: Path, manifest: TaskManifest) -> list[str]:
    """已存标签优先；否则默认用运行时名称作为唯一标签。"""
    stored = load_task_tags(task_dir)
    if stored:
        return stored
    return [RUNTIME_LABEL.get(manifest.runtime, manifest.runtime)]


def save_task_tags(task_dir: Path, tags: list[str]) -> None:
    """写入 task.json 的 tags 字段；无文件时按当前 manifest 创建。"""
    manifest = load_task_manifest(task_dir)
    normalized = normalize_tags(tags)
    manifest_file = task_dir / "task.json"
    data: dict[str, object] = {}
    if manifest_file.is_file():
        data = json.loads(manifest_file.read_text(encoding="utf-8"))
    data["runtime"] = manifest.runtime
    data["entry"] = manifest.entry
    data["tags"] = normalized
    manifest_file.write_text(
        json.dumps(data, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )


def is_valid_task_dir(task_dir: Path) -> bool:
    try:
        if not task_dir.is_dir():
            return False
        if not (task_dir / "schedule.cron").is_file():
            return False
        load_task_manifest(task_dir)
        return True
    except (OSError, ValueError, FileNotFoundError, json.JSONDecodeError):
        return False
