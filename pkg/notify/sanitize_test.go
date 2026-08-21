package notify

import (
	"strings"
	"testing"
	"time"
)

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
