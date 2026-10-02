package storage

import (
	"net/url"
	"strings"
)

// NormalizeTarget applies simple canonicalization rules suitable for identity.
func NormalizeTarget(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	// If it looks like a URL, normalize scheme/host/trailing slash.
	// Wildcard hosts used to drop the path, so https://*.example.com/admin
	// and https://*.example.com/api collapsed to the same identity key and
	// one target was deleted on upsert.
	if u, ok := parseTargetURL(s); ok {
		u.Host = strings.ToLower(u.Host)
		// Default the scheme before stripping default ports, or "//host:443"
		// keeps its port now and loses it on the next pass.
		if u.Scheme == "" {
			u.Scheme = "https"
		}
		if u.Scheme == "http" && u.Port() == "80" {
			u.Host = strings.TrimSuffix(u.Host, ":80")
		}
		if u.Scheme == "https" && u.Port() == "443" {
			u.Host = strings.TrimSuffix(u.Host, ":443")
		}
		u.Path = strings.TrimRight(u.Path, "/")
		// Commit only if the result is itself still a URL; otherwise a second
		// pass would take the domain branch and rewrite it (e.g. "//::#A").
		if out := u.String(); isTargetURL(out) {
			return out
		}
	}
	// Wildcards/domains
	s = strings.ToLower(s)
	// Trim slashes and the root dot together: trimming "." first left
	// "example.com./" as "example.com.", which a second pass changed again.
	trimmed := strings.TrimRight(s, "/.")
	if len(trimmed) < len(s) {
		// Trimming can expose a URL shape ("//host:1." -> "//host:1"), so
		// renormalize; the input only shrinks, so this terminates.
		return NormalizeTarget(trimmed)
	}
	return trimmed
}

// parseTargetURL parses s as a URL with a real hostname. A port-only
// authority ("//:443") is not a URL target.
func parseTargetURL(s string) (*url.URL, bool) {
	u, err := url.Parse(s)
	return u, err == nil && u.Hostname() != ""
}

func isTargetURL(s string) bool {
	_, ok := parseTargetURL(s)
	return ok
}

// NormalizeProgramURL ensures consistent program URL identity.
func NormalizeProgramURL(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		u.Host = strings.ToLower(u.Host)
		// Strip a trailing slash, including a root-only "/". Leaving "/" in
		// place made https://host and https://host/ different UNIQUE keys
		// while lookup treated them as the same program.
		u.Path = strings.TrimRight(u.Path, "/")
		if u.Scheme == "" {
			u.Scheme = "https"
		}
		return u.String()
	}
	return s
}
