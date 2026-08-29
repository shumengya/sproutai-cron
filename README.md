# sproutai-cron — 定时任务系统

树萌芽的定时任务管理框架。每个任务一目录，**控制面为 Go 单二进制 `cronctl`**（CLI + 调度 + Gin WebUI），任务脚本支持多语言。

---

## 目录结构

```
sproutai-cron/
├── cmd/cronctl/               # Go 入口
├── internal/                  # 核心库（manager / runner / daemon / api …）
├── webui/
│   ├── embed.go               # go:embed 打进二进制
│   └── frontend/              # 管理面板源文件（构建时嵌入）
├── dist/
│   ├── windows-amd64/cronctl.exe
│   └── linux-amd64/cronctl
├── AGENTS.md                  # AI 新建任务规范
└── cron-tasks/                # ★ 所有任务与模板
    ├── template-python/
    ├── template-javascript/
    ├── template-bash/
    ├── template-powershell/
    ├── <task-id>/
    │   ├── task.json
    │   ├── run.py|run.js|run.sh|run.ps1
    │   ├── schedule.cron
    │   └── logs/<task-id>.log
    └── .disabled/
        └── <task-id>/
```

**任务 ID 命名**：`<主机名>-<功能描述>`，例如 `china-earthquake-fetch`。

---

## 编译

需安装 [Go](https://go.dev/) 1.21+。前端 `webui/frontend` 会通过 `go:embed` **打进二进制**，`cronctl web` 无需外挂静态目录。

```bash
# 推荐：在 sproutai monorepo 根目录整包构建（含 cronctl 交叉编译）
node build.mjs

# 或仅编译本仓库
go build -o cronctl.exe ./cmd/cronctl          # Windows 本机
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o dist/linux-amd64/cronctl ./cmd/cronctl
```

产物：

| 文件 | 平台 |
|------|------|
| `dist/windows-amd64/cronctl.exe` | Windows |
| `dist/linux-amd64/cronctl` | Linux |

```bash
# 仅本机开发
go build -o cronctl.exe ./cmd/cronctl
```

控制面**不需要** Python / Node / uv。仅当任务脚本本身使用对应语言时，本机需有该解释器。

---

## 用法

### 1. 注册 `cronctl`（推荐）

跨平台同一实现：`cronctl install`（Go，**始终覆盖**包装器/环境变量）。  
定位项目根优先 **当前目录**，不沿用旧的 `SPROUTAI_CRON_ROOT`，便于在 release 副本或另一份仓库上重新注册。

**Windows：**

```bat
cd D:\path\to\sproutai-cron
go build -o cronctl.exe ./cmd/cronctl
.\cronctl.exe install
rem 或: cronctl install --root D:\path\to\sproutai-cron
rem 新开终端：
cronctl status
```

**Linux / macOS：**

```bash
cd /path/to/sproutai-cron
go build -o cronctl ./cmd/cronctl
./cronctl install   # 或 ./dist/linux-amd64/cronctl install
cronctl status
```

### 2. 命令速查

| 命令 | 说明 |
|------|------|
| `cronctl install` | **注册到用户 PATH**（`~/.local/bin`） |
| `cronctl serve` | **启动常驻调度** |
| `cronctl web` | **WebUI**（Gin，默认同启 serve） |
| `cronctl status` | 调度器是否在跑 + 任务调度 / 下次执行 |
| `cronctl list` | 列出全部任务详情 |
| `cronctl get <任务名>` | 查看单个任务详情 |
| `cronctl enable / disable / toggle` | 开关 |
| `cronctl run <任务名>` | 立即执行一次 |
| `cronctl log <任务名> [--lines N]` | 读取日志末尾 |
| `cronctl create <任务名> [--runtime …]` | 从模板新建（默认禁用） |
| `cronctl delete <任务名> [--force]` | 删除 |
| `cronctl update-schedule …` | 修改调度 / 描述 / 标签 |
| `cronctl templates` | 列出语言模板 |
| `cronctl notify feishu --title … --body …` | 飞书通知 |

```bash
cronctl serve
cronctl status
cronctl run template-python
cronctl enable china-earthquake-fetch
cronctl create my-demo-task --runtime python
```

### 3. 开关与调度

**不再依赖** Linux `/etc/cron.d` 或 Windows 任务计划。  
只要 **`cronctl serve`（或 `cronctl web`）一直在跑**，就会**秒级**检查已启用任务的 `schedule.cron`。

| 状态 | 目录位置 | serve 到点 | `cronctl run` |
|------|----------|------------|---------------|
| 启用 | `cron-tasks/<task-id>/` | 执行 | 执行 |
| 禁用 | `cron-tasks/.disabled/<task-id>/` | 跳过 | 跳过 |

`schedule.cron` 示例：

```cron
# 每天 08:00（经典五段）
0 8 * * *

# 或现代 DSL：
# @every 10s
# @random 1s 60s
# @random 30m 3h
# @holiday cn-national-day
# @on 12-25 00:00
# @weekly mon 10:00
```

禁用 / 互斥锁 / 日志由 **Go runner** 统一处理；间隔类状态在 `cron-tasks/.schedule-state/`。

### 4. 新建任务

```bash
cronctl create smallmengya-my-new-task --runtime python
# 编辑 cron-tasks/.../run.py 与 schedule.cron
cronctl run smallmengya-my-new-task
# 确认后再 enable
```

#### task.json

```json
{
  "runtime": "python",
  "entry": "run.py",
  "tags": ["example"]
}
```

| runtime | 入口 | 依赖 |
|---------|------|------|
| `python` | `run.py` | Python 3（仅任务） |
| `javascript` | `run.js` | Node.js |
| `bash` | `run.sh` | bash |
| `powershell` | `run.ps1` | pwsh / powershell |

### 5. WebUI

前端已嵌入二进制，直接：

```bash
cronctl web
# 或
./dist/windows-amd64/cronctl.exe web
./dist/linux-amd64/cronctl web
```

默认 http://127.0.0.1:8765/

---

## 环境变量

| 变量 | 说明 |
|------|------|
| `SPROUTAI_CRON_ROOT` / `CRON_ROOT` | 项目根（含 `cron-tasks/`） |
| `SPROUTAI_CRON_WEB_HOST` / `PORT` | WebUI 监听 |
| `LARK_NOTICE_WEBHOOK` | 飞书机器人 webhook |
| `CRON_LOG_MAX_BYTES` | 日志轮转阈值（默认 10MB） |

任务执行时注入：`CRON_TASK_ID`、`CRON_TASK_DIR`、`CRON_ROOT`。

---

## 开发

```bash
go run ./cmd/cronctl status
go run ./cmd/cronctl web
go test ./...
```

规范见 [AGENTS.md](./AGENTS.md)，Agent Skill 见 [skills/sproutai-cron/SKILL.md](./skills/sproutai-cron/SKILL.md)。
