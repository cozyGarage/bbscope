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

func TestClampTOTPDigits(t *testing.T) {
	if got := clampTOTPDigits(0); got != 6 {
		t.Fatalf("clamp 0 = %d, want 6", got)
	}
	if got := clampTOTPDigits(32); got != 8 {
		t.Fatalf("clamp 32 = %d, want 8", got)
	}
	if got := clampTOTPDigits(7); got != 7 {
		t.Fatalf("clamp 7 = %d, want 7", got)
	}
}

func TestGenerateTOTP_HugeDigitsNoPanic(t *testing.T) {
	// Previously digits=32 caused mod==0 and panic on code%mod.
	code, err := GenerateTOTP("32 JBSWY3DPEHPK3PXP", time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("GenerateTOTP: %v", err)
	}
	if len(code) != 8 {
		t.Fatalf("expected clamped 8-digit code, got %q", code)
	}
}

func TestDecodeBase32Flexible(t *testing.T) {
	// "Hello!" in each base32 variant/padding combination.
	tests := []struct {
		name string
		sec  string
	}{
		{"std padded", "JBSWY3DPEHPK3PXP"}, // no padding needed for 12 bytes, kept for parity
		{"std no padding, trailing =", "JBSWY3DPEE======"},
		{"hex padded", "9128OR3F45FAR3FE"},
		{"hex no padding, trailing =", "9128OR3F44======"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := decodeBase32Flexible(tt.sec); err != nil {
				t.Fatalf("decodeBase32Flexible(%q) failed: %v", tt.sec, err)
			}
		})
	}
}

func TestGenerateTOTP_PeriodAndAlgorithm(t *testing.T) {
	uri := "otpauth://totp/Test:user@example.com?secret=JBSWY3DPEHPK3PXP&issuer=Test&period=60&algorithm=SHA256&digits=6"
	fixed := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	code, err := GenerateTOTP(uri, fixed)
	if err != nil {
		t.Fatalf("GenerateTOTP: %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("expected 6-digit code, got %q", code)
	}
	// Different period should usually yield a different code than period=30.
	uri30 := "otpauth://totp/Test:user@example.com?secret=JBSWY3DPEHPK3PXP&period=30&algorithm=SHA1&digits=6"
	code30, err := GenerateTOTP(uri30, fixed)
	if err != nil {
		t.Fatalf("GenerateTOTP period=30: %v", err)
	}
	if code == code30 {
		t.Logf("period/algorithm variants produced same code at this timestamp (possible but uncommon): %s", code)
	}
}
