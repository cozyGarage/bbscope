# bbscope Glossary

This glossary defines terminology and concepts used throughout bbscope and bug bounty hunting.

---

## A

### AI Normalization
The process of using Large Language Models (LLMs) to clean up and expand messy scope entries. For example, transforming `example.(com|org)` into separate entries for `example.com` and `example.org`.

### API Token
A secret key used to authenticate with platform APIs. Different from username/password authentication. Typically generated in platform account settings.

### Asset
A target within a program's scope. Can be a domain, URL, IP, mobile app, or other resource.

---

## B

### BBP (Bug Bounty Program)
A program that offers monetary rewards for valid security vulnerability reports.

### Bearer Token
An authentication token passed in the `Authorization: Bearer <token>` HTTP header. Used by Intigriti and YesWeHack.

### Basic Auth
HTTP authentication using base64-encoded `username:password`. Used by HackerOne's API.

---

## C

### Category
The type classification of a scope entry. Examples:
- `wildcard` - Wildcard domains
- `url` - Websites and URLs
- `cidr` - IP address ranges
- `android` - Android applications
- `ios` - iOS applications
- `hardware` - Physical devices

### CIDR (Classless Inter-Domain Routing)
IP address range notation, e.g., `192.168.1.0/24` represents 256 IP addresses.

### Change
A scope modification event. Types:
- `added` - New target added to scope
- `removed` - Target removed from scope
- `updated` - Target metadata changed

### Cobra
A Go library for creating CLI applications. bbscope uses Cobra for its command structure.

### Concurrency
The number of parallel operations. In bbscope, this typically refers to simultaneous program scope fetches.

---

## D

### Database (DB)
PostgreSQL database where bbscope stores scope data for persistence and querying.

### Delimiter
Character used to separate output fields. Default is space (` `).

### Docker
Container platform for running bbscope in isolated environments.

---

## E

### Entry
A single scope item stored in the database, containing target, category, in-scope status, and metadata.

---

## F

### Flag
Command-line option that modifies command behavior. Examples:
- `--db` - Save to database
- `--bbp-only` - Only bounty programs
- `--private-only` - Only private programs

---

## G

### gjson
Go library for JSON parsing. Used to extract data from platform API responses.

### go-retryablehttp
HTTP client library with automatic retry logic. Used for reliable API communication.

---

## H

### Handle
Unique identifier for a program on a platform. For example, in `https://hackerone.com/uber`, "uber" is the handle.

### HTTP Proxy
Intermediate server for inspecting HTTP traffic. Useful for debugging API interactions.

---

## I

### Immunefi
Bug bounty platform focused on blockchain and cryptocurrency projects.

### In-Scope
Targets that are allowed for security testing.

### Intigriti
European bug bounty platform.

---

## L

### LLM (Large Language Model)
AI model (like GPT-4) used for intelligent scope normalization.

### Log Level
Verbosity of logging output:
- `debug` - Most detailed
- `info` - Standard (default)
- `warn` - Warnings only
- `error` - Errors only
- `fatal` - Critical errors only

---

## N

### Normalization
Converting scope entries to a consistent format:
- Lowercase conversion
- Removing trailing slashes/dots
- URL scheme normalization
- Port normalization

---

## O

### OOS (Out of Scope)
Targets explicitly excluded from testing. Testing these may violate program rules.

### OTP (One-Time Password)
Time-based authentication code for 2FA. Generated using TOTP algorithm.

### Output Flags
Characters specifying what to include in output:
- `t` - Target
- `d` - Description
- `c` - Category
- `u` - Program URL

---

## P

### Platform
Bug bounty hosting service:
- HackerOne (h1)
- Bugcrowd (bc)
- Intigriti (it)
- YesWeHack (ywh)
- Immunefi

### Platform Poller
Interface implementation for fetching scope from a specific platform.

### PostgreSQL
Open-source relational database used by bbscope for data storage.

### Private Program
Bug bounty program accessible only by invitation.

### Program
A bug bounty program hosted on a platform, representing a company's security testing initiative.

### Proxy
HTTP proxy server for intercepting and inspecting traffic.

### Public Program
Bug bounty program open to all researchers.

---

## R

### Rate Limiting
Restricting request frequency to avoid overwhelming APIs or triggering bans.

### RFC3339
Standard date-time format: `2024-01-15T10:30:00Z`

### Root Command
The base `bbscope` command from which all subcommands branch.

---

## S

### Schema
Database table structure. bbscope auto-creates tables on first connection.

### Scope
The defined boundaries of what can be tested in a bug bounty program.

### ScopeElement
Data structure representing a single scope item with target, description, and category.

### Session Cookie
Authentication token stored as browser cookie. Used for Bugcrowd authentication.

---

## T

### Target
The actual scope item - a domain, URL, IP, app identifier, etc.

### TOTP (Time-based One-Time Password)
Algorithm for generating 2FA codes. Standard implementation per RFC 6238.

---

## U

### Unified Category
Normalized category name that maps multiple platform-specific names to one standard name.

### Upsert
Database operation that inserts new records or updates existing ones. Used for scope synchronization.

---

## V

### Variant
An expanded or normalized version of a scope target created by AI normalization.

### VDP (Vulnerability Disclosure Program)
Program that accepts vulnerability reports but typically doesn't offer monetary rewards.

### Viper
Go library for configuration management. Handles bbscope's config file and environment variables.

---

## W

### WAF (Web Application Firewall)
Security system that can block automated requests. Bugcrowd uses WAF that requires rate limiting.

### whttp
bbscope's HTTP client wrapper providing retry logic, debugging, and proxy support.

### Wildcard
Domain pattern matching any subdomain, e.g., `*.example.com` matches `foo.example.com`.

---

## Y

### YesWeHack
European bug bounty platform, particularly popular in France.

---

## Acronyms Quick Reference

| Acronym | Meaning |
|---------|---------|
| API | Application Programming Interface |
| BBP | Bug Bounty Program |
| CIDR | Classless Inter-Domain Routing |
| CLI | Command Line Interface |
| DB | Database |
| HTTP | HyperText Transfer Protocol |
| JSON | JavaScript Object Notation |
| JWT | JSON Web Token |
| LLM | Large Language Model |
| OOS | Out of Scope |
| OTP | One-Time Password |
| PR | Pull Request |
| TLS | Transport Layer Security |
| TOTP | Time-based One-Time Password |
| URL | Uniform Resource Locator |
| VDP | Vulnerability Disclosure Program |
| WAF | Web Application Firewall |
