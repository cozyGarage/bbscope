package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/viper"

	"github.com/cozyGarage/bbscope/v2/pkg/notify"
	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

// readNotifyConfig loads a YAML fragment into viper the same way initConfig
// would, so these tests exercise the real decode path rather than a hand-built
// notify.Config.
func readNotifyConfig(t *testing.T, yaml string) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetConfigType("yaml")
	if err := viper.ReadConfig(strings.NewReader(yaml)); err != nil {
		t.Fatalf("reading test config: %v", err)
	}
}

// TestLoadChangeNotifierDecodesDocumentedConfig pins the config shape published
// in the README. viper decodes with mapstructure rather than yaml, so a missing
// mapstructure tag silently yields an empty config and no notifications.
func TestLoadChangeNotifierDecodesDocumentedConfig(t *testing.T) {
	readNotifyConfig(t, `
notifications:
  slack:
    webhook: "https://hooks.slack.test/services/X/Y/Z"
    events: ["added", "removed"]
  discord:
    webhook: "https://discord.test/api/webhooks/X/Y"
  telegram:
    bot_token: "token"
    chat_id: "chat"
  email:
    smtp_host: "smtp.example.com"
    smtp_port: 587
    from: "alerts@example.com"
    to: ["security@example.com"]
  webhook:
    url: "https://api.example.com/webhooks/bbscope"
    headers:
      Authorization: "Bearer TOKEN"
`)

	n := loadChangeNotifier()
	if n == nil {
		t.Fatal("expected notifiers to be loaded from the documented config block")
	}

	got := map[string]bool{}
	for _, notifier := range n.notifiers {
		got[notifier.Name()] = true
	}
	for _, want := range []string{"slack", "discord", "telegram", "email", "webhook"} {
		if !got[want] {
			t.Errorf("notifier %q was not loaded; got %v", want, got)
		}
	}
}

// TestLoadChangeNotifierAbsentOrEmpty covers the two no-op cases. A nil result
// is meaningful: Dispatch is defined on a nil receiver so callers need no guard.
func TestLoadChangeNotifierAbsentOrEmpty(t *testing.T) {
	readNotifyConfig(t, "db_url: postgres://example\n")
	if n := loadChangeNotifier(); n != nil {
		t.Errorf("expected nil notifier when the config block is absent, got %+v", n)
	}

	// Present but with nothing usable configured.
	readNotifyConfig(t, "notifications:\n  slack:\n    events: [\"added\"]\n")
	if n := loadChangeNotifier(); n != nil {
		t.Errorf("expected nil notifier when no channel has a destination, got %+v", n)
	}
}

func TestChangeNotifierNilDispatchIsSafe(t *testing.T) {
	var n *changeNotifier
	n.Dispatch(context.Background(), []storage.Change{{ChangeType: "added"}})
}

// TestChangeNotifierDispatch is the end-to-end check that a detected change
// reaches a configured channel.
func TestChangeNotifierDispatch(t *testing.T) {
	var (
		mu       sync.Mutex
		received []notify.ChangeEvent
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var ev notify.ChangeEvent
		if err := json.Unmarshal(body, &ev); err != nil {
			t.Errorf("payload is not a ChangeEvent: %v", err)
		}
		mu.Lock()
		received = append(received, ev)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	readNotifyConfig(t, "notifications:\n  webhook:\n    url: \""+srv.URL+"\"\n")
	n := loadChangeNotifier()
	if n == nil {
		t.Fatal("expected a webhook notifier")
	}

	occurred := time.Unix(1700000000, 0).UTC()
	n.Dispatch(context.Background(), []storage.Change{
		{
			ChangeType:       "added",
			Platform:         "h1",
			ProgramURL:       "https://hackerone.com/example",
			Handle:           "example",
			TargetNormalized: "api.example.com",
			Category:         "url",
			InScope:          true,
			IsBBP:            true,
			OccurredAt:       occurred,
		},
		{
			ChangeType:       "removed",
			Platform:         "h1",
			ProgramURL:       "https://hackerone.com/example",
			Handle:           "example",
			TargetNormalized: "old.example.com",
			Category:         "url",
			OccurredAt:       occurred,
		},
	})

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(received))
	}
	if received[0].Type != "added" || received[0].Target != "api.example.com" {
		t.Errorf("unexpected first event: %+v", received[0])
	}
	if received[0].Platform != "h1" || received[0].ProgramHandle != "example" || !received[0].InScope {
		t.Errorf("event fields not carried through: %+v", received[0])
	}
	if received[1].Type != "removed" || received[1].Target != "old.example.com" {
		t.Errorf("unexpected second event: %+v", received[1])
	}
}

// TestChangeNotifierDispatchNoChanges guards the quiet case: an unchanged poll
// must not produce any traffic.
func TestChangeNotifierDispatchNoChanges(t *testing.T) {
	var called int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	readNotifyConfig(t, "notifications:\n  webhook:\n    url: \""+srv.URL+"\"\n")
	n := loadChangeNotifier()
	if n == nil {
		t.Fatal("expected a webhook notifier")
	}

	n.Dispatch(context.Background(), nil)
	n.Dispatch(context.Background(), []storage.Change{})

	if called != 0 {
		t.Errorf("expected no notifications for an unchanged poll, got %d", called)
	}
}

// TestChangeNotifierRespectsEventFilter confirms the per-channel events list is
// honored end to end, not just in the notifier unit tests.
func TestChangeNotifierRespectsEventFilter(t *testing.T) {
	var (
		mu       sync.Mutex
		received []notify.ChangeEvent
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var ev notify.ChangeEvent
		_ = json.Unmarshal(body, &ev)
		mu.Lock()
		received = append(received, ev)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	readNotifyConfig(t, "notifications:\n  webhook:\n    url: \""+srv.URL+"\"\n    events: [\"removed\"]\n")
	n := loadChangeNotifier()
	if n == nil {
		t.Fatal("expected a webhook notifier")
	}

	n.Dispatch(context.Background(), []storage.Change{
		{ChangeType: "added", TargetNormalized: "new.example.com"},
		{ChangeType: "removed", TargetNormalized: "gone.example.com"},
	})

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 || received[0].Type != "removed" {
		t.Fatalf("expected only the removed event, got %+v", received)
	}
}

// TestChangeNotifierFailureDoesNotPanic covers a broken endpoint: delivery
// errors are logged, never propagated into the poll.
func TestChangeNotifierFailureDoesNotPanic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	readNotifyConfig(t, "notifications:\n  webhook:\n    url: \""+srv.URL+"\"\n")
	n := loadChangeNotifier()
	if n == nil {
		t.Fatal("expected a webhook notifier")
	}

	n.Dispatch(context.Background(), []storage.Change{{ChangeType: "added", TargetNormalized: "x.example.com"}})
}

// TestChangeToEventPrefersAINormalizedTarget documents which target string
// users actually see in a notification.
func TestChangeToEventPrefersAINormalizedTarget(t *testing.T) {
	ev := changeToEvent(storage.Change{
		TargetRaw:          "https://*.example.com/**",
		TargetNormalized:   "*.example.com",
		TargetAINormalized: "example.com",
	})
	if ev.Target != "example.com" {
		t.Errorf("Target = %q, want the AI-normalized form", ev.Target)
	}

	ev = changeToEvent(storage.Change{
		TargetRaw:        "https://*.example.com/**",
		TargetNormalized: "*.example.com",
	})
	if ev.Target != "*.example.com" {
		t.Errorf("Target = %q, want the normalized form when no AI variant exists", ev.Target)
	}

	ev = changeToEvent(storage.Change{TargetRaw: "raw.example.com"})
	if ev.Target != "raw.example.com" {
		t.Errorf("Target = %q, want the raw form as a last resort", ev.Target)
	}
}
