package bugcrowd

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
	"github.com/cozyGarage/bbscope/v2/pkg/scope"
)

func withBaseURL(t *testing.T, url string) {
	t.Helper()
	origURL := apiBaseURL
	origInterval := rateLimitInterval
	apiBaseURL = url
	rateLimitInterval = 0
	t.Cleanup(func() {
		apiBaseURL = origURL
		rateLimitInterval = origInterval
	})
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
	bbpPage1 := readTestdata(t, "engagements_bbp.json")
	vdpPage := readTestdata(t, "engagements_vdp.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/engagements.json") {
			http.NotFound(w, r)
			return
		}
		cookie := r.Header.Get("Cookie")
		if !strings.Contains(cookie, "_bugcrowd_session=tok") {
			http.Error(w, "missing session", http.StatusUnauthorized)
			return
		}
		switch r.URL.Query().Get("category") {
		case "bug_bounty":
			_, _ = io.WriteString(w, bbpPage1)
		case "vdp":
			_, _ = io.WriteString(w, vdpPage)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	withBaseURL(t, srv.URL)

	p := NewPollerFromToken("tok")

	got, err := p.ListProgramHandles(context.Background(), platforms.PollOptions{})
	if err != nil {
		t.Fatalf("ListProgramHandles: %v", err)
	}
	if want := []string{"/acme", "/private-co", "/vdp-prog"}; !equal(got, want) {
		t.Fatalf("default handles = %v, want %v", got, want)
	}

	got, err = p.ListProgramHandles(context.Background(), platforms.PollOptions{BountyOnly: true})
	if err != nil {
		t.Fatalf("BountyOnly ListProgramHandles: %v", err)
	}
	if want := []string{"/acme", "/private-co"}; !equal(got, want) {
		t.Fatalf("bounty-only handles = %v, want %v", got, want)
	}

	got, err = p.ListProgramHandles(context.Background(), platforms.PollOptions{PrivateOnly: true, BountyOnly: true})
	if err != nil {
		t.Fatalf("PrivateOnly ListProgramHandles: %v", err)
	}
	if want := []string{"/private-co"}; !equal(got, want) {
		t.Fatalf("private-only handles = %v, want %v", got, want)
	}
}

func TestFetchProgramScope_TargetGroups(t *testing.T) {
	groups := readTestdata(t, "target_groups.json")
	inTargets := readTestdata(t, "targets_in.json")
	oosTargets := readTestdata(t, "targets_oos.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/acme/target_groups":
			_, _ = io.WriteString(w, groups)
		case "/programs/acme/targets.json":
			_, _ = io.WriteString(w, inTargets)
		case "/programs/acme/oos.json":
			_, _ = io.WriteString(w, oosTargets)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	withBaseURL(t, srv.URL)

	p := NewPollerFromToken("tok")
	pd, err := p.FetchProgramScope(context.Background(), "/acme", platforms.PollOptions{Categories: "all"})
	if err != nil {
		t.Fatalf("FetchProgramScope: %v", err)
	}
	if want := srv.URL + "/acme"; pd.Url != want {
		t.Errorf("program URL = %q, want %q", pd.Url, want)
	}
	if len(pd.InScope) != 2 {
		t.Fatalf("expected 2 in-scope, got %+v", pd.InScope)
	}
	if pd.InScope[1].Target != "api.acme.com" {
		t.Errorf("empty uri should fall back to name, got %q", pd.InScope[1].Target)
	}
	if len(pd.OutOfScope) != 1 || pd.OutOfScope[0].Target != "https://blog.acme.com" {
		t.Fatalf("expected OOS blog, got %+v", pd.OutOfScope)
	}

	pd, err = p.FetchProgramScope(context.Background(), "/acme", platforms.PollOptions{Categories: "url"})
	if err != nil {
		t.Fatalf("category filter FetchProgramScope: %v", err)
	}
	if len(pd.InScope) != 2 {
		t.Fatalf("url filter should keep website+api in-scope, got %+v", pd.InScope)
	}
	pd, err = p.FetchProgramScope(context.Background(), "/acme", platforms.PollOptions{Categories: "android"})
	if err != nil {
		t.Fatalf("android filter FetchProgramScope: %v", err)
	}
	if len(pd.InScope) != 0 || len(pd.OutOfScope) != 0 {
		t.Fatalf("android filter should drop website targets, got in=%+v oos=%+v", pd.InScope, pd.OutOfScope)
	}
}

func TestFetchProgramScope_EngagementBrief(t *testing.T) {
	html := readTestdata(t, "engagement_brief.html")
	brief := readTestdata(t, "engagement_brief.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/engagements/acme":
			_, _ = io.WriteString(w, html)
		case "/engagements/acme/brief/1.json":
			_, _ = io.WriteString(w, brief)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	withBaseURL(t, srv.URL)

	p := NewPollerFromToken("tok")
	pd, err := p.FetchProgramScope(context.Background(), "/engagements/acme", platforms.PollOptions{Categories: "all"})
	if err != nil {
		t.Fatalf("FetchProgramScope engagement: %v", err)
	}
	if len(pd.InScope) != 1 || pd.InScope[0].Target != "https://app.acme.com" {
		t.Fatalf("unexpected in-scope: %+v", pd.InScope)
	}
	if len(pd.OutOfScope) != 1 || pd.OutOfScope[0].Target != "old" {
		t.Fatalf("unexpected OOS: %+v", pd.OutOfScope)
	}
}

func TestGetProgramHandles_WAFBanned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, "banned")
	}))
	defer srv.Close()
	withBaseURL(t, srv.URL)

	_, err := GetProgramHandles("tok", "bug_bounty", false)
	if err == nil || !strings.Contains(err.Error(), "WAF banned") {
		t.Fatalf("expected WAF banned error, got %v", err)
	}
}

func TestGetProgramHandles_Non2xxAndHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, "<html>login</html>")
	}))
	defer srv.Close()
	withBaseURL(t, srv.URL)

	_, err := GetProgramHandles("tok", "bug_bounty", false)
	if err == nil {
		t.Fatal("expected an error for a non-2xx listing page, not an empty list")
	}
}

func TestGetProgramHandles_HTML200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "<html>login</html>")
	}))
	defer srv.Close()
	withBaseURL(t, srv.URL)

	_, err := GetProgramHandles("tok", "bug_bounty", false)
	if err == nil {
		t.Fatal("expected an error when the listing body has no engagements array")
	}
}

func TestListProgramHandles_VDPError(t *testing.T) {
	bbp := readTestdata(t, "engagements_bbp.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("category") == "vdp" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, bbp)
	}))
	defer srv.Close()
	withBaseURL(t, srv.URL)

	p := NewPollerFromToken("tok")
	if _, err := p.ListProgramHandles(context.Background(), platforms.PollOptions{}); err == nil {
		t.Fatal("expected VDP listing failure to fail the poller list")
	}
}

func TestExtractScopeFromTargetTable_RejectsOffOrigin(t *testing.T) {
	orig := apiBaseURL
	apiBaseURL = "https://bugcrowd.com"
	t.Cleanup(func() { apiBaseURL = orig })

	err := extractScopeFromTargetTable("https://evil.example/targets.json", "all", "tok", &scope.ProgramData{}, true)
	if err == nil {
		t.Fatal("expected off-origin targets_url to be rejected")
	}
}

func TestDefaultAPIBaseURL(t *testing.T) {
	if apiBaseURL != "https://bugcrowd.com" {
		t.Fatalf("apiBaseURL = %q, want https://bugcrowd.com", apiBaseURL)
	}
	if rateLimitInterval <= 0 {
		t.Fatalf("rateLimitInterval should be positive by default, got %v", rateLimitInterval)
	}
}
