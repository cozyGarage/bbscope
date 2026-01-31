# bbscope Improvement Suggestions

This document outlines potential improvements for bbscope, organized by category and priority. Each suggestion includes rationale, implementation considerations, and estimated effort.

---

## ✅ Recently Implemented Improvements

The following improvements have been implemented:

| # | Improvement | Status | Location |
|---|-------------|--------|----------|
| 1 | Fix insecure TLS configuration | ✅ Done | `pkg/platforms/bugcrowd/bugcrowd.go` |
| 2 | Reduce retry limits (99999 → 10) | ✅ Done | `pkg/whttp/whttp.go` |
| 3 | Redact sensitive headers in debug | ✅ Done | `pkg/whttp/whttp.go` |
| 4 | Add non-root Docker user | ✅ Done | `Dockerfile` |
| 5 | Config file permissions warning | ✅ Done | `cmd/root.go` |
| 6 | Add version command | ✅ Done | `cmd/version.go` |
| 7 | Validate database name in SQL | ✅ Done | `pkg/storage/storage.go` |
| 8 | Create Makefile for builds | ✅ Done | `Makefile` |
| 9 | Add input validation package | ✅ Done | `pkg/validate/validate.go` |
| 10 | Add graceful shutdown | ✅ Done | `main.go`, `cmd/root.go` |
| 11 | Redact DB password in error logs | ✅ Done | `pkg/storage/storage.go` |

---

## Table of Contents

- [Recently Implemented](#-recently-implemented-improvements)
- [Remaining High Priority](#remaining-high-priority-improvements)
- [Security Improvements](#security-improvements)
- [Architecture Improvements](#architecture-improvements)
- [Feature Additions](#feature-additions)
- [Developer Experience](#developer-experience)
- [Performance Optimizations](#performance-optimizations)
- [Testing Improvements](#testing-improvements)
- [Documentation Improvements](#documentation-improvements)

---

## Remaining High Priority Improvements

### ~~1. Fix Insecure TLS Configuration~~ ✅ IMPLEMENTED

**Status:** ✅ Implemented

Changes made:
- Updated to use TLS 1.2 minimum
- Removed forced weak cipher suites  
- `InsecureSkipVerify` only enabled when proxy is explicitly configured
- Location: `pkg/platforms/bugcrowd/bugcrowd.go`

---

### ~~2. Reduce Retry Limits~~ ✅ IMPLEMENTED

**Status:** ✅ Implemented

Changes made:
- Reduced from 99999 to 10 retries
- Added `RetryWaitMin = 1s` and `RetryWaitMax = 30s`
- Location: `pkg/whttp/whttp.go`

---

### ~~3. Redact Sensitive Data in Debug Output~~ ✅ IMPLEMENTED

**Status:** ✅ Implemented

Changes made:
- Added `isSensitiveHeader()` function
- Headers like Authorization, Cookie, X-Csrf-Token now show `[REDACTED]` in debug output
- Location: `pkg/whttp/whttp.go`

---

### 4. Add Non-Root User to Dockerfile

**Current Issue:**
Container runs as root user.

**Location:** `Dockerfile`

**Recommended Changes:**
```dockerfile
FROM alpine:3.19

# Create non-root user
RUN adduser -D -g '' -u 1000 bbscope

WORKDIR /home/bbscope

COPY --from=builder /app/bbscope .

USER bbscope

ENTRYPOINT ["./bbscope"]
```

**Effort:** Low (30 minutes)
**Impact:** Medium (Security)

---

## Security Improvements

### 5. OS Keychain Integration

**Description:**
Store credentials in OS-native secure storage instead of plaintext config file.

**Implementation:**
```go
// Use keyring package
import "github.com/zalando/go-keyring"

func getCredential(service, key string) (string, error) {
    // Try keychain first
    if val, err := keyring.Get(service, key); err == nil {
        return val, nil
    }
    // Fall back to config file
    return viper.GetString(key), nil
}
```

**Supported Platforms:**
- macOS: Keychain
- Windows: Credential Manager  
- Linux: Secret Service (GNOME Keyring, KWallet)

**Effort:** Medium (4-6 hours)
**Impact:** High (Security)

---

### 6. Validate Database Name in Auto-Creation

**Current Issue:**
Database name from URL is used directly in SQL.

**Location:** `pkg/storage/storage.go`

**Recommended Changes:**
```go
func validateDatabaseName(name string) error {
    // Only allow alphanumeric and underscore
    matched, _ := regexp.MatchString(`^[a-zA-Z_][a-zA-Z0-9_]*$`, name)
    if !matched || len(name) > 63 {
        return fmt.Errorf("invalid database name: %s", name)
    }
    return nil
}
```

**Effort:** Low (1 hour)
**Impact:** Medium (Security)

---

### ~~7. Config File Permissions Warning~~ ✅ IMPLEMENTED

**Status:** ✅ Implemented

Changes made:
- Added `checkConfigPermissions()` function in `cmd/root.go`
- Warns user if config file has world-readable permissions (Unix)
- Config file now created with chmod 0600 by default
- Location: `cmd/root.go`

---

## Architecture Improvements

### 8. Context Propagation Throughout

**Current Issue:**
Some code paths don't properly propagate context for cancellation.

**Recommendation:**
Audit all goroutines and HTTP requests to ensure they respect context cancellation:

```go
// Ensure all HTTP requests use context
req = req.WithContext(ctx)

// Check context in loops
select {
case <-ctx.Done():
    return ctx.Err()
default:
    // continue processing
}
```

**Effort:** Medium (4-6 hours)
**Impact:** Medium (Reliability)

---

### 9. Interface-Based HTTP Client

**Description:**
Make HTTP client injectable for better testing.

**Current Issue:**
Platform pollers use global HTTP client, making testing difficult.

**Recommended Changes:**
```go
type HTTPClient interface {
    Do(req *http.Request) (*http.Response, error)
}

type Poller struct {
    client HTTPClient
}

func NewPoller(client HTTPClient) *Poller {
    if client == nil {
        client = http.DefaultClient
    }
    return &Poller{client: client}
}
```

**Effort:** Medium (4-8 hours per platform)
**Impact:** High (Testability)

---

### 10. Event-Based Change Notifications

**Description:**
Implement a pub/sub system for scope changes.

**Use Cases:**
- Webhooks on scope changes
- Real-time notifications
- Integration with other tools

**Implementation:**
```go
type ChangeEvent struct {
    Type      string // "added", "removed", "updated"
    Change    Change
    Timestamp time.Time
}

type ChangeNotifier interface {
    Notify(event ChangeEvent) error
}

type WebhookNotifier struct {
    URL string
}

func (w *WebhookNotifier) Notify(event ChangeEvent) error {
    // POST to webhook URL
}
```

**Effort:** High (1-2 days)
**Impact:** High (Feature value)

---

### 11. Plugin System for Platforms

**Description:**
Allow external platform implementations without modifying core code.

**Implementation:**
```go
// Use Go plugins or hashicorp/go-plugin
type PlatformPlugin interface {
    PlatformPoller
    Version() string
}

// Load plugins from ~/.bbscope/plugins/
func LoadPlugins() ([]PlatformPoller, error)
```

**Effort:** High (2-3 days)
**Impact:** Medium (Extensibility)

---

## Feature Additions

### 12. Export to Multiple Formats

**Description:**
Add more export formats beyond txt/json/csv.

**New Formats:**
- YAML
- XML
- Markdown table
- Nuclei template format
- Amass scope file format

**Effort:** Low-Medium (1-2 hours per format)
**Impact:** Medium (Usability)

---

### 13. Scheduled Polling (Daemon Mode)

**Description:**
Run bbscope as a background service with scheduled polls.

**Implementation:**
```bash
bbscope daemon --interval 1h --db --ai
```

```go
func runDaemon(interval time.Duration) {
    ticker := time.NewTicker(interval)
    for range ticker.C {
        pollAllPlatforms()
    }
}
```

**Effort:** Medium (4-6 hours)
**Impact:** High (Automation)

---

### 14. Notification Integrations

**Description:**
Send notifications on scope changes.

**Integrations:**
- Slack
- Discord
- Email
- Telegram
- Custom webhook

**Implementation:**
```yaml
notifications:
  slack:
    webhook: "https://hooks.slack.com/..."
    events: ["added", "removed"]
  email:
    smtp: "smtp.gmail.com:587"
    recipients: ["user@example.com"]
```

**Effort:** Medium (1-2 hours per integration)
**Impact:** High (Usability)

---

### 15. Program Metadata Tracking

**Description:**
Track additional program information beyond scope.

**Metadata:**
- Program state (active, paused, closed)
- Reward ranges
- Response times
- Program type (private/public)
- Tags/categories

**Database Changes:**
```sql
ALTER TABLE programs ADD COLUMN
    metadata JSONB DEFAULT '{}';
```

**Effort:** Medium (4-6 hours)
**Impact:** Medium (Feature value)

---

### 16. Diff/Compare Command

**Description:**
Compare scope between two points in time.

**Implementation:**
```bash
bbscope db diff --from "2024-01-01" --to "2024-02-01"
```

**Effort:** Medium (4-6 hours)
**Impact:** Medium (Usability)

---

### 17. Scope Import/Export

**Description:**
Import scope from files or export for backup.

**Implementation:**
```bash
bbscope db export --format json > backup.json
bbscope db import --file backup.json
```

**Effort:** Medium (4-6 hours)
**Impact:** Medium (Data portability)

---

### 18. REST API Mode

**Description:**
Run bbscope as an HTTP server for API access.

**Implementation:**
```bash
bbscope serve --port 8080
```

**Endpoints:**
- `GET /api/programs` - List programs
- `GET /api/targets` - List targets
- `POST /api/poll` - Trigger poll
- `GET /api/changes` - Get changes

**Effort:** High (1-2 days)
**Impact:** High (Integration capabilities)

---

## Developer Experience

### 19. Makefile for Common Tasks

**Description:**
Add Makefile for standardized development workflows.

```makefile
.PHONY: build test lint clean

build:
	go build -o bbscope .

test:
	go test -v ./...

lint:
	golangci-lint run

clean:
	rm -f bbscope coverage.out

docker:
	docker build -t bbscope:dev .

release:
	goreleaser release --snapshot --clean
```

**Effort:** Low (1-2 hours)
**Impact:** Medium (Developer productivity)

---

### 20. GoReleaser Integration

**Description:**
Automate release builds with GoReleaser.

**.goreleaser.yaml:**
```yaml
builds:
  - env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64

archives:
  - format: tar.gz
    name_template: "{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}"

dockers:
  - image_templates:
      - "ghcr.io/sw33tlie/bbscope:{{ .Tag }}"
      - "ghcr.io/sw33tlie/bbscope:latest"

changelog:
  sort: asc
  filters:
    exclude:
      - '^docs:'
      - '^test:'
```

**Effort:** Low (2-3 hours)
**Impact:** High (Release automation)

---

### ~~21. Version Command~~ ✅ IMPLEMENTED

**Status:** ✅ Implemented

Changes made:
- Created new `cmd/version.go` file
- Prints version, commit, build date, Go version, and OS/Arch
- Version variables can be set via ldflags at build time:
  ```bash
  go build -ldflags "-X main.Version=1.0.0 -X main.Commit=abc123 -X main.BuildDate=2024-01-01"
  ```
- Location: `cmd/version.go`, `cmd/root.go`

---

## Performance Optimizations

### 22. Connection Pooling Configuration

**Description:**
Make database connection pool configurable.

**Implementation:**
```go
func Open(connectionString string, opts ...Option) (*DB, error) {
    db, _ := sql.Open("postgres", connectionString)
    db.SetMaxOpenConns(25)
    db.SetMaxIdleConns(5)
    db.SetConnMaxLifetime(5 * time.Minute)
    // ...
}
```

**Effort:** Low (1-2 hours)
**Impact:** Medium (Performance at scale)

---

### 23. Batch API Requests Where Possible

**Description:**
Some platforms may support batch endpoints that aren't currently used.

**Effort:** Medium (Varies by platform)
**Impact:** Medium (Performance)

---

### 24. Response Caching

**Description:**
Cache API responses for short periods to reduce redundant requests.

**Implementation:**
```go
type CachedClient struct {
    client HTTPClient
    cache  *cache.Cache
    ttl    time.Duration
}
```

**Effort:** Medium (4-6 hours)
**Impact:** Medium (Performance, API rate limits)

---

## Testing Improvements

### 25. Integration Test Suite

**Description:**
Add comprehensive integration tests using test containers.

**Implementation:**
```go
func TestIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }
    
    // Start PostgreSQL container
    ctx := context.Background()
    postgres, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: testcontainers.ContainerRequest{
            Image:        "postgres:16-alpine",
            ExposedPorts: []string{"5432/tcp"},
            Env: map[string]string{
                "POSTGRES_DB":       "test",
                "POSTGRES_USER":     "test",
                "POSTGRES_PASSWORD": "test",
            },
        },
        Started: true,
    })
    // ... run tests
}
```

**Effort:** Medium (4-8 hours)
**Impact:** High (Code quality)

---

### 26. Mock Platform Server

**Description:**
Create mock servers for each platform for testing.

**Implementation:**
```go
func NewMockHackerOneServer() *httptest.Server {
    mux := http.NewServeMux()
    mux.HandleFunc("/v1/hackers/programs", handlePrograms)
    mux.HandleFunc("/v1/hackers/programs/", handleProgramScope)
    return httptest.NewServer(mux)
}
```

**Effort:** High (1-2 days)
**Impact:** High (Testability)

---

### 27. Fuzzing Tests

**Description:**
Add fuzz testing for parsing functions.

```go
func FuzzNormalizeTarget(f *testing.F) {
    f.Add("*.example.com")
    f.Add("https://example.com/path")
    f.Add("")
    f.Fuzz(func(t *testing.T, input string) {
        NormalizeTarget(input)
        // Should not panic
    })
}
```

**Effort:** Low (2-3 hours)
**Impact:** Medium (Robustness)

---

## Documentation Improvements

### 28. Interactive Examples

**Description:**
Add executable examples using asciinema recordings.

**Effort:** Low (2-3 hours)
**Impact:** Medium (User onboarding)

---

### 29. API Documentation Generation

**Description:**
Generate API docs from code comments using godoc or pkgsite.

**Effort:** Low (1-2 hours)
**Impact:** Medium (Developer experience)

---

### 30. Changelog Automation

**Description:**
Automatically generate changelog from commit messages.

**Tools:**
- conventional-changelog
- git-cliff
- github-changelog-generator

**Effort:** Low (1-2 hours)
**Impact:** Medium (Release management)

---

## Implementation Roadmap

### Phase 1: Quick Wins (1-2 weeks)
- [x] Fix TLS configuration ✅
- [x] Reduce retry limits ✅
- [x] Redact debug headers ✅
- [x] Add non-root Docker user ✅
- [x] Add version command ✅
- [x] Create Makefile ✅

### Phase 2: Security Hardening (2-3 weeks)
- [x] Validate database name ✅
- [x] Config permissions warning ✅
- [x] Redact DB passwords in logs ✅
- [ ] OS keychain integration
- [ ] Security documentation

### Phase 3: Architecture Improvements
- [x] Add input validation package ✅
- [x] Add graceful shutdown ✅
- [ ] Interface-based HTTP client
- [ ] Context propagation audit

### Phase 3: Core Features (4-6 weeks)
- [ ] Scheduled polling
- [ ] Notification integrations
- [ ] Diff/compare command
- [ ] Export/import

### Phase 4: Advanced Features (2-3 months)
- [ ] REST API mode
- [ ] Plugin system
- [ ] Event-based notifications

### Phase 5: Testing & Quality (Ongoing)
- [ ] Integration test suite
- [ ] Mock platform servers
- [ ] Fuzz testing
- [ ] CI/CD improvements

---

## Contributing

These suggestions are open for implementation! If you'd like to work on any of these:

1. Check if there's an existing issue or PR
2. Create an issue to discuss the approach
3. Fork the repository
4. Implement the feature
5. Submit a pull request

See [CONTRIBUTING.md](CONTRIBUTING.md) for detailed guidelines.
