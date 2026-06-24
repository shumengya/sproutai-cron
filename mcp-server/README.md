# SproutClaw Cron MCP Server

通过 [Model Context Protocol](https://modelcontextprotocol.io/) 让 AI Agent 管理本项目的定时任务。

## 安装

```bash
pip install -r requirements.txt
# 或
pip install fastmcp
```

## 在 Cursor 中使用

项目已包含 `.cursor/mcp.json`。打开本仓库后 Cursor 会自动加载 `sproutclaw-cron` MCP 服务器。

手动测试（stdio）：

```bash
python mcp-server/server.py
```

## 环境变量

| 变量 | 说明 |
|---|---|
| `SPROUTCLAW_CRON_ROOT` | 定时任务项目根目录；默认自动推断为 `mcp-server/` 的上级目录 |

## 工具一览

| 工具 | 说明 |
|---|---|
| `cron_list_tasks` | 列出全部任务 |
| `cron_get_task` | 任务详情 |
| `cron_enable_task` / `cron_disable_task` / `cron_toggle_task` | 开关 |
| `cron_run_task` | 立即执行 |
| `cron_get_log` | 读日志 |
| `cron_update_schedule` | 改 cron 表达式 / 描述 / 标签 |
| `cron_sync_cron` | 同步 cron.d / schtasks |
| `cron_create_task` | 从模板创建新任务 |
| `cron_list_templates` | 可用模板 |
