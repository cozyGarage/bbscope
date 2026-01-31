# bbscope Architecture Documentation

## Overview

**bbscope** is a powerful command-line tool designed for bug bounty hunters to aggregate, store, and manage program scopes from multiple bug bounty platforms. It's built in Go and follows a modular, layered architecture.

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              CLI Interface                                  │
│                            (Cobra Commands)                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│   poll         │   db          │   get         │   other commands           │
├────────────────┴────────────────┴───────────────┴───────────────────────────┤
│                           Core Business Logic                               │
│  ┌─────────────┐  ┌──────────────┐  ┌─────────────┐  ┌──────────────────┐   │
│  │  Platforms  │  │   Storage    │  │    Scope    │  │   AI Normalizer  │   │
│  │   Pollers   │  │  (Postgres)  │  │  Processing │  │    (LLM API)     │   │
│  └─────────────┘  └──────────────┘  └─────────────┘  └──────────────────┘   │
├─────────────────────────────────────────────────────────────────────────────┤
│                        Infrastructure Layer                                 │
│  ┌─────────────┐  ┌──────────────┐  ┌─────────────┐  ┌──────────────────┐   │
│  │    whttp    │  │     OTP      │  │   Utils     │  │    Wildcards     │   │
│  │ (HTTP Lib)  │  │  (2FA TOTP)  │  │  (Helpers)  │  │   Processing     │   │
│  └─────────────┘  └──────────────┘  └─────────────┘  └──────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         External Services                                   │
│  ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌──────────┐ ┌─────────────┐  │
│  │ HackerOne  │ │  Bugcrowd  │ │ Intigriti  │ │ YesWeHack│ │  Immunefi   │  │
│  │    API     │ │    Web     │ │    API     │ │   API    │ │    Web      │  │
│  └────────────┘ └────────────┘ └────────────┘ └──────────┘ └─────────────┘  │
│  ┌────────────────────────────┐  ┌───────────────────────────────────────┐  │
│  │       PostgreSQL DB        │  │           OpenAI API                  │  │
│  └────────────────────────────┘  └───────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Directory Structure

```
bbscope/
├── main.go                 # Application entry point
├── go.mod                  # Go module definition
├── Dockerfile              # Container build configuration
├── cmd/                    # CLI commands (Cobra)
│   ├── root.go             # Root command & configuration
│   ├── poll.go             # Main poll command
│   ├── poll_h1.go          # HackerOne polling subcommand
│   ├── poll_bc.go          # Bugcrowd polling subcommand
│   ├── poll_it.go          # Intigriti polling subcommand
│   ├── poll_ywh.go         # YesWeHack polling subcommand
│   ├── poll_immunefi.go    # Immunefi polling subcommand
│   ├── poll_dev.go         # Development testing command
│   ├── db.go               # Database interaction commands
│   ├── db_ignore.go        # Program ignore/unignore commands
│   ├── get.go              # Parent for get subcommands
│   ├── get_wildcards.go    # Extract wildcards from DB
│   ├── get_domains.go      # Extract domains from DB
│   ├── get_urls.go         # Extract URLs from DB
│   ├── get_cidrs.go        # Extract CIDRs from DB
│   ├── get_ips.go          # Extract IPs from DB
│   └── dev.go              # Development utilities
├── internal/               # Private application code
│   └── utils/
│       └── utils.go        # Logging, IP/CIDR validation
├── pkg/                    # Public library packages
│   ├── ai/                 # AI/LLM integration
│   │   ├── normalizer.go   # OpenAI normalizer implementation
│   │   └── normalizer_test.go
│   ├── otp/                # TOTP 2FA support
│   │   └── otp.go          # TOTP code generation
│   ├── platforms/          # Platform-specific polling
│   │   ├── platform.go     # PlatformPoller interface
│   │   ├── hackerone/      # HackerOne implementation
│   │   ├── bugcrowd/       # Bugcrowd implementation
│   │   ├── intigriti/      # Intigriti implementation
│   │   ├── yeswehack/      # YesWeHack implementation
│   │   └── immunefi/       # Immunefi implementation
│   ├── scope/              # Scope data structures & processing
│   │   └── scope.go        # ScopeElement, ProgramData, categories
│   ├── storage/            # Database layer
│   │   ├── storage.go      # Main DB operations
│   │   ├── types.go        # Data types (Entry, Change, etc.)
│   │   ├── normalize.go    # URL/target normalization
│   │   ├── transform.go    # Aggressive transformations
│   │   └── extra.go        # Additional utilities
│   ├── whttp/              # HTTP client wrapper
│   │   └── whttp.go        # Retryable HTTP with debugging
│   └── wildcards/          # Wildcard domain processing
│       └── wildcards.go    # Domain extraction & filtering
└── docs/                   # Documentation (you are here)
```

## Core Components

### 1. CLI Layer (`cmd/`)

Built with [Cobra](https://github.com/spf13/cobra), the CLI provides a hierarchical command structure:

```
bbscope
├── poll                    # Fetch scopes from platforms
│   ├── h1                  # HackerOne
│   ├── bc                  # Bugcrowd
│   ├── it                  # Intigriti
│   ├── ywh                 # YesWeHack
│   └── immunefi            # Immunefi
└── db                      # Database operations
    ├── stats               # Show statistics
    ├── print               # Print raw scope data
    ├── changes             # Recent scope changes
    ├── find                # Search scopes
    ├── shell               # Open psql shell
    ├── add                 # Add custom target
    ├── ignore              # Ignore a program
    ├── unignore            # Unignore a program
    └── get                 # Extract targets
        ├── wildcards       # Wildcard domains
        ├── domains         # All domains
        ├── urls            # HTTP/HTTPS URLs
        ├── cidrs           # CIDR ranges
        └── ips             # IP addresses
```

**Key Files:**
- `root.go`: Configuration initialization via Viper, legacy command redirection
- `poll.go`: Main polling logic, multi-platform orchestration
- `db.go`: Database query commands

### 2. Platform Pollers (`pkg/platforms/`)

Each platform implements the `PlatformPoller` interface:

```go
type PlatformPoller interface {
    Name() string
    Authenticate(ctx context.Context, cfg AuthConfig) error
    ListProgramHandles(ctx context.Context, opts PollOptions) ([]string, error)
    FetchProgramScope(ctx context.Context, handle string, opts PollOptions) (scope.ProgramData, error)
}
```

**Platform Implementations:**

| Platform | Auth Method | API Type | Rate Limiting |
|----------|-------------|----------|---------------|
| HackerOne | Basic Auth (username:token) | REST API | Built-in retry |
| Bugcrowd | Email/Password + OTP or Session Cookie | Web scraping | 1 req/sec worker |
| Intigriti | Bearer Token | REST API | Built-in retry |
| YesWeHack | Email/Password + OTP or Bearer Token | REST API | Built-in retry |
| Immunefi | None (public) | Web scraping | Built-in retry |

### 3. Storage Layer (`pkg/storage/`)

PostgreSQL-based persistence with three main tables:

```sql
-- Programs table
CREATE TABLE programs (
    id SERIAL PRIMARY KEY,
    platform TEXT NOT NULL,
    handle TEXT NOT NULL,
    url TEXT NOT NULL UNIQUE,
    first_seen_at TIMESTAMP,
    last_seen_at TIMESTAMP,
    strict INTEGER DEFAULT 0,
    disabled INTEGER DEFAULT 0,
    is_ignored INTEGER DEFAULT 0
);

-- Raw targets from platforms
CREATE TABLE targets_raw (
    id SERIAL PRIMARY KEY,
    program_id INTEGER REFERENCES programs(id),
    target TEXT NOT NULL,
    category TEXT NOT NULL,
    description TEXT,
    in_scope INTEGER NOT NULL,
    is_bbp INTEGER DEFAULT 0,
    first_seen_at TIMESTAMP,
    last_seen_at TIMESTAMP,
    UNIQUE(program_id, category, target)
);

-- AI-normalized variants
CREATE TABLE targets_ai_enhanced (
    id SERIAL PRIMARY KEY,
    target_id INTEGER REFERENCES targets_raw(id) ON DELETE CASCADE,
    target_ai_normalized TEXT NOT NULL,
    category TEXT,
    in_scope INTEGER,
    first_seen_at TIMESTAMP,
    last_seen_at TIMESTAMP,
    UNIQUE(target_id, target_ai_normalized)
);

-- Change history for auditing
CREATE TABLE scope_changes (
    id SERIAL PRIMARY KEY,
    occurred_at TIMESTAMP,
    program_url TEXT NOT NULL,
    platform TEXT NOT NULL,
    handle TEXT NOT NULL,
    target_normalized TEXT NOT NULL,
    target_raw TEXT,
    target_ai_normalized TEXT,
    category TEXT NOT NULL,
    in_scope INTEGER NOT NULL,
    is_bbp INTEGER DEFAULT 0,
    change_type TEXT CHECK (change_type IN ('added','updated','removed'))
);
```

**Key Operations:**
- `UpsertProgramEntries()`: Atomic upsert with change detection
- `ListEntries()`: Query entries with filters
- `ListRecentChanges()`: Audit trail queries

### 4. AI Normalizer (`pkg/ai/`)

LLM-powered scope normalization for cleaning messy platform data:

```go
type Normalizer interface {
    NormalizeTargets(ctx context.Context, info ProgramInfo, items []TargetItem) ([]TargetItem, error)
}
```

**Features:**
- Batched processing to minimize API calls
- Concurrent batch execution with configurable limits
- Fallback to original on LLM failure
- Support for OpenAI-compatible APIs
- Expansion of regex-like patterns (e.g., `example.(com|org)` → `example.com`, `example.org`)

### 5. HTTP Layer (`pkg/whttp/`)

Wrapper around `go-retryablehttp` with:
- Automatic retry with exponential backoff
- Debug logging of requests/responses
- Proxy support for debugging
- HTML title extraction
- UTF-8 handling

### 6. Scope Processing (`pkg/scope/`)

**Category Normalization:**

Platform-specific categories are unified into standard categories:

| Unified | Platform-specific strings |
|---------|--------------------------|
| `wildcard` | wildcard |
| `url` | url, website, web, web-application, api, ip_address |
| `cidr` | cidr, iprange |
| `android` | android, google_play_app_id, other_apk, mobile-application-android |
| `ios` | ios, apple, apple_store_app_id, testflight, mobile-application-ios |
| `ai` | ai_model |
| `hardware` | hardware, device, iot |
| `blockchain` | smart_contract |
| `binary` | windows_app_store_app_id, downloadable_executables |
| `code` | source_code |
| `other` | other, aws_cloud_config, application, network |

## Data Flow

### Polling Flow

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐     ┌──────────────┐
│   User      │────▶│  poll cmd    │────▶│  Platform   │────▶│   External   │
│  Command    │     │  (poll.go)   │     │   Poller    │     │   Platform   │
└─────────────┘     └──────────────┘     └─────────────┘     └──────────────┘
                           │                    │
                           │              ┌─────┴─────┐
                           │              │ Raw Scope │
                           │              │   Data    │
                           │              └─────┬─────┘
                           ▼                    │
                    ┌──────────────┐            │
                    │ AI Normalizer│◀───────────┘
                    │  (optional)  │
                    └──────┬───────┘
                           │
                           ▼
                    ┌──────────────┐     ┌──────────────┐
                    │   Storage    │────▶│  PostgreSQL  │
                    │    Layer     │     │   Database   │
                    └──────────────┘     └──────────────┘
                           │
                           ▼
                    ┌──────────────┐
                    │   Change     │
                    │   Output     │
                    └──────────────┘
```

### Query Flow

```
┌─────────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│   User      │────▶│   db get     │────▶│   Storage    │────▶│  PostgreSQL  │
│  Command    │     │   command    │     │    Layer     │     │   Database   │
└─────────────┘     └──────────────┘     └──────────────┘     └──────────────┘
                           │
                           ▼
                    ┌──────────────┐
                    │  Wildcards/  │
                    │  Transform   │
                    └──────┬───────┘
                           │
                           ▼
                    ┌──────────────┐
                    │   Formatted  │
                    │    Output    │
                    └──────────────┘
```

## Configuration

Configuration is managed via Viper with the following sources (in order of precedence):

1. Command-line flags
2. Environment variables
3. Config file (`~/.bbscope.yaml`)
4. Defaults

**Config Structure:**

```yaml
db_url: "postgres://user:pass@localhost:5432/bbscope?sslmode=disable"

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
  endpoint: ""
  max_batch: 25
  max_concurrency: 3
```

## Concurrency Model

- **Polling**: Concurrent program fetching per platform (default: 5 workers)
- **Bugcrowd**: Single-threaded due to WAF rate limiting (1 req/sec)
- **AI Normalization**: Concurrent batch processing (configurable concurrency)
- **Database**: Connection pooling via `database/sql`

## Error Handling

- HTTP requests use `go-retryablehttp` for automatic retry
- Platform-specific error messages (e.g., WAF ban detection for Bugcrowd)
- Safety check prevents accidental scope wipes (`ErrAbortingScopeWipe`)
- Graceful degradation when AI normalization fails

## Dependencies

| Package | Purpose |
|---------|---------|
| `spf13/cobra` | CLI framework |
| `spf13/viper` | Configuration management |
| `lib/pq` | PostgreSQL driver |
| `hashicorp/go-retryablehttp` | HTTP client with retry |
| `sirupsen/logrus` | Structured logging |
| `tidwall/gjson` | JSON parsing |
| `PuerkitoBio/goquery` | HTML parsing |
| `weppos/publicsuffix-go` | Domain suffix handling |

## Extension Points

1. **New Platform**: Implement `PlatformPoller` interface
2. **New AI Provider**: Implement `Normalizer` interface
3. **New Output Format**: Extend print/format functions in `cmd/`
4. **New Target Type**: Add to `getAndPrintTargets()` in `cmd/get.go`
