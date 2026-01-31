# bbscope Development Guide

This guide covers the development workflow, debugging techniques, and advanced topics for contributing to bbscope.

---

## Table of Contents

- [Development Environment Setup](#development-environment-setup)
- [Building and Running](#building-and-running)
- [Testing](#testing)
- [Debugging](#debugging)
- [Code Organization](#code-organization)
- [Working with Platforms](#working-with-platforms)
- [Database Development](#database-development)
- [AI Integration Development](#ai-integration-development)
- [Release Process](#release-process)

---

## Development Environment Setup

### Required Tools

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.24+ | Primary language |
| PostgreSQL | 12+ | Database |
| Docker | Latest | Containerization |
| Git | Latest | Version control |

### Optional Tools

| Tool | Purpose |
|------|---------|
| `golangci-lint` | Comprehensive linting |
| `staticcheck` | Static analysis |
| `gosec` | Security scanning |
| `dlv` | Go debugger |
| `pgcli` | Better PostgreSQL CLI |

### IDE Setup

#### VS Code

Recommended extensions:
- Go (official)
- PostgreSQL
- Docker

`.vscode/settings.json`:
```json
{
    "go.lintTool": "golangci-lint",
    "go.lintFlags": ["--fast"],
    "go.testFlags": ["-v"],
    "go.coverOnSave": true,
    "go.coverageDecorator": {
        "type": "highlight"
    }
}
```

`.vscode/launch.json`:
```json
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Debug bbscope",
            "type": "go",
            "request": "launch",
            "mode": "auto",
            "program": "${workspaceFolder}",
            "args": ["poll", "h1", "--db"],
            "env": {
                "BBSCOPE_DEBUG": "1"
            }
        },
        {
            "name": "Debug Tests",
            "type": "go",
            "request": "launch",
            "mode": "test",
            "program": "${workspaceFolder}/pkg/storage",
            "args": ["-test.v"]
        }
    ]
}
```

### Initial Setup

```bash
# Clone repository
git clone https://github.com/sw33tLie/bbscope.git
cd bbscope

# Install dependencies
go mod download

# Set up pre-commit hooks (optional)
cat > .git/hooks/pre-commit << 'EOF'
#!/bin/sh
go fmt ./...
go vet ./...
go test ./... -short
EOF
chmod +x .git/hooks/pre-commit

# Set up development database
docker run --name bbscope-dev \
  -e POSTGRES_USER=bbscope \
  -e POSTGRES_PASSWORD=devpass \
  -e POSTGRES_DB=bbscope \
  -p 5432:5432 \
  -d postgres:16-alpine

# Create config file
cat > ~/.bbscope-dev.yaml << EOF
db_url: "postgres://bbscope:devpass@localhost:5432/bbscope?sslmode=disable"
EOF
```

---

## Building and Running

### Build Commands

```bash
# Standard build
go build -o bbscope .

# Build with debug symbols
go build -gcflags="all=-N -l" -o bbscope .

# Build for specific platform
GOOS=linux GOARCH=amd64 go build -o bbscope-linux .
GOOS=darwin GOARCH=arm64 go build -o bbscope-macos .
GOOS=windows GOARCH=amd64 go build -o bbscope.exe .

# Optimized release build
go build -ldflags="-w -s" -o bbscope .

# Build with version info
VERSION=$(git describe --tags --always)
go build -ldflags="-w -s -X main.version=${VERSION}" -o bbscope .
```

### Running During Development

```bash
# Run directly with go run
go run . poll h1 --db

# Run with custom config
go run . --config ~/.bbscope-dev.yaml poll h1

# Run with debug logging
go run . -l debug poll h1

# Run with HTTP debugging
go run . --debug-http poll h1

# Run with proxy (for intercepting requests)
go run . --proxy http://127.0.0.1:8080 poll h1
```

### Docker Development

```bash
# Build local image
docker build -t bbscope:dev .

# Run local image
docker run --rm bbscope:dev --help

# Run with config mount
docker run --rm \
  -v ~/.bbscope.yaml:/root/.bbscope.yaml \
  --network host \
  bbscope:dev poll h1 --db

# Interactive shell in container
docker run --rm -it --entrypoint sh bbscope:dev
```

---

## Testing

### Running Tests

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run specific package
go test -v ./pkg/storage/...
go test -v ./pkg/ai/...

# Run specific test
go test -v ./pkg/ai/... -run TestNormalizerScenarios

# Run with race detection
go test -race ./...

# Run short tests only (skip integration)
go test -short ./...

# Run with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
open coverage.html

# Coverage by package
go test -coverprofile=coverage.out -covermode=atomic ./...
go tool cover -func=coverage.out
```

### Writing Tests

#### Unit Test Pattern

```go
func TestNormalizeTarget(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {
            name:     "lowercase domain",
            input:    "EXAMPLE.COM",
            expected: "example.com",
        },
        {
            name:     "remove trailing dot",
            input:    "example.com.",
            expected: "example.com",
        },
        {
            name:     "normalize URL",
            input:    "HTTPS://Example.com:443/path/",
            expected: "https://example.com/path",
        },
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            result := NormalizeTarget(tc.input)
            if result != tc.expected {
                t.Errorf("NormalizeTarget(%q) = %q, want %q",
                    tc.input, result, tc.expected)
            }
        })
    }
}
```

#### Integration Test Pattern

```go
func TestDatabaseIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }

    // Use test database
    dbURL := os.Getenv("TEST_DB_URL")
    if dbURL == "" {
        t.Skip("TEST_DB_URL not set")
    }

    db, err := storage.Open(dbURL)
    if err != nil {
        t.Fatalf("failed to open database: %v", err)
    }
    defer db.Close()

    // Test operations...
}
```

#### Mock HTTP Server

```go
func TestFetchProgramScope(t *testing.T) {
    // Create mock server
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path == "/api/programs/test" {
            w.Header().Set("Content-Type", "application/json")
            json.NewEncoder(w).Encode(map[string]interface{}{
                "data": []map[string]interface{}{
                    {"asset_identifier": "*.example.com", "asset_type": "wildcard"},
                },
            })
            return
        }
        w.WriteHeader(http.StatusNotFound)
    }))
    defer server.Close()

    // Use mock server URL in tests
    poller := &Poller{baseURL: server.URL}
    // ...
}
```

### Test Database Setup

```bash
# Create test database
docker run --name bbscope-test \
  -e POSTGRES_USER=test \
  -e POSTGRES_PASSWORD=test \
  -e POSTGRES_DB=bbscope_test \
  -p 5433:5432 \
  -d postgres:16-alpine

# Run integration tests
TEST_DB_URL="postgres://test:test@localhost:5433/bbscope_test?sslmode=disable" \
  go test -v ./pkg/storage/...
```

---

## Debugging

### Log Levels

```bash
# Available levels: debug, info, warn, error, fatal
go run . -l debug poll h1
```

### HTTP Debugging

```bash
# Print all HTTP requests and responses
go run . --debug-http poll h1
```

### Using Proxy for Inspection

```bash
# Start Burp Suite or mitmproxy on port 8080
go run . --proxy http://127.0.0.1:8080 poll bc
```

### Delve Debugger

```bash
# Install delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Debug from command line
dlv debug . -- poll h1 --db

# Attach to running process
dlv attach $(pgrep bbscope)

# Common delve commands:
# (dlv) break pkg/storage/storage.go:200
# (dlv) continue
# (dlv) print variable
# (dlv) next
# (dlv) step
# (dlv) stack
```

### Database Debugging

```bash
# Connect to database
psql "postgres://bbscope:devpass@localhost:5432/bbscope"

# Useful queries
SELECT * FROM programs LIMIT 10;
SELECT * FROM targets_raw WHERE program_id = 1;
SELECT * FROM scope_changes ORDER BY occurred_at DESC LIMIT 20;

# Check for issues
SELECT platform, COUNT(*) FROM programs GROUP BY platform;
SELECT category, COUNT(*) FROM targets_raw GROUP BY category;
```

---

## Code Organization

### Package Dependencies

```
main.go
    └── cmd/
        ├── root.go
        │   ├── internal/utils
        │   └── pkg/whttp
        ├── poll.go
        │   ├── pkg/platforms/*
        │   ├── pkg/scope
        │   ├── pkg/storage
        │   └── pkg/ai
        └── db.go
            ├── pkg/storage
            ├── pkg/scope
            └── pkg/wildcards
```

### Adding New Features

1. **New CLI Command**: Add to `cmd/` directory
2. **New Platform**: Add to `pkg/platforms/<name>/`
3. **New Data Type**: Add to `pkg/storage/types.go`
4. **New Utility**: Add to `internal/utils/` or appropriate `pkg/`

### File Naming Conventions

```
cmd/
  poll_<platform>.go      # Platform-specific poll command
  get_<type>.go           # Data extraction command

pkg/<package>/
  <package>.go            # Main implementation
  <package>_test.go       # Tests
  types.go                # Type definitions (if many)
```

---

## Working with Platforms

### Understanding Platform Authentication

| Platform | Auth Type | Implementation |
|----------|-----------|----------------|
| HackerOne | Basic Auth | Direct API token |
| Bugcrowd | Session Cookie | Web login + OTP |
| Intigriti | Bearer Token | API token |
| YesWeHack | Bearer Token | Web login + OTP or token |
| Immunefi | None | Public scraping |

### Testing Platform Changes

```bash
# Test specific platform
go run . poll h1 -l debug

# Test with limited scope
go run . poll h1 --category wildcard

# Test without database (dry run)
go run . poll h1  # Without --db flag

# Compare outputs
go run . poll h1 > before.txt
# Make changes...
go run . poll h1 > after.txt
diff before.txt after.txt
```

### Platform API Investigation

When platforms change their API:

1. Use browser DevTools to inspect requests
2. Export HAR file for reference
3. Use `--debug-http` to see current requests
4. Compare and update implementation

---

## Database Development

### Schema Changes

When modifying the database schema:

1. Update schema in `pkg/storage/storage.go`
2. Schema is auto-migrated on connection
3. For breaking changes, consider migration strategy

```go
// Example: Adding a new column
const schema = `
CREATE TABLE IF NOT EXISTS programs (
    -- existing columns...
    new_column TEXT DEFAULT ''
);
-- Add column if not exists
DO $$ BEGIN
    ALTER TABLE programs ADD COLUMN new_column TEXT DEFAULT '';
EXCEPTION
    WHEN duplicate_column THEN NULL;
END $$;
`
```

### Query Optimization

```bash
# Enable query logging in PostgreSQL
docker exec -it bbscope-dev psql -U bbscope -c "ALTER SYSTEM SET log_statement = 'all';"
docker restart bbscope-dev
docker logs -f bbscope-dev 2>&1 | grep "statement:"
```

### Database Reset

```bash
# Drop and recreate
docker exec -it bbscope-dev psql -U bbscope -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"

# Or restart container
docker stop bbscope-dev
docker rm bbscope-dev
# Re-run docker run command
```

---

## AI Integration Development

### Testing Without API Calls

```go
// Use mock in tests
type mockNormalizer struct {
    results map[string][]string
}

func (m *mockNormalizer) NormalizeTargets(ctx context.Context, info ProgramInfo, items []TargetItem) ([]TargetItem, error) {
    // Return predefined results
    return items, nil
}
```

### Local LLM Testing

```bash
# Use local Ollama
ollama serve

# Configure bbscope for local endpoint
cat >> ~/.bbscope.yaml << EOF
ai:
  provider: "openai"
  endpoint: "http://localhost:11434/v1/chat/completions"
  model: "llama2"
  api_key: "dummy"
EOF
```

### Debugging AI Requests

```bash
# Enable debug to see prompts
go run . -l debug --debug-http poll h1 --db --ai
```

---

## Release Process

### Version Tagging

```bash
# Create release tag
git tag -a v2.1.0 -m "Release v2.1.0"
git push origin v2.1.0
```

### Build Release Binaries

```bash
# Build for all platforms
GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o dist/bbscope-linux-amd64 .
GOOS=linux GOARCH=arm64 go build -ldflags="-w -s" -o dist/bbscope-linux-arm64 .
GOOS=darwin GOARCH=amd64 go build -ldflags="-w -s" -o dist/bbscope-darwin-amd64 .
GOOS=darwin GOARCH=arm64 go build -ldflags="-w -s" -o dist/bbscope-darwin-arm64 .
GOOS=windows GOARCH=amd64 go build -ldflags="-w -s" -o dist/bbscope-windows-amd64.exe .
```

### Docker Release

```bash
# Build and push
docker build -t ghcr.io/sw33tlie/bbscope:v2.1.0 .
docker push ghcr.io/sw33tlie/bbscope:v2.1.0

# Tag as latest
docker tag ghcr.io/sw33tlie/bbscope:v2.1.0 ghcr.io/sw33tlie/bbscope:latest
docker push ghcr.io/sw33tlie/bbscope:latest
```

### Pre-release Checklist

- [ ] All tests pass
- [ ] No linting errors
- [ ] Documentation updated
- [ ] CHANGELOG updated
- [ ] Version number correct
- [ ] Docker build successful
- [ ] Manual testing completed

---

## Common Development Tasks

### Add New Category

1. Update `pkg/scope/scope.go`:
```go
var unificationMap = map[string][]string{
    // ...
    "newcat": {"newcat", "platform_specific_name"},
}
```

2. Update documentation

### Add New Output Format

1. Update command in `cmd/`:
```go
case "newformat":
    // Implement formatting
```

2. Add flag option
3. Update help text

### Add Configuration Option

1. Add to `cmd/root.go`:
```go
viper.SetDefault("new.option", "default")
```

2. Add to config file template in README
3. Use in code:
```go
value := viper.GetString("new.option")
```

---

## Troubleshooting Development Issues

### Module Issues

```bash
# Clear module cache
go clean -modcache

# Verify modules
go mod verify

# Update all dependencies
go get -u ./...
go mod tidy
```

### Build Issues

```bash
# Verbose build
go build -v .

# Check for CGO issues (should not need CGO)
CGO_ENABLED=0 go build .
```

### Test Issues

```bash
# Clear test cache
go clean -testcache

# Run tests with output on failure only
go test ./... 2>&1 | grep -A 5 "FAIL"
```
