package platforms

import "strings"

// canonicalNames maps every accepted spelling onto the short name stored in
// the database and used by poller.Name() (h1, bc, it, ywh, immunefi, custom, dev).
var canonicalNames = map[string]string{
	"hackerone": "h1",
	"h1":        "h1",
	"bugcrowd":  "bc",
	"bc":        "bc",
	"intigriti": "it",
	"it":        "it",
	"yeswehack": "ywh",
	"ywh":       "ywh",
	"immunefi":  "immunefi",
	"custom":    "custom",
	"dev":       "dev",
}

// CanonicalName returns the short platform name used in storage and poller.Name.
// Unknown names are lowercased and trimmed but otherwise passed through so
// custom/test platforms keep working.
func CanonicalName(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return ""
	}
	if c, ok := canonicalNames[n]; ok {
		return c
	}
	return n
}

// KnownPlatform reports whether name is a built-in platform alias (including
// custom and the hidden dev poller).
func KnownPlatform(name string) bool {
	_, ok := canonicalNames[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

// MatchingNames returns every spelling that should match name in a database
// filter: the canonical short name plus long aliases. Unknown names yield a
// single lowercased token so test platforms still round-trip.
func MatchingNames(name string) []string {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" || n == "all" {
		return nil
	}
	canon := CanonicalName(n)
	seen := map[string]bool{canon: true}
	out := []string{canon}
	for alias, c := range canonicalNames {
		if c == canon && !seen[alias] {
			seen[alias] = true
			out = append(out, alias)
		}
	}
	return out
}
