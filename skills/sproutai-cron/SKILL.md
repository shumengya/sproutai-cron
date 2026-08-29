---
name: sproutai-cron
description: >-
  用本仓库的 cronctl（Go 单二进制）管理 sproutai-cron 定时任务：编译/注册、serve 调度、
  status/list/get、enable/disable/toggle、run、log、create/delete、改 schedule.cron、
  打开 WebUI、飞书通知。在用户提到 cronctl、sproutai-cron、sprout-cron、cron-tasks、
  schedule.cron、task.json、cronctl install、install-cronctl、定时任务开关、任务没按时跑、调度器未启动、
  试跑任务、看任务日志、新建/删除 cron 任务、改执行时间、cron 管理面板/WebUI，
  或运行 /sproutai-cron 时使用。也适用于在本仓库语境下说「每天八点跑一下」「把地震抓取
  开开/关掉」「任务启用了但没触发」等，只要操作对象是本项目的 cron-tasks 任务而非
  系统 crontab、GitHub Actions、k8s CronJob 或其他无关调度系统。
  
---

# SproutAI Cron（cronctl）

本 skill 指导你通过 **cronctl** 管理本仓库的多语言定时任务。调度由 `cronctl serve`（或 `web`）常驻进程完成，**不**写入系统 cron.d / 任务计划。

细节与完整 flag 见 [references/api.md](references/api.md)；编译/安装/根目录定位见 [reference.md](reference.md)。仓库约定见 [AGENTS.md](../../AGENTS.md)。

## 何时用 / 何时不用

| 用本 skill | 不用（避免误触发） |
|------------|-------------------|
| 操作 `cron-tasks/` 下任务、调用 `cronctl` | 编辑 Linux `/etc/cron*`、`crontab -e`、Windows 任务计划 |
| serve 是否在跑、任务 enable、日志、试跑 | CI 定时（GitHub Actions `schedule`）、k8s CronJob、云函数定时 |
| 在本仓库新建「每天跑脚本」类任务 | 与 sproutai-cron 无关的通用「写个 cron 表达式」问答 |

## 调用入口（Windows 默认 Git Bash 时尤其注意）

优先已注册的全局命令；失败再本地二进制。

| 环境 | 推荐调用 | 说明 |
|------|----------|------|
| **Git Bash（本机默认）** | `cronctl.exe` 或 `cronctl` | Git Bash **不会**解析 `.cmd`；`cronctl install` 会写 `cronctl.exe` 副本 + bash 包装 + `cronctl.cmd` |
| Windows PowerShell / CMD | `cronctl` | 走 `cronctl.cmd` |
| Linux / macOS | `cronctl` | `~/.local/bin/cronctl` 包装 |
| 未安装时回退 | `./cronctl.exe`、`dist/.../cronctl` | 需能定位含 `cron-tasks/` 的根 |

**注册（跨平台同一 Go 实现，可覆盖）：**

```text
# 必须在目标仓库目录内执行（优先 cwd，不沿用旧 SPROUTAI_CRON_ROOT）
cd /path/to/sproutai-cron   # 例如 release 副本或开发仓
go build -o cronctl.exe ./cmd/cronctl   # 或在 sproutai 根目录 node build.mjs
.\cronctl.exe install       # 覆盖写入 ~/.local/bin + 环境变量
# 或: cronctl install --root D:\path\to\sproutai-cron
# 新开终端后: cronctl status
```

Agent 在 Windows 上的试探顺序：

1. `cronctl.exe status --json`
2. `cronctl status --json`
3. `./cronctl.exe` / `dist/windows-amd64/cronctl.exe`
4. 提示 `go build ./cmd/cronctl`（或根目录 `node build.mjs`）+ `cronctl install`，并**新开终端**

机器可读输出加 `--json`，不要用正则硬抠表格。

## 调度怎么工作（读 status 时用）

1. **`cronctl web` 默认同进程启动调度器**；`--no-serve` 才会关掉。
2. 只有调度器在跑时，已 **enable** 的任务才会按 `schedule.cron` **秒级**评估（`@every` / `@random` / cron / `@on` / `@holiday`）。
3. `status`：调度器状态 + 任务表「调度 / 下次执行」。
4. **根目录必须一致**（`SPROUTAI_CRON_ROOT`）；`web` 日志里的 `root:` 为准。
5. 排障：调度器 → enable → 表达式 → `next_run_at` → `log`。

### schedule.cron 现代 DSL

| 表达式 | 含义 |
|--------|------|
| `@every 1s` / `@every 10s` | 固定间隔 |
| `@random 1s 60s` / `@random 30m 3h` | 随机间隔 |
| `0 10 * * 1` | 每周一 10:00（五段 cron） |
| `0 0 1 * *` | 每月 1 日 00:00 |
| `@holiday christmas` / `@on 12-25 00:00` | 圣诞节 |
| `@holiday cn-national-day` / `@on 10-01 00:00` | 国庆 |
| `@weekly mon 10:00` / `@monthly 1 00:00` | 语法糖 |

```text
调度器 ✅  → 定时链路通（serve 或 web）
调度器 ❌  → cronctl serve  或  cronctl web
```

JSON：`serve_running`、`tasks[].enabled`、`tasks[].schedule`、`tasks[].schedule_kind`、`tasks[].next_run_at`。

## 意图 → 命令

| 意图 | 命令 |
|------|------|
| 总览 / 默认真相 | `cronctl status`（默认可无子命令） |
| 列表（含描述） | `cronctl list` |
| 单任务 | `cronctl get <id>` |
| 启停 | `enable`/`on` · `disable`/`off` · `toggle` |
| 立刻跑一次 | `cronctl run <id>` |
| 日志 | `cronctl log <id> [--lines 200]` |
| 新建 | `cronctl create <id> [--runtime python\|javascript\|bash\|powershell] [--enable]` |
| 改点时间 | `cronctl update-schedule <id> "0 8 * * *" [--description "…"]` |
| 删除 | `cronctl delete <id> [--force]`（`rm` 同义） |
| 起调度 | `cronctl serve` |
| 注册全局命令 | `cronctl install`（`--self` 优先当前 exe） |
| 面板 | `cronctl web`（默认同启 serve；http://127.0.0.1:8765/） |
| 模板 | `cronctl templates` |

完整参数与 Web API：[references/api.md](references/api.md)。

## 推荐流程

### 首次或「命令找不到」

1. 有 `dist/` 可跳过编译；否则在 sproutai 根目录 `node build.mjs`，或 `go build ./cmd/cronctl`。
2. `cronctl install` 注册 PATH。
3. `cronctl serve` 常驻，再 `status` 确认调度器为运行中。

### 新建任务

1. `create <id> --runtime …` — 默认**禁用**，避免半成品进调度。
2. 改入口脚本与 `schedule.cron`（可先只写五段表达式）。
3. `run` → `log` 验证业务；用户明确要求后再 `enable`。
4. enable 前确认调度器已在跑，否则「开了也不到点执行」。

命名倾向 `<主机或场景>-<功能>`（如 `china-earthquake-fetch`）。  
`task.json` 声明 `runtime` + `entry`；控制面不依赖 Python，只有脚本本身是 Python 时才需要本机解释器。

### 排障「没按时跑」 / 「web 开了但调度器仍 ❌」

```text
cronctl.exe status --json          # Windows Git Bash 优先 .exe
  → serve_running false
      → 是否同一 SPROUTAI_CRON_ROOT / web 日志里的 root:
      → 是否 --no-serve
      → 删除陈旧锁后重启 web/serve：
          cron-tasks/.sprout-cron.serve.lock
          cron-tasks/.sprout-cron.serve.pid
      → 另起 cronctl serve 对照
  → enabled? schedule? log?
```

## 判断原则（附原因）

- **模板目录 `template-*` 当作复制源**：改它们会影响以后所有 `create`；用户没要求就别改、别删。
- **新建保持禁用直到试跑通过**：防止错误脚本按点轰炸；`--enable` 仅在用户明确要求时用。
- **改脚本后先 `run` 再依赖定时**：定时失败难以及时发现，手动跑+log 更短反馈环。
- **优先 cronctl，少手改目录搬迁**：enable/disable 会正确处理 `.disabled/`；手挪目录容易与锁/日志路径不一致。
- **解析用 `--json`**：人类表格为阅读设计，列宽会变。

## 任务落盘位置

```text
cron-tasks/<id>/           启用中
cron-tasks/.disabled/<id>/ 禁用
  task.json · schedule.cron · run.* · logs/<id>.log
```

Runner 注入：`CRON_TASK_ID`、`CRON_TASK_DIR`、`CRON_ROOT`。根定位：`SPROUTAI_CRON_ROOT` / `CRON_ROOT` 等，见 [reference.md](reference.md)。

## 输出约定（对用户）

- 操作后用一两句话说明：做了什么、调度器/任务当前状态、下一步（如需 serve 或 enable）。
- 展示关键命令输出摘要即可，不必整屏粘贴，除非用户要完整日志。
