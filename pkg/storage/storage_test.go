package storage

import (
	"strings"
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
		ProgramURL:       "https://hackerone.com/example",
		Handle:           "example",
		TargetNormalized: "*.example.com",
		Category:         "wildcard",
		InScope:          true,
		IsBBP:            true,
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

func TestErrAbortingPartialSync(t *testing.T) {
	if ErrAbortingPartialSync == nil {
		t.Error("ErrAbortingPartialSync should not be nil")
	}
	if ErrAbortingPartialSync.Error() == "" {
		t.Error("ErrAbortingPartialSync.Error() should not be empty")
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

func TestShouldAbortPartialSync(t *testing.T) {
	tests := []struct {
		name        string
		activeCount int
		removeCount int
		want        bool
	}{
		{"no removals", 10, 0, false},
		{"more than half", 10, 6, true},
		{"larger platform over ratio", 100, 51, true},
		{"larger platform under ratio", 100, 49, false},

		// Exactly half is aborted: that much churn in one poll is already well
		// outside normal behavior and is the signature of a truncated response.
		{"exactly half", 10, 5, true},
		{"small platform at ratio", 4, 2, true},

		// The ratio applies at every scale. These previously returned false
		// because platforms with fewer than ten active programs were exempt,
		// which let one bad poll disable every program a small platform had.
		{"small platform wiped entirely", 9, 9, true},
		{"small platform over ratio", 4, 3, true},

		// A two-program platform hits the half mark on one genuine removal, so
		// the ratio is not applied there; a full wipe is still refused.
		{"two programs, one removed", 2, 1, false},
		{"two programs, both removed", 2, 2, true},
		{"three programs, one removed", 3, 1, false},
		{"three programs, two removed", 3, 2, true},

		// A one-program platform wiped by an empty listing is the same full-wipe
		// signature as a larger platform returning nothing.
		{"single program removed", 1, 1, true},

		// Degenerate inputs.
		{"no active programs", 0, 0, false},
		{"negative removals", 10, -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldAbortPartialSync(tt.activeCount, tt.removeCount)
			if got != tt.want {
				t.Fatalf("shouldAbortPartialSync(%d, %d) = %v, want %v", tt.activeCount, tt.removeCount, got, tt.want)
			}
		})
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

func TestEscapeLikePattern(t *testing.T) {
	got := escapeLikePattern(`100%_done\now`)
	want := `100\%\_done\\now`
	if got != want {
		t.Fatalf("escapeLikePattern = %q, want %q", got, want)
	}
}

// TestRedactConnectionStringNeverLeaksPassword covers both connection-string
// forms pgx accepts. url.Parse succeeds on a keyword/value DSN without
// populating User, so that form used to be returned with the password intact.
func TestRedactConnectionStringNeverLeaksPassword(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "url form",
			in:   "postgres://bbscope:s3cret@127.0.0.1:5432/bbscope?sslmode=disable",
			want: "postgres://bbscope:%2A%2A%2A%2A@127.0.0.1:5432/bbscope?sslmode=disable",
		},
		{
			name: "keyword value dsn",
			in:   "host=db user=u password=s3cret dbname=bbscope",
			want: "host=db user=u password=**** dbname=bbscope",
		},
		{
			name: "quoted password containing spaces",
			in:   "host=db password='s3 cret pass' dbname=bbscope",
			want: "host=db password=**** dbname=bbscope",
		},
		{
			name: "sslpassword and sslkey are secrets too",
			in:   "host=db sslpassword=s3cret sslkey=/k/e/y",
			want: "host=db sslpassword=**** sslkey=****",
		},
		{
			name: "url with no password is untouched",
			in:   "postgres://bbscope@127.0.0.1:5432/bbscope",
			want: "postgres://bbscope@127.0.0.1:5432/bbscope",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := redactConnectionString(tc.in)
			if got != tc.want {
				t.Errorf("redactConnectionString(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if strings.Contains(got, "s3cret") || strings.Contains(got, "s3 cret") {
				t.Errorf("redactConnectionString(%q) leaked the password: %q", tc.in, got)
			}
		})
	}
}

func TestUniqueIdentityStats(t *testing.T) {
	entries := []UpsertEntry{
		{TargetRaw: "https://example.com", Category: "url"},
		{TargetRaw: "https://example.com/", Category: "URL"},
		{TargetRaw: "https://other.example", Category: "url"},
		{TargetRaw: "   ", Category: "url"},
	}
	unique, dups := uniqueIdentityStats(entries)
	if unique != 2 {
		t.Fatalf("unique = %d, want 2", unique)
	}
	if dups != 1 {
		t.Fatalf("duplicates = %d, want 1", dups)
	}
}

func TestSplitPlatformListExpandsAliases(t *testing.T) {
	got := splitPlatformList("bugcrowd, H1")
	has := func(s string) bool {
		for _, g := range got {
			if g == s {
				return true
			}
		}
		return false
	}
	if !has("bc") || !has("bugcrowd") || !has("h1") || !has("hackerone") {
		t.Fatalf("splitPlatformList = %v, want bc/bugcrowd and h1/hackerone", got)
	}
	if splitPlatformList("all") != nil && len(splitPlatformList("all")) != 0 {
		t.Fatalf("all should be dropped, got %v", splitPlatformList("all"))
	}
}
