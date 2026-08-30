package cmd

import (
	"strings"
	"testing"
)

func TestMaskValue(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"short", "*****"},
		{"12345678", "********"},
		{"abcdefghijklmnop", "abcd********mnop"},
	}
	for _, tc := range tests {
		if got := maskValue(tc.in); got != tc.want {
			t.Errorf("maskValue(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	got := maskValue("supersecretvalue")
	if strings.Contains(got, "secret") {
		t.Fatalf("mask leaked inner bytes: %q", got)
	}
}
