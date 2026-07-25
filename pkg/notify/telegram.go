package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// TelegramNotifier sends notifications to Telegram
type TelegramNotifier struct {
	config *TelegramConfig
	client *http.Client
}

// NewTelegramNotifier creates a new Telegram notifier
func NewTelegramNotifier(cfg *TelegramConfig) *TelegramNotifier {
	return &TelegramNotifier{
		config: cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Name returns the notifier name
func (t *TelegramNotifier) Name() string {
	return "telegram"
}

// Send sends a notification to Telegram
func (t *TelegramNotifier) Send(ctx context.Context, event ChangeEvent) error {
	if !shouldSend(t.config.Events, event.Type) {
		return nil
	}

	message := formatTelegramMessage(event)

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.config.BotToken)

	payload := telegramMessage{
		ChatID:    t.config.ChatID,
		Text:      message,
		ParseMode: "HTML",
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Telegram message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send Telegram notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Telegram returned status %d", resp.StatusCode)
	}

	return nil
}

// telegramMessage represents a Telegram message
type telegramMessage struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

// formatTelegramMessage creates a Telegram message from a change event
func formatTelegramMessage(event ChangeEvent) string {
	emoji := getTelegramEmoji(event.Type)

	var scopeIcon string
	if event.InScope {
		scopeIcon = "✅"
	} else {
		scopeIcon = "❌"
	}

	var bountyIcon string
	if event.IsBBP {
		bountyIcon = "💰"
	} else {
		bountyIcon = "🎁"
	}

	// Use HTML formatting
	message := fmt.Sprintf(
		"%s <b>Scope Change on %s</b>\n\n"+
			"<b>Type:</b> %s\n"+
			"<b>Category:</b> %s\n"+
			"<b>Target:</b> <code>%s</code>\n"+
			"<b>Program:</b> <a href=\"%s\">%s</a>\n"+
			"<b>Status:</b> %s In-Scope • %s Bounty\n",
		emoji,
		event.Platform,
		event.Type,
		event.Category,
		escapeHTML(event.Target),
		event.ProgramURL,
		escapeHTML(event.ProgramHandle),
		scopeIcon,
		bountyIcon,
	)

	return message
}

// getTelegramEmoji returns the emoji for a change type
func getTelegramEmoji(changeType string) string {
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

// escapeHTML escapes HTML special characters for Telegram
func escapeHTML(s string) string {
	s = replaceString(s, "&", "&amp;")
	s = replaceString(s, "<", "&lt;")
	s = replaceString(s, ">", "&gt;")
	return s
}

func replaceString(s, old, new string) string {
	// Simple string replacement
	result := ""
	for i := 0; i < len(s); i++ {
		if i+len(old) <= len(s) && s[i:i+len(old)] == old {
			result += new
			i += len(old) - 1
		} else {
			result += string(s[i])
		}
	}
	return result
}
