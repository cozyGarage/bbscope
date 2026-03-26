package notify

import (
	"context"
	"time"
)

// Notifier sends notifications about scope changes
type Notifier interface {
	Send(ctx context.Context, event ChangeEvent) error
	Name() string
}

// ChangeEvent represents a scope change notification
type ChangeEvent struct {
	Type          string // "added", "removed", "updated"
	Platform      string // e.g., "hackerone"
	ProgramURL    string
	ProgramHandle string
	Target        string
	Category      string
	InScope       bool
	IsBBP         bool
	OccurredAt    time.Time
}

// Config holds notification configuration
type Config struct {
	Slack    *SlackConfig    `yaml:"slack,omitempty"`
	Discord  *DiscordConfig  `yaml:"discord,omitempty"`
	Telegram *TelegramConfig `yaml:"telegram,omitempty"`
	Email    *EmailConfig    `yaml:"email,omitempty"`
	Webhook  *WebhookConfig  `yaml:"webhook,omitempty"`
}

// SlackConfig holds Slack notification settings
type SlackConfig struct {
	WebhookURL string   `yaml:"webhook"`
	Events     []string `yaml:"events,omitempty"` // e.g., ["added", "removed"]
	Username   string   `yaml:"username,omitempty"`
	Icon       string   `yaml:"icon,omitempty"`
}

// DiscordConfig holds Discord notification settings
type DiscordConfig struct {
	WebhookURL string   `yaml:"webhook"`
	Events     []string `yaml:"events,omitempty"`
	Username   string   `yaml:"username,omitempty"`
}

// TelegramConfig holds Telegram notification settings
type TelegramConfig struct {
	BotToken string   `yaml:"bot_token"`
	ChatID   string   `yaml:"chat_id"`
	Events   []string `yaml:"events,omitempty"`
}

// EmailConfig holds email notification settings
type EmailConfig struct {
	SMTPHost string   `yaml:"smtp_host"`
	SMTPPort int      `yaml:"smtp_port"`
	From     string   `yaml:"from"`
	To       []string `yaml:"to"`
	Username string   `yaml:"username,omitempty"`
	Password string   `yaml:"password,omitempty"`
	Events   []string `yaml:"events,omitempty"`
}

// WebhookConfig holds custom webhook settings
type WebhookConfig struct {
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers,omitempty"`
	Events  []string          `yaml:"events,omitempty"`
}

// LoadNotifiers creates notifiers from configuration
func LoadNotifiers(cfg *Config) []Notifier {
	if cfg == nil {
		return nil
	}

	var notifiers []Notifier

	if cfg.Slack != nil && cfg.Slack.WebhookURL != "" {
		notifiers = append(notifiers, NewSlackNotifier(cfg.Slack))
	}

	if cfg.Discord != nil && cfg.Discord.WebhookURL != "" {
		notifiers = append(notifiers, NewDiscordNotifier(cfg.Discord))
	}

	if cfg.Telegram != nil && cfg.Telegram.BotToken != "" && cfg.Telegram.ChatID != "" {
		notifiers = append(notifiers, NewTelegramNotifier(cfg.Telegram))
	}

	if cfg.Email != nil && cfg.Email.SMTPHost != "" {
		notifiers = append(notifiers, NewEmailNotifier(cfg.Email))
	}

	if cfg.Webhook != nil && cfg.Webhook.URL != "" {
		notifiers = append(notifiers, NewWebhookNotifier(cfg.Webhook))
	}

	return notifiers
}

// shouldSend checks if an event should be sent based on configuration
func shouldSend(configEvents []string, eventType string) bool {
	if len(configEvents) == 0 {
		return true // Send all events if not specified
	}

	for _, evt := range configEvents {
		if evt == eventType || evt == "all" {
			return true
		}
	}
	return false
}
