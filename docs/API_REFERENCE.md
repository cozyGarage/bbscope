# bbscope API Reference

This document provides a comprehensive reference for bbscope's internal APIs and data structures. It's intended for developers who want to understand or extend the codebase.

---

## Table of Contents

- [CLI Commands](#cli-commands)
- [Platform Poller Interface](#platform-poller-interface)
- [Storage API](#storage-api)
- [AI Normalizer API](#ai-normalizer-api)
- [Data Types](#data-types)
- [Utility Functions](#utility-functions)

---

## CLI Commands

### Root Command

```
bbscope [flags]
```

**Global Flags:**

| Flag | Type | Description |
|------|------|-------------|
| `--config` | string | Config file path (default: `~/.bbscope.yaml`) |
| `--proxy` | string | HTTP proxy URL (e.g., `http://127.0.0.1:8080`) |
| `--loglevel` | string | Log level: debug, info, warn, error, fatal |
| `--debug-http` | bool | Enable HTTP request/response debugging |

---

### Poll Commands

#### `bbscope poll [flags]`

Poll all configured platforms.

**Flags:**

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--category` | | string | `"all"` | Filter by category (wildcard, url, cidr, etc.) |
| `--db` | | bool | `false` | Save to database and show changes |
| `--ai` | | bool | `false` | Enable AI normalization |
| `--concurrency` | | int | `5` | Concurrent fetches per platform |
| `--since` | | string | `""` | Show changes since RFC3339 timestamp |
| `--oos` | | bool | `false` | Include out-of-scope targets |
| `--output` | `-o` | string | `"tu"` | Output flags: t=target, d=description, c=category, u=URL |
| `--delimiter` | `-d` | string | `" "` | Field delimiter |
| `--bbp-only` | `-b` | bool | `false` | Only programs with bounties |
| `--private-only` | `-p` | bool | `false` | Only private programs |

#### Platform-Specific Commands

```bash
bbscope poll h1 [flags]        # HackerOne
bbscope poll bc [flags]        # Bugcrowd
bbscope poll it [flags]        # Intigriti
bbscope poll ywh [flags]       # YesWeHack
bbscope poll immunefi [flags]  # Immunefi
```

**HackerOne Specific:**
| Flag | Type | Description |
|------|------|-------------|
| `--user` | string | HackerOne username |
| `--token` | string | HackerOne API token |

**Bugcrowd Specific:**
| Flag | Type | Description |
|------|------|-------------|
| `--token` | string | Session cookie (`_crowdcontrol_session_key`) |
| `--email` | string | Bugcrowd email |
| `--password` | string | Bugcrowd password |
| `--otp-secret` | string | 2FA TOTP secret |

---

### Database Commands

#### `bbscope db stats`

Display database statistics.

**Output Example:**
```
PLATFORM   PROGRAMS   IN-SCOPE   OUT-OF-SCOPE
h1              150       2340            456
bc               89       1234            234
TOTAL           239       3574            690
```

#### `bbscope db print [flags]`

Print scope data from database.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--platform` | string | `""` | Filter by platform |
| `--program` | string | `""` | Filter by program URL |
| `--oos` | bool | `false` | Include out-of-scope |
| `--since` | string | `""` | Filter by time (RFC3339) |
| `--format` | string | `"txt"` | Output format: txt, json, csv |
| `--include-ignored` | bool | `false` | Include ignored programs |

#### `bbscope db changes [flags]`

Show recent scope changes.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--limit` | int | `50` | Number of changes to show |

#### `bbscope db get <subcommand> [flags]`

Extract specific target types.

**Subcommands:**
- `wildcards` - Get wildcard domains
- `domains` - Get all domains
- `urls` - Get HTTP/HTTPS URLs
- `cidrs` - Get CIDR ranges
- `ips` - Get IP addresses

**Common Flags:**
| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--aggressive` | `-a` | bool | `false` | Extract root domains from URLs |
| `--platform` | | string | `"all"` | Filter by platform |
| `--output` | `-o` | string | `"t"` | Output flags |
| `--delimiter` | `-d` | string | `" "` | Field delimiter |

#### `bbscope db ignore --program-url <url>`

Mark a program as ignored.

#### `bbscope db unignore --program-url <url>`

Remove ignore status from a program.

#### `bbscope db add [flags]`

Add a custom target.

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--target` | `-t` | string | | Target to add (comma-separated) |
| `--category` | `-c` | string | `"wildcard"` | Target category |
| `--program-url` | `-u` | string | `"custom"` | Associated program URL |

---

## Platform Poller Interface

```go
package platforms

import (
    "context"
    "github.com/cozyGarage/bbscope/v2/pkg/scope"
)

// PollOptions configures polling behavior
type PollOptions struct {
    BountyOnly  bool   // Only programs with bounties
    PrivateOnly bool   // Only private programs
    Categories  string // Comma-separated category filter
}

// AuthConfig carries authentication credentials
type AuthConfig struct {
    Username  string
    Email     string
    Password  string
    Token     string
    OtpSecret string
    Proxy     string
}

// PlatformPoller defines the interface for platform implementations
type PlatformPoller interface {
    // Name returns the platform identifier (e.g., "h1", "bc")
    Name() string
    
    // Authenticate configures the poller with credentials
    // Returns nil for platforms that don't require auth
    Authenticate(ctx context.Context, cfg AuthConfig) error
    
    // ListProgramHandles returns all program handles matching options
    ListProgramHandles(ctx context.Context, opts PollOptions) ([]string, error)
    
    // FetchProgramScope retrieves scope for a specific program
    FetchProgramScope(ctx context.Context, handle string, opts PollOptions) (scope.ProgramData, error)
}
```

### Implementing a New Platform

```go
package newplatform

import (
    "context"
    "github.com/cozyGarage/bbscope/v2/pkg/platforms"
    "github.com/cozyGarage/bbscope/v2/pkg/scope"
)

type Poller struct {
    apiKey string
    client *http.Client
}

func NewPoller(apiKey string) *Poller {
    return &Poller{
        apiKey: apiKey,
        client: &http.Client{Timeout: 30 * time.Second},
    }
}

func (p *Poller) Name() string {
    return "newplatform"
}

func (p *Poller) Authenticate(ctx context.Context, cfg platforms.AuthConfig) error {
    if cfg.Token != "" {
        p.apiKey = cfg.Token
    }
    return nil
}

func (p *Poller) ListProgramHandles(ctx context.Context, opts platforms.PollOptions) ([]string, error) {
    // API call to list programs
    // Apply opts.BountyOnly and opts.PrivateOnly filters
    return []string{"program1", "program2"}, nil
}

func (p *Poller) FetchProgramScope(ctx context.Context, handle string, opts platforms.PollOptions) (scope.ProgramData, error) {
    // API call to fetch scope
    return scope.ProgramData{
        Url: "https://newplatform.com/" + handle,
        InScope: []scope.ScopeElement{
            {Target: "*.example.com", Category: "wildcard"},
        },
    }, nil
}
```

---

## Storage API

### Opening Database Connection

```go
import "github.com/cozyGarage/bbscope/v2/pkg/storage"

// Open connects to PostgreSQL and runs migrations
db, err := storage.Open("postgres://user:pass@localhost/bbscope?sslmode=disable")
if err != nil {
    log.Fatal(err)
}
defer db.Close()
```

### Core Methods

```go
// UpsertProgramEntries inserts or updates scope entries for a program
// Returns a list of changes (added, updated, removed)
func (d *DB) UpsertProgramEntries(
    ctx context.Context,
    programURL, platform, handle string,
    entries []UpsertEntry,
) ([]Change, error)

// ListEntries queries entries with filtering options
func (d *DB) ListEntries(ctx context.Context, opts ListOptions) ([]Entry, error)

// ListRecentChanges returns recent scope changes
func (d *DB) ListRecentChanges(ctx context.Context, limit int) ([]Change, error)

// GetStats returns per-platform statistics
func (d *DB) GetStats(ctx context.Context) ([]PlatformStats, error)

// GetActiveProgramCount returns count of non-disabled programs for a platform
func (d *DB) GetActiveProgramCount(ctx context.Context, platform string) (int64, error)

// SetProgramIgnoredStatus marks programs as ignored/unignored
func (d *DB) SetProgramIgnoredStatus(ctx context.Context, urlPattern string, ignored bool) error

// SearchEntries performs text search across scopes
func (d *DB) SearchEntries(ctx context.Context, query string) ([]Entry, error)
```

### ListOptions

```go
type ListOptions struct {
    Platform       string    // Filter by platform
    ProgramFilter  string    // Filter by program URL/handle
    Since          time.Time // Only entries seen after this time
    IncludeOOS     bool      // Include out-of-scope
    IncludeIgnored bool      // Include ignored programs
}
```

---

## AI Normalizer API

### Configuration

```go
import "github.com/cozyGarage/bbscope/v2/pkg/ai"

cfg := ai.Config{
    Provider:       "openai",
    APIKey:         "sk-...",
    Model:          "gpt-4o-mini",
    Endpoint:       "", // Empty for default OpenAI endpoint
    MaxBatch:       25,
    MaxConcurrency: 10,
    Proxy:          "", // Optional proxy URL
}

normalizer, err := ai.NewNormalizer(cfg)
if err != nil {
    log.Fatal(err)
}
```

### Normalizer Interface

```go
type Normalizer interface {
    // NormalizeTargets processes raw scope items and returns normalized versions
    // Each item may be expanded into multiple variants
    NormalizeTargets(
        ctx context.Context,
        info ProgramInfo,
        items []storage.TargetItem,
    ) ([]storage.TargetItem, error)
}

// ProgramInfo provides context for normalization
type ProgramInfo struct {
    ProgramURL string
    Platform   string
    Handle     string
}
```

### Usage Example

```go
items := []storage.TargetItem{
    {URI: "example.(com|org)", Category: "url", InScope: true},
    {URI: "  *.example.com  ", Category: "wildcard", InScope: true},
}

info := ai.ProgramInfo{
    ProgramURL: "https://hackerone.com/example",
    Platform:   "h1",
    Handle:     "example",
}

normalized, err := normalizer.NormalizeTargets(ctx, info, items)
// normalized contains expanded/cleaned items with Variants populated
```

---

## Data Types

### Scope Types

```go
package scope

// ScopeElement represents a single scope target
type ScopeElement struct {
    Target      string // The target identifier
    Description string // Additional context/notes
    Category    string // Type: wildcard, url, cidr, etc.
}

// ProgramData contains all scope information for a program
type ProgramData struct {
    Url        string          // Program URL
    InScope    []ScopeElement  // In-scope targets
    OutOfScope []ScopeElement  // Out-of-scope targets
}
```

### Storage Types

```go
package storage

// Entry represents a stored scope entry
type Entry struct {
    ProgramURL           string
    Platform             string
    Handle               string
    TargetNormalized     string
    TargetRaw            string
    BaseTargetNormalized string
    BaseTargetRaw        string
    Category             string
    Description          string
    InScope              bool
    IsBBP                bool
    IsHistorical         bool
    Source               string
}

// Change represents a scope modification event
type Change struct {
    OccurredAt         time.Time
    ProgramURL         string
    Platform           string
    Handle             string
    TargetNormalized   string
    TargetRaw          string
    TargetAINormalized string
    Category           string
    InScope            bool
    IsBBP              bool
    ChangeType         string // "added", "updated", "removed"
}

// UpsertEntry represents data for upserting
type UpsertEntry struct {
    ProgramURL       string
    Platform         string
    Handle           string
    TargetNormalized string
    TargetRaw        string
    Category         string
    Description      string
    InScope          bool
    IsBBP            bool
    Variants         []EntryVariant
}

// EntryVariant represents an AI-generated variant
type EntryVariant struct {
    AINormalized string
    HasInScope   bool
    InScope      bool
    HasCategory  bool
    Category     string
}

// TargetItem is used for AI normalization input/output
type TargetItem struct {
    URI         string
    Category    string
    Description string
    InScope     bool
    IsBBP       bool
    Variants    []TargetVariant
}

// TargetVariant represents an expansion of a target
type TargetVariant struct {
    Value       string
    HasInScope  bool
    InScope     bool
    HasCategory bool
    Category    string
}
```

---

## Utility Functions

### Category Functions (pkg/scope)

```go
// NormalizeCategory maps platform-specific categories to unified names
func NormalizeCategory(category string) string

// UnifiedCategories returns all valid unified category names
func UnifiedCategories() []string

// IsUnifiedCategory checks if a category is valid
func IsUnifiedCategory(category string) bool

// GetAllStringsForCategories expands category filter to platform strings
// Returns nil for "all" (no filter)
func GetAllStringsForCategories(input string) []string
```

### Normalization Functions (pkg/storage)

```go
// NormalizeTarget canonicalizes a target string
// - Lowercases
// - Removes trailing slashes/dots
// - Normalizes URL schemes and ports
func NormalizeTarget(s string) string

// NormalizeProgramURL ensures consistent program URL format
func NormalizeProgramURL(s string) string

// AggressiveTransform extracts root domains from URLs
func AggressiveTransform(target string) string
```

### IP/CIDR Utilities (internal/utils)

```go
// IsCIDR checks if a string is a valid CIDR (x.x.x.x/xx)
func IsCIDR(cidr string) bool

// IsIP checks if a string is a valid IPv4 or IPv6 address
func IsIP(ip string) bool

// IsIPRange checks if a string is a valid IP range (x.x.x.x-y.y.y.y)
func IsIPRange(ipRange string) bool
```

### OTP Functions (pkg/otp)

```go
// GenerateTOTP generates a TOTP code for the given secret at time t
// Supports multiple secret formats:
// - Raw base32
// - "<digits> <base32>"
// - otpauth:// URI
func GenerateTOTP(secret string, t time.Time) (string, error)
```

### HTTP Functions (pkg/whttp)

```go
// SendHTTPRequest performs an HTTP request with retry logic
func SendHTTPRequest(wReq *WHTTPReq, customClient *retryablehttp.Client) (*WHTTPRes, error)

// SetupProxy configures the global HTTP client with a proxy
func SetupProxy(proxyURL string) error

// GetDefaultClient returns the shared retryable HTTP client
func GetDefaultClient() *retryablehttp.Client
```

### Wildcard Functions (pkg/wildcards)

```go
// Collect extracts wildcard domains from entries
// Returns map[domain]map[programURL]struct{}
func Collect(entries []storage.Entry, opts Options) map[string]map[string]struct{}

// CollectSorted returns wildcards sorted alphabetically
func CollectSorted(entries []storage.Entry, opts Options) []Result

// NormalizeForSubdomainTools cleans wildcards for use with subdomain tools
func NormalizeForSubdomainTools(target string) string

// WildcardHasPath checks if a wildcard includes a path component
func WildcardHasPath(target string) bool

// BlacklistedSuffixes contains domains to exclude (shared hosting, etc.)
var BlacklistedSuffixes []string
```

---

## Configuration (Viper Keys)

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `db_url` | string | `""` | PostgreSQL connection URL |
| `hackerone.username` | string | `""` | HackerOne username |
| `hackerone.token` | string | `""` | HackerOne API token |
| `bugcrowd.email` | string | `""` | Bugcrowd email |
| `bugcrowd.password` | string | `""` | Bugcrowd password |
| `bugcrowd.otpsecret` | string | `""` | Bugcrowd 2FA secret |
| `intigriti.token` | string | `""` | Intigriti API token |
| `yeswehack.email` | string | `""` | YesWeHack email |
| `yeswehack.password` | string | `""` | YesWeHack password |
| `yeswehack.otpsecret` | string | `""` | YesWeHack 2FA secret |
| `ai.provider` | string | `"openai"` | AI provider name |
| `ai.api_key` | string | `""` | AI API key |
| `ai.model` | string | `"gpt-4o-mini"` | AI model name |
| `ai.endpoint` | string | `""` | Custom API endpoint |
| `ai.max_batch` | int | `25` | Targets per AI request |
| `ai.max_concurrency` | int | `3` | Concurrent AI requests |

---

## Error Types

```go
package storage

// ErrAbortingScopeWipe is returned when an update would remove all targets
var ErrAbortingScopeWipe = errors.New("aborting update to prevent wiping out all targets for a program")
```

---

## Database Schema

```sql
-- Programs
CREATE TABLE programs (
    id            SERIAL PRIMARY KEY,
    platform      TEXT NOT NULL,
    handle        TEXT NOT NULL,
    url           TEXT NOT NULL UNIQUE,
    first_seen_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    strict        INTEGER NOT NULL DEFAULT 0,
    disabled      INTEGER NOT NULL DEFAULT 0,
    is_ignored    INTEGER NOT NULL DEFAULT 0
);

-- Raw targets
CREATE TABLE targets_raw (
    id            SERIAL PRIMARY KEY,
    program_id    INTEGER NOT NULL REFERENCES programs(id),
    target        TEXT NOT NULL,
    category      TEXT NOT NULL,
    description   TEXT,
    in_scope      INTEGER NOT NULL,
    is_bbp        INTEGER NOT NULL DEFAULT 0,
    first_seen_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(program_id, category, target)
);

-- AI-enhanced variants
CREATE TABLE targets_ai_enhanced (
    id                   SERIAL PRIMARY KEY,
    target_id            INTEGER NOT NULL REFERENCES targets_raw(id) ON DELETE CASCADE,
    target_ai_normalized TEXT NOT NULL,
    category             TEXT,
    in_scope             INTEGER,
    first_seen_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at         TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(target_id, target_ai_normalized)
);

-- Change audit log
CREATE TABLE scope_changes (
    id                   SERIAL PRIMARY KEY,
    occurred_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    program_url          TEXT NOT NULL,
    platform             TEXT NOT NULL,
    handle               TEXT NOT NULL,
    target_normalized    TEXT NOT NULL,
    target_raw           TEXT NOT NULL DEFAULT '',
    target_ai_normalized TEXT NOT NULL DEFAULT '',
    category             TEXT NOT NULL,
    in_scope             INTEGER NOT NULL,
    is_bbp               INTEGER NOT NULL DEFAULT 0,
    change_type          TEXT NOT NULL CHECK (change_type IN ('added','updated','removed'))
);
```
