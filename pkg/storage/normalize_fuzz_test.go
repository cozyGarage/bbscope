package storage

import "testing"

func FuzzNormalizeTarget(f *testing.F) {
	f.Add("*.example.com")
	f.Add("https://example.com/path")
	f.Add("HTTPS://EXAMPLE.COM:443/a/")
	f.Add("")
	f.Add("not a url")
	f.Fuzz(func(t *testing.T, input string) {
		_ = NormalizeTarget(input)
		_ = NormalizeProgramURL(input)
		_ = identityKey(input, "url")
	})
}
