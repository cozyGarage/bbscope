package immunefi

import "testing"

func TestExtractJSONArray(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", `[1,2,3]trailing`, `[1,2,3]`},
		{"nested", `[[1],[2,[3]]]xyz`, `[[1],[2,[3]]]`},
		{"brackets inside string", `["a]b","c"]rest`, `["a]b","c"]`},
		{"escaped quote in string", `["a\"]b"]rest`, `["a\"]b"]`},
		{"empty array", `[]abc`, `[]`},
		{"not an array", `{"a":1}`, ``},
		{"unterminated", `[1,2,3`, ``},
		{"empty string", ``, ``},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractJSONArray(tc.input); got != tc.want {
				t.Errorf("extractJSONArray(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
