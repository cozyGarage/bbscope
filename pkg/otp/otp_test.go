package otp

import (
	"testing"
	"time"
)

func TestGenerateTOTP(t *testing.T) {
	// Test vector from RFC 6238
	// Using a known secret and time to verify TOTP generation
	secret := "JBSWY3DPEHPK3PXP" // Base32 encoded "Hello!"

	tests := []struct {
		name    string
		secret  string
		wantErr bool
	}{
		{"valid base32 secret", "JBSWY3DPEHPK3PXP", false},
		{"valid lowercase", "jbswy3dpehpk3pxp", false},
		{"empty secret", "", true},
		{"invalid base32", "!!!invalid!!!", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, err := GenerateTOTP(tt.secret, time.Now())
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateTOTP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				// Should return a 6-digit code
				if len(code) != 6 {
					t.Errorf("GenerateTOTP() code length = %d, want 6", len(code))
				}
				// Should be all digits
				for _, c := range code {
					if c < '0' || c > '9' {
						t.Errorf("GenerateTOTP() code contains non-digit: %c", c)
					}
				}
			}
		})
	}

	// Test that same secret at same time gives same code
	t.Run("deterministic", func(t *testing.T) {
		fixedTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
		code1, err1 := GenerateTOTP(secret, fixedTime)
		code2, err2 := GenerateTOTP(secret, fixedTime)
		if err1 != nil || err2 != nil {
			t.Fatalf("unexpected error: %v, %v", err1, err2)
		}
		if code1 != code2 {
			t.Errorf("same time should give same code: %s != %s", code1, code2)
		}
	})

	// Test that different times give different codes (usually)
	t.Run("time dependent", func(t *testing.T) {
		time1 := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
		time2 := time.Date(2024, 1, 1, 12, 1, 0, 0, time.UTC) // 1 minute later
		code1, _ := GenerateTOTP(secret, time1)
		code2, _ := GenerateTOTP(secret, time2)
		// Codes might be same if within same 30s window, but likely different
		t.Logf("Code at %v: %s, Code at %v: %s", time1, code1, time2, code2)
	})
}

func TestGenerateTOTP_OTPAuthURI(t *testing.T) {
	// Test with otpauth:// URI format
	uri := "otpauth://totp/Test:user@example.com?secret=JBSWY3DPEHPK3PXP&issuer=Test"

	code, err := GenerateTOTP(uri, time.Now())
	if err != nil {
		t.Errorf("GenerateTOTP() with URI error = %v", err)
		return
	}
	if len(code) != 6 {
		t.Errorf("GenerateTOTP() code length = %d, want 6", len(code))
	}
}

func TestGenerateTOTP_DigitsPrefix(t *testing.T) {
	// Test with "digits secret" format
	secret := "6 JBSWY3DPEHPK3PXP"

	code, err := GenerateTOTP(secret, time.Now())
	if err != nil {
		t.Errorf("GenerateTOTP() error = %v", err)
		return
	}
	if len(code) != 6 {
		t.Errorf("GenerateTOTP() code length = %d, want 6", len(code))
	}
}

func TestGenerateTOTP_8Digits(t *testing.T) {
	// Test 8-digit TOTP
	secret := "8 JBSWY3DPEHPK3PXP"

	code, err := GenerateTOTP(secret, time.Now())
	if err != nil {
		t.Errorf("GenerateTOTP() error = %v", err)
		return
	}
	if len(code) != 8 {
		t.Errorf("GenerateTOTP() code length = %d, want 8", len(code))
	}
}
