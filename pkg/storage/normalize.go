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
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		u.Host = strings.ToLower(u.Host)
		if u.Scheme == "http" && u.Port() == "80" {
			u.Host = strings.TrimSuffix(u.Host, ":80")
		}
		if u.Scheme == "https" && u.Port() == "443" {
			u.Host = strings.TrimSuffix(u.Host, ":443")
		}
		u.Path = strings.TrimRight(u.Path, "/")
		if u.Scheme == "" {
			u.Scheme = "https"
		}
		return u.String()
	}
	// Wildcards/domains
	s = strings.ToLower(s)
	s = strings.TrimSuffix(s, ".")
	if strings.HasSuffix(s, "/") {
		s = strings.TrimRight(s, "/")
	}
	return s
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
