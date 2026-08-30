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

// Config holds notification configuration.
//
// The mapstructure tags mirror the yaml tags because viper decodes with
// mapstructure, not the yaml package. Without them viper would look for keys
// named after the Go fields (for example "webhookurl" rather than "webhook")
// and silently produce an empty config.
type Config struct {
	Slack    *SlackConfig    `yaml:"slack,omitempty" mapstructure:"slack"`
	Discord  *DiscordConfig  `yaml:"discord,omitempty" mapstructure:"discord"`
	Telegram *TelegramConfig `yaml:"telegram,omitempty" mapstructure:"telegram"`
	Email    *EmailConfig    `yaml:"email,omitempty" mapstructure:"email"`
	Webhook  *WebhookConfig  `yaml:"webhook,omitempty" mapstructure:"webhook"`
}

// SlackConfig holds Slack notification settings
type SlackConfig struct {
	WebhookURL string   `yaml:"webhook" mapstructure:"webhook"`
	Events     []string `yaml:"events,omitempty" mapstructure:"events"` // e.g., ["added", "removed"]
	Username   string   `yaml:"username,omitempty" mapstructure:"username"`
	Icon       string   `yaml:"icon,omitempty" mapstructure:"icon"`
}

// DiscordConfig holds Discord notification settings
type DiscordConfig struct {
	WebhookURL string   `yaml:"webhook" mapstructure:"webhook"`
	Events     []string `yaml:"events,omitempty" mapstructure:"events"`
	Username   string   `yaml:"username,omitempty" mapstructure:"username"`
}

// TelegramConfig holds Telegram notification settings
type TelegramConfig struct {
	BotToken string   `yaml:"bot_token" mapstructure:"bot_token"`
	ChatID   string   `yaml:"chat_id" mapstructure:"chat_id"`
	Events   []string `yaml:"events,omitempty" mapstructure:"events"`
}

// EmailConfig holds email notification settings
type EmailConfig struct {
	SMTPHost string   `yaml:"smtp_host" mapstructure:"smtp_host"`
	SMTPPort int      `yaml:"smtp_port" mapstructure:"smtp_port"`
	From     string   `yaml:"from" mapstructure:"from"`
	To       []string `yaml:"to" mapstructure:"to"`
	Username string   `yaml:"username,omitempty" mapstructure:"username"`
	Password string   `yaml:"password,omitempty" mapstructure:"password"`
	Events   []string `yaml:"events,omitempty" mapstructure:"events"`
}

// WebhookConfig holds custom webhook settings
type WebhookConfig struct {
	URL     string            `yaml:"url" mapstructure:"url"`
	Headers map[string]string `yaml:"headers,omitempty" mapstructure:"headers"`
	Events  []string          `yaml:"events,omitempty" mapstructure:"events"`
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

	if cfg.Email != nil && cfg.Email.SMTPHost != "" && len(cfg.Email.To) > 0 {
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
