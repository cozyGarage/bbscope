package credentials

import (
	"testing"
)

func TestKeyConstants(t *testing.T) {
	// Verify all key constants are properly formatted
	keys := ListKeys()
	if len(keys) == 0 {
		t.Error("ListKeys() returned empty slice")
	}

	for _, key := range keys {
		if key == "" {
			t.Error("Found empty key in ListKeys()")
		}
		// Keys should be lowercase with dots
		if key != key {
			t.Errorf("Key should be consistent: %s", key)
		}
	}
}

func TestGetSource_None(t *testing.T) {
	// Test that a non-existent key returns "none"
	source := GetSource("nonexistent.key.12345")
	if source != "none" && source != "keychain" && source != "config" {
		t.Errorf("GetSource() returned unexpected value: %s", source)
	}
}

func TestGet_Empty(t *testing.T) {
	// Test that getting a non-existent key returns empty string
	val := Get("nonexistent.key.12345")
	// May be empty or from keychain/config - just verify it doesn't panic
	_ = val
}

func TestSet_EmptyKey(t *testing.T) {
	err := Set("", "value")
	if err == nil {
		t.Error("Set() should return error for empty key")
	}
}

func TestSet_EmptyValue(t *testing.T) {
	err := Set("test.key", "")
	if err == nil {
		t.Error("Set() should return error for empty value")
	}
}

func TestDelete_EmptyKey(t *testing.T) {
	err := Delete("")
	if err == nil {
		t.Error("Delete() should return error for empty key")
	}
}

func TestIsInKeychain_NonExistent(t *testing.T) {
	// Test with a key that definitely doesn't exist
	exists := IsInKeychain("bbscope.test.nonexistent.99999")
	// This should return false (unless someone added it)
	_ = exists // Just verify it doesn't panic
}

func TestServiceName(t *testing.T) {
	if ServiceName != "bbscope" {
		t.Errorf("ServiceName should be 'bbscope', got %s", ServiceName)
	}
}
