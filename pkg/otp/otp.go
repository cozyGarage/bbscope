package otp

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // sha1 is required by classic TOTP (RFC 4226)
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"hash"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// GenerateTOTP creates a TOTP code for the provided base32 secret at time t.
// Supports otpauth:// URIs with digits, period, and algorithm (SHA1/SHA256/SHA512).
func GenerateTOTP(secret string, t time.Time) (string, error) {
	key, digits, period, algo, err := parseTOTPSecret(secret)
	if err != nil {
		return "", err
	}
	defer secureWipe(key)

	digits = clampTOTPDigits(digits)
	if period <= 0 {
		period = 30
	}
	step := uint64(t.Unix() / int64(period)) //nolint:gosec // safe conversion for TOTP time step
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], step)
	mac := hmac.New(algo, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	defer secureWipe(sum)

	offset := sum[len(sum)-1] & 0x0F
	code := (uint32(sum[offset])&0x7F)<<24 | (uint32(sum[offset+1])&0xFF)<<16 | (uint32(sum[offset+2])&0xFF)<<8 | (uint32(sum[offset+3]) & 0xFF)
	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	code = code % mod
	format := fmt.Sprintf("%%0%dd", digits)
	return fmt.Sprintf(format, code), nil
}

// clampTOTPDigits keeps digit counts in the RFC-common range (6–8).
// Unbounded values overflow uint32 modulus and can panic on digits==32 (mod==0).
func clampTOTPDigits(digits int) int {
	if digits < 6 {
		return 6
	}
	if digits > 8 {
		return 8
	}
	return digits
}

// secureWipe zeros out a byte slice to prevent sensitive data from lingering in memory.
// Note: This is a best-effort approach; the Go runtime may still have copies.
func secureWipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// parseTOTPSecret supports multiple formats:
// - "<digits> <base32>"
// - raw base32 (std/hex, with or without padding)
// - otpauth:// URI (digits, period, algorithm)
func parseTOTPSecret(s string) ([]byte, int, int, func() hash.Hash, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, 0, 0, nil, fmt.Errorf("empty secret")
	}
	if strings.HasPrefix(strings.ToLower(s), "otpauth://") {
		u, err := url.Parse(s)
		if err != nil {
			return nil, 0, 0, nil, err
		}
		q := u.Query()
		sec := q.Get("secret")
		digits := 6
		if d := q.Get("digits"); d != "" {
			if v, err := strconv.Atoi(d); err == nil { //nolint:govet // intentional new scope
				digits = v
			}
		}
		period := 30
		if p := q.Get("period"); p != "" {
			if v, err := strconv.Atoi(p); err == nil && v > 0 {
				period = v
			}
		}
		algo, err := hashFromAlgorithm(q.Get("algorithm"))
		if err != nil {
			return nil, 0, 0, nil, err
		}
		k, err := decodeBase32Flexible(sec)
		return k, digits, period, algo, err
	}
	parts := strings.Fields(s)
	digits := 6
	if len(parts) >= 2 {
		if v, err := strconv.Atoi(parts[0]); err == nil {
			digits = v
			s = strings.Join(parts[1:], "")
		}
	}
	k, err := decodeBase32Flexible(s)
	if err != nil {
		return nil, 0, 0, nil, err
	}
	return k, digits, 30, sha1.New, nil
}

func hashFromAlgorithm(name string) (func() hash.Hash, error) {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "", "SHA1":
		return sha1.New, nil
	case "SHA256":
		return sha256.New, nil
	case "SHA512":
		return sha512.New, nil
	default:
		return nil, fmt.Errorf("unsupported TOTP algorithm %q", name)
	}
}

func decodeBase32Flexible(sec string) ([]byte, error) {
	raw := strings.TrimSpace(sec)
	if raw == "" {
		return nil, fmt.Errorf("empty secret")
	}
	upper := strings.ToUpper(raw)
	noPad := strings.TrimRight(upper, "=")

	// Try standard base32 with padding
	if k, err := base32.StdEncoding.DecodeString(upper); err == nil && len(k) > 0 {
		return k, nil
	}
	// Try standard base32 without padding
	if k, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(noPad); err == nil && len(k) > 0 {
		return k, nil
	}
	// Try base32hex with padding
	if k, err := base32.HexEncoding.DecodeString(upper); err == nil && len(k) > 0 {
		return k, nil
	}
	// Try base32hex without padding
	if k, err := base32.HexEncoding.WithPadding(base32.NoPadding).DecodeString(noPad); err == nil && len(k) > 0 {
		return k, nil
	}
	return nil, fmt.Errorf("invalid base32 secret")
}
