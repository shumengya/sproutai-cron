// Package cli implements the cronctl command line interface.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"sproutai-cron/internal/api"
	"sproutai-cron/internal/daemon"
	"sproutai-cron/internal/install"
	"sproutai-cron/internal/manager"
	"sproutai-cron/internal/notify"
	"sproutai-cron/internal/root"
	"sproutai-cron/internal/runner"
)

// Execute runs the CLI.
func Execute() {
	// default command: status when no args
	if len(os.Args) == 1 {
		os.Args = append(os.Args, "status")
	} else if !isKnownCommand(os.Args[1]) && !strings.HasPrefix(os.Args[1], "-") {
		// bare task ids → status <ids>
		os.Args = append([]string{os.Args[0], "status"}, os.Args[1:]...)
	}

	var useJSON bool
	rootCmd := &cobra.Command{
		Use:           "cronctl",
		Short:         "管理 sproutai-cron 定时任务",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	rootCmd.PersistentFlags().BoolVar(&useJSON, "json", false, "以 JSON 格式输出结果")

	// status
	var statusTasks []string
	statusCmd := &cobra.Command{
		Use:   "status [task-id...]",
		Short: "查看任务状态（默认）",
		RunE: func(cmd *cobra.Command, args []string) error {
			cronRoot, err := root.Find()
			if err != nil {
				return err
			}
			_ = manager.MigrateLegacy(cronRoot)
			ids := args
			if len(ids) == 0 {
				ids = manager.IterTaskIDs(cronRoot)
			} else if len(ids) == 1 && ids[0] == "all" {
				ids = manager.IterTaskIDs(cronRoot)
			}
			serveOn := daemon.IsRunning(cronRoot)
			var tasks []manager.TaskInfo
			var errRows []string
			for _, id := range ids {
				info, err := manager.BuildTaskInfo(cronRoot, id)
				if err != nil {
					errRows = append(errRows, fmt.Sprintf("%s (%v)", id, err))
					continue
				}
				tasks = append(tasks, info)
			}
			if useJSON {
				return printJSON(map[string]any{"serve_running": serveOn, "count": len(tasks), "tasks": tasks})
			}
			printStatusView(serveOn, tasks, errRows)
			return nil
		},
	}
	_ = statusTasks
	rootCmd.AddCommand(statusCmd)

	// list
	rootCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "列出所有任务详情",
		RunE: func(cmd *cobra.Command, args []string) error {
			cronRoot, err := root.Find()
			if err != nil {
				return err
			}
			infos, err := manager.ListTasks(cronRoot)
			if err != nil {
				return err
			}
			if useJSON {
				return printJSON(map[string]any{"count": len(infos), "tasks": infos})
			}
			if len(infos) == 0 {
				fmt.Println("未找到可管理的任务。")
				return nil
			}
			printTaskTable(infos, true)
			return nil
		},
	})

	// get
	rootCmd.AddCommand(&cobra.Command{
		Use:   "get <task-id>",
		Short: "获取任务详情",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cronRoot, err := root.Find()
			if err != nil {
				return err
			}
			info, err := manager.BuildTaskInfo(cronRoot, args[0])
			if err != nil {
				return err
			}
			if useJSON {
				return printJSON(info)
			}
			printTaskInfo(info)
			return nil
		},
	})

	// enable / disable / toggle / on / off
	addStateCmd := func(name, help string, enabled *bool, toggle bool) {
		rootCmd.AddCommand(&cobra.Command{
			Use:   name + " [task-id...]",
			Short: help,
			RunE: func(cmd *cobra.Command, args []string) error {
				cronRoot, err := root.Find()
				if err != nil {
					return err
				}
				ids, err := resolveTaskIDs(cronRoot, args)
				if err != nil {
					return err
				}
				if len(ids) == 0 {
					return fmt.Errorf("%s 需要指定任务名", name)
				}
				var lastErr error
				for _, id := range ids {
					var info manager.TaskInfo
					var message string
					if toggle {
						info, message, err = manager.ToggleTask(cronRoot, id)
					} else {
						info, message, err = manager.SetTaskState(cronRoot, id, *enabled)
					}
					if err != nil {
						lastErr = err
						if useJSON {
							_ = printJSON(map[string]any{"ok": false, "error": err.Error(), "task_id": id})
						} else {
							fmt.Fprintf(os.Stderr, "%s: %v\n", id, err)
						}
						continue
					}
					if useJSON {
						_ = printJSON(map[string]any{"task": info, "message": message})
					} else {
						fmt.Printf("%s %s\n", enabledText(info.Enabled), id)
						if message != "" {
							fmt.Printf("  → %s\n", message)
						}
					}
				}
				return lastErr
			},
		})
	}
	en, dis := true, false
	addStateCmd("enable", "启用任务", &en, false)
	addStateCmd("disable", "禁用任务", &dis, false)
	addStateCmd("on", "启用任务", &en, false)
	addStateCmd("off", "禁用任务", &dis, false)
	addStateCmd("toggle", "切换任务状态", nil, true)

	// run
	rootCmd.AddCommand(&cobra.Command{
		Use:   "run [task-id...]",
		Short: "立即执行任务",
		RunE: func(cmd *cobra.Command, args []string) error {
			cronRoot, err := root.Find()
			if err != nil {
				return err
			}
			ids, err := resolveTaskIDs(cronRoot, args)
			if err != nil {
				return err
			}
			if len(ids) == 0 {
				return fmt.Errorf("run 需要指定任务名")
			}
			exitCode := 0
			for _, id := range ids {
				code, err := runner.Run(cronRoot, id)
				if err != nil {
					if useJSON {
						_ = printJSON(map[string]any{"task_id": id, "error": err.Error()})
					} else {
						fmt.Fprintf(os.Stderr, "%s: %v\n", id, err)
					}
					exitCode = 1
					continue
				}
				if useJSON {
					_ = printJSON(map[string]any{"task_id": id, "exit_code": code})
				} else {
					marker := "▶"
					if runtime.GOOS == "windows" {
						marker = ">"
					}
					fmt.Printf("%s %s (exit=%d)\n", marker, id, code)
				}
				if code != 0 {
					exitCode = code
				}
			}
			if exitCode != 0 {
				os.Exit(exitCode)
			}
			return nil
		},
	})

	// serve
	var serveOnce bool
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "启动常驻调度（到点执行已启用任务）",
		RunE: func(cmd *cobra.Command, args []string) error {
			if useJSON && !serveOnce {
				_ = printJSON(map[string]any{
					"ok":    false,
					"error": "serve 为常驻进程，请去掉 --json，或使用 --once 做单次探测",
				})
				return fmt.Errorf("serve 不支持 --json（除非 --once）")
			}
			cronRoot, err := root.Find()
			if err != nil {
				return err
			}
			_ = manager.MigrateLegacy(cronRoot)
			return daemon.RunServeLoop(cronRoot, serveOnce)
		},
	}
	serveCmd.Flags().BoolVar(&serveOnce, "once", false, "只评估当前分钟后退出")
	rootCmd.AddCommand(serveCmd)

	// log
	var logLines int
	logCmd := &cobra.Command{
		Use:   "log <task-id>",
		Short: "读取任务日志",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cronRoot, err := root.Find()
			if err != nil {
				return err
			}
			content, err := manager.ReadLogTail(cronRoot, args[0], logLines)
			if err != nil {
				return err
			}
			if useJSON {
				return printJSON(map[string]any{"task_id": args[0], "lines": logLines, "content": content})
			}
			if content != "" {
				fmt.Print(content)
				if !strings.HasSuffix(content, "\n") {
					fmt.Println()
				}
			}
			return nil
		},
	}
	logCmd.Flags().IntVar(&logLines, "lines", 200, "日志行数")
	rootCmd.AddCommand(logCmd)

	// create
	var createRuntime string
	var createEnable bool
	createCmd := &cobra.Command{
		Use:   "create <task-id>",
		Short: "从模板创建新任务（默认禁用）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cronRoot, err := root.Find()
			if err != nil {
				return err
			}
			info, message, templateName, err := manager.CreateTask(cronRoot, args[0], createRuntime, createEnable)
			if err != nil {
				return err
			}
			label := "已创建并禁用"
			if createEnable {
				label = "已创建并启用"
			}
			if useJSON {
				return printJSON(map[string]any{"task": info, "message": or(message, label), "template": templateName})
			}
			fmt.Printf("%s: %s (template: %s)\n", label, args[0], templateName)
			if message != "" {
				fmt.Printf("  → %s\n", message)
			}
			return nil
		},
	}
	createCmd.Flags().StringVar(&createRuntime, "runtime", "python", "python|javascript|bash|powershell")
	createCmd.Flags().BoolVar(&createEnable, "enable", false, "创建后立即启用")
	rootCmd.AddCommand(createCmd)

	// delete / rm
	var deleteForce bool
	deleteFn := func(cmd *cobra.Command, args []string) error {
		cronRoot, err := root.Find()
		if err != nil {
			return err
		}
		var lastErr error
		for _, id := range args {
			result, err := manager.DeleteTask(cronRoot, id, deleteForce)
			if err != nil {
				lastErr = err
				if useJSON {
					_ = printJSON(map[string]any{"ok": false, "error": err.Error(), "task_id": id})
				} else {
					fmt.Fprintf(os.Stderr, "%s: %v\n", id, err)
				}
				continue
			}
			if useJSON {
				result["ok"] = true
				_ = printJSON(result)
			} else {
				fmt.Printf("已删除: %s\n", id)
				if m, ok := result["message"].(string); ok && m != "" {
					fmt.Printf("  → %s\n", m)
				}
			}
		}
		return lastErr
	}
	delCmd := &cobra.Command{Use: "delete <task-id...>", Short: "删除任务目录", Args: cobra.MinimumNArgs(1), RunE: deleteFn}
	delCmd.Flags().BoolVarP(&deleteForce, "force", "f", false, "任务正在运行时仍强制删除")
	rmCmd := &cobra.Command{Use: "rm <task-id...>", Short: "删除任务目录", Args: cobra.MinimumNArgs(1), RunE: deleteFn}
	rmCmd.Flags().BoolVarP(&deleteForce, "force", "f", false, "任务正在运行时仍强制删除")
	rootCmd.AddCommand(delCmd, rmCmd)

	// update-schedule
	var schedDesc string
	var schedTags []string
	updCmd := &cobra.Command{
		Use:   "update-schedule <task-id> <cron>",
		Short: "更新调度表达式与描述",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cronRoot, err := root.Find()
			if err != nil {
				return err
			}
			hasTags := cmd.Flags().Changed("tags")
			info, message, err := manager.UpdateTaskSchedule(cronRoot, args[0], schedDesc, args[1], schedTags, hasTags)
			if err != nil {
				return err
			}
			if useJSON {
				return printJSON(map[string]any{"task": info, "message": or(message, "已保存")})
			}
			fmt.Printf("已保存: %s\n", args[0])
			if message != "" {
				fmt.Printf("  → %s\n", message)
			}
			return nil
		},
	}
	updCmd.Flags().StringVar(&schedDesc, "description", "", "描述")
	updCmd.Flags().StringSliceVar(&schedTags, "tags", nil, "标签")
	rootCmd.AddCommand(updCmd)

	// templates
	rootCmd.AddCommand(&cobra.Command{
		Use:   "templates",
		Short: "列出可用语言模板",
		RunE: func(cmd *cobra.Command, args []string) error {
			cronRoot, err := root.Find()
			if err != nil {
				return err
			}
			templates := manager.ListTemplates(cronRoot)
			if useJSON {
				return printJSON(map[string]any{"templates": templates})
			}
			for _, t := range templates {
				status := "[缺失]"
				if t["exists"].(bool) {
					status = "[OK]"
				}
				fmt.Printf("%-12s %-24s %-10s %s\n", t["runtime"], t["template_dir"], t["entry"], status)
			}
			return nil
		},
	})

	// install — register cronctl into ~/.local/bin (cross-platform, overwrite OK)
	var installSelf bool
	var installRoot string
	installCmd := &cobra.Command{
		Use:   "install",
		Short: "注册 cronctl 到用户 PATH（~/.local/bin，可覆盖）",
		Long: `将 cronctl 注册为当前用户的全局命令（Windows / macOS / Linux 同一实现，始终覆盖写入）。

定位项目根（忽略旧的 SPROUTAI_CRON_ROOT 优先权）:
  1. --root <路径>
  2. 当前工作目录向上查找 cron-tasks/
  3. 当前可执行文件所在树
  4. 环境变量（仅回退）

Windows: 写入 cronctl.cmd + cronctl.exe 副本 + Git Bash 包装，并设置 SPROUTAI_CRON_ROOT
Unix:    写入 ~/.local/bin/cronctl 包装脚本

请在目标仓库目录内执行:
  .\cronctl.exe install
  # 或: cronctl install --root D:\path\to\sproutai-cron`,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := install.Run(install.Options{
				Root:       installRoot,
				PreferSelf: installSelf,
			})
			if err != nil {
				if useJSON {
					_ = printJSON(map[string]any{"ok": false, "error": err.Error()})
				}
				return err
			}
			if useJSON {
				return printJSON(map[string]any{"ok": true, "result": res})
			}
			install.Print(os.Stdout, res)
			return nil
		},
	}
	installCmd.Flags().BoolVar(&installSelf, "self", false, "强制使用当前正在运行的可执行文件（默认：项目内运行时已优先自身）")
	installCmd.Flags().StringVar(&installRoot, "root", "", "指定项目根（默认从当前目录查找，不优先旧环境变量）")
	rootCmd.AddCommand(installCmd)

	// web
	var webHost, webPort string
	var webNoServe bool
	webCmd := &cobra.Command{
		Use:   "web",
		Short: "启动 WebUI（Gin）",
		RunE: func(cmd *cobra.Command, args []string) error {
			cronRoot, err := root.Find()
			if err != nil {
				return err
			}
			_ = manager.MigrateLegacy(cronRoot)
			if host := os.Getenv("SPROUTAI_CRON_WEB_HOST"); host != "" && !cmd.Flags().Changed("host") {
				webHost = host
			}
			if port := os.Getenv("SPROUTAI_CRON_WEB_PORT"); port != "" && !cmd.Flags().Changed("port") {
				webPort = port
			}
			// Frontend is embedded in the binary (webui/frontend via go:embed).
			srv := api.New(cronRoot)
			if !webNoServe {
				srv.EnsureScheduler()
				if daemon.IsRunning(cronRoot) {
					fmt.Println("[webui] scheduler: running (serve in-process)")
				} else {
					fmt.Println("[webui] scheduler: NOT running — timed tasks will not fire")
					fmt.Println("[webui] tip: check another cronctl on a different SPROUTAI_CRON_ROOT, or delete stale cron-tasks/.sprout-cron.serve.*")
				}
			} else {
				fmt.Println("[webui] scheduler: skipped (--no-serve)")
			}
			addr := webHost + ":" + webPort
			fmt.Printf("[webui] root: %s\n", cronRoot)
			fmt.Printf("[webui] starting: http://127.0.0.1:%s/ (embedded UI)\n", webPort)
			return srv.Engine().Run(addr)
		},
	}
	webCmd.Flags().StringVar(&webHost, "host", "0.0.0.0", "监听地址")
	webCmd.Flags().StringVar(&webPort, "port", "8765", "端口")
	webCmd.Flags().BoolVar(&webNoServe, "no-serve", false, "不自动启动调度器")
	rootCmd.AddCommand(webCmd)

	// notify feishu
	var notifyTitle, notifyBody string
	notifyCmd := &cobra.Command{
		Use:   "notify",
		Short: "发送通知",
	}
	feishuCmd := &cobra.Command{
		Use:   "feishu",
		Short: "发送飞书 Markdown 通知",
		RunE: func(cmd *cobra.Command, args []string) error {
			if notifyTitle == "" || notifyBody == "" {
				return fmt.Errorf("需要 --title 与 --body")
			}
			if err := notify.SendMarkdown(notifyTitle, notifyBody); err != nil {
				if useJSON {
					_ = printJSON(map[string]any{"ok": false, "error": err.Error()})
				}
				return err
			}
			if useJSON {
				return printJSON(map[string]any{"ok": true})
			}
			fmt.Println("已发送飞书通知")
			return nil
		},
	}
	feishuCmd.Flags().StringVar(&notifyTitle, "title", "", "标题")
	feishuCmd.Flags().StringVar(&notifyBody, "body", "", "Markdown 正文")
	notifyCmd.AddCommand(feishuCmd)
	rootCmd.AddCommand(notifyCmd)

	if err := rootCmd.Execute(); err != nil {
		if !useJSON {
			fmt.Fprintln(os.Stderr, err)
		} else {
			_ = printJSON(map[string]any{"ok": false, "error": err.Error()})
		}
		os.Exit(1)
	}
}

func isKnownCommand(s string) bool {
	switch s {
	case "status", "list", "get", "enable", "disable", "toggle", "on", "off",
		"run", "serve", "log", "create", "delete", "rm", "update-schedule",
		"templates", "web", "notify", "install", "help", "completion":
		return true
	}
	return false
}

func resolveTaskIDs(cronRoot string, args []string) ([]string, error) {
	if len(args) == 0 {
		return nil, nil
	}
	if len(args) == 1 && args[0] == "all" {
		return manager.IterTaskIDs(cronRoot), nil
	}
	return args, nil
}

func enabledText(on bool) string {
	if on {
		return "✅"
	}
	return "❌"
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printTaskInfo(info manager.TaskInfo) {
	sched := "-"
	if info.Schedule != nil {
		sched = *info.Schedule
	}
	desc := "-"
	if info.Description != nil {
		desc = *info.Description
	}
	logUp := "-"
	if info.LogUpdated != nil {
		logUp = *info.LogUpdated
	}
	fmt.Printf("task_id     : %s\n", info.TaskID)
	fmt.Printf("enabled     : %s\n", enabledText(info.Enabled))
	fmt.Printf("running     : %v\n", info.Running)
	fmt.Printf("schedule    : %s\n", sched)
	fmt.Printf("description : %s\n", desc)
	fmt.Printf("runtime     : %s\n", info.Runtime)
	fmt.Printf("tags        : %s\n", strings.Join(info.Tags, ", "))
	fmt.Printf("log_size    : %d\n", info.LogSize)
	fmt.Printf("log_updated : %s\n", logUp)
	fmt.Printf("task_dir    : %s\n", info.TaskDir)
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// ── status / list pretty printers ──────────────────────────────────────────

func rule(width int) string {
	if width < 40 {
		width = 40
	}
	return strings.Repeat("═", width)
}

func printStatusView(serveOn bool, tasks []manager.TaskInfo, errRows []string) {
	enabledN, runningN := 0, 0
	for _, t := range tasks {
		if t.Enabled {
			enabledN++
		}
		if t.Running {
			runningN++
		}
	}

	cols := measureTaskCols(tasks)
	// 2 leading spaces + status(4) + gaps + columns + last col (log 19)
	width := 2 + 4 + 2 + cols.idW + 2 + cols.rtW + 2 + cols.schW + 2 + 19
	if width < 56 {
		width = 56
	}

	fmt.Println(rule(width))
	fmt.Println("  sprout-cron")

	if serveOn {
		fmt.Printf("  调度器    %s  运行中\n", enabledText(true))
	} else {
		fmt.Printf("  调度器    %s  未启动\n", enabledText(false))
		fmt.Println("  提示      请运行: cronctl serve  或  cronctl web（默认同启调度）")
	}

	if len(tasks) == 0 && len(errRows) == 0 {
		fmt.Println("  任务      (无)")
		fmt.Println(rule(width))
		return
	}

	summary := fmt.Sprintf("共 %d · 启用 %d · 禁用 %d", len(tasks), enabledN, len(tasks)-enabledN)
	if runningN > 0 {
		summary += fmt.Sprintf(" · 执行中 %d", runningN)
	}
	fmt.Printf("  任务      %s\n", summary)
	fmt.Println(rule(width))
	printTaskTable(tasks, false, cols, width)

	if len(errRows) > 0 {
		fmt.Println(rule(width))
		fmt.Println("  无法读取:")
		for _, row := range errRows {
			fmt.Printf("    · %s\n", row)
		}
	}
	fmt.Println(rule(width))
}

type taskCols struct {
	idW, rtW, schW int
}

func measureTaskCols(tasks []manager.TaskInfo) taskCols {
	c := taskCols{idW: 8, rtW: 8, schW: 8}
	for _, t := range tasks {
		if n := displayWidth(t.TaskID); n > c.idW {
			c.idW = n
		}
		if n := displayWidth(t.Runtime); n > c.rtW {
			c.rtW = n
		}
		sch := "-"
		if t.Schedule != nil && *t.Schedule != "" {
			sch = *t.Schedule
		}
		if n := displayWidth(sch); n > c.schW {
			c.schW = n
		}
	}
	if c.idW > 32 {
		c.idW = 32
	}
	if c.rtW > 14 {
		c.rtW = 14
	}
	if c.schW > 28 {
		c.schW = 28
	}
	return c
}

// printTaskTable renders tasks as an aligned table.
// withDesc=true adds a description column (used by list).
// Optional trailing args: cols (taskCols), ruleWidth (int).
func printTaskTable(tasks []manager.TaskInfo, withDesc bool, opts ...any) {
	if len(tasks) == 0 {
		return
	}
	var c taskCols
	ruleWidth := 0
	for _, o := range opts {
		switch v := o.(type) {
		case taskCols:
			c = v
		case int:
			ruleWidth = v
		}
	}
	if c.idW == 0 {
		c = measureTaskCols(tasks)
	}
	idW, rtW, schW := c.idW, c.rtW, c.schW
	if ruleWidth <= 0 {
		lastW := 19
		if withDesc {
			lastW = 12
		}
		ruleWidth = 2 + 4 + 2 + idW + 2 + rtW + 2 + schW + 2 + lastW
		if ruleWidth < 56 {
			ruleWidth = 56
		}
	}

	// header
	lastHead := "下次执行"
	if withDesc {
		lastHead = "描述"
	}
	fmt.Printf("  %s  %s  %s  %s  %s\n",
		padRight("状态", 4),
		padRight("任务", idW),
		padRight("运行时", rtW),
		padRight("调度", schW),
		lastHead,
	)
	// one continuous rule (not per-column segments)
	fmt.Println(rule(ruleWidth))

	for _, t := range tasks {
		st := enabledText(t.Enabled)
		// emoji ≈ 2 cells; pad status column to ~4 cells visually
		stCell := st + "  "
		if t.Running {
			stCell = st + "▶ "
		}

		sch := "-"
		if t.Schedule != nil && *t.Schedule != "" {
			sch = *t.Schedule
		}
		rt := t.Runtime
		if rt == "" {
			rt = "-"
		}

		last := "-"
		if withDesc {
			if t.Description != nil {
				last = *t.Description
			} else {
				last = ""
			}
		} else if t.NextRunAt != nil && *t.NextRunAt != "" {
			last = *t.NextRunAt
		}

		fmt.Printf("  %s%s  %s  %s  %s\n",
			stCell,
			padRight(truncate(t.TaskID, idW), idW),
			padRight(truncate(rt, rtW), rtW),
			padRight(truncate(sch, schW), schW),
			last,
		)
	}
}

// displayWidth estimates terminal columns for s (ASCII=1, most CJK/emoji=2).
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		switch {
		case r == 0xFE0F: // variation selector
			continue
		case r == 0x200D: // ZWJ
			continue
		case r >= 0x1F300: // emoji blocks (rough)
			w += 2
		case r >= 0x1100 && isWideRune(r):
			w += 2
		default:
			w++
		}
	}
	return w
}

func isWideRune(r rune) bool {
	// Common East Asian / fullwidth ranges (simplified).
	switch {
	case r >= 0x1100 && r <= 0x115F:
		return true
	case r >= 0x2E80 && r <= 0xA4CF:
		return true
	case r >= 0xAC00 && r <= 0xD7A3:
		return true
	case r >= 0xF900 && r <= 0xFAFF:
		return true
	case r >= 0xFE10 && r <= 0xFE6F:
		return true
	case r >= 0xFF00 && r <= 0xFF60:
		return true
	case r >= 0xFFE0 && r <= 0xFFE6:
		return true
	case r >= 0x20000 && r <= 0x3FFFD:
		return true
	}
	return false
}

func padRight(s string, width int) string {
	pad := width - displayWidth(s)
	if pad <= 0 {
		return s
	}
	return s + strings.Repeat(" ", pad)
}

func truncate(s string, maxWidth int) string {
	if displayWidth(s) <= maxWidth {
		return s
	}
	if maxWidth <= 1 {
		return "…"
	}
	// cut runes until it fits with ellipsis
	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := 1
		if r >= 0x1100 && (isWideRune(r) || r >= 0x1F300) {
			rw = 2
		}
		if w+rw > maxWidth-1 {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	b.WriteRune('…')
	return b.String()
}
