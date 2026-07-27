package wildcards

import (
	"reflect"
	"testing"

	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

func TestBlacklistedSuffixes(t *testing.T) {
	if len(BlacklistedSuffixes) == 0 {
		t.Error("BlacklistedSuffixes should not be empty")
	}

	expectedSuffixes := []string{
		"amazonaws.com",
		"herokuapp.com",
		"github.io",
		"vercel.app",
	}
	have := make(map[string]bool, len(BlacklistedSuffixes))
	for _, s := range BlacklistedSuffixes {
		have[s] = true
	}
	for _, expected := range expectedSuffixes {
		if !have[expected] {
			t.Errorf("BlacklistedSuffixes missing %q", expected)
		}
	}
}

func TestNonDomainCategories(t *testing.T) {
	expectedCategories := []string{"android", "ios", "binary", "code", "ai", "hardware", "blockchain"}
	for _, cat := range expectedCategories {
		if _, ok := NonDomainCategories[cat]; !ok {
			t.Errorf("NonDomainCategories missing %q", cat)
		}
	}
	if _, ok := NonDomainCategories["url"]; ok {
		t.Error("NonDomainCategories should not contain 'url'")
	}
}

// TestIsBlacklistedSuffix exercises the real production function (previously the
// test reimplemented its own copy, which could pass while prod regressed).
func TestIsBlacklistedSuffix(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"test.amazonaws.com", true},
		{"app.herokuapp.com", true},
		{"user.github.io", true},
		{"amazonaws.com", true},  // exact match
		{"sub.vercel.app", true}, // multi-label suffix
		{"a.b.c.amazonaws.com", true},
		{"example.com", false},
		{"my-company.com", false},
		{"com", false},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := IsBlacklistedSuffix(tt.host); got != tt.want {
				t.Errorf("IsBlacklistedSuffix(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

func TestNormalizeForSubdomainTools(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"wildcard prefix stripped", "*.example.com", "example.com"},
		{"url host extracted", "https://portal.example.com/login", "portal.example.com"},
		{"port stripped", "example.com:8443", "example.com"},
		{"trailing dot-star to com", "example.*", "example.com"},
		{"tld placeholder to com", "example.<tld>", "example.com"},
		{"parentheses removed", "(example).com", "example.com"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeForSubdomainTools(tc.input); got != tc.want {
				t.Errorf("NormalizeForSubdomainTools(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestWildcardHasPath(t *testing.T) {
	tests := []struct {
		target string
		want   bool
	}{
		{"*.example.com", false},
		{"https://*.example.com", false},
		{"*.example.com/admin", true},
		{"https://example.com/path", true},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			if got := WildcardHasPath(tt.target); got != tt.want {
				t.Errorf("WildcardHasPath(%q) = %v, want %v", tt.target, got, tt.want)
			}
		})
	}
}

func TestCollectSorted(t *testing.T) {
	entries := []storage.Entry{
		{ProgramURL: "https://h1.example/p", TargetNormalized: "*.beta.example.com", Category: "wildcard", InScope: true},
		{ProgramURL: "https://h1.example/p", TargetNormalized: "https://shop.alpha.example.com", Category: "url", InScope: true},
		{ProgramURL: "https://h1.example/p", TargetNormalized: "*.cloudfront.net", Category: "wildcard", InScope: true},
	}

	got := CollectSorted(entries, Options{Aggressive: true})

	var domains []string
	for _, r := range got {
		domains = append(domains, r.Domain)
	}
	want := []string{"example.com"} // beta+alpha collapse to root; cloudfront.net is blacklisted
	if !reflect.DeepEqual(domains, want) {
		t.Fatalf("CollectSorted domains = %v, want %v", domains, want)
	}
	if len(got) == 1 && (len(got[0].ProgramURLs) != 1 || got[0].ProgramURLs[0] != "https://h1.example/p") {
		t.Fatalf("expected single program url, got %v", got[0].ProgramURLs)
	}
}
