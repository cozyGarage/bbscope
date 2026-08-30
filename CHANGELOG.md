# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project aims
to follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Tagged releases also ship auto-generated notes via GoReleaser; this file tracks
higher-level, human-curated changes.

## [Unreleased]

### Added
- Code review playbook and subsystem checklists in `docs/CODE_REVIEW.md`; PR template links to them.
- Shared platform alias map (`pkg/platforms.CanonicalName` / `MatchingNames`) used by poll filters, `db --platform`, and `validate.Platform`.
- Webhook destination policy: `http`/`https` only, reject cloud-metadata and link-local IPs, allow loopback.
- Fuzz targets for `NormalizeTarget`, platform aliases, and webhook URL policy (`make test-fuzz`).
- Unit tests for `db export` / `db ignore` / config masking and TUI key handling.
- CI secret scan (Gitleaks, blocking), short fuzz job, and Docker image scan (Trivy, advisory).

### Changed
- `whttp.SetupProxy` clones a client instead of mutating the process-global transport.
- `bbscope daemon` authenticates pollers once at startup and reuses them; re-auth only on auth-like errors.
- HackerOne, YesWeHack, and Intigriti interpolate `url.PathEscape`d handles/ids into request paths.
- Duplicate identity keys in one upsert payload log a warning (first entry wins).
- Slack/Discord notification titles and usernames are escaped like other untrusted fields.
- Docker CI always loads the image locally so Trivy can scan it, then pushes to GHCR on non-PR events.
- GoReleaser GitHub Action is pinned to `~> v2` (release + `goreleaser check`).
- `make test` / coverage use `-covermode=atomic`.

### Added
- `db get domains|urls|ips|cidrs` now support a `--platform` filter (previously
  only `wildcards` did).
- Schema migration framework (`schema_migrations` table) so database schema
  changes upgrade existing databases safely.
- `TEST_DB_URL`-gated PostgreSQL integration tests for the storage layer, plus
  broader unit tests for the AI, notify, wildcards, and poll-orchestration code.
- httptest-backed Bugcrowd and Immunefi poller tests (completing provider coverage).
- HackerOne, Intigriti, and YesWeHack poller fixtures under each package's
  `testdata/` directory (matching Bugcrowd/Immunefi).
- Dependabot configuration and a Codecov coverage gate.
- Project meta: `SECURITY.md`, `CODE_OF_CONDUCT.md`, `.editorconfig`, issue and
  pull-request templates.
- Code review playbook and subsystem checklists in `docs/CODE_REVIEW.md`.
- Shared platform alias map (`pkg/platforms.CanonicalName` / `MatchingNames`)
  used by poll filters, `db --platform`, and `validate.Platform`.
- Webhook destination policy: `http`/`https` only; reject cloud-metadata and
  link-local IPs; allow loopback.

### Changed
- Polling now honors context cancelation (Ctrl-C stops in-flight work).
- `UpsertProgramEntries` runs as a single atomic transaction.
- Split `pkg/storage` into `storage.go` / `schema.go` / `upsert.go` /
  `queries.go` / `programs.go` (no behavior change).
- Migrated the PostgreSQL driver from `lib/pq` to `jackc/pgx/v5` via
  `database/sql` (`pgx/stdlib`); toolchain raised to Go 1.26 (from 1.25) for current
  module and stdlib security fixes (`pgx` v5.9.2, `x/net` v0.56, `x/text` v0.39).
- HTTP client caps response bodies at 100 MiB; the `--proxy` transport uses a
  modern TLS 1.2 floor instead of pinning TLS 1.1.
- Upgraded `cobra` (1.2.1 → 1.10.x) and `viper` (1.8.1 → 1.21.x); pinned
  `golangci-lint` in CI for reproducible linting.
- Platform poller fixtures live under `testdata/`; `MockPoller` implements
  `PlatformPoller` for `cmd/poll` orchestration tests.
- `whttp.SetupProxy` clones a client instead of mutating the process-global
  transport.
- `bbscope daemon` authenticates pollers once at startup and reuses them;
  re-auth only on auth-like errors.
- HackerOne, YesWeHack, and Intigriti interpolate `url.PathEscape`d handles
  into request paths.
- Duplicate identity keys in one upsert payload log a warning (first entry wins).
- Slack/Discord notification titles and usernames are escaped like other
  untrusted fields.

### Fixed
- `db get domains` no longer emits IP addresses or CIDR/IP ranges as domains.
- HackerOne program listing and Intigriti fetching no longer retry unbounded;
  the Intigriti handle maps are no longer subject to a data race.
- Worker/library code paths no longer call `log.Fatal`, so a single program's
  error cannot terminate the whole run (`pkg/scope` now warns and skips invalid
  `--output` flags instead of aborting).
- HackerOne scope-page retries are context-aware (same pattern as list retries).
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
