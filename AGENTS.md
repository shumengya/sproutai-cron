# sproutclaw-cron 开发规则

## 新建定时任务

**必须**从 `_template/` 复制（`_template` 本身是可运行的 hello world 示例任务）。

```bash
TASK_ID="<主机名>-<功能描述>"
CRON_ROOT="/shumengya/project/agent/sproutclaw-cron"
cp -a "$CRON_ROOT/_template" "$CRON_ROOT/$TASK_ID"
sed -i "s/_template/$TASK_ID/g" "$CRON_ROOT/$TASK_ID/schedule.cron"
chmod +x "$CRON_ROOT/$TASK_ID/switch.sh"
```

然后：

1. 修改 `run.py`：保留 `task_is_disabled` / `task_logging` / `acquire_cron_lock` 结构，替换 `hello world` 为实际逻辑
2. 修改 `schedule.cron`：调整 cron 表达式与注释说明
3. 可选：添加任务专属 JSON 配置（如 `targets.json`）
4. 用 `python3 cronctl.py run <task-id>` 试跑，确认日志正常
5. 默认保持关闭；用户要求启用时再 `cronctl enable <task-id>`

## 不要修改

- `_template/` 是 hello world 示例任务，也是新任务复制源；除非用户要求，不要改其示例行为
- 不要删除或移动已有任务的 `schedule.cron` 里对 `cronctl.py run` 的调用方式

## 任务目录约定

```
<task-id>/
├── run.py
├── schedule.cron
├── switch.sh          # 可选
├── logs/<task-id>.log # 运行时自动创建
└── *.json             # 可选配置
```

禁用任务位于 `.disabled/<task-id>/`，开关用 `cronctl enable|disable|toggle`。

## 管理命令

```bash
python3 /shumengya/project/agent/sproutclaw-cron/cronctl.py status
python3 /shumengya/project/agent/sproutclaw-cron/cronctl.py run <task-id>
python3 /shumengya/project/agent/sproutclaw-cron/cronctl.py enable <task-id>
```

WebUI：`/shumengya/project/agent/sproutclaw-cron/webui/start.sh`
