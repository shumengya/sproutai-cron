# cronctl 命令参考

- 控制面为 **Go 单二进制**（Gin WebUI + CLI + serve）。
- 默认**人机可读文本**（表格 / 分区线）；加 `--json` 输出 JSON。
- 通过 `SPROUTAI_CRON_ROOT` / `CRON_ROOT` / 可执行文件位置 / cwd 向上查找，定位含 `cron-tasks/` 的仓库根。

## 安装与调用

```bash
# 编译
# 整包：在 sproutai 根目录 node build.mjs
go build -o cronctl.exe ./cmd/cronctl

# 注册全局命令（Go 统一：internal/install）
cronctl install              # 推荐
cronctl install --self       # 优先当前正在运行的 exe
```

Windows 会写入 `~/.local/bin` 下的 `cronctl.cmd` + `cronctl.exe` + bash 包装，并设置用户 `PATH` / `SPROUTAI_CRON_ROOT`。  
Unix 写入 `~/.local/bin/cronctl` 包装。Git Bash 请用 `cronctl` / `cronctl.exe`。

`cronctl web` 默认 in-process 启动与 `serve` 相同的调度循环；`status` 通过 `cron-tasks/.sprout-cron.serve.pid` 判断。  
`web` 与 `status` 必须同一仓库根，否则会误报「未启动」。

## 命令一览

| 命令 | 说明 |
|------|------|
| `serve [--once]` | 常驻调度（到点执行已启用任务）；`--once` 只评估当前分钟 |
| `web [--host] [--port] [--no-serve]` | 启动 WebUI（Gin）；默认附带 serve |
| `status [task-id…]` | **默认命令**；调度器是否在跑 + 任务表 |
| `list` | 列出全部任务（表：状态 / id / runtime / 调度 / 描述） |
| `get <task-id>` | 单任务详情 |
| `enable` / `on <task-id…>` | 启用（移出 `.disabled/`） |
| `disable` / `off <task-id…>` | 禁用 |
| `toggle <task-id…>` | 切换开关 |
| `run <task-id>` | 立即执行一次 |
| `log <task-id> [--lines N]` | 日志末尾（默认 200，最大 2000） |
| `create <task-id> [--runtime …] [--enable]` | 从模板创建（默认禁用） |
| `delete` / `rm <task-id…> [--force]` | 删除任务目录 |
| `update-schedule <task-id> <cron> [--description] [--tags …]` | 改调度与标签 |
| `templates` | 列出语言模板 |
| `install [--self]` | 注册到用户 PATH（跨平台） |
| `notify feishu --title … --body …` | 发送飞书 Markdown |

全局 flag：`--json`。

## status 文本格式

```
════════════════════════════════════════
  sprout-cron
════════════════════════════════════════
  调度器    ✅  运行中     # 或 ❌ 未启动 + 提示 cronctl serve
  任务      共 N · 启用 X · 禁用 Y  [· 执行中 Z]
────────────────────────────────────────
  状态  任务  运行时  调度  最近日志
  ✅    id    python  …     2026-…
  ❌    …
════════════════════════════════════════
```

`status --json` 字段：

```json
{
  "serve_running": false,
  "count": 6,
  "tasks": [
    {
      "task_id": "…",
      "enabled": true,
      "running": false,
      "schedule": "0 8 * * *",
      "description": "…",
      "log_size": 0,
      "log_updated": "…",
      "task_dir": "…",
      "runtime": "python",
      "tags": []
    }
  ]
}
```

## 示例

```bash
go build -o cronctl.exe ./cmd/cronctl
cronctl serve
cronctl status
cronctl list --json
cronctl create my-task --runtime python
cronctl run my-task
cronctl enable my-task
cronctl update-schedule my-task "0 9 * * *" --description "每天 09:00"
cronctl delete my-task
cronctl web
```

## 调度

1. 运行 `cronctl serve`（须保持进程存活）或 `cronctl web`（默认同启）
2. 每分钟读取已启用任务的 `schedule.cron` 五段表达式
3. 命中则后台调用与 `cronctl run` 相同的执行逻辑
4. 单实例锁：`cron-tasks/.sprout-cron.serve.lock` + `.sprout-cron.serve.pid`

**不使用** Linux `/etc/cron.d` 或 Windows 任务计划程序。

## schedule.cron（现代 DSL）

```cron
# 描述（可选）
0 8 * * *                 # 五段 cron
# @every 10s
# @random 1s 60s
# @random 30m 3h
# @on 12-25 00:00
# @holiday christmas
# @holiday cn-national-day
# @weekly mon 10:00
# @monthly 1 00:00
# */10 * * * * *          # 六段（秒）
```

serve **秒级**唤醒；`@every`/`@random` 状态：`cron-tasks/.schedule-state/<id>.json`。

## create / delete

- create：从 `template-*` 复制，默认禁用
- delete：禁止删模板；运行中需 `--force`；不支持 `all`

## task.json

```json
{
  "runtime": "python",
  "entry": "run.py"
}
```

`runtime`：`python` | `javascript` | `bash` | `powershell`

## WebUI API

| 方法 | 路径 |
|------|------|
| GET | `/api/health` · `/api/serve` · `/api/runtimes` |
| GET | `/api/tasks` · `/api/tasks/{id}` |
| POST | `/api/tasks/{id}/enable\|disable\|toggle\|run` |
| GET | `/api/tasks/{id}/log` |
| PATCH | `/api/tasks/{id}/schedule` |

（同时挂载 `/cron/api/*`）

## 源码布局

```
cmd/cronctl/          # 入口
internal/             # api / cli / daemon / manager / runner / root …
webui/
  embed.go            # go:embed frontend
  frontend/           # 源文件（构建时嵌入二进制）
dist/
  windows-amd64/cronctl.exe
  linux-amd64/cronctl
cron-tasks/           # 任务脚本（多语言）
skills/sproutai-cron/ # Agent skill
internal/install/     # 注册逻辑（Go）
```

构建：sproutai 根目录 `node build.mjs`，或本仓 `go build ./cmd/cronctl`（`CGO_ENABLED=0`，前端进二进制）。
