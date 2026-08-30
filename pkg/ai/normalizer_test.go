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
		{"example.com", "com", false},
		{"example.co.uk", "co.uk", false},
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

// TestVariantAllowedRejectsWidening pins the direction constraint: an AI
// variant may restate or narrow the original target, never widen it. Accepting
// a parent domain would report assets the program never put in scope.
func TestVariantAllowedRejectsWidening(t *testing.T) {
	tests := []struct {
		name     string
		original string
		variant  string
		want     bool
	}{
		// Widening — all must be rejected.
		{"parent domain of a single host", "foo.example.com", "example.com", false},
		{"public suffix label", "foo.example.com", "com", false},
		{"wildcard over a single host", "foo.example.com", "*.example.com", false},
		{"registrable domain of a deep host", "api.internal.example.co.uk", "example.co.uk", false},
		{"sibling of the original", "foo.example.com", "bar.example.com", false},
		{"parent of a wildcard base", "*.foo.example.com", "example.com", false},
		{"unrelated host", "foo.example.com", "evil.com", false},
		{"suffix-confusable host", "example.com", "example.com.evil.net", false},
		{"scheme and path wrapping the suffix", "example.com", "http://evil.com/x.example.com", false},
		{"wildcard over the original apex", "example.com", "*.example.com", false},
		{"suffix match on a truncated original", "app.*", "evil.app", false},
		{"path-scoped URL restated as apex", "https://example.com/api", "example.com", false},

		// Restating or narrowing — all must be accepted.
		{"exact restatement", "example.com", "example.com", true},
		{"subdomain of the original", "example.com", "api.example.com", true},
		{"apex of an explicit wildcard", "https://*.example.com/**", "example.com", true},
		{"subdomain under an explicit wildcard", "*.example.com", "api.example.com", true},
		{"completes a right-truncated original", "example.*", "example.com", true},
		{"completes to a multi-label public suffix", "example.*", "example.co.uk", true},
		{"alternation expansion", "example.(it|com)", "example.it", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := variantAllowed(tc.original, tc.variant); got != tc.want {
				t.Errorf("variantAllowed(%q, %q) = %v, want %v", tc.original, tc.variant, got, tc.want)
			}
		})
	}
}

// TestMergeNormalizedDropsWidenedVariants checks the constraint holds through
// the merge path the poller actually calls, not just the predicate.
func TestMergeNormalizedDropsWidenedVariants(t *testing.T) {
	items := []storage.TargetItem{
		{URI: "shop.example.com", Category: "url", Description: "storefront", InScope: true},
	}
	// The model proposes the apex plus a sibling; both widen scope.
	norm := map[int]normalizedResult{
		0: {Targets: []string{"example.com", "admin.example.com"}},
	}

	out := mergeNormalized(items, 0, norm)
	if len(out) != 1 {
		t.Fatalf("expected the original item preserved, got %d items", len(out))
	}
	if len(out[0].Variants) != 0 {
		t.Fatalf("widened variants should be dropped, got %#v", out[0].Variants)
	}
	if out[0].URI != "shop.example.com" || out[0].Description != "storefront" || !out[0].InScope {
		t.Fatalf("original item was altered: %#v", out[0])
	}
}
