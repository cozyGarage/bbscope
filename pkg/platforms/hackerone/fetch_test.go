package hackerone

import (
	"context"
	"fmt"
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

func TestListProgramHandles(t *testing.T) {
	page1Tpl := readTestdata(t, "programs_page1.json")
	page2 := readTestdata(t, "programs_page2.json")

	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			_, _ = io.WriteString(w, page2)
			return
		}
		_, _ = io.WriteString(w, fmt.Sprintf(page1Tpl, srvURL))
	}))
	defer srv.Close()
	srvURL = srv.URL
	withBaseURL(t, srv.URL)

	p := NewPoller("user", "token")

	// Default: every program whose submission_state is "open", across both pages.
	got, err := p.ListProgramHandles(context.Background(), platforms.PollOptions{})
	if err != nil {
		t.Fatalf("ListProgramHandles: %v", err)
	}
	if want := []string{"open-bounty", "private-prog", "second-page"}; !equal(got, want) {
		t.Fatalf("default handles = %v, want %v", got, want)
	}

	// BountyOnly keeps only programs that offer bounties.
	got, _ = p.ListProgramHandles(context.Background(), platforms.PollOptions{BountyOnly: true})
	if want := []string{"open-bounty"}; !equal(got, want) {
		t.Fatalf("bounty-only handles = %v, want %v", got, want)
	}

	// PrivateOnly keeps only soft_launched programs.
	got, _ = p.ListProgramHandles(context.Background(), platforms.PollOptions{PrivateOnly: true})
	if want := []string{"private-prog"}; !equal(got, want) {
		t.Fatalf("private-only handles = %v, want %v", got, want)
	}
}

func TestFetchProgramScope(t *testing.T) {
	body := readTestdata(t, "structured_scopes.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()
	withBaseURL(t, srv.URL)

	p := NewPoller("user", "token")

	pd, err := p.FetchProgramScope(context.Background(), "acme", platforms.PollOptions{Categories: "all"})
	if err != nil {
		t.Fatalf("FetchProgramScope: %v", err)
	}
	if pd.Url != "https://hackerone.com/acme" {
		t.Errorf("unexpected program URL: %q", pd.Url)
	}
	if len(pd.InScope) != 2 {
		t.Fatalf("expected 2 in-scope elements, got %d (%+v)", len(pd.InScope), pd.InScope)
	}
	if len(pd.OutOfScope) != 1 || pd.OutOfScope[0].Target != "oos.example.com" {
		t.Fatalf("expected 1 OOS element oos.example.com, got %+v", pd.OutOfScope)
	}

	// Category filter: only URL assets should be considered in-scope.
	pd, _ = p.FetchProgramScope(context.Background(), "acme", platforms.PollOptions{Categories: "url"})
	if len(pd.InScope) != 1 || pd.InScope[0].Target != "in.example.com" {
		t.Fatalf("expected only the URL in-scope target, got %+v", pd.InScope)
	}
}

func TestFetchProgramScope_BountyOnlyKeepsEarlierOOS(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			_, _ = fmt.Fprintf(w, `{"data":[{"attributes":{"asset_type":"URL","asset_identifier":"oos.example.com","eligible_for_submission":false,"eligible_for_bounty":false}}],"links":{"next":%q}}`, "http://"+r.Host+"/page2")
			return
		}
		_, _ = io.WriteString(w, `{"data":[{"attributes":{"asset_type":"URL","asset_identifier":"in.example.com","eligible_for_submission":true,"eligible_for_bounty":true}}],"links":{}}`)
	}))
	defer srv.Close()
	withBaseURL(t, srv.URL)

	p := NewPoller("user", "token")
	pd, err := p.FetchProgramScope(context.Background(), "acme", platforms.PollOptions{Categories: "all", BountyOnly: true})
	if err != nil {
		t.Fatalf("FetchProgramScope: %v", err)
	}
	if len(pd.InScope) != 1 || pd.InScope[0].Target != "in.example.com" {
		t.Fatalf("in-scope = %+v", pd.InScope)
	}
	if len(pd.OutOfScope) != 1 || pd.OutOfScope[0].Target != "oos.example.com" {
		t.Fatalf("OOS from page 1 was dropped: %+v", pd.OutOfScope)
	}
}

func TestFetchProgramScope_Non200WithData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"data":[],"errors":[{"detail":"unauthorized"}]}`)
	}))
	defer srv.Close()
	withBaseURL(t, srv.URL)

	p := NewPoller("user", "token")
	if _, err := p.FetchProgramScope(context.Background(), "acme", platforms.PollOptions{Categories: "all"}); err == nil {
		t.Fatal("expected an error for a 401 body that still contains a data array")
	}
}

func TestAllowSameOriginNextURL(t *testing.T) {
	got, err := allowSameOriginNextURL("https://api.hackerone.com", "https://api.hackerone.com/v1/hackers/programs?page=2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "page=2") {
		t.Fatalf("unexpected allowed URL: %q", got)
	}

	if _, err := allowSameOriginNextURL("https://api.hackerone.com", "https://evil.example/steal"); err == nil {
		t.Fatal("expected off-origin pagination URL to be rejected")
	}
}

func TestListProgramHandlesRejectsOffOriginNext(t *testing.T) {
	page1 := readTestdata(t, "programs_off_origin_next.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, page1)
	}))
	defer srv.Close()
	withBaseURL(t, srv.URL)

	p := NewPoller("user", "token")
	if _, err := p.ListProgramHandles(context.Background(), platforms.PollOptions{}); err == nil {
		t.Fatal("expected off-origin links.next to fail")
	}
}

func TestListProgramHandlesSkipsEmptyHandle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[
			{"attributes":{"handle":"","state":"public_mode","submission_state":"open","offers_bounties":true}},
			{"attributes":{"handle":"real-prog","state":"public_mode","submission_state":"open","offers_bounties":true}}
		]}`)
	}))
	defer srv.Close()
	withBaseURL(t, srv.URL)

	got, err := NewPoller("user", "token").ListProgramHandles(context.Background(), platforms.PollOptions{})
	if err != nil {
		t.Fatalf("ListProgramHandles: %v", err)
	}
	if !equal(got, []string{"real-prog"}) {
		t.Fatalf("handles = %v, want [real-prog] (empty handle must be skipped)", got)
	}
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

func TestFetchProgramScopeEscapesHandle(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		_, _ = io.WriteString(w, `{"data":[],"links":{}}`)
	}))
	defer srv.Close()
	withBaseURL(t, srv.URL)

	_, err := NewPoller("user", "token").FetchProgramScope(context.Background(), "acme?x", platforms.PollOptions{})
	if err != nil {
		t.Fatalf("FetchProgramScope: %v", err)
	}
	if !strings.Contains(gotPath, "acme%3Fx") {
		t.Fatalf("path = %q, want PathEscape of handle", gotPath)
	}
}
