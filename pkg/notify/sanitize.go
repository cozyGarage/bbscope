package notify

import (
	"html"
	"net/url"
	"strings"
)

// sanitizeHeaderField strips CR/LF so untrusted values cannot inject email headers.
func sanitizeHeaderField(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	return s
}

// escapeHTMLText escapes text for HTML bodies (email / Telegram HTML mode).
func escapeHTMLText(s string) string {
	return html.EscapeString(s)
}

// safeHTTPURL returns the URL string when it is http(s); otherwise empty.
func safeHTTPURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return u.String()
	default:
		return ""
	}
}

// escapeTelegramHTML escapes Telegram HTML special characters including quotes
// used inside attributes.
func escapeTelegramHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

// formatSlackLink builds a Slack mrkdwn link, falling back to plain text when
// the URL is unsafe or label/URL would break link syntax.
func formatSlackLink(rawURL, label string) string {
	safe := safeHTTPURL(rawURL)
	label = strings.ReplaceAll(label, "|", " ")
	label = strings.ReplaceAll(label, ">", " ")
	label = strings.ReplaceAll(label, "<", " ")
	if safe == "" || strings.ContainsAny(safe, "|>") {
		if label == "" {
			return rawURL
		}
		return label
	}
	if label == "" {
		label = safe
	}
	return "<" + safe + "|" + label + ">"
}

// formatDiscordLink builds a Discord markdown link with basic sanitization.
func formatDiscordLink(label, rawURL string) string {
	safe := safeHTTPURL(rawURL)
	label = strings.ReplaceAll(label, "]", "")
	label = strings.ReplaceAll(label, "[", "")
	label = strings.ReplaceAll(label, "\n", " ")
	if safe == "" || strings.Contains(safe, ")") {
		if label == "" {
			return rawURL
		}
		return label
	}
	if label == "" {
		label = safe
	}
	return "[" + label + "](" + safe + ")"
}

// escapeDiscordInlineCode escapes backticks in values shown inside `code` spans.
func escapeDiscordInlineCode(s string) string {
	return strings.ReplaceAll(s, "`", "'")
}
