# sproutai-cron 开发规则

## 控制面（Go 单二进制）

调度与管理由 **Go 编译的 `cronctl`** 完成（**不再依赖 Python 控制面**）：

```bash
# 编译（前端 go:embed 进二进制）
# 整包：在 sproutai 根目录 node build.mjs
# 仅本仓：go build -o cronctl.exe ./cmd/cronctl
# 产物也可在 dist/windows-amd64/cronctl.exe  dist/linux-amd64/cronctl

cronctl serve    # 常驻调度
cronctl web      # WebUI（Gin，嵌入式前端，默认同启 serve）
```

**不使用** 系统 cron.d / 任务计划。`schedule.cron` 支持现代化 DSL（秒级）：

| 写法 | 含义 |
|------|------|
| `0 8 * * *` | 经典五段 cron（分 时 日 月 周） |
| `*/10 * * * * *` | 六段（含秒） |
| `@every 10s` | 固定间隔 |
| `@random 1s 60s` / `@random 30m 3h` | 随机间隔 |
| `@on 12-25 00:00` | 每年固定日 |
| `@holiday christmas` / `@holiday cn-national-day` | 节日别名 |
| `@weekly mon 10:00` / `@monthly 1 00:00` | 语法糖 |

调度由 `cronctl serve` / `web` **秒级**到点触发（非系统 cron）。

## 新建定时任务

**所有任务与模板均在 `cron-tasks/` 下。** 默认从 `cron-tasks/template-python/` 复制：

| 模板目录 | 语言 | 入口 |
|---|---|---|
| `cron-tasks/template-python` | Python（默认） | `run.py` |
| `cron-tasks/template-javascript` | JavaScript | `run.js` |
| `cron-tasks/template-bash` | Bash | `run.sh` |
| `cron-tasks/template-powershell` | PowerShell | `run.ps1` |

推荐：

```bash
cronctl create <主机名>-<功能描述> --runtime python
# 编辑入口与 schedule.cron
cronctl run <task-id>
cronctl enable <task-id>   # 用户要求时再开；须已运行 serve
```

然后：

1. **任意语言**：只写业务逻辑；**禁用 / 锁 / 日志由 `cronctl run` / serve 统一处理**
2. 脚本 stdout/stderr 会进入 `logs/<task-id>.log`
3. 环境变量：`CRON_TASK_ID` / `CRON_TASK_DIR` / `CRON_ROOT`
4. 改 `schedule.cron`（cron 或 `@every` / `@random` 等）
5. `cronctl run` 试跑 → 默认保持禁用 → 用户要求再 `enable`

## task.json

```json
{
  "runtime": "python",
  "entry": "run.py"
}
```

`runtime`：`python` | `javascript` | `bash` | `powershell`

> 若任务脚本为 Python，本机仍需安装 Python 解释器；控制面本身不需要 Python。

## 不要修改

- `template*` 示例与复制源；除非用户要求
- 不要改成依赖系统 cron.d / schtasks

## 任务目录约定

```
cron-tasks/
├── <task-id>/
│   ├── task.json
│   ├── run.py|run.js|run.sh|run.ps1
│   ├── schedule.cron      # 五段 cron + 注释
│   └── logs/<task-id>.log
└── .disabled/
    └── <task-id>/
```

## 管理命令

```bash
cronctl serve
cronctl status
cronctl run <task-id>
cronctl enable <task-id>
cronctl web
```

WebUI：`cronctl web`（静态资源已嵌入二进制）

## AI Agent 集成

- **Skill**：`skills/sproutai-cron/SKILL.md` — 通过全局 `cronctl` 管理
