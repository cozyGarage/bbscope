# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project aims
to follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Tagged releases also ship auto-generated notes via GoReleaser; this file tracks
higher-level, human-curated changes.

## [Unreleased]

### Added
- `db get domains|urls|ips|cidrs` now support a `--platform` filter (previously
  only `wildcards` did).
- Schema migration framework (`schema_migrations` table) so database schema
  changes upgrade existing databases safely.
- `TEST_DB_URL`-gated PostgreSQL integration tests for the storage layer, plus
  broader unit tests for the AI, notify, wildcards, and poll-orchestration code.
- httptest-backed Bugcrowd and Immunefi poller tests (completing provider coverage).
- Dependabot configuration and a Codecov coverage gate.
- Project meta: `SECURITY.md`, `CODE_OF_CONDUCT.md`, `.editorconfig`, issue and
  pull-request templates.

### Changed
- Polling now honors context cancelation (Ctrl-C stops in-flight work).
- `UpsertProgramEntries` runs as a single atomic transaction.
- Split `pkg/storage` into `storage.go` / `schema.go` / `upsert.go` /
  `queries.go` / `programs.go` (no behavior change).
- Migrated the PostgreSQL driver from `lib/pq` to `jackc/pgx/v5` via
  `database/sql` (`pgx/stdlib`); toolchain raised to Go 1.26 (from 1.25) for current
  module and stdlib security fixes (`pgx` v5.9.2, `x/net` v0.55, `x/text` v0.39).
- HTTP client caps response bodies at 100 MiB; the `--proxy` transport uses a
  modern TLS 1.2 floor instead of pinning TLS 1.1.
- Upgraded `cobra` (1.2.1 → 1.10.x) and `viper` (1.8.1 → 1.21.x); pinned
  `golangci-lint` in CI for reproducible linting.

### Fixed
- `db get domains` no longer emits IP addresses or CIDR/IP ranges as domains.
- HackerOne program listing and Intigriti fetching no longer retry unbounded;
  the Intigriti handle maps are no longer subject to a data race.
- Worker/library code paths no longer call `log.Fatal`, so a single program's
  error cannot terminate the whole run.
- `db shell` redacts the database URL and passes the password via `PGPASSWORD`
  instead of exposing it on the process command line.
- Bugcrowd `poll bc` now honors `--category` for both target-group and
  engagement brief scope extraction (previously hard-coded to `all`).
- Bugcrowd login accepts path-relative `redirect_to` values resolved against
  `identity.bugcrowd.com`, while still rejecting off-origin hosts.
- YesWeHack TOTP verification payload is JSON-marshaled instead of string-concatenated.
- `pkg/whttp` invalid-URL unit test no longer burns ~3 minutes on default retries.
- `SyncPlatformPrograms` soft-disables missing programs (retains targets) and
  aborts mass disables from partial polls.
- `UpsertProgramEntries` reads and writes under one transaction with row locks.
- Notify channels escape untrusted scope fields; AI variants must derive from
  the source URI; Immunefi/IT/YWH treat HTTP/parse failures as errors.
