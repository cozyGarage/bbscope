// Package validate provides input validation helpers for bbscope
package validate

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	// Common regex patterns
	domainRegex   = regexp.MustCompile(`^(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)
	wildcardRegex = regexp.MustCompile(`^\*\.(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)
	ipv4Regex     = regexp.MustCompile(`^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$`)
	cidrV4Regex   = regexp.MustCompile(`^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)/(?:[0-9]|[1-2][0-9]|3[0-2])$`)
	handleRegex   = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}$`)
)

// ValidationError represents a validation failure
type ValidationError struct {
	Field   string
	Value   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed for %s: %s (got: %q)", e.Field, e.Message, e.Value)
}

// Domain validates a domain name
func Domain(domain string) error {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return &ValidationError{Field: "domain", Value: domain, Message: "cannot be empty"}
	}
	if len(domain) > 253 {
		return &ValidationError{Field: "domain", Value: domain, Message: "exceeds maximum length of 253 characters"}
	}
	if !domainRegex.MatchString(domain) {
		return &ValidationError{Field: "domain", Value: domain, Message: "invalid domain format"}
	}
	return nil
}

// Wildcard validates a wildcard domain (e.g., *.example.com)
func Wildcard(wildcard string) error {
	wildcard = strings.TrimSpace(strings.ToLower(wildcard))
	if wildcard == "" {
		return &ValidationError{Field: "wildcard", Value: wildcard, Message: "cannot be empty"}
	}
	if !wildcardRegex.MatchString(wildcard) {
		return &ValidationError{Field: "wildcard", Value: wildcard, Message: "invalid wildcard format (expected *.domain.tld)"}
	}
	return nil
}

// IPv4 validates an IPv4 address
func IPv4(ip string) error {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return &ValidationError{Field: "ip", Value: ip, Message: "cannot be empty"}
	}
	if !ipv4Regex.MatchString(ip) {
		return &ValidationError{Field: "ip", Value: ip, Message: "invalid IPv4 address"}
	}
	return nil
}

// CIDR validates a CIDR notation (e.g., 192.168.1.0/24)
func CIDR(cidr string) error {
	cidr = strings.TrimSpace(cidr)
	if cidr == "" {
		return &ValidationError{Field: "cidr", Value: cidr, Message: "cannot be empty"}
	}
	if !cidrV4Regex.MatchString(cidr) {
		return &ValidationError{Field: "cidr", Value: cidr, Message: "invalid CIDR notation"}
	}
	return nil
}

// URL validates a URL string
func URL(rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return &ValidationError{Field: "url", Value: rawURL, Message: "cannot be empty"}
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return &ValidationError{Field: "url", Value: rawURL, Message: fmt.Sprintf("parse error: %v", err)}
	}
	if parsed.Scheme == "" {
		return &ValidationError{Field: "url", Value: rawURL, Message: "missing scheme (http/https)"}
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return &ValidationError{Field: "url", Value: rawURL, Message: "scheme must be http or https"}
	}
	if parsed.Host == "" {
		return &ValidationError{Field: "url", Value: rawURL, Message: "missing host"}
	}
	if parsed.User != nil {
		return &ValidationError{Field: "url", Value: rawURL, Message: "userinfo is not allowed"}
	}
	return nil
}

// Handle validates a program handle
func Handle(handle string) error {
	handle = strings.TrimSpace(handle)
	if handle == "" {
		return &ValidationError{Field: "handle", Value: handle, Message: "cannot be empty"}
	}
	if !handleRegex.MatchString(handle) {
		return &ValidationError{Field: "handle", Value: handle, Message: "must be alphanumeric with optional hyphens/underscores, 1-63 chars"}
	}
	return nil
}

// Platform validates a platform name (full names or short CLI aliases).
func Platform(platform string) error {
	platform = strings.TrimSpace(strings.ToLower(platform))
	validPlatforms := []string{
		"hackerone", "h1",
		"bugcrowd", "bc",
		"intigriti", "it",
		"yeswehack", "ywh",
		"immunefi",
		"custom",
		"dev",
	}
	for _, valid := range validPlatforms {
		if platform == valid {
			return nil
		}
	}
	return &ValidationError{
		Field:   "platform",
		Value:   platform,
		Message: fmt.Sprintf("must be one of: %s", strings.Join(validPlatforms, ", ")),
	}
}

// DatabaseURL validates a PostgreSQL connection string and returns a sanitized
// version. Both URL form ("postgres://user:pass@host/db") and libpq
// keyword/value DSN form ("host=... password=...") are accepted, matching
// what storage.Open (via sql.Open("pgx", ...)) actually connects with; DSNs
// are passed through as-is since url.Parse cannot validate their structure.
func DatabaseURL(connURL string) (sanitized string, err error) {
	connURL = strings.TrimSpace(connURL)
	if connURL == "" {
		return "", &ValidationError{Field: "database_url", Value: "", Message: "cannot be empty"}
	}
	if isKeywordValueDSN(connURL) {
		return connURL, nil
	}
	parsed, err := url.Parse(connURL)
	if err != nil {
		return "", &ValidationError{Field: "database_url", Value: "[redacted]", Message: fmt.Sprintf("parse error: %v", err)}
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return "", &ValidationError{Field: "database_url", Value: "[redacted]", Message: "scheme must be postgres or postgresql"}
	}
	if parsed.Host == "" {
		return "", &ValidationError{Field: "database_url", Value: "[redacted]", Message: "missing host"}
	}

	// Return sanitized URL (password redacted)
	if parsed.User != nil {
		sanitized = fmt.Sprintf("%s://%s:****@%s%s", parsed.Scheme, parsed.User.Username(), parsed.Host, parsed.Path)
	} else {
		sanitized = fmt.Sprintf("%s://%s%s", parsed.Scheme, parsed.Host, parsed.Path)
	}
	return sanitized, nil
}

// isKeywordValueDSN reports whether connStr looks like a libpq keyword/value
// DSN rather than a URL. Anything carrying a scheme is treated as a URL.
func isKeywordValueDSN(connStr string) bool {
	if strings.Contains(connStr, "://") {
		return false
	}
	return strings.Contains(connStr, "=")
}

// NotEmpty validates that a string is not empty
func NotEmpty(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return &ValidationError{Field: field, Value: value, Message: "cannot be empty"}
	}
	return nil
}

// MaxLength validates that a string doesn't exceed a maximum length
func MaxLength(field, value string, max int) error {
	if len(value) > max {
		return &ValidationError{Field: field, Value: value, Message: fmt.Sprintf("exceeds maximum length of %d characters", max)}
	}
	return nil
}
