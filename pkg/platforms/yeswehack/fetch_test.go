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

func TestListProgramHandles_MissingItems(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"pagination":{"nb_pages":1}}`)
	}))
	defer srv.Close()
	withBaseURL(t, srv.URL)

	if _, err := NewPoller("tok").ListProgramHandles(context.Background(), platforms.PollOptions{}); err == nil {
		t.Fatal("expected an error when listing JSON has no items array")
	}
}

func TestFetchProgramScope_MissingOutOfScope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"scopes":[{"scope":"api.example.com"}]}`)
	}))
	defer srv.Close()
	withBaseURL(t, srv.URL)

	if _, err := NewPoller("tok").FetchProgramScope(context.Background(), "acme", platforms.PollOptions{}); err == nil {
		t.Fatal("expected an error when the program body has no out_of_scope array")
	}
}

func TestFetchProgramScope_MissingScopes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"name":"acme"}`)
	}))
	defer srv.Close()
	withBaseURL(t, srv.URL)

	if _, err := NewPoller("tok").FetchProgramScope(context.Background(), "acme", platforms.PollOptions{}); err == nil {
		t.Fatal("expected an error when the program body has no scopes array")
	}
}

func TestListProgramHandles_NonOKStatus(t *testing.T) {
	// Use 400 (not retried by go-retryablehttp) so the test stays fast.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"boom"}`)
	}))
	defer srv.Close()
	withBaseURL(t, srv.URL)

	p := NewPoller("tok")
	if _, err := p.ListProgramHandles(context.Background(), platforms.PollOptions{}); err == nil {
		t.Fatal("expected error on non-2xx list response")
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

// TestListProgramHandlesRaggedFields covers programs that omit optional flags.
// gjson's `items.#.field` path skips absent fields rather than emitting null,
// so zipping those arrays positionally panicked on the first incomplete item.
func TestListProgramHandlesRaggedFields(t *testing.T) {
	const body = `{"items":[
		{"slug":"complete","bounty":true,"public":true,"disabled":false},
		{"slug":"sparse"},
		{"slug":"gone","disabled":true}
	],"pagination":{"nb_pages":1}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()
	withBaseURL(t, srv.URL)

	tests := []struct {
		name string
		opts platforms.PollOptions
		want []string
	}{
		// A program missing "disabled" is not treated as disabled.
		{"no filters", platforms.PollOptions{}, []string{"complete", "sparse"}},
		// Missing "bounty" cannot be assumed to pay.
		{"bounty only", platforms.PollOptions{BountyOnly: true}, []string{"complete"}},
		// Missing "public" cannot be assumed public, so it survives the filter.
		{"private only", platforms.PollOptions{PrivateOnly: true}, []string{"sparse"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewPoller("tok").ListProgramHandles(context.Background(), tc.opts)
			if err != nil {
				t.Fatalf("ListProgramHandles() error = %v", err)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("ListProgramHandles() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestFetchProgramScopeRaggedFields covers a scope entry with no scope_type,
// which previously panicked while indexing the parallel scope_type array.
func TestFetchProgramScopeRaggedFields(t *testing.T) {
	const body = `{"scopes":[
		{"scope":"api.example.com","scope_type":"web-application"},
		{"scope":"no-type.example.com"},
		{"scope_type":"web-application"}
	],"out_of_scope":["legacy.example.com"]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()
	withBaseURL(t, srv.URL)

	pd, err := NewPoller("tok").FetchProgramScope(context.Background(), "acme", platforms.PollOptions{})
	if err != nil {
		t.Fatalf("FetchProgramScope() error = %v", err)
	}

	// Both entries carrying a target are kept; the one with no target is skipped.
	want := []string{"api.example.com", "no-type.example.com"}
	var got []string
	for _, s := range pd.InScope {
		got = append(got, s.Target)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("InScope targets = %v, want %v", got, want)
	}
	if len(pd.InScope) > 1 && pd.InScope[1].Category != "" {
		t.Errorf("entry without scope_type should carry an empty category, got %q", pd.InScope[1].Category)
	}
	if len(pd.OutOfScope) != 1 || pd.OutOfScope[0].Target != "legacy.example.com" {
		t.Errorf("OutOfScope = %#v, want one legacy.example.com entry", pd.OutOfScope)
	}
}
