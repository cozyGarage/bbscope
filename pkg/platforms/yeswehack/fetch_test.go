package yeswehack

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

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestListProgramHandles(t *testing.T) {
	list := readTestdata(t, "programs_list.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, list)
	}))
	defer srv.Close()
	withBaseURL(t, srv.URL)

	p := NewPoller("tok")

	// Disabled programs are always skipped.
	got, err := p.ListProgramHandles(context.Background(), platforms.PollOptions{})
	if err != nil {
		t.Fatalf("ListProgramHandles: %v", err)
	}
	if want := []string{"acme", "private-co"}; !equal(got, want) {
		t.Fatalf("default handles = %v, want %v", got, want)
	}

	got, _ = p.ListProgramHandles(context.Background(), platforms.PollOptions{BountyOnly: true})
	if want := []string{"acme"}; !equal(got, want) {
		t.Fatalf("bounty-only handles = %v, want %v", got, want)
	}

	got, _ = p.ListProgramHandles(context.Background(), platforms.PollOptions{PrivateOnly: true})
	if want := []string{"private-co"}; !equal(got, want) {
		t.Fatalf("private-only handles = %v, want %v", got, want)
	}
}

func TestFetchProgramScope(t *testing.T) {
	body := readTestdata(t, "program_scope.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/programs/acme") {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()
	withBaseURL(t, srv.URL)

	p := NewPoller("tok")

	pd, err := p.FetchProgramScope(context.Background(), "acme", platforms.PollOptions{Categories: "all"})
	if err != nil {
		t.Fatalf("FetchProgramScope: %v", err)
	}
	if pd.Url != "https://yeswehack.com/programs/acme" {
		t.Errorf("unexpected program URL: %q", pd.Url)
	}
	if len(pd.InScope) != 2 {
		t.Fatalf("expected 2 in-scope, got %+v", pd.InScope)
	}
	if len(pd.OutOfScope) != 1 || pd.OutOfScope[0].Target != "blog.acme.com" {
		t.Fatalf("expected OOS blog.acme.com, got %+v", pd.OutOfScope)
	}
}
