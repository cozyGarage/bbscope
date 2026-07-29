package ai

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

func TestNormalizerScenarios(t *testing.T) {
	falseVal := false
	tests := []struct {
		name     string
		baseID   int
		input    []storage.TargetItem
		norm     map[int]normalizedResult
		expected []storage.TargetItem
	}{
		{
			name:   "expands targets and keeps metadata",
			baseID: 5,
			input: []storage.TargetItem{
				{URI: "example.*", Category: "wildcard", Description: "main", InScope: true},
				{URI: "example.(it|com)", Category: "url", Description: "alt", InScope: false},
			},
			norm: map[int]normalizedResult{
				5: {Targets: []string{"example.com"}},
				6: {Targets: []string{"example.it", " example.com "}},
			},
			expected: []storage.TargetItem{
				{
					URI:         "example.*",
					Category:    "wildcard",
					Description: "main",
					InScope:     true,
					Variants: []storage.TargetVariant{
						{Value: "example.com"},
					},
				},
				{
					URI:         "example.(it|com)",
					Category:    "url",
					Description: "alt",
					InScope:     false,
					Variants: []storage.TargetVariant{
						{Value: "example.it"},
						{Value: "example.com"},
					},
				},
			},
		},
		{
			name:   "falls back to original when missing",
			baseID: 0,
			input: []storage.TargetItem{
				{URI: "original", Category: "url"},
			},
			norm: map[int]normalizedResult{},
			expected: []storage.TargetItem{
				{URI: "original", Category: "url"},
			},
		},
		{
			name:   "overrides in scope",
			baseID: 0,
			input: []storage.TargetItem{
				{URI: "example.com", Category: "url", InScope: true},
			},
			norm: map[int]normalizedResult{
				0: {Targets: []string{"example.com"}, InScope: &falseVal},
			},
			expected: []storage.TargetItem{
				{
					URI:      "example.com",
					Category: "url",
					InScope:  true,
					Variants: []storage.TargetVariant{
						{Value: "example.com", HasInScope: true, InScope: false},
					},
				},
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			out := mergeNormalized(tc.input, tc.baseID, tc.norm)
			if !reflect.DeepEqual(out, tc.expected) {
				t.Fatalf("input:\n%s\nexpected:\n%s\nactual:\n%s",
					mustJSON(tc.input), mustJSON(tc.expected), mustJSON(out))
			}
		})
	}

	t.Run("sanitize deduplicates", func(t *testing.T) {
		in := []string{"Example.COM ", " example.com", "  "}
		out := sanitizeTargets(in)
		if len(out) != 1 || out[0] != "example.com" {
			t.Fatalf("sanitize failed: %v", out)
		}
	})

	t.Run("drops invented unrelated targets", func(t *testing.T) {
		out := mergeNormalized(
			[]storage.TargetItem{{URI: "example.com", Category: "url"}},
			0,
			map[int]normalizedResult{0: {Targets: []string{"evil.com", "example.com"}}},
		)
		if len(out) != 1 {
			t.Fatalf("expected 1 item, got %#v", out)
		}
		// exact normalized match without overrides is skipped; invented host dropped
		if len(out[0].Variants) != 0 {
			t.Fatalf("expected invented variant dropped, got %#v", out[0].Variants)
		}
	})
}

func TestVariantAllowed(t *testing.T) {
	tests := []struct {
		original, variant string
		want              bool
	}{
		{"example.com", "example.com", true},
		{"example.*", "example.com", true},
		{"example.(it|com)", "example.it", true},
		{"https://*.example.com/**", "example.com", true},
		{"example.com", "evil.com", false},
		{"example.com", "totally-unrelated.net", false},
	}
	for _, tc := range tests {
		if got := variantAllowed(tc.original, tc.variant); got != tc.want {
			t.Errorf("variantAllowed(%q, %q) = %v, want %v", tc.original, tc.variant, got, tc.want)
		}
	}
}

func mustJSON(v any) string {
	data, _ := json.MarshalIndent(v, "", "  ")
	return string(data)
}
