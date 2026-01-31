// Package credentials provides secure credential storage using OS-native keychains.
// It supports macOS Keychain, Windows Credential Manager, and Linux Secret Service.
package credentials

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
	"github.com/zalando/go-keyring"
)

const (
	// ServiceName is the identifier used in the OS keychain
	ServiceName = "bbscope"
)

// Credential keys for each platform
const (
	// HackerOne
	KeyHackerOneUsername = "hackerone.username"
	KeyHackerOneToken    = "hackerone.token"

	// Bugcrowd
	KeyBugcrowdEmail     = "bugcrowd.email"
	KeyBugcrowdPassword  = "bugcrowd.password"
	KeyBugcrowdOTPSecret = "bugcrowd.otpsecret"

	// Intigriti
	KeyIntigritiToken = "intigriti.token"

	// YesWeHack
	KeyYesWeHackEmail     = "yeswehack.email"
	KeyYesWeHackPassword  = "yeswehack.password"
	KeyYesWeHackOTPSecret = "yeswehack.otpsecret"
)

// Get retrieves a credential, trying the OS keychain first, then falling back to config file.
// This allows gradual migration from config file to keychain storage.
func Get(key string) string {
	// Normalize key
	key = strings.ToLower(strings.TrimSpace(key))

	// Try OS keychain first
	if val, err := keyring.Get(ServiceName, key); err == nil && val != "" {
		return val
	}

	// Fall back to config file (viper)
	return viper.GetString(key)
}

// Set stores a credential in the OS keychain.
func Set(key, value string) error {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return fmt.Errorf("credential key cannot be empty")
	}
	if value == "" {
		return fmt.Errorf("credential value cannot be empty")
	}
	return keyring.Set(ServiceName, key, value)
}

// Delete removes a credential from the OS keychain.
func Delete(key string) error {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return fmt.Errorf("credential key cannot be empty")
	}
	return keyring.Delete(ServiceName, key)
}

// IsInKeychain checks if a credential exists in the OS keychain.
func IsInKeychain(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	val, err := keyring.Get(ServiceName, key)
	return err == nil && val != ""
}

// GetSource returns where a credential is stored ("keychain", "config", or "none").
func GetSource(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))

	// Check keychain first
	if val, err := keyring.Get(ServiceName, key); err == nil && val != "" {
		return "keychain"
	}

	// Check config file
	if viper.GetString(key) != "" {
		return "config"
	}

	return "none"
}

// MigrateToKeychain moves a credential from config file to keychain.
// Returns true if migration occurred, false if already in keychain or not in config.
func MigrateToKeychain(key string) (bool, error) {
	key = strings.ToLower(strings.TrimSpace(key))

	// Check if already in keychain
	if IsInKeychain(key) {
		return false, nil
	}

	// Get from config
	value := viper.GetString(key)
	if value == "" {
		return false, nil
	}

	// Store in keychain
	if err := Set(key, value); err != nil {
		return false, fmt.Errorf("failed to store in keychain: %w", err)
	}

	return true, nil
}

// ListKeys returns all known credential keys for reference.
func ListKeys() []string {
	return []string{
		KeyHackerOneUsername,
		KeyHackerOneToken,
		KeyBugcrowdEmail,
		KeyBugcrowdPassword,
		KeyBugcrowdOTPSecret,
		KeyIntigritiToken,
		KeyYesWeHackEmail,
		KeyYesWeHackPassword,
		KeyYesWeHackOTPSecret,
	}
}
