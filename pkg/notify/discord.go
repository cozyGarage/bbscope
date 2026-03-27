package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// DiscordNotifier sends notifications to Discord
type DiscordNotifier struct {
	config *DiscordConfig
	client *http.Client
}

// NewDiscordNotifier creates a new Discord notifier
func NewDiscordNotifier(cfg *DiscordConfig) *DiscordNotifier {
	return &DiscordNotifier{
		config: cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Name returns the notifier name
func (d *DiscordNotifier) Name() string {
	return "discord"
}

// Send sends a notification to Discord
func (d *DiscordNotifier) Send(ctx context.Context, event ChangeEvent) error {
	if !shouldSend(d.config.Events, event.Type) {
		return nil
	}

	message := formatDiscordMessage(event, d.config)
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal Discord message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", d.config.WebhookURL, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send Discord notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Discord returned status %d", resp.StatusCode)
	}

	return nil
}

// discordMessage represents a Discord webhook message
type discordMessage struct {
	Username string         `json:"username,omitempty"`
	Content  string         `json:"content,omitempty"`
	Embeds   []discordEmbed `json:"embeds,omitempty"`
}

// discordEmbed represents a Discord embed
type discordEmbed struct {
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description,omitempty"`
	Color       int            `json:"color,omitempty"`
	Fields      []discordField `json:"fields,omitempty"`
	Footer      *discordFooter `json:"footer,omitempty"`
	Timestamp   string         `json:"timestamp,omitempty"`
}

// discordField represents a field in a Discord embed
type discordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

// discordFooter represents a footer in a Discord embed
type discordFooter struct {
	Text string `json:"text"`
}

// formatDiscordMessage creates a Discord message from a change event
func formatDiscordMessage(event ChangeEvent, cfg *DiscordConfig) discordMessage {
	color := getDiscordColor(event.Type)
	emoji := getDiscordEmoji(event.Type)

	username := "bbscope"
	if cfg.Username != "" {
		username = cfg.Username
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

	title := fmt.Sprintf("%s Scope Change on %s", emoji, event.Platform)
	description := fmt.Sprintf("**Target:** `%s`", event.Target)

	embed := discordEmbed{
		Title:       title,
		Description: description,
		Color:       color,
		Fields: []discordField{
			{Name: "Type", Value: event.Type, Inline: true},
			{Name: "Category", Value: event.Category, Inline: true},
			{Name: "Program", Value: fmt.Sprintf("[%s](%s)", event.ProgramHandle, event.ProgramURL), Inline: false},
			{Name: "Status", Value: fmt.Sprintf("%s • %s", scopeStatus, bountyStatus), Inline: false},
		},
		Footer: &discordFooter{
			Text: "bbscope",
		},
		Timestamp: event.OccurredAt.Format(time.RFC3339),
	}

	return discordMessage{
		Username: username,
		Embeds:   []discordEmbed{embed},
	}
}

// getDiscordColor returns the color for a change type (decimal)
func getDiscordColor(changeType string) int {
	switch changeType {
	case "added":
		return 0x00ff00 // green
	case "removed":
		return 0xff0000 // red
	case "updated":
		return 0xffaa00 // orange
	default:
		return 0x808080 // gray
	}
}

// getDiscordEmoji returns the emoji for a change type
func getDiscordEmoji(changeType string) string {
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
