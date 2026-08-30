package intigriti

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
)

func withBaseURL(t *testing.T, url string) {
	t.Helper()
	orig := apiBaseURL
	apiBaseURL = url
	t.Cleanup(func() { apiBaseURL = orig })
}

func readTestdata(t *testing.T, name string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(thisFile), "testdata", name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read testdata %s: %v", name, err)
	}
	return string(b)
}

func newPoller(t *testing.T) *Poller {
	t.Helper()
	p := NewPoller()
	if err := p.Authenticate(context.Background(), platforms.AuthConfig{Token: "tok"}); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	return p
}

func TestListAndFetch(t *testing.T) {
	list := readTestdata(t, "programs_list.json")
	scopeBody := readTestdata(t, "program_scope.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/programs/123"):
			_, _ = io.WriteString(w, scopeBody)
		case strings.HasPrefix(r.URL.Path, "/programs"):
			_, _ = io.WriteString(w, list)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	withBaseURL(t, srv.URL)

	p := newPoller(t)

	handles, err := p.ListProgramHandles(context.Background(), platforms.PollOptions{})
	if err != nil {
		t.Fatalf("ListProgramHandles: %v", err)
	}
	if len(handles) != 1 || handles[0] != "acme/acme-web" {
		t.Fatalf("handles = %v, want [acme/acme-web]", handles)
	}

	pd, err := p.FetchProgramScope(context.Background(), "acme/acme-web", platforms.PollOptions{Categories: "all"})
	if err != nil {
		t.Fatalf("FetchProgramScope: %v", err)
	}
	if len(pd.InScope) != 1 || pd.InScope[0].Target != "*.acme.com" {
		t.Fatalf("in-scope = %+v, want [*.acme.com]", pd.InScope)
	}
	if len(pd.OutOfScope) != 1 || pd.OutOfScope[0].Target != "oos.acme.com" {
		t.Fatalf("out-of-scope = %+v, want [oos.acme.com]", pd.OutOfScope)
	}
}

func TestListProgramHandles_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	withBaseURL(t, srv.URL)

	p := newPoller(t)
	if _, err := p.ListProgramHandles(context.Background(), platforms.PollOptions{}); err == nil {
		t.Fatal("expected error on 401, got nil")
	}
}

func TestListProgramHandles_EmptyRecordsWithMaxCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"maxCount":100,"records":[]}`)
	}))
	defer srv.Close()
	withBaseURL(t, srv.URL)

	p := newPoller(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := p.ListProgramHandles(ctx, platforms.PollOptions{}); err == nil {
		t.Fatal("expected an error for an empty records page with maxCount>0")
	}
}

func TestListProgramHandles_HTML200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `<html>blocked</html>`)
	}))
	defer srv.Close()
	withBaseURL(t, srv.URL)

	p := newPoller(t)
	if _, err := p.ListProgramHandles(context.Background(), platforms.PollOptions{}); err == nil {
		t.Fatal("expected an error for a 200 HTML listing page")
	}
}

func TestListProgramHandles_NonOKStatus(t *testing.T) {
	// Use 400 (not retried by go-retryablehttp) so the test stays fast.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, "bad request")
	}))
	defer srv.Close()
	withBaseURL(t, srv.URL)

	p := newPoller(t)
	if _, err := p.ListProgramHandles(context.Background(), platforms.PollOptions{}); err == nil {
		t.Fatal("expected error on 400, got nil")
	}
}
