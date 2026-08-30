package platforms

import (
	"strings"
	"testing"
)

func FuzzCanonicalName(f *testing.F) {
	f.Add("HackerOne")
	f.Add("bc")
	f.Add("")
	f.Add("all")
	f.Add("not-a-platform")
	f.Fuzz(func(t *testing.T, input string) {
		got := CanonicalName(input)
		trimmed := strings.ToLower(strings.TrimSpace(input))
		if trimmed == "" {
			if got != "" {
				t.Fatalf("CanonicalName(%q) = %q, want empty", input, got)
			}
			return
		}
		if KnownPlatform(input) {
			switch got {
			case "h1", "bc", "it", "ywh", "immunefi", "custom", "dev":
			default:
				t.Fatalf("CanonicalName(%q) = %q for known platform", input, got)
			}
			if len(MatchingNames(input)) == 0 {
				t.Fatalf("MatchingNames(%q) empty for known platform", input)
			}
			return
		}
		if trimmed == "all" {
			if MatchingNames(input) != nil {
				t.Fatalf("MatchingNames(%q) = %v, want nil", input, MatchingNames(input))
			}
			return
		}
		names := MatchingNames(input)
		if len(names) != 1 || names[0] != got {
			t.Fatalf("MatchingNames(%q) = %v, CanonicalName = %q", input, names, got)
		}
	})
}
