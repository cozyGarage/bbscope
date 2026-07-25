package cmd

import (
	"reflect"
	"testing"

	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

func entries(targets ...string) []storage.Entry {
	out := make([]storage.Entry, 0, len(targets))
	for _, t := range targets {
		out = append(out, storage.Entry{TargetNormalized: t})
	}
	return out
}

func TestFilterTargets(t *testing.T) {
	all := entries(
		"*.example.com",
		"api.example.com",
		"https://portal.example.com/login",
		"http://legacy.example.com",
		"203.0.113.5",
		"198.51.100.0/24",
		"203.0.113.10-203.0.113.20",
		"https://198.51.100.7/health",
	)

	tests := []struct {
		name       string
		targetType string
		aggressive bool
		want       []string
	}{
		{
			name:       "domains excludes urls ips cidrs and ranges",
			targetType: "domains",
			want:       []string{"*.example.com", "api.example.com"},
		},
		{
			name:       "urls only keeps http(s) targets",
			targetType: "urls",
			want:       []string{"https://portal.example.com/login", "http://legacy.example.com", "https://198.51.100.7/health"},
		},
		{
			name:       "ips keeps bare ips and hosts extracted from urls",
			targetType: "ips",
			want:       []string{"203.0.113.5", "198.51.100.7"},
		},
		{
			name:       "cidrs keeps cidr ranges and ip ranges",
			targetType: "cidrs",
			want:       []string{"198.51.100.0/24", "203.0.113.10-203.0.113.20"},
		},
		{
			name:       "wildcards keeps only star-dot targets",
			targetType: "wildcards",
			want:       []string{"*.example.com"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := filterTargets(all, tc.targetType, tc.aggressive)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("filterTargets(%q) = %#v, want %#v", tc.targetType, got, tc.want)
			}
		})
	}
}

// TestFilterDomainsDropsIPTargets is a focused regression test for the bug where
// `db get domains` emitted IP addresses and CIDR ranges as if they were domains.
func TestFilterDomainsDropsIPTargets(t *testing.T) {
	in := entries("203.0.113.5", "198.51.100.0/24", "10.0.0.1-10.0.0.9", "good.example.com")
	got := filterTargets(in, "domains", false)
	want := []string{"good.example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("domains filter should drop IP-like targets: got %#v, want %#v", got, want)
	}
}

func TestFilterDomainsAggressiveExtractsRootFromURL(t *testing.T) {
	in := entries("https://portal.sub.example.com/login")
	got := filterTargets(in, "domains", true)
	want := []string{"*.example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("aggressive domains should extract root domain: got %#v, want %#v", got, want)
	}
}

func TestIsDomainTarget(t *testing.T) {
	cases := map[string]bool{
		"example.com":               true,
		"*.example.com":             true,
		"a.b.example.com":           true,
		"":                          false,
		"localhost":                 false,
		"203.0.113.5":               false,
		"198.51.100.0/24":           false,
		"203.0.113.1-203.0.113.9":   false,
		"http://example.com":        false,
		"https://example.com/login": false,
	}
	for in, want := range cases {
		if got := isDomainTarget(in); got != want {
			t.Errorf("isDomainTarget(%q) = %v, want %v", in, got, want)
		}
	}
}
