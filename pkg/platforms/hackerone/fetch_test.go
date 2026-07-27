package hackerone

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
)

func withBaseURL(t *testing.T, url string) {
	t.Helper()
	orig := apiBaseURL
	apiBaseURL = url
	t.Cleanup(func() { apiBaseURL = orig })
}

func TestListProgramHandles(t *testing.T) {
	page1 := `{"data":[
		{"attributes":{"handle":"open-bounty","state":"public_mode","submission_state":"open","offers_bounties":true}},
		{"attributes":{"handle":"private-prog","state":"soft_launched","submission_state":"open","offers_bounties":false}},
		{"attributes":{"handle":"closed-prog","state":"public_mode","submission_state":"disabled","offers_bounties":true}}
	],"links":{"next":"%s/v1/hackers/programs?page=2"}}`
	page2 := `{"data":[
		{"attributes":{"handle":"second-page","state":"public_mode","submission_state":"open","offers_bounties":false}}
	],"links":{}}`

	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			_, _ = io.WriteString(w, page2)
			return
		}
		_, _ = io.WriteString(w, fmt.Sprintf(page1, srvURL))
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
	body := `{"data":[
		{"attributes":{"asset_type":"URL","asset_identifier":"in.example.com","eligible_for_submission":true,"eligible_for_bounty":true,"instruction":"scope"}},
		{"attributes":{"asset_type":"URL","asset_identifier":"oos.example.com","eligible_for_submission":false,"eligible_for_bounty":false,"instruction":"no"}},
		{"attributes":{"asset_type":"CIDR","asset_identifier":"10.0.0.0/8","eligible_for_submission":true,"eligible_for_bounty":true,"instruction":"net"}}
	],"links":{}}`

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
