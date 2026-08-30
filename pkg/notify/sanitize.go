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
			return escapeSlackText(rawURL)
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
			return escapeDiscordMarkdown(rawURL)
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
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "`", "'")
	return neutralizeDiscordMentions(s)
}

// discordMarkdownReplacer escapes Discord markdown control characters in
// plain (non-code-span) field values so untrusted text (e.g. an AI-invented
// scope category) cannot alter message formatting.
var discordMarkdownReplacer = strings.NewReplacer(
	"\\", "\\\\",
	"*", "\\*",
	"_", "\\_",
	"~", "\\~",
	"`", "\\`",
	"|", "\\|",
	">", "\\>",
)

// escapeDiscordMarkdown escapes Discord markdown control characters.
func escapeDiscordMarkdown(s string) string {
	return neutralizeDiscordMentions(discordMarkdownReplacer.Replace(s))
}

func neutralizeDiscordMentions(s string) string {
	s = strings.ReplaceAll(s, "@everyone", "@\u200beveryone")
	s = strings.ReplaceAll(s, "@here", "@\u200bhere")
	s = strings.ReplaceAll(s, "<@", "<@\u200b")
	return s
}

// escapeSlackText escapes Slack mrkdwn's reserved characters. Escaping "<"
// and ">" is required by Slack's format spec and also prevents untrusted
// text from being interpreted as link or special-mention syntax (e.g.
// "<!everyone>").
func escapeSlackText(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
