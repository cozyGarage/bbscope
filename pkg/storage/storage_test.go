package storage

import (
	"testing"
	"time"
)

func TestValidateDatabaseName(t *testing.T) {
	tests := []struct {
		name    string
		dbName  string
		wantErr bool
	}{
		// Valid names
		{"simple lowercase", "bbscope", false},
		{"with underscore", "bb_scope", false},
		{"starts with underscore", "_bbscope", false},
		{"with numbers", "bbscope123", false},
		{"single char", "a", false},
		{"max length", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false}, // 63 chars

		// Invalid names
		{"empty", "", true},
		{"starts with number", "123bbscope", true},
		{"contains hyphen", "bb-scope", true},
		{"contains space", "bb scope", true},
		{"contains dot", "bb.scope", true},
		{"special chars", "bb@scope", true},
		{"too long", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true}, // 64 chars
		{"sql injection attempt", "bbscope; DROP TABLE users;", true},
		{"quotes", `"bbscope"`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDatabaseName(tt.dbName)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDatabaseName(%q) error = %v, wantErr %v", tt.dbName, err, tt.wantErr)
			}
		})
	}
}

func TestRedactConnectionString(t *testing.T) {
	tests := []struct {
		name    string
		connStr string
		want    string
	}{
		{
			"with password",
			"postgres://user:secretpassword@localhost:5432/db",
			"postgres://user:%2A%2A%2A%2A@localhost:5432/db", // URL-encoded ****
		},
		{
			"without password",
			"postgres://user@localhost:5432/db",
			"postgres://user@localhost:5432/db",
		},
		{
			"no credentials",
			"postgres://localhost:5432/db",
			"postgres://localhost:5432/db",
		},
		{
			"with query params",
			"postgres://user:pass@localhost/db?sslmode=disable",
			"postgres://user:%2A%2A%2A%2A@localhost/db?sslmode=disable", // URL-encoded ****
		},
		{
			"complex password",
			"postgres://admin:p@ss=w0rd!@localhost/db",
			"postgres://admin:%2A%2A%2A%2A@localhost/db", // URL-encoded ****
		},
		{
			"invalid URL",
			"not a valid url ::::",
			"[invalid connection string]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactConnectionString(tt.connStr)
			if got != tt.want {
				t.Errorf("redactConnectionString(%q) = %q, want %q", tt.connStr, got, tt.want)
			}
		})
	}
}

func TestUpsertEntry(t *testing.T) {
	// Test UpsertEntry struct creation
	entry := UpsertEntry{
		TargetNormalized: "*.example.com",
		TargetRaw:        "*.example.com",
		Category:         "wildcard",
		Description:      "Main wildcard",
		InScope:          true,
		IsBBP:            true,
	}

	if entry.TargetNormalized != "*.example.com" {
		t.Errorf("TargetNormalized = %q, want %q", entry.TargetNormalized, "*.example.com")
	}
	if !entry.InScope {
		t.Error("InScope should be true")
	}
	if !entry.IsBBP {
		t.Error("IsBBP should be true")
	}
}

func TestChangeStruct(t *testing.T) {
	change := Change{
		ProgramURL:       "https://hackerone.com/example",
		Platform:         "hackerone",
		Handle:           "example",
		TargetNormalized: "*.example.com",
		TargetRaw:        "*.example.com",
		Category:         "wildcard",
		InScope:          true,
		IsBBP:            true,
		ChangeType:       "added",
	}

	if change.ChangeType != "added" {
		t.Errorf("ChangeType = %q, want %q", change.ChangeType, "added")
	}
	if change.Platform != "hackerone" {
		t.Errorf("Platform = %q, want %q", change.Platform, "hackerone")
	}
}

func TestEntryStruct(t *testing.T) {
	entry := Entry{
		ProgramURL:        "https://hackerone.com/example",
		Handle:            "example",
		TargetNormalized:  "*.example.com",
		Category:          "wildcard",
		InScope:           true,
		IsBBP:             true,
	}

	if entry.Handle != "example" {
		t.Errorf("Handle = %q, want %q", entry.Handle, "example")
	}
}

func TestErrAbortingScopeWipe(t *testing.T) {
	if ErrAbortingScopeWipe == nil {
		t.Error("ErrAbortingScopeWipe should not be nil")
	}
	if ErrAbortingScopeWipe.Error() == "" {
		t.Error("ErrAbortingScopeWipe.Error() should not be empty")
	}
}

func TestErrInvalidDatabaseName(t *testing.T) {
	if ErrInvalidDatabaseName == nil {
		t.Error("ErrInvalidDatabaseName should not be nil")
	}
	if ErrInvalidDatabaseName.Error() == "" {
		t.Error("ErrInvalidDatabaseName.Error() should not be empty")
	}
}

func TestDefaultPoolConfig(t *testing.T) {
	cfg := DefaultPoolConfig()

	if cfg.MaxOpenConns != 25 {
		t.Errorf("MaxOpenConns = %d, want 25", cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns != 5 {
		t.Errorf("MaxIdleConns = %d, want 5", cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime != 5*time.Minute {
		t.Errorf("ConnMaxLifetime = %v, want 5m", cfg.ConnMaxLifetime)
	}
	if cfg.ConnMaxIdleTime != 5*time.Minute {
		t.Errorf("ConnMaxIdleTime = %v, want 5m", cfg.ConnMaxIdleTime)
	}
}

func TestPoolConfigCustomValues(t *testing.T) {
	cfg := PoolConfig{
		MaxOpenConns:    50,
		MaxIdleConns:    10,
		ConnMaxLifetime: 10 * time.Minute,
		ConnMaxIdleTime: 2 * time.Minute,
	}

	if cfg.MaxOpenConns != 50 {
		t.Errorf("MaxOpenConns = %d, want 50", cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns != 10 {
		t.Errorf("MaxIdleConns = %d, want 10", cfg.MaxIdleConns)
	}
}
