# bbscope Documentation

Welcome to the bbscope documentation! This guide will help you understand, use, and contribute to bbscope.

## Quick Links

| Document | Description |
|----------|-------------|
| [README](../README.md) | Installation, configuration, and usage examples |
| [Architecture](ARCHITECTURE.md) | System design, components, and data flow |
| [Code Review](CODE_REVIEW.md) | Review playbook, checklists, and live findings |
| [Security Audit](SECURITY_AUDIT.md) | Historical security assessment |
| [API Reference](API_REFERENCE.md) | Complete API and data type reference |
| [Development](DEVELOPMENT.md) | Development setup and workflow |
| [Contributing](CONTRIBUTING.md) | How to contribute to the project |
| [Troubleshooting](TROUBLESHOOTING.md) | Common issues and solutions |
| [Improvements](IMPROVEMENTS.md) | Suggested enhancements and roadmap |
| [Glossary](GLOSSARY.md) | Terminology and concepts |
| [TUI Quick Reference](TUI_QUICKREF.md) | Interactive terminal UI keyboard shortcuts |
| [TUI Architecture](TUI_ARCHITECTURE.md) | Terminal UI implementation notes |
| [Migration](MIGRATION.md) | `sw33tLie` → `cozyGarage` namespace migration |

---

## What is bbscope?

**bbscope** is a command-line tool that aggregates bug bounty program scopes from multiple platforms:

- **HackerOne** (h1)
- **Bugcrowd** (bc)
- **Intigriti** (it)
- **YesWeHack** (ywh)
- **Immunefi**

It helps bug bounty hunters:
- Fetch and normalize scope data
- Store scope in a PostgreSQL database
- Track scope changes over time
- Extract specific target types (wildcards, URLs, CIDRs, IPs)
- Optionally use AI to clean up messy scope entries

---

## Getting Started

### Installation

```bash
# Using Go
go install github.com/cozyGarage/bbscope/v2@latest

# Using Docker
docker pull ghcr.io/cozygarage/bbscope:latest
```

### First Run

```bash
# Creates ~/.bbscope.yaml
bbscope --help

# Edit config with your credentials
vim ~/.bbscope.yaml

# Poll a platform
bbscope poll h1

# Poll all platforms and save to database
bbscope poll --db
```

---

## Documentation Map

### For Users

1. **Start here**: [README](../README.md)
   - Installation
   - Configuration
   - Basic usage examples

2. **When things go wrong**: [Troubleshooting](TROUBLESHOOTING.md)
   - Common errors and solutions
   - Platform-specific issues
   - Debug techniques

### For Developers

1. **Understand the codebase**: [Architecture](ARCHITECTURE.md)
   - Project structure
   - Component relationships
   - Data flow diagrams

2. **Set up development**: [Development](DEVELOPMENT.md)
   - Environment setup
   - Building and testing
   - Debugging techniques

3. **Contribute changes**: [Contributing](CONTRIBUTING.md)
   - Workflow
   - Coding standards
   - PR process

4. **Use internal APIs**: [API Reference](API_REFERENCE.md)
   - Interfaces
   - Data types
   - Database schema

### For Security Reviewers

1. **How to review (start here)**: [Code Review Playbook](CODE_REVIEW.md)
   - Severity and merge bar
   - Subsystem checklists
   - Live findings backlog

2. **Historical assessment**: [Security Audit](SECURITY_AUDIT.md)
   - Original vulnerability write-up
   - Accepted risks also listed in [SECURITY.md](../SECURITY.md)

### For Project Maintainers

1. **Future direction**: [Improvements](IMPROVEMENTS.md)
   - Enhancement proposals
   - Implementation roadmap
   - Priority matrix

---

## Core Concepts

### Platforms
Bug bounty platforms where programs are hosted. Each has different APIs and authentication methods.

### Programs
Bug bounty programs hosted on platforms. Each program has a unique URL/handle.

### Scope
The targets (domains, IPs, apps) that are allowed (in-scope) or forbidden (out-of-scope) for testing.

### Categories
Types of scope entries:
- `wildcard` - Wildcard domains (*.example.com)
- `url` - URLs and websites
- `cidr` - IP ranges
- `android` / `ios` - Mobile apps
- `hardware` - Physical devices
- And more...

### Polling
Fetching scope data from platforms via their APIs.

### Normalization
Converting messy scope strings into clean, consistent formats. Can be done:
- Automatically (basic cleaning)
- With AI assistance (advanced cleanup)

---

## Command Structure

```
bbscope
├── poll                    # Fetch scopes from platforms
│   ├── h1                  # HackerOne
│   ├── bc                  # Bugcrowd
│   ├── it                  # Intigriti
│   ├── ywh                 # YesWeHack
│   └── immunefi            # Immunefi
├── db                      # Database operations
│   ├── stats               # Database statistics
│   ├── print               # Print scope data
│   ├── changes             # Recent changes
│   ├── find                # Search scopes
│   ├── shell               # Open psql shell
│   ├── add                 # Add custom target
│   ├── ignore              # Ignore a program
│   ├── unignore            # Unignore a program
│   └── get                 # Extract targets
│       ├── wildcards       # Wildcard domains
│       ├── domains         # All domains
│       ├── urls            # HTTP/HTTPS URLs
│       ├── cidrs           # CIDR ranges
│       └── ips             # IP addresses
├── daemon                  # Scheduled background polling
├── tui                     # Interactive terminal UI
└── version                 # Version information
```

---

## Common Workflows

### Daily Scope Checking
```bash
# Poll all configured platforms, save to DB
bbscope poll --db -b -p

# View today's changes
bbscope db changes --limit 20
```

### Subdomain Enumeration Pipeline
```bash
# Get wildcards and pipe to subdomain tools
bbscope db get wildcards -a | subfinder | httpx
```

### Export for Other Tools
```bash
# Get all URLs
bbscope db get urls > urls.txt

# Get CIDRs for network scanning
bbscope db get cidrs > cidrs.txt
```

---

## Getting Help

- **GitHub Issues**: [Report bugs or request features](https://github.com/cozyGarage/bbscope/issues)
- **Troubleshooting Guide**: [Common problems and solutions](TROUBLESHOOTING.md)
- **Website**: [bbscope.com](https://bbscope.com) - Public scope browser

---

## License

bbscope is open source software. See the [LICENSE](../LICENSE) file for details.
