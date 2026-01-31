package bugcrowd

import (
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
}
