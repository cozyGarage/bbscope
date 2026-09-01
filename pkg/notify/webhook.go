package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
)

// WebhookNotifier sends notifications to a custom webhook
type WebhookNotifier struct {
	config *WebhookConfig
	client *http.Client
}

// NewWebhookNotifier creates a new webhook notifier
func NewWebhookNotifier(cfg *WebhookConfig) *WebhookNotifier {
	return &WebhookNotifier{
		config: cfg,
		client: newWebhookHTTPClient(),
	}
}

// Name returns the notifier name
func (w *WebhookNotifier) Name() string {
	return "webhook"
}

// Send sends a notification to the custom webhook
func (w *WebhookNotifier) Send(ctx context.Context, event ChangeEvent) error {
	if !shouldSend(w.config.Events, event.Type) {
		return nil
	}
	if err := webhookDestinationAllowed(ctx, w.config.URL, net.DefaultResolver.LookupIPAddr); err != nil {
		return fmt.Errorf("webhook destination: %w", err)
	}

	// Send the raw event as JSON
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", w.config.URL, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Add custom headers
	for key, value := range w.config.Headers {
		req.Header.Set(key, value)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}
