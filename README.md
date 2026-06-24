# sproutclaw-cron — 定时任务系统

树萌芽的定时任务管理框架。每个任务一目录，共享公共库，统一管理开关与日志。

---

## 目录结构

```
/shumengya/project/agent/sproutclaw-cron/
├── cronctl.py                        # 统一管理 CLI（enable/disable/toggle/status）
├── AGENTS.md                         # AI 新建任务规范（必读）
├── _template/                        # Python 示例 + 默认复制模板
├── _template-javascript/             # JavaScript (Node.js) 示例
├── _template-bash/                   # Bash 示例
├── _template-powershell/             # PowerShell 示例
├── lib/
│   └── shumengya_cron/
│       ├── runner.py                 # TaskContext、日志轮转、互斥锁
│       ├── task_manifest.py          # task.json 解析、runtime 推断
│       ├── external_runner.py        # 非 Python 任务 subprocess 启动
│       ├── notify.py                 # 飞书 Markdown 通知 + 邮件降级
│       └── ssh.py                    # 远端 SSH 辅助（bash -lc）
├── <task-id>/                        # 任务目录（启用状态）
│   ├── task.json                     # 可选：runtime + entry
│   ├── run.py / run.js / run.sh / run.ps1
│   ├── schedule.cron                 # 系统 cron 配置（复制到 /etc/cron.d/）
│   ├── targets.json                  # 可选：任务自定义配置（JSON）
│   ├── logs/<task-id>.log            # 运行日志（自动创建）
│   └── <task-id>.lock                # flock 互斥锁文件（自动创建）
└── .disabled/                        # 禁用任务统一存放目录
    └── <task-id>/                    # 关闭的任务移入此处
```

**任务 ID 命名约定**：`<主机名>-<功能描述>`，例如 `bigmengya-docker-image-update`。

---

## 快速上手

### 查看状态
```bash
python3 /shumengya/project/agent/sproutclaw-cron/cronctl.py status
```

### 开关任务
```bash
python3 /shumengya/project/agent/sproutclaw-cron/cronctl.py enable  <任务名>
python3 /shumengya/project/agent/sproutclaw-cron/cronctl.py disable <任务名>
python3 /shumengya/project/agent/sproutclaw-cron/cronctl.py toggle  <任务名>
python3 /shumengya/project/agent/sproutclaw-cron/cronctl.py enable  all      # 全部开启
python3 /shumengya/project/agent/sproutclaw-cron/cronctl.py disable all      # 全部关闭
```

### 手动运行任务
```bash
# 推荐：通过 cronctl（自动识别 .disabled/ 下的任务）
python3 /shumengya/project/agent/sproutclaw-cron/cronctl.py run <任务名>

# 也可直接运行 Python 任务（启用或 .disabled/<任务名> 目录均可）
python3 /shumengya/project/agent/sproutclaw-cron/.disabled/<任务名>/run.py
# 其他语言任务请始终用 cronctl run
```

### 安装到系统 cron

**Linux**：`cronctl enable` 会将 `schedule.cron` 复制到 `/etc/cron.d/<task-id>`；`disable` **只**把任务目录移入 `.disabled/<task-id>/`，**不会**删除 `/etc/cron.d/` 条目。cron 到点仍会触发，但禁用后任务会自动跳过。

```bash
python3 /shumengya/project/agent/sproutclaw-cron/cronctl.py sync-cron all   # 仅同步 cron.d，不改开关
```

**Windows**：`enable` / `sync-cron` 会通过 `schtasks` 注册任务计划（任务名 `sproutclaw-<task-id>`）。目前自动转换 `分 时 * * *` 形式的 cron 表达式为「每天 HH:MM」；复杂表达式需手动创建计划任务。

```powershell
python D:\SmyProjects\AI\sproutclaw-cron\cronctl.py sync-cron all
python D:\SmyProjects\AI\sproutclaw-cron\cronctl.py enable  <任务名>
```

开关机制与 Linux 相同：禁用后目录移入 `.disabled/`，计划任务仍会触发，但任务会自动跳过。

---

## 开关机制

- **启用**：从 `.disabled/<task-id>/` 移回 `<task-id>/`
- **关闭**：从 `<task-id>/` 移入 `.disabled/<task-id>/`
- 禁用时 `cronctl run` 会直接跳过，不写日志、不获取锁
- `/etc/cron.d/<task-id>` 可一直保留；`schedule.cron` 通过 `cronctl.py run <task-id>` 调度

---

## 多语言支持

调度入口统一为 `cronctl.py run`（Python）。业务脚本语言由 `task.json` 或入口文件决定：

| runtime | 入口 | 依赖 |
|---|---|---|
| `python`（默认） | `run.py` | Python 3 |
| `javascript` | `run.js` | Node.js |
| `bash` | `run.sh` | bash |
| `powershell` | `run.ps1` | pwsh / powershell |

### task.json

```json
{
  "runtime": "python",
  "entry": "run.py"
}
```

无 `task.json` 时：有 `run.py` 视为 Python 任务（向后兼容）。

---

## 新建任务（模板）

**默认从 `_template/`（Python）复制**；JavaScript / Bash / PowerShell 见 `_template-javascript` 等目录。

```bash
TASK_ID="smallmengya-my-new-task"
CRON_ROOT="/shumengya/project/agent/sproutclaw-cron"

cp -a "$CRON_ROOT/_template" "$CRON_ROOT/$TASK_ID"
sed -i "s/_template/$TASK_ID/g" "$CRON_ROOT/$TASK_ID/schedule.cron"

# 编辑 run.py 与 schedule.cron 后试跑
python3 "$CRON_ROOT/cronctl.py" run "$TASK_ID"
python3 "$CRON_ROOT/cronctl.py" enable "$TASK_ID"
```

详细约定见 `_template/README.md`；AI 代理见根目录 `AGENTS.md`。

### AI Agent 集成

- **Skill**：`.cursor/skills/sproutclaw-cron/` — Cursor Agent 自动识别定时任务操作规范
- **MCP**：`.cursor/mcp.json` + `mcp-server/server.py` — 暴露 `cron_list_tasks`、`cron_run_task` 等工具
- 首次使用 MCP：`pip install -r mcp-server/requirements.txt`，然后在 Cursor 设置中确认 MCP 服务器已启用

---

## 公共库 API

### `runner.py`

| 接口 | 说明 |
|---|---|
| `TaskContext.from_task_id(task_id, *, log_mode, cron_root)` | 构造任务上下文（自动识别 `.disabled/<task-id>/`） |
| `task_is_disabled(ctx)` | 任务目录位于 `.disabled/` 下则返回 True |
| `task_is_enabled(ctx)` | 同上取反 |
| `set_task_enabled(ctx, enabled)` | 重命名目录切换状态 |
| `task_logging(ctx)` | 上下文管理器，返回 `log(msg)` 函数；轮转日志、tee 到终端 |
| `acquire_cron_lock(lock_file, *, log)` | flock 互斥锁，已占用时 yield False |
| `cron_log_rotate(log_file, max_bytes)` | 超过阈值时轮转日志（默认 10MB） |
| `append_raw_to_log(log_file, text)` | 将命令原始输出追加到日志 |
| `task_output_prefix(task_id)` | 返回 `[smallmengya][AI-CLI-Update]` 格式前缀 |
| `LogMode.REDIRECT_STD` | stdout/stderr 重定向到日志（默认） |
| `LogMode.DIRECT_FILE` | 仅写文件，不重定向标准流 |

### `notify.py`

| 接口 | 说明 |
|---|---|
| `TaskResult` | 任务结果容器，`add_ok/add_skip/add_fail(item)` 累积条目，`.success` / `.status_cn` / `.total` 供汇总使用 |
| `send_task_summary(ctx, result, start_time, *, log, extra_fields)` | 发送标准 ok/skip/fail 汇总通知（飞书 + 邮件降级） |
| `send_feishu_markdown(title, markdown, *, log)` | 发飞书通知；失败自动降级邮件 |
| `markdown_list(items)` | `["a","b"]` → `"- a\n- b"` |
| `markdown_fields(items)` | `[("key","val")]` → `"- **key**：val\n..."` |

### `ssh.py`

| 接口 | 说明 |
|---|---|
| `ssh_bash_lc(host, remote_cmd)` | 用 `bash -lc` 执行远程命令，PATH 与登录 shell 一致 |

---

## 环境变量

### 公共（所有任务）

| 变量 | 默认 | 说明 |
|---|---|---|
| `CRON_LOG_MAX_BYTES` | `10485760`（10MB）| 日志轮转阈值 |
| `CRON_LOCK_BUSY_MSG` | 已有任务在运行… | 锁冲突时的日志消息 |
| `LARK_NOTICE_WEBHOOK` | 内置默认 webhook | 飞书机器人 webhook |
| `LARK_NOTICE_API_SRC` | `/shumengya/project/python/lark-notice-api/src` | lark-notice-api 源码路径 |
| `CRON_MAIL_ENABLED` | `0` | 设为 `1` 开启邮件降级通知 |
| `MAIL_API_SCRIPT` | mengya-mail-api 路径 | 邮件发送脚本路径 |
| `MAIL_TO` | `mail@smyhub.com` | 收件人 |

### bigmengya-docker-container-restart

| 变量 | 默认 | 说明 |
|---|---|---|
| `REMOTE_HOST` | `bigmengya` | SSH 目标主机 |
| `REMOTE_DOCKER_BIN` | `docker` | 远端 docker 命令路径 |
| `DB_CONTAINERS` | mysql-8 redis-7 … | 目标容器名（空格分隔） |
| `DB_PATTERNS` | mysql redis mongo … | 镜像名匹配模式（空格分隔） |

### bigmengya-docker-image-update

| 变量 | 默认 | 说明 |
|---|---|---|
| `REMOTE_HOST` | `bigmengya` | SSH 目标主机 |
| `PULL_RETRIES` | `3` | pull 失败重试次数 |
| `PULL_RETRY_DELAY_SECONDS` | `15` | 重试间隔秒数 |

### smallmengya-ai-cli-update

| 变量 | 默认 | 说明 |
|---|---|---|
| `CRON_TASK_PATH` | `/root/bin:/root/.opencode/bin:…` | 查找 CLI 工具的 PATH |
| `UPDATE_TIMEOUT_SECONDS` | `500` | 单个工具更新超时秒数 |
| `CLI_TARGETS` | 全部 | 逗号分隔，限制只更新指定工具 |
| `HTTP_PROXY` / `HTTPS_PROXY` | `socks5://192.168.1.1:7891` | 代理地址 |
| `NPM_REGISTRY_URL` | `https://registry.npmmirror.com` | npm 镜像 |

### smallmengya-gitea-repo-sync

| 变量 | 默认 | 说明 |
|---|---|---|
| `BASE_DIR` | `/shumengya/project/cloudflare` | 本地仓库基础目录 |
| `SYNC_DELAY_SECONDS` | `2` | 每个仓库同步间隔秒数 |
| `GIT_SSH_COMMAND` | BatchMode=yes … | git SSH 参数 |

---

## 剩余可优化方向

以下问题有优化价值，但影响面较小，暂未实施：

### 1. lib 路径注入样板
每个 `run.py` 顶部都有 3 行 `sys.path.insert` 代码，是为了支持 `schedule.cron` 直接调用 `python3 run.py`。
若将 `schedule.cron` 改为调用 `cronctl run <task-id>`，则 `run.py` 里的路径注入可以全部删掉。

---

## 任务配置文件格式（JSON）

任务自定义配置统一使用 JSON，不存在时任务会自动生成默认内容。

### `targets.json`（bigmengya-docker-image-update）

```json
[
  {"label": "myapp", "workdir": "/shumengya/docker/myapp", "service": "myapp"},
  {"label": "another", "workdir": "/shumengya/docker/another", "service": "web"}
]
```

| 字段 | 说明 |
|---|---|
| `label` | 显示名（日志/通知中使用） |
| `workdir` | compose 项目目录（远端路径） |
| `service` | compose service 名 |

### `repos.json`（smallmengya-gitea-repo-sync）

```json
[
  {"repo": "shumengya/my-repo"},
  {"repo": "shumengya/another-repo", "local": "/custom/local/path"}
]
```

| 字段 | 说明 |
|---|---|
| `repo` | `owner/repo`（省略 owner 默认 `shumengya`） |
| `local` | 本地同步路径（省略时用 `BASE_DIR/repo名`） |

---

## 依赖说明

这套系统**只依赖 Python 3 标准库**（`subprocess`、`pathlib` 等），无需安装任何 PyPI 包。文件锁在 Linux 用 `fcntl`，Windows 用 `msvcrt`；定时调度在 Linux 用 `/etc/cron.d/`，Windows 用任务计划程序（`schtasks`）。

通知功能依赖两个本地项目（非 PyPI 依赖，可缺失时降级）：
- `lark-notice-api`：飞书通知，路径 `/shumengya/project/python/lark-notice-api/`
- `mengya-mail-api`：邮件降级，路径 `/shumengya/project/skills/mengya-mail-skills/`

SSH 功能依赖系统 `ssh` 命令和已配置的 SSH 密钥对。
