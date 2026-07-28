package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func sampleEvent() ChangeEvent {
	return ChangeEvent{
		Type:          "added",
		Platform:      "hackerone",
		ProgramURL:    "https://hackerone.com/example",
		ProgramHandle: "example",
		Target:        "*.example.com",
		Category:      "wildcard",
		InScope:       true,
		IsBBP:         true,
		OccurredAt:    time.Unix(1700000000, 0),
	}
}

func TestShouldSend(t *testing.T) {
	tests := []struct {
		name   string
		events []string
		typ    string
		want   bool
	}{
		{"empty sends all", nil, "added", true},
		{"explicit match", []string{"added", "removed"}, "added", true},
		{"explicit miss", []string{"removed"}, "added", false},
		{"all keyword", []string{"all"}, "updated", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldSend(tc.events, tc.typ); got != tc.want {
				t.Errorf("shouldSend(%v, %q) = %v, want %v", tc.events, tc.typ, got, tc.want)
			}
		})
	}
}

func TestWebhookNotifier_Send(t *testing.T) {
	var received atomic.Int32
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		var ev ChangeEvent
		if err := json.Unmarshal(body, &ev); err != nil {
			t.Errorf("payload is not a ChangeEvent: %v", err)
		}
		if ev.Target != "*.example.com" {
			t.Errorf("unexpected target in payload: %q", ev.Target)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewWebhookNotifier(&WebhookConfig{
		URL:     srv.URL,
		Headers: map[string]string{"Authorization": "Bearer tok"},
	})
	if err := n.Send(context.Background(), sampleEvent()); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if received.Load() != 1 {
		t.Fatalf("expected webhook to be called once, got %d", received.Load())
	}
	if gotAuth != "Bearer tok" {
		t.Fatalf("custom header not forwarded, got %q", gotAuth)
	}
}

func TestWebhookNotifier_EventFilter(t *testing.T) {
	var received atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Only "removed" is subscribed; an "added" event must be skipped.
	n := NewWebhookNotifier(&WebhookConfig{URL: srv.URL, Events: []string{"removed"}})
	if err := n.Send(context.Background(), sampleEvent()); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if received.Load() != 0 {
		t.Fatalf("expected event to be filtered out, but webhook was called %d times", received.Load())
	}
}

func TestWebhookNotifier_StatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := NewWebhookNotifier(&WebhookConfig{URL: srv.URL})
	if err := n.Send(context.Background(), sampleEvent()); err == nil {
		t.Fatal("expected error for non-2xx response, got nil")
	}
}

func TestSlackNotifier_Send(t *testing.T) {
	var received atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		var msg slackMessage
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			t.Errorf("slack payload decode: %v", err)
		}
		if len(msg.Attachments) == 0 {
			t.Errorf("expected a slack attachment, got none")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewSlackNotifier(&SlackConfig{WebhookURL: srv.URL})
	if err := n.Send(context.Background(), sampleEvent()); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if received.Load() != 1 {
		t.Fatalf("expected slack webhook called once, got %d", received.Load())
	}
}

func TestLoadNotifiers(t *testing.T) {
	cfg := &Config{
		Slack:   &SlackConfig{WebhookURL: "https://hooks.slack.test/x"},
		Webhook: &WebhookConfig{URL: "https://webhook.test/x"},
		Discord: &DiscordConfig{}, // no URL -> skipped
	}
	notifiers := LoadNotifiers(cfg)
	if len(notifiers) != 2 {
		t.Fatalf("expected 2 configured notifiers (slack, webhook), got %d", len(notifiers))
	}
	names := map[string]bool{}
	for _, n := range notifiers {
		names[n.Name()] = true
	}
	if !names["slack"] || !names["webhook"] {
		t.Fatalf("expected slack and webhook notifiers, got %v", names)
	}
	if LoadNotifiers(nil) != nil {
		t.Fatal("LoadNotifiers(nil) should return nil")
	}
}

func TestLoadNotifiersSkipsEmailWithoutRecipients(t *testing.T) {
	cfg := &Config{
		Email: &EmailConfig{SMTPHost: "smtp.example.com", To: nil},
	}
	if got := LoadNotifiers(cfg); len(got) != 0 {
		t.Fatalf("expected email without recipients to be skipped, got %d notifiers", len(got))
	}
}

func TestEmailNotifierEmptyTo(t *testing.T) {
	n := NewEmailNotifier(&EmailConfig{SMTPHost: "smtp.example.com", From: "a@b.c", To: nil})
	err := n.Send(context.Background(), ChangeEvent{Type: "added", Target: "x.example"})
	if err == nil {
		t.Fatal("expected error for empty recipients")
	}
}
