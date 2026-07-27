package otp

import (
	"testing"
	"time"
)

// TestGenerateTOTP_RFC6238Vectors validates GenerateTOTP against the well-known
// RFC 6238 Appendix B test vectors for the SHA-1 variant. The seed is the ASCII
// string "12345678901234567890" (base32 "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ").
// bbscope emits 6-digit codes, so we assert the low 6 digits of each vector.
func TestGenerateTOTP_RFC6238Vectors(t *testing.T) {
	const secret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

	tests := []struct {
		unix int64
		want string // low 6 digits of the RFC's 8-digit vector
	}{
		{59, "287082"},          // 94287082
		{1111111109, "081804"},  // 07081804
		{1111111111, "050471"},  // 14050471
		{1234567890, "005924"},  // 89005924
		{2000000000, "279037"},  // 69279037
		{20000000000, "353130"}, // 65353130
	}

	for _, tc := range tests {
		got, err := GenerateTOTP(secret, time.Unix(tc.unix, 0).UTC())
		if err != nil {
			t.Fatalf("GenerateTOTP(t=%d): %v", tc.unix, err)
		}
		if got != tc.want {
			t.Errorf("GenerateTOTP(t=%d) = %q, want %q", tc.unix, got, tc.want)
		}
	}
}
