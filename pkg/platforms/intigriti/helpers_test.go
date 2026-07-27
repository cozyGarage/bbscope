package intigriti

import (
	"reflect"
	"testing"
)

func TestGetCategoryID(t *testing.T) {
	tests := []struct {
		input string
		want  []int
	}{
		{"url", []int{1}},
		{"cidr", []int{4}},
		{"mobile", []int{2, 3}},
		{"wildcard", []int{7}},
		{"all", nil},
		{"", nil},
		{"unknown-category", nil},
		{"URL", []int{1}}, // case-insensitive
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			if got := getCategoryID(tc.input); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("getCategoryID(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestIsInArray(t *testing.T) {
	arr := []int{1, 2, 3}
	if !isInArray(2, arr) {
		t.Error("isInArray(2, [1,2,3]) = false, want true")
	}
	if isInArray(9, arr) {
		t.Error("isInArray(9, [1,2,3]) = true, want false")
	}
	if isInArray(1, nil) {
		t.Error("isInArray(1, nil) = true, want false")
	}
}
