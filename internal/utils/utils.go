package utils

import (
	"net"
	"net/url"
	"strings"

	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
)

func AreSlicesEqual(a, b []string) bool {
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

var Log = logrus.New()

func SetLogLevel(level string) {
	// We are not using logrus' trace and panic levels
	switch strings.ToLower(level) {
	case "debug":
		Log.SetLevel(log.DebugLevel)
	case "info":
		Log.SetLevel(log.InfoLevel)
	case "warning", "warn":
		Log.SetLevel(log.WarnLevel)
	case "error":
		Log.SetLevel(log.ErrorLevel)
	case "fatal":
		Log.SetLevel(log.FatalLevel)
	default:
		log.Fatal("Bad error level string")
	}
}

// IsCIDR checks if a string is a valid CIDR range (x.x.x.x/xx)
func IsCIDR(cidr string) bool {
	if !strings.Contains(cidr, "/") {
		return false
	}
	_, _, err := net.ParseCIDR(cidr)
	return err == nil
}

// IsIP checks if a string is a valid IP address (IPv4 or IPv6)
func IsIP(ip string) bool {
	// Remove any surrounding square brackets for IPv6 addresses
	ip = strings.Trim(ip, "[]")

	// Parse the IP address
	parsedIP := net.ParseIP(ip)

	// If parsedIP is not nil, it's a valid IP address
	return parsedIP != nil
}

// IsIPRange checks if a string is a valid IP range in the format x.x.x.x-y.y.y.y
func IsIPRange(ipRange string) bool {
	parts := strings.Split(ipRange, "-")
	if len(parts) != 2 {
		return false
	}

	startIP := net.ParseIP(strings.TrimSpace(parts[0]))
	endIP := net.ParseIP(strings.TrimSpace(parts[1]))

	return startIP != nil && endIP != nil
}

// RedactURL redacts the password from a URL string for safe logging.
// Example: "postgres://user:secret@host/db" -> "postgres://user:****@host/db"
func RedactURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		// If we can't parse it, return a generic redacted message
		return "[invalid URL]"
	}

	if parsed.User == nil {
		return rawURL // No credentials to redact
	}

	// Check if password exists
	if _, hasPassword := parsed.User.Password(); hasPassword {
		parsed.User = url.UserPassword(parsed.User.Username(), "****")
	}

	return parsed.String()
}
