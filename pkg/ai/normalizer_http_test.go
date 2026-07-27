package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

// newChatServer returns an httptest server that answers the OpenAI chat
// completions contract with the supplied assistant content.
func newChatServer(t *testing.T, status int, assistantContent string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("missing/incorrect Authorization header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status >= 300 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{"message": "boom"},
			})
			return
		}
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": assistantContent}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func TestNewNormalizer_UnsupportedProvider(t *testing.T) {
	_, err := NewNormalizer(Config{Provider: "definitely-not-real", APIKey: "x"})
	if err == nil {
		t.Fatal("expected error for unsupported provider, got nil")
	}
}

func TestNewNormalizer_RequiresAPIKey(t *testing.T) {
	if _, err := NewNormalizer(Config{Provider: "openai"}); err == nil {
		t.Fatal("expected error when API key is missing, got nil")
	}
}

func TestNormalizeTargets_HappyPath(t *testing.T) {
	// The model rewrites the messy target into a clean wildcard variant.
	content := `{"items":[{"id":0,"normalized":["example.com"],"category":"wildcard"}]}`
	srv := newChatServer(t, http.StatusOK, content)
	defer srv.Close()

	n, err := NewNormalizer(Config{Provider: "openai", APIKey: "test-key", Endpoint: srv.URL})
	if err != nil {
		t.Fatalf("NewNormalizer: %v", err)
	}

	items := []storage.TargetItem{{URI: "https://*.example.com/**", Category: "url", InScope: true}}
	out, err := n.NormalizeTargets(context.Background(), ProgramInfo{ProgramURL: "https://x/y"}, items)
	if err != nil {
		t.Fatalf("NormalizeTargets: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 item back, got %d", len(out))
	}
	if len(out[0].Variants) != 1 || out[0].Variants[0].Value != "example.com" {
		t.Fatalf("expected a single 'example.com' variant, got %+v", out[0].Variants)
	}
	if !out[0].Variants[0].HasCategory || out[0].Variants[0].Category != "wildcard" {
		t.Fatalf("expected wildcard category on variant, got %+v", out[0].Variants[0])
	}
}

func TestNormalizeTargets_APIError(t *testing.T) {
	srv := newChatServer(t, http.StatusInternalServerError, "")
	defer srv.Close()

	n, err := NewNormalizer(Config{Provider: "openai", APIKey: "test-key", Endpoint: srv.URL})
	if err != nil {
		t.Fatalf("NewNormalizer: %v", err)
	}

	items := []storage.TargetItem{{URI: "a.example.com", Category: "url", InScope: true}}
	if _, err := n.NormalizeTargets(context.Background(), ProgramInfo{}, items); err == nil {
		t.Fatal("expected error from failing API, got nil")
	}
}

func TestNormalizeTargets_Empty(t *testing.T) {
	n, err := NewNormalizer(Config{Provider: "openai", APIKey: "test-key", Endpoint: "http://unused"})
	if err != nil {
		t.Fatalf("NewNormalizer: %v", err)
	}
	out, err := n.NormalizeTargets(context.Background(), ProgramInfo{}, nil)
	if err != nil || out != nil {
		t.Fatalf("expected (nil, nil) for empty input, got (%v, %v)", out, err)
	}
}
