package bugcrowd

import (
	"strings"
	"testing"
)

func TestConstants(t *testing.T) {
	// Test that constants are defined correctly
	if USER_AGENT == "" {
		t.Error("USER_AGENT should not be empty")
	}

	if RATE_LIMIT_SLEEP_SECONDS <= 0 {
		t.Errorf("RATE_LIMIT_SLEEP_SECONDS should be positive, got %d", RATE_LIMIT_SLEEP_SECONDS)
	}

	if WAF_BANNED_ERROR == "" {
		t.Error("WAF_BANNED_ERROR should not be empty")
	}
}

func TestUserAgentFormat(t *testing.T) {
	// Verify the user agent looks like a valid browser string
	if len(USER_AGENT) < 20 {
		t.Errorf("USER_AGENT seems too short: %s", USER_AGENT)
	}

	// Should contain Mozilla (standard for browser user agents)
	if len(USER_AGENT) > 0 && USER_AGENT[0:7] != "Mozilla" {
		t.Errorf("USER_AGENT should start with 'Mozilla', got: %s", USER_AGENT[:20])
	}
}

func TestRateLimitChannel(t *testing.T) {
	// Verify the rate limit channel was initialized
	if rateLimitRequestChan == nil {
		t.Error("rateLimitRequestChan should be initialized")
	}
}

func TestWAFBannedErrorMessage(t *testing.T) {
	expected := "you are temporarily WAF banned, change IP or wait a few hours"
	if WAF_BANNED_ERROR != expected {
		t.Errorf("WAF_BANNED_ERROR = %v, want %v", WAF_BANNED_ERROR, expected)
	}
	if !strings.Contains(WAF_BANNED_ERROR, "WAF") {
		t.Errorf("WAF_BANNED_ERROR should mention WAF, got: %s", WAF_BANNED_ERROR)
	}
}

func TestValidateBugcrowdRedirectURL(t *testing.T) {
	if err := validateBugcrowdRedirectURL("https://bugcrowd.com/home"); err != nil {
		t.Fatalf("bugcrowd.com should be allowed: %v", err)
	}
	if err := validateBugcrowdRedirectURL("https://identity.bugcrowd.com/login"); err != nil {
		t.Fatalf("identity.bugcrowd.com should be allowed: %v", err)
	}
	if err := validateBugcrowdRedirectURL("https://evil.example/phish"); err == nil {
		t.Fatal("off-origin redirect should be rejected")
	}
	if err := validateBugcrowdRedirectURL("http://bugcrowd.com/home"); err == nil {
		t.Fatal("http redirect should be rejected")
	}

	got, err := sanitizeBugcrowdRedirectURL("/dashboard")
	if err != nil {
		t.Fatalf("relative redirect should be allowed: %v", err)
	}
	if got != "https://identity.bugcrowd.com/dashboard" {
		t.Fatalf("relative redirect resolved to %q", got)
	}
	if _, err := sanitizeBugcrowdRedirectURL("dashboard"); err == nil {
		t.Fatal("non-absolute relative path should be rejected")
	}
	if _, err := sanitizeBugcrowdRedirectURL("//evil.example/phish"); err == nil {
		t.Fatal("scheme-relative URL should be rejected")
	}
}

func TestResolveBugcrowdAPIURL(t *testing.T) {
	orig := apiBaseURL
	apiBaseURL = "https://bugcrowd.com"
	t.Cleanup(func() { apiBaseURL = orig })

	got, err := resolveBugcrowdAPIURL("/programs/acme/targets.json")
	if err != nil {
		t.Fatalf("relative path: %v", err)
	}
	if got != "https://bugcrowd.com/programs/acme/targets.json" {
		t.Fatalf("resolved = %q", got)
	}
	if _, err := resolveBugcrowdAPIURL("https://evil.example/steal"); err == nil {
		t.Fatal("absolute off-origin URL should be rejected")
	}
	if _, err := resolveBugcrowdAPIURL("//evil.example/steal"); err == nil {
		t.Fatal("scheme-relative URL should be rejected")
	}
	if _, err := resolveBugcrowdAPIURL("//user@evil.example/steal"); err == nil {
		t.Fatal("userinfo URL should be rejected")
	}
}
