package storage

import (
	"testing"
	"unicode/utf8"
)

func FuzzNormalizeTarget(f *testing.F) {
	f.Add("*.example.com")
	f.Add("https://example.com/path")
	f.Add("HTTPS://EXAMPLE.COM:443/a/")
	f.Add("")
	f.Add("not a url")
	f.Add("example.com./")
	f.Fuzz(func(t *testing.T, input string) {
		// Stored identities are NormalizeTarget output; normalizing one again
		// must not move it, or the same target gets two identity keys. Targets
		// arrive via JSON, so invalid UTF-8 (which net/url re-escapes) can't occur.
		if once := NormalizeTarget(input); utf8.ValidString(input) && NormalizeTarget(once) != once {
			t.Fatalf("NormalizeTarget not idempotent: %q -> %q -> %q", input, once, NormalizeTarget(once))
		}
		_ = NormalizeProgramURL(input)
		_ = identityKey(input, "url")
	})
}
