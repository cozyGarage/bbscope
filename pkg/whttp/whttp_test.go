package whttp

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

func TestIsSensitiveHeader(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{"authorization lowercase", "authorization", true},
		{"Authorization mixed", "Authorization", true},
		{"AUTHORIZATION uppercase", "AUTHORIZATION", true},
		{"cookie", "cookie", true},
		{"Cookie", "Cookie", true},
		{"set-cookie", "Set-Cookie", true},
		{"proxy-authorization", "Proxy-Authorization", true},
		{"x-csrf-token", "x-csrf-token", true},
		{"X-Csrf-Token", "X-Csrf-Token", true},
		{"x-api-key", "x-api-key", true},
		{"x-auth-token", "x-auth-token", true},
		{"content-type", "content-type", false},
		{"accept", "accept", false},
		{"user-agent", "user-agent", false},
		{"x-custom-header", "x-custom-header", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSensitiveHeader(tt.header)
			if got != tt.want {
				t.Errorf("isSensitiveHeader(%q) = %v, want %v", tt.header, got, tt.want)
			}
		})
	}
}

func TestRedactDebugBody(t *testing.T) {
	if got := redactDebugBody(`{"password":"secret"}`); got != "[REDACTED BODY]" {
		t.Fatalf("expected redacted auth body, got %q", got)
	}
	if got := redactDebugBody(`{"ok":true}`); got != `{"ok":true}` {
		t.Fatalf("expected non-sensitive body unchanged, got %q", got)
	}
}

func TestGetDefaultClient(t *testing.T) {
	client := GetDefaultClient()
	if client == nil {
		t.Fatal("GetDefaultClient() returned nil")
	}
	if client.RetryMax != 10 {
		t.Errorf("RetryMax = %d, want 10", client.RetryMax)
	}
}

func TestSendHTTPRequest_Success(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><head><title>Test Page</title></head><body>Hello</body></html>"))
	}))
	defer server.Close()

	req := &WHTTPReq{
		URL:    server.URL,
		Method: "GET",
	}

	res, err := SendHTTPRequest(req, nil)
	if err != nil {
		t.Fatalf("SendHTTPRequest() error = %v", err)
	}

	if res.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", res.StatusCode)
	}

	if res.HTTPTitle != "Test Page" {
		t.Errorf("HTTPTitle = %q, want %q", res.HTTPTitle, "Test Page")
	}

	if res.BodyString == "" {
		t.Error("BodyString is empty")
	}
}

func TestSendHTTPRequest_CustomHeaders(t *testing.T) {
	var receivedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	req := &WHTTPReq{
		URL:    server.URL,
		Method: "GET",
		Headers: []WHTTPHeader{
			{Name: "X-Custom-Header", Value: "custom-value"},
			{Name: "Accept", Value: "application/json"},
		},
	}

	_, err := SendHTTPRequest(req, nil)
	if err != nil {
		t.Fatalf("SendHTTPRequest() error = %v", err)
	}

	if receivedHeaders.Get("X-Custom-Header") != "custom-value" {
		t.Errorf("X-Custom-Header = %q, want %q", receivedHeaders.Get("X-Custom-Header"), "custom-value")
	}
}

func TestSendHTTPRequest_POST(t *testing.T) {
	var receivedBody string
	var receivedMethod string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	req := &WHTTPReq{
		URL:    server.URL,
		Method: "POST",
		Body:   `{"key": "value"}`,
		Headers: []WHTTPHeader{
			{Name: "Content-Type", Value: "application/json"},
		},
	}

	_, err := SendHTTPRequest(req, nil)
	if err != nil {
		t.Fatalf("SendHTTPRequest() error = %v", err)
	}

	if receivedMethod != "POST" {
		t.Errorf("Method = %q, want POST", receivedMethod)
	}

	if receivedBody != `{"key": "value"}` {
		t.Errorf("Body = %q, want %q", receivedBody, `{"key": "value"}`)
	}
}

func TestSendHTTPRequest_404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("Not Found"))
	}))
	defer server.Close()

	req := &WHTTPReq{
		URL:    server.URL,
		Method: "GET",
	}

	res, err := SendHTTPRequest(req, nil)
	if err != nil {
		t.Fatalf("SendHTTPRequest() error = %v", err)
	}

	if res.StatusCode != 404 {
		t.Errorf("StatusCode = %d, want 404", res.StatusCode)
	}
}

func TestSendHTTPRequest_InvalidURL(t *testing.T) {
	req := &WHTTPReq{
		URL:    "http://invalid.url.that.does.not.exist.example:99999",
		Method: "GET",
	}

	// Use a no-retry client so permanent DNS failures do not wait on the
	// default RetryMax=10 / 1s..30s backoff (which made `go test ./...` ~3min).
	client := retryablehttp.NewClient()
	client.RetryMax = 0
	client.HTTPClient.Timeout = 2 * time.Second
	client.Logger = log.New(io.Discard, "", 0)

	_, err := SendHTTPRequest(req, client)
	if err == nil {
		t.Error("expected error for invalid URL, got nil")
	}
}

func TestWHTTPReq_DefaultMethod(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Method = %q, want GET (default)", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	req := &WHTTPReq{
		URL: server.URL,
		// Method not specified, should default to GET
	}

	_, err := SendHTTPRequest(req, nil)
	if err != nil {
		t.Fatalf("SendHTTPRequest() error = %v", err)
	}
}
