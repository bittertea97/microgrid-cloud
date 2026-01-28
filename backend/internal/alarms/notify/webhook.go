package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Channel delivers rendered content.
type Channel interface {
	Send(ctx context.Context, content string) error
}

type webhookPayload struct {
	MsgType string       `json:"msgtype"`
	Text    webhookText  `json:"text"`
	Markdown *webhookMarkdown `json:"markdown,omitempty"`
}

type webhookText struct {
	Content string `json:"content"`
}

type webhookMarkdown struct {
	Content string `json:"content"`
}

// WebhookChannel sends notifications to a webhook endpoint.
type WebhookChannel struct {
	url    string
	client *http.Client
}

// WebhookOption configures the webhook channel.
type WebhookOption func(*WebhookChannel)

// WithHTTPClient overrides the HTTP client.
func WithHTTPClient(client *http.Client) WebhookOption {
	return func(ch *WebhookChannel) {
		if client != nil {
			ch.client = client
		}
	}
}

// NewWebhookChannel constructs a webhook channel.
func NewWebhookChannel(url string, opts ...WebhookOption) (*WebhookChannel, error) {
	if url == "" {
		return nil, errors.New("webhook channel: empty url")
	}
	channel := &WebhookChannel{
		url:    url,
		client: &http.Client{Timeout: 10 * time.Second},
	}
	for _, opt := range opts {
		opt(channel)
	}
	return channel, nil
}

// Send posts the content using DingTalk/WeCom-compatible payload.
func (w *WebhookChannel) Send(ctx context.Context, content string) error {
	if w == nil || w.url == "" {
		return errors.New("webhook channel: empty url")
	}
	payload := webhookPayload{
		MsgType: "text",
		Text:    webhookText{Content: content},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook channel: non-2xx response %d", resp.StatusCode)
	}
	return nil
}
