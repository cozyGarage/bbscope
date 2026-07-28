package immunefi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
)

func withTestTransport(t *testing.T, url string) {
	t.Helper()
	origURL := PLATFORM_URL
	origRetries := maxRetries
	origSleep := sleepFunc
	PLATFORM_URL = url
	maxRetries = 2
	sleepFunc = func(time.Duration) {}
	t.Cleanup(func() {
		PLATFORM_URL = origURL
		maxRetries = origRetries
		sleepFunc = origSleep
	})
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
	// Mimic an RSC payload containing an embedded bounties array.
	body := `self.__next_f.push([1,null]) "bounties":[{"id":"acme","inviteOnly":false},{"id":"secret","inviteOnly":true},{"id":"beta","inviteOnly":false}]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bug-bounty/" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()
	withTestTransport(t, srv.URL)

	p := &Poller{}
	got, err := p.ListProgramHandles(context.Background(), platforms.PollOptions{})
	if err != nil {
		t.Fatalf("ListProgramHandles: %v", err)
	}
	want := []string{
		srv.URL + "/bug-bounty/acme/information/",
		srv.URL + "/bug-bounty/beta/information/",
	}
	if !equal(got, want) {
		t.Fatalf("handles = %v, want %v", got, want)
	}
}

func TestFetchProgramScope(t *testing.T) {
	body := `prefix "assets":[{"url":"https://app.acme.com","type":"websites_and_applications","description":"web"},{"url":"0xabc","type":"smart_contract","description":"vault"}] suffix`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()
	withTestTransport(t, srv.URL)

	handle := srv.URL + "/bug-bounty/acme/information/"
	p := &Poller{}

	pd, err := p.FetchProgramScope(context.Background(), handle, platforms.PollOptions{Categories: "all"})
	if err != nil {
		t.Fatalf("FetchProgramScope: %v", err)
	}
	if pd.Url != handle {
		t.Errorf("Url = %q, want %q", pd.Url, handle)
	}
	if len(pd.InScope) != 2 {
		t.Fatalf("expected 2 assets, got %+v", pd.InScope)
	}

	pd, err = p.FetchProgramScope(context.Background(), handle, platforms.PollOptions{Categories: "web"})
	if err != nil {
		t.Fatalf("web filter: %v", err)
	}
	if len(pd.InScope) != 1 || pd.InScope[0].Target != "https://app.acme.com" {
		t.Fatalf("expected only web asset, got %+v", pd.InScope)
	}

	pd, err = p.FetchProgramScope(context.Background(), handle, platforms.PollOptions{Categories: "contracts"})
	if err != nil {
		t.Fatalf("contracts filter: %v", err)
	}
	if len(pd.InScope) != 1 || pd.InScope[0].Target != "0xabc" {
		t.Fatalf("expected only contract asset, got %+v", pd.InScope)
	}
}

func TestFetchWithRetry_RateLimitThenOK(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = io.WriteString(w, `ok`)
	}))
	defer srv.Close()
	withTestTransport(t, srv.URL)

	res, err := fetchWithRetry(srv.URL + "/")
	if err != nil {
		t.Fatalf("fetchWithRetry: %v", err)
	}
	if res.BodyString != "ok" {
		t.Fatalf("body = %q", res.BodyString)
	}
	if hits != 2 {
		t.Fatalf("expected 2 hits, got %d", hits)
	}
}

func TestListProgramHandles_MissingBounties(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `no bounties here`)
	}))
	defer srv.Close()
	withTestTransport(t, srv.URL)

	p := &Poller{}
	got, err := p.ListProgramHandles(context.Background(), platforms.PollOptions{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty handles, got %v", got)
	}
}
