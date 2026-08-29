// Package notify sends Feishu/Lark webhook notifications.
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const defaultWebhook = ""

// SendMarkdown posts an interactive card with markdown body.
func SendMarkdown(title, markdown string) error {
	if title == "" || markdown == "" {
		return fmt.Errorf("飞书通知内容为空")
	}
	webhook := os.Getenv("LARK_NOTICE_WEBHOOK")
	if webhook == "" {
		webhook = defaultWebhook
	}
	if webhook == "" {
		return fmt.Errorf("未配置飞书 webhook")
	}
	text := bytes.ReplaceAll([]byte(markdown), []byte("\r\n"), []byte("\n"))
	text = bytes.ReplaceAll(text, []byte("\r"), []byte("\n"))
	payload := map[string]any{
		"msg_type": "interactive",
		"card": map[string]any{
			"schema": "2.0",
			"config": map[string]any{"wide_screen_mode": true, "enable_forward": true},
			"header": map[string]any{
				"title": map[string]any{"tag": "plain_text", "content": title},
			},
			"body": map[string]any{
				"elements": []map[string]any{
					{"tag": "markdown", "content": string(text)},
				},
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(webhook, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("网络错误: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("响应不是 JSON: %s", string(respBody))
	}
	if code, ok := result["code"]; ok {
		switch c := code.(type) {
		case float64:
			if c != 0 {
				return fmt.Errorf("飞书返回错误: code=%v msg=%v", code, result["msg"])
			}
		}
	}
	return nil
}
