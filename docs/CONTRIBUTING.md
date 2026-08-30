# Contributing to bbscope

Thank you for your interest in contributing to bbscope! This document provides guidelines and instructions for contributing to the project.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Project Structure](#project-structure)
- [Contribution Workflow](#contribution-workflow)
- [Coding Standards](#coding-standards)
- [Testing Guidelines](#testing-guidelines)
- [Documentation](#documentation)
- [Submitting Changes](#submitting-changes)

---

## Code of Conduct

- Be respectful and inclusive
- Provide constructive feedback
- Focus on the issue, not the person
- Help newcomers get started

---

## Getting Started

### Prerequisites

- **Go 1.26+** (or the version specified in go.mod)
- **PostgreSQL 12+** (for database features)
- **Git**
- **Make** (optional, for convenience scripts)

### Fork and Clone

```bash
# Fork the repository on GitHub, then:
git clone https://github.com/YOUR_USERNAME/bbscope.git
cd bbscope
git remote add upstream https://github.com/cozyGarage/bbscope.git
```

---

## Development Setup

### 1. Install Dependencies

```bash
go mod download
```

### 2. Set Up Local Database

```bash
# Using Docker
docker run --name bbscope-dev-db \
  -e POSTGRES_USER=bbscope \
  -e POSTGRES_PASSWORD=devpass \
  -e POSTGRES_DB=bbscope \
  -p 5432:5432 \
  -d postgres:alpine

# Or use an existing PostgreSQL instance
```

### 3. Create Configuration

```bash
# Create config file
cat > ~/.bbscope.yaml << EOF
db_url: "postgres://bbscope:devpass@localhost:5432/bbscope?sslmode=disable"

hackerone:
  username: ""
  token: ""

bugcrowd:
  email: ""
  password: ""
  otpsecret: ""

intigriti:
  token: ""

yeswehack:
  email: ""
  password: ""
  otpsecret: ""

ai:
  provider: "openai"
  api_key: ""
  model: "gpt-4o-mini"
EOF
```

### 4. Build and Test

```bash
# Build the binary
go build -o bbscope .

# Run tests
go test ./...

# Run with verbose output
go test -v ./...
```

---

## Project Structure

```
bbscope/
├── cmd/                    # CLI commands (Cobra)
│   ├── root.go             # Root command, config init
│   ├── poll.go             # Main polling logic
│   ├── poll_*.go           # Platform-specific poll commands
│   ├── db.go               # Database commands
│   └── get_*.go            # Data extraction commands
├── internal/               # Private packages
│   └── utils/              # Shared utilities
├── pkg/                    # Public packages
│   ├── ai/                 # AI/LLM integration
│   ├── otp/                # 2FA TOTP support
│   ├── platforms/          # Platform pollers
│   │   ├── platform.go     # Interface definition
│   │   ├── hackerone/      
│   │   ├── bugcrowd/       
│   │   ├── intigriti/      
│   │   ├── yeswehack/      
│   │   └── immunefi/       
│   ├── scope/              # Scope data types
│   ├── storage/            # Database layer
│   ├── whttp/              # HTTP client
│   └── wildcards/          # Wildcard processing
└── docs/                   # Documentation
```

### Key Interfaces

```go
// Platform Poller - implement for new platforms
type PlatformPoller interface {
    Name() string
    Authenticate(ctx context.Context, cfg AuthConfig) error
    ListProgramHandles(ctx context.Context, opts PollOptions) ([]string, error)
    FetchProgramScope(ctx context.Context, handle string, opts PollOptions) (scope.ProgramData, error)
}

// AI Normalizer - implement for new AI providers
type Normalizer interface {
    NormalizeTargets(ctx context.Context, info ProgramInfo, items []TargetItem) ([]TargetItem, error)
}
```

---

## Contribution Workflow

### 1. Create a Branch

```bash
# Sync with upstream
git fetch upstream
git checkout main
git merge upstream/main

# Create feature branch
git checkout -b feature/your-feature-name
# Or for bugfixes:
git checkout -b fix/issue-description
```

### 2. Make Changes

- Write code following the [Coding Standards](#coding-standards)
- Add tests for new functionality
- Update documentation as needed

### 3. Test Your Changes

```bash
# Run all tests
go test ./...

# Run tests with race detection
go test -race ./...

# Run specific package tests
go test ./pkg/storage/...

# Run with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### 4. Commit Your Changes

Use clear, descriptive commit messages:

```bash
# Good examples:
git commit -m "feat: add support for new platform XYZ"
git commit -m "fix: handle empty scope response from Bugcrowd"
git commit -m "docs: update installation instructions"
git commit -m "refactor: simplify URL normalization logic"
git commit -m "test: add tests for wildcard extraction"
```

Commit message prefixes:
- `feat:` - New feature
- `fix:` - Bug fix
- `docs:` - Documentation changes
- `refactor:` - Code refactoring
- `test:` - Test additions/changes
- `chore:` - Build, CI, dependency updates

### 5. Push and Create PR

```bash
git push origin feature/your-feature-name
```

Then create a Pull Request on GitHub.

---

## Coding Standards

### General Guidelines

1. **Follow Go conventions** - Use `gofmt` and `go vet`
2. **Keep functions focused** - Single responsibility
3. **Handle errors properly** - Don't ignore errors
4. **Add context to errors** - Use `fmt.Errorf("context: %w", err)`
5. **Document exported items** - Add GoDoc comments

### Code Style

```go
// Good: Clear, documented function
// FetchProgramScope retrieves the in-scope and out-of-scope targets
// for the specified program handle.
func (p *Poller) FetchProgramScope(ctx context.Context, handle string, opts PollOptions) (scope.ProgramData, error) {
    // Implementation
}

// Good: Error handling with context
if err != nil {
    return nil, fmt.Errorf("fetching program %s: %w", handle, err)
}

// Good: Early returns for validation
func process(input string) error {
    if input == "" {
        return errors.New("input cannot be empty")
    }
    // Main logic here
}
```

### Naming Conventions

```go
// Package names: lowercase, single word
package storage

// Interfaces: often end with -er
type Normalizer interface {}

// Exported constants: CamelCase
const MaxBatchSize = 25

// Unexported: camelCase
var defaultTimeout = 30 * time.Second

// Acronyms: consistent case
type HTTPClient struct {}  // Not: HttpClient
func (c *Client) GetURL()  // Not: GetUrl
```

### Linting

Run before committing:

```bash
# Format code
go fmt ./...

# Run vet
go vet ./...

# (Optional) Run staticcheck
staticcheck ./...

# (Optional) Run golangci-lint
golangci-lint run
```

---

## Testing Guidelines

### Test File Naming

```
pkg/ai/normalizer.go       -> pkg/ai/normalizer_test.go
pkg/storage/storage.go     -> pkg/storage/storage_test.go
```

### Test Structure

```go
func TestFunctionName(t *testing.T) {
    // Arrange
    input := "test input"
    expected := "expected output"
    
    // Act
    result := FunctionName(input)
    
    // Assert
    if result != expected {
        t.Errorf("FunctionName(%q) = %q, want %q", input, result, expected)
    }
}

// Table-driven tests (preferred)
func TestNormalizeCategory(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {"url category", "website", "url"},
        {"wildcard category", "wildcard", "wildcard"},
        {"unknown category", "unknown", "unknown"},
    }
    
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            result := NormalizeCategory(tc.input)
            if result != tc.expected {
                t.Errorf("got %q, want %q", result, tc.expected)
            }
        })
    }
}
```

### What to Test

- [ ] Happy path (normal operation)
- [ ] Edge cases (empty input, nil values)
- [ ] Error conditions
- [ ] Boundary conditions
- [ ] Concurrent access (if applicable)

### Mocking

For testing with external services, create interfaces and mock implementations:

```go
// Interface for HTTP client
type httpClient interface {
    Do(req *http.Request) (*http.Response, error)
}

// Mock implementation for tests
type mockHTTPClient struct {
    response *http.Response
    err      error
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
    return m.response, m.err
}
```

---

## Documentation

### Code Documentation

```go
// Package scope provides data structures and utilities for managing
// bug bounty program scope information.
package scope

// ScopeElement represents a single target within a program's scope.
// It contains the target identifier, description, and category.
type ScopeElement struct {
    // Target is the actual scope item (domain, URL, app identifier, etc.)
    Target string
    
    // Description provides additional context about the target
    Description string
    
    // Category indicates the type of target (url, wildcard, android, etc.)
    Category string
}
```

### README Updates

When adding features:
1. Update usage examples
2. Add new commands to command reference
3. Update configuration options

### Changelog

For significant changes, add an entry to CHANGELOG.md (if exists) or note in PR description.

---

## Submitting Changes

### Pull Request Checklist

- [ ] Code follows project style guidelines
- [ ] Tests pass locally (`go test ./...`); `make test-integration` if storage/cmd DB paths changed; `make test-fuzz` for parser changes
- [ ] New functionality includes tests
- [ ] Documentation updated (if applicable)
- [ ] Commit messages are clear
- [ ] PR description explains the changes

### PR Description Template

```markdown
## Description
Brief description of changes

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update

## Testing
How was this tested?

## Related Issues
Fixes #123
```

### Review Process

1. CI must be green (test, lint, gosec, tidy, build, secret scan, fuzz). `govulncheck` and image Trivy are advisory.
2. A maintainer reviews using the [code review playbook](CODE_REVIEW.md): severity, merge bar, and the checklists for subsystems the PR touched.
3. Address requested changes. Do not resolve a Critical/High finding with “will fix later” unless it is an accepted risk already listed in the playbook.
4. Security vulnerabilities go through [SECURITY.md](../SECURITY.md) (private reporting), not a public PR discussion of exploit details.
5. Once approved, a maintainer will merge.

---

## Adding a New Platform

To add support for a new bug bounty platform:

### 1. Create Platform Package

```bash
mkdir -p pkg/platforms/newplatform
```

### 2. Implement PlatformPoller Interface

```go
// pkg/platforms/newplatform/poller.go
package newplatform

import (
    "context"
    "github.com/cozyGarage/bbscope/v2/pkg/platforms"
    "github.com/cozyGarage/bbscope/v2/pkg/scope"
)

type Poller struct {
    token string
}

func NewPoller(token string) *Poller {
    return &Poller{token: token}
}

func (p *Poller) Name() string {
    return "newplatform"
}

func (p *Poller) Authenticate(ctx context.Context, cfg platforms.AuthConfig) error {
    // Implement authentication
    return nil
}

func (p *Poller) ListProgramHandles(ctx context.Context, opts platforms.PollOptions) ([]string, error) {
    // Implement program listing
    return nil, nil
}

func (p *Poller) FetchProgramScope(ctx context.Context, handle string, opts platforms.PollOptions) (scope.ProgramData, error) {
    // Implement scope fetching
    return scope.ProgramData{}, nil
}
```

### 3. Add CLI Command

```go
// cmd/poll_newplatform.go
package cmd

import (
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
    npplatform "github.com/cozyGarage/bbscope/v2/pkg/platforms/newplatform"
)

var pollNewPlatformCmd = &cobra.Command{
    Use:   "newplatform",
    Short: "Poll NewPlatform for scope",
    RunE: func(cmd *cobra.Command, args []string) error {
        token := viper.GetString("newplatform.token")
        poller := npplatform.NewPoller(token)
        return runPollWithPollers(cmd, []platforms.PlatformPoller{poller})
    },
}

func init() {
    pollCmd.AddCommand(pollNewPlatformCmd)
    pollNewPlatformCmd.Flags().String("token", "", "API token")
}
```

### 4. Add Configuration

Update `cmd/root.go`:

```go
viper.SetDefault("newplatform.token", "")
```

### 5. Update Documentation

- Update README.md with new platform info
- Add configuration section
- Add usage examples

---

## Getting Help

- **Issues**: Open a GitHub issue for bugs or feature requests
- **Discussions**: Use GitHub Discussions for questions
- **Security**: Report security issues privately (see SECURITY.md)

---

## Recognition

Contributors will be recognized in:
- GitHub contributors list
- Release notes (for significant contributions)

Thank you for contributing to bbscope! 🎉
