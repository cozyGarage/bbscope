package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
)

// SlackNotifier sends notifications to Slack
type SlackNotifier struct {
	config *SlackConfig
	client *http.Client
}

// NewSlackNotifier creates a new Slack notifier
func NewSlackNotifier(cfg *SlackConfig) *SlackNotifier {
	return &SlackNotifier{
		config: cfg,
		client: newWebhookHTTPClient(),
	}
}

// Name returns the notifier name
func (s *SlackNotifier) Name() string {
	return "slack"
}

// Send sends a notification to Slack
func (s *SlackNotifier) Send(ctx context.Context, event ChangeEvent) error {
	if !shouldSend(s.config.Events, event.Type) {
		return nil
	}

	if err := webhookDestinationAllowed(ctx, s.config.WebhookURL, net.DefaultResolver.LookupIPAddr); err != nil {
		return fmt.Errorf("slack webhook destination: %w", err)
	}

	message := formatSlackMessage(event, s.config)
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal Slack message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.config.WebhookURL, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send Slack notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Slack returned status %d", resp.StatusCode)
	}

	return nil
}

// slackMessage represents a Slack webhook message
type slackMessage struct {
	Username    string            `json:"username,omitempty"`
	IconEmoji   string            `json:"icon_emoji,omitempty"`
	Text        string            `json:"text,omitempty"`
	Attachments []slackAttachment `json:"attachments,omitempty"`
}

// slackAttachment represents a Slack message attachment
type slackAttachment struct {
	Color  string       `json:"color,omitempty"`
	Title  string       `json:"title,omitempty"`
	Text   string       `json:"text,omitempty"`
	Fields []slackField `json:"fields,omitempty"`
	Footer string       `json:"footer,omitempty"`
	TS     int64        `json:"ts,omitempty"`
}

// slackField represents a field in a Slack attachment
type slackField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// formatSlackMessage creates a Slack message from a change event
func formatSlackMessage(event ChangeEvent, cfg *SlackConfig) slackMessage {
	color := getSlackColor(event.Type)
	emoji := getSlackEmoji(event.Type)

	username := "bbscope"
	if cfg.Username != "" {
		username = escapeSlackText(escapeNotifierUsername(cfg.Username))
	}

	icon := ":dart:"
	if cfg.Icon != "" {
		icon = escapeSlackText(cfg.Icon)
	}

	var scopeStatus string
	if event.InScope {
		scopeStatus = "✅ In-Scope"
	} else {
		scopeStatus = "❌ Out-of-Scope"
	}

	var bountyStatus string
	if event.IsBBP {
		bountyStatus = "💰 Bounty"
	} else {
		bountyStatus = "🎁 No Bounty"
	}

	title := fmt.Sprintf("%s Scope Change on %s", emoji, escapeSlackText(event.Platform))

	attachment := slackAttachment{
		Color: color,
		Title: title,
		Fields: []slackField{
			{Title: "Type", Value: escapeSlackText(event.Type), Short: true},
			{Title: "Category", Value: escapeSlackText(event.Category), Short: true},
			{Title: "Target", Value: escapeSlackText(event.Target), Short: false},
			{Title: "Program", Value: formatSlackLink(event.ProgramURL, event.ProgramHandle), Short: false},
			{Title: "Status", Value: fmt.Sprintf("%s • %s", scopeStatus, bountyStatus), Short: false},
		},
		Footer: "bbscope",
		TS:     event.OccurredAt.Unix(),
	}

	return slackMessage{
		Username:    username,
		IconEmoji:   icon,
		Attachments: []slackAttachment{attachment},
	}
}

// getSlackColor returns the color for a change type
func getSlackColor(changeType string) string {
	switch changeType {
	case "added":
		return "good" // green
	case "removed":
		return "danger" // red
	case "updated":
		return "warning" // yellow
	default:
		return "#808080" // gray
	}
}

// getSlackEmoji returns the emoji for a change type
func getSlackEmoji(changeType string) string {
	switch changeType {
	case "added":
		return "🆕"
	case "removed":
		return "🗑️"
	case "updated":
		return "🔄"
	default:
		return "📝"
	}
}
