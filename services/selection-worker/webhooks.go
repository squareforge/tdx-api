package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type WebhookEvent struct {
	Event          string      `json:"event"`
	TaskID         string      `json:"task_id,omitempty"`
	TaskName       string      `json:"task_name,omitempty"`
	TaskType       string      `json:"task_type,omitempty"`
	RunID          string      `json:"run_id,omitempty"`
	Status         string      `json:"status,omitempty"`
	Message        string      `json:"message,omitempty"`
	MatchedCount   int         `json:"matched_count,omitempty"`
	MatchedSymbols []string    `json:"matched_symbols,omitempty"`
	Result         interface{} `json:"result,omitempty"`
	RunAt          string      `json:"run_at"`
}

// A remote rejection is safe to retry. Network failures may have happened after
// delivery, so holding alerts keep those attempts as uncertain instead.
type webhookRejectedError struct{ error }

func sendWebhooks(ctx context.Context, hooks []Webhook, event WebhookEvent) []string {
	if len(hooks) == 0 {
		return nil
	}
	event.RunAt = nowText()
	logs := make([]string, 0, len(hooks))

	for _, hook := range hooks {
		if !webhookAllowsEvent(hook, event.Event) {
			continue
		}
		if err := sendWebhook(ctx, hook, event); err != nil {
			logs = append(logs, fmt.Sprintf("%s 发送失败: %v", hook.Name, err))
		} else {
			logs = append(logs, fmt.Sprintf("%s 已发送", hook.Name))
		}
	}
	return logs
}

func sendWebhook(ctx context.Context, hook Webhook, event WebhookEvent) error {
	if !webhookAllowsEvent(hook, event.Event) {
		return nil
	}
	if event.RunAt == "" {
		event.RunAt = nowText()
	}
	client := &http.Client{Timeout: 20 * time.Second}
	method := strings.ToUpper(hook.Method)
	if method == "" {
		method = http.MethodPost
	}
	raw, err := buildWebhookPayload(hook, event)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, hook.URL, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	var headers map[string]string
	if err := json.Unmarshal([]byte(hook.HeadersJSON), &headers); err == nil {
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return webhookRejectedError{fmt.Errorf("返回状态: %s", resp.Status)}
	}
	if readErr != nil {
		return readErr
	}
	if err := validateWebhookResponse(hook, body); err != nil {
		return webhookRejectedError{err}
	}
	return nil
}

func buildWebhookPayload(hook Webhook, event WebhookEvent) ([]byte, error) {
	if isFeishuWebhook(hook.URL) {
		return json.Marshal(map[string]interface{}{
			"msg_type": "text",
			"content": map[string]string{
				"text": renderWebhookText(event),
			},
		})
	}
	return json.Marshal(event)
}

func isFeishuWebhook(url string) bool {
	url = strings.ToLower(strings.TrimSpace(url))
	return strings.Contains(url, "open.feishu.cn/open-apis/bot/v2/hook/") ||
		strings.Contains(url, "open.larksuite.com/open-apis/bot/v2/hook/")
}

func renderWebhookText(event WebhookEvent) string {
	lines := []string{"TDX 股票数据终端通知"}
	if event.Message != "" {
		lines = append(lines, event.Message)
	}
	if event.Event != "" {
		lines = append(lines, "事件: "+event.Event)
	}
	if event.TaskName != "" {
		lines = append(lines, "任务: "+event.TaskName)
	}
	if event.Status != "" {
		lines = append(lines, "状态: "+event.Status)
	}
	if event.MatchedCount > 0 {
		label := "命中数量"
		if event.TaskType == "selection_tracking" {
			label = "更新数量"
		}
		lines = append(lines, fmt.Sprintf("%s: %d", label, event.MatchedCount))
	}
	if len(event.MatchedSymbols) > 0 {
		symbols := event.MatchedSymbols
		if len(symbols) > 20 {
			symbols = symbols[:20]
		}
		lines = append(lines, "命中股票: "+strings.Join(symbols, ", "))
	}
	if event.RunAt != "" {
		lines = append(lines, "时间: "+event.RunAt)
	}
	return strings.Join(lines, "\n")
}

func validateWebhookResponse(hook Webhook, body []byte) error {
	text := strings.TrimSpace(string(body))
	if text == "" || !isFeishuWebhook(hook.URL) {
		return nil
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil
	}
	if code, ok := numericField(resp, "code"); ok && code != 0 {
		return fmt.Errorf("%s", text)
	}
	if code, ok := numericField(resp, "StatusCode"); ok && code != 0 {
		return fmt.Errorf("%s", text)
	}
	return nil
}

func numericField(m map[string]interface{}, key string) (float64, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case string:
		if x == "0" {
			return 0, true
		}
	}
	return 0, false
}

func webhookAllowsEvent(hook Webhook, event string) bool {
	var events []string
	if err := json.Unmarshal([]byte(hook.Events), &events); err != nil {
		return true
	}
	if len(events) == 0 {
		return true
	}
	for _, item := range events {
		if item == "*" || item == event || (item == "automation.finished" && strings.HasSuffix(event, ".finished")) {
			return true
		}
	}
	return false
}
