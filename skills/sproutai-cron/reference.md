# sproutai-cron 补充说明

主流程见 [SKILL.md](SKILL.md)。完整命令与 HTTP API 见 [references/api.md](references/api.md)。

## 控制面

- 语言：**Go**（Gin WebUI + cobra CLI + serve 调度）
- 入口：`cmd/cronctl`
- 编译：
  - 整包：sproutai 根目录 `node build.mjs` → `dist/windows-amd64/`、`dist/linux-amd64/`（`CGO_ENABLED=0`，前端嵌入）
  - 本机开发：`go build -o cronctl.exe ./cmd/cronctl`
- **注册（Go 统一实现）**：`cronctl install`
  - 代码：`internal/install`
  - Windows（`%USERPROFILE%\.local\bin`）：
    - `cronctl.cmd` — CMD / PowerShell
    - `cronctl.exe` — 二进制副本（Git Bash 用）
    - `cronctl` — bash 包装
  - Unix：`~/.local/bin/cronctl` 包装脚本
  - 用户环境变量：`SPROUTAI_CRON_ROOT=<仓库根>`
  - 可选：`cronctl install --self` 优先当前可执行文件

## 根目录定位

顺序（`internal/root`）：

1. `SPROUTAI_CRON_ROOT` / `CRON_ROOT`
2. 可执行文件所在目录附近
3. 自 cwd 向上查找含 `cron-tasks/` 的目录

## 任务侧

- `cron-tasks/<id>/` 与 `cron-tasks/.disabled/<id>/`
- `task.json`：`runtime` / `entry`
- 禁用 / 锁 / 日志由 Go runner 统一处理
- stdout/stderr → `logs/<task-id>.log`
- 注入：`CRON_TASK_ID`、`CRON_TASK_DIR`、`CRON_ROOT`

## CLI 输出约定

| 命令 | 人类可读 | `--json` |
|------|----------|----------|
| `status` | 分区线 + 调度器 + 任务表（最近日志） | `serve_running`、`count`、`tasks[]` |
| `list` | 任务表（含描述） | `count`、`tasks[]` |
| `get` | 键值详情 | 单任务对象 |
| `install` | 写入路径与说明 | `ok`、`result` |

## 核心包

`internal/manager` · `runner` · `daemon` · `cronexpr` · `api` · `cli` · `install` · `notify` · `root`
