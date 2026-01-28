package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// WebhookNotifier sends alerts via webhook.
type WebhookNotifier struct {
	url    string
	client *http.Client
}

type webhookPayload struct {
	MsgType string      `json:"msgtype"`
	Text    webhookText `json:"text"`
}

type webhookText struct {
	Content string `json:"content"`
}

// NewWebhookNotifier constructs a notifier.
func NewWebhookNotifier(url string) *WebhookNotifier {
	return &WebhookNotifier{
		url:    url,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Notify sends an alert to webhook.
func (n *WebhookNotifier) Notify(ctx context.Context, msg AlertMessage) error {
	if n == nil || n.url == "" {
		return errors.New("webhook notifier: empty url")
	}
	content := formatAlertMessage(msg)
	payload := webhookPayload{
		MsgType: "text",
		Text:    webhookText{Content: content},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return errors.New("webhook notifier: non-2xx")
	}
	return nil
}

func formatAlertMessage(msg AlertMessage) string {
	var b strings.Builder
	b.WriteString("[Shadowrun Alert]\n")
	if msg.TenantID != "" {
		fmt.Fprintf(&b, "Tenant: %s\n", msg.TenantID)
	}
	if msg.StationID != "" {
		fmt.Fprintf(&b, "Station: %s\n", msg.StationID)
	}
	if msg.Month != "" {
		fmt.Fprintf(&b, "Month: %s\n", msg.Month)
	}
	if msg.ReportID != "" {
		fmt.Fprintf(&b, "Report: %s\n", msg.ReportID)
	}
	if msg.ReportURL != "" {
		fmt.Fprintf(&b, "Report URL: %s\n", msg.ReportURL)
	}
	if msg.RecommendedAction != "" {
		fmt.Fprintf(&b, "Suggested: %s\n", msg.RecommendedAction)
	}
	if len(msg.DiffSummary) > 0 {
		if raw, err := json.Marshal(msg.DiffSummary); err == nil {
			fmt.Fprintf(&b, "Diff Summary: %s\n", string(raw))
		}
	}
	return strings.TrimSpace(b.String())
}
