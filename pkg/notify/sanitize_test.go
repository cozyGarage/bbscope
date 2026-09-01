package notify

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestSafeHTTPURLRejectsUserinfo(t *testing.T) {
	if got := safeHTTPURL("https://trusted.com@evil.com/path"); got != "" {
		t.Fatalf("userinfo URL must be rejected, got %q", got)
	}
	if got := safeHTTPURL("https://evil.com@example.com/"); got != "" {
		t.Fatalf("userinfo URL must be rejected, got %q", got)
	}
	if got := safeHTTPURL("https://example.com/path"); got != "https://example.com/path" {
		t.Fatalf("plain https URL = %q, want unchanged", got)
	}
}

func TestEmailToHeaderListsEveryRecipient(t *testing.T) {
	got := emailToHeader([]string{"one@example.com", "two@example.com"})
	if got != "one@example.com, two@example.com" {
		t.Fatalf("emailToHeader = %q, want both recipients", got)
	}
	injected := emailToHeader([]string{"one@example.com", "two@example.com\r\nBcc: evil@example.com"})
	if !strings.Contains(injected, "two@example.com") {
		t.Fatalf("second recipient missing from To header: %q", injected)
	}
	if strings.Contains(injected, "\r") || strings.Contains(injected, "\n") {
		t.Fatalf("CR/LF survived sanitization: %q", injected)
	}
}

func TestFormatEmailBodyEscapesHTML(t *testing.T) {
	body := formatEmailBody(ChangeEvent{
		Type:          "added",
		Platform:      "<script>h1</script>",
		ProgramURL:    "javascript:alert(1)",
		ProgramHandle: "acme\" onclick=x",
		Target:        "<img src=x onerror=alert(1)>",
		Category:      "url<script>",
		OccurredAt:    time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
	})
	if strings.Contains(body, "<script>") || strings.Contains(body, "<img src=x") {
		t.Fatalf("expected HTML escaping, got body containing raw tags")
	}
	if strings.Contains(body, `href="javascript:`) {
		t.Fatalf("javascript: URLs must not be used as href")
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Fatalf("expected escaped platform/script content, got: %s", body)
	}
}

func TestFormatTelegramEscapesAndSafeURL(t *testing.T) {
	msg := formatTelegramMessage(ChangeEvent{
		Type:          "added",
		Platform:      "h1<script>",
		ProgramURL:    "https://hackerone.com/acme",
		ProgramHandle: "acme<b>",
		Target:        "a<b>",
		Category:      "url",
	})
	if strings.Contains(msg, "<script>") {
		t.Fatalf("expected telegram HTML escaping of script, got: %s", msg)
	}
	if !strings.Contains(msg, "acme&lt;b&gt;") || !strings.Contains(msg, "a&lt;b&gt;") {
		t.Fatalf("expected escaped angle brackets in handle/target, got: %s", msg)
	}
	if !strings.Contains(msg, `href="https://hackerone.com/acme"`) {
		t.Fatalf("expected safe https href, got: %s", msg)
	}
}

func TestFormatSlackLink(t *testing.T) {
	got := formatSlackLink("https://example.com/p", "acme|x")
	if got != "<https://example.com/p|acme x>" {
		t.Fatalf("unexpected slack link: %q", got)
	}
	if got := formatSlackLink("javascript:alert(1)", "x"); got != "x" {
		t.Fatalf("unsafe slack URL should fall back to label, got %q", got)
	}
	raw := formatSlackLink("javascript:alert(1)<!everyone>", "")
	if strings.Contains(raw, "<!everyone>") {
		t.Fatalf("rejected Slack URL must not return raw mention markup, got %q", raw)
	}
}

func TestFormatDiscordLinkAndCode(t *testing.T) {
	got := formatDiscordLink("acme]", "https://example.com/p")
	if got != "[acme](https://example.com/p)" {
		t.Fatalf("unexpected discord link: %q", got)
	}
	if escapeDiscordInlineCode("a`b") != "a'b" {
		t.Fatal("expected backtick escape")
	}
}

func TestFormatDiscordMessageEscapesTypeAndCategory(t *testing.T) {
	msg := formatDiscordMessage(ChangeEvent{
		Type:     "added *bold* `code` |pipe",
		Category: "url_italic_ <mention>",
		Target:   "example.com",
		Platform: "h1",
	}, &DiscordConfig{})
	for _, f := range msg.Embeds[0].Fields {
		if f.Name != "Type" && f.Name != "Category" {
			continue
		}
		if strings.ContainsAny(f.Value, "*_~`|") && !strings.Contains(f.Value, "\\") {
			t.Fatalf("expected markdown control chars to be escaped in %s: %q", f.Name, f.Value)
		}
	}
}

func TestFormatDiscordMessageDisablesMentions(t *testing.T) {
	msg := formatDiscordMessage(ChangeEvent{
		Type:     "added",
		Category: "@everyone",
		Target:   "a\nb@here",
		Platform: "h1",
	}, &DiscordConfig{})
	if msg.AllowedMentions == nil || len(msg.AllowedMentions.Parse) != 0 {
		t.Fatalf("expected allowed_mentions.parse to be empty, got %+v", msg.AllowedMentions)
	}
	for _, f := range msg.Embeds[0].Fields {
		if f.Name == "Category" && strings.Contains(f.Value, "@everyone") {
			t.Fatalf("category still contains a live @everyone mention: %q", f.Value)
		}
	}
	if strings.Contains(msg.Embeds[0].Description, "\n") {
		t.Fatalf("target newlines must not break the inline code span: %q", msg.Embeds[0].Description)
	}
}

func TestFormatSlackMessageEscapesFields(t *testing.T) {
	msg := formatSlackMessage(ChangeEvent{
		Type:     "added",
		Category: "url",
		Target:   "<!everyone> a<b>",
		Platform: "h1",
	}, &SlackConfig{})
	for _, f := range msg.Attachments[0].Fields {
		if f.Title != "Target" {
			continue
		}
		if strings.Contains(f.Value, "<!everyone>") {
			t.Fatalf("expected Slack special-mention syntax to be escaped, got: %q", f.Value)
		}
	}
}

func TestFormatSlackMessageEscapesTitleAndUsername(t *testing.T) {
	msg := formatSlackMessage(ChangeEvent{
		Type:     "added",
		Platform: "<!everyone>",
		Target:   "example.com",
	}, &SlackConfig{Username: "ops <!channel>"})
	if strings.Contains(msg.Attachments[0].Title, "<!everyone>") {
		t.Fatalf("platform must be escaped in Slack title: %q", msg.Attachments[0].Title)
	}
	if strings.Contains(msg.Username, "<!channel>") && !strings.Contains(msg.Username, "&lt;") {
		t.Fatalf("username must be escaped: %q", msg.Username)
	}
}

func TestFormatDiscordMessageEscapesTitleAndUsername(t *testing.T) {
	msg := formatDiscordMessage(ChangeEvent{
		Type:     "added",
		Platform: "*h1* @everyone",
		Target:   "example.com",
	}, &DiscordConfig{Username: "@everyone"})
	if strings.Contains(msg.Embeds[0].Title, "*h1*") && !strings.Contains(msg.Embeds[0].Title, "\\*") {
		t.Fatalf("platform markdown must be escaped in Discord title: %q", msg.Embeds[0].Title)
	}
	if strings.Contains(msg.Username, "@everyone") {
		t.Fatalf("username must neutralize @everyone: %q", msg.Username)
	}
}

func TestWebhookURLAllowed(t *testing.T) {
	if err := webhookURLAllowed("https://example.com/hook"); err != nil {
		t.Fatalf("public https should be allowed: %v", err)
	}
	if err := webhookURLAllowed("http://127.0.0.1:8080/hook"); err != nil {
		t.Fatalf("loopback should be allowed: %v", err)
	}
	if err := webhookURLAllowed("https://169.254.169.254/latest/meta-data"); err == nil {
		t.Fatal("cloud metadata IP must be rejected")
	}
	if err := webhookURLAllowed("http://metadata.google.internal/"); err == nil {
		t.Fatal("metadata hostname must be rejected")
	}
	if err := webhookURLAllowed("ftp://example.com/x"); err == nil {
		t.Fatal("non-http scheme must be rejected")
	}
	if err := webhookURLAllowed("https://user:pass@example.com/x"); err == nil {
		t.Fatal("userinfo must be rejected")
	}
}

func TestWebhookDestinationRejectsDNSResolvedMetadata(t *testing.T) {
	lookup := func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("169.254.169.254")}}, nil
	}
	if err := webhookDestinationAllowed(context.Background(), "https://attacker.example/hook", lookup); err == nil {
		t.Fatal("hostname resolving to metadata must be rejected")
	}
}
