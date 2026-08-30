# Code Review Playbook

How to review changes to `bbscope`, and the current findings backlog.

The [Security Audit](SECURITY_AUDIT.md) is a historical assessment. **This file is the live review process and backlog.** Report new vulnerabilities privately per [SECURITY.md](../SECURITY.md).

## Merge bar

A change is ready to merge when:

1. CI is green: test (with Postgres), lint, gosec, `go mod tidy`, cross-build. `govulncheck` is advisory.
2. The PR template checklists for **touched subsystems** are filled in (see below).
3. New behavior has tests. Platform pollers use `httptest`, not live network.
4. No Critical/High finding is introduced. Medium findings need an explicit “accepted” note or a follow-up.

CI already covers formatting, obvious vet/staticcheck issues, race detector, and a gosec pass with documented excludes. Humans still own data-loss paths, auth/proxy/TLS, redirect allowlists, notify markup, and credential precedence.

## Severity

| Level | Meaning | Examples |
|-------|---------|----------|
| Critical | Immediate data loss or credential theft in default use | Scope wipe, mass program disable, `--debug-http` dumping tokens |
| High | Security or integrity bug with a realistic trigger | Open redirect with auth attached, shared HTTP transport races, notify mention injection |
| Medium | Wrong defaults, silent empty-success, flag/config mismatch | Alias drift (`bugcrowd` vs `bc`), daemon re-auth every tick, duplicate identity keys dropped quietly |
| Low | Duplication, coverage gaps, docs drift | DRY of identical SQL, TUI simulated polling |

**Accepted risks** (do not re-litigate unless the code changed):

- TLS verification is skipped only when the user sets `--proxy` or `ai.insecure_skip_verify` (see [SECURITY.md](../SECURITY.md)).
- SHA-1 is used only for classic TOTP (RFC 6238).
- `db shell` passes the password via `PGPASSWORD`, not the process command line.
- Parent `bbscope poll` and `daemon` read credentials from keychain then config. They have **no** per-platform `--token` flags; subcommands (`poll h1`, `poll bc`, …) use `flagOrCredential` (flag > keychain > config).
- `config migrate` copies secrets into the keychain and **leaves YAML values in place**. Keychain wins at read time; delete plaintext from `~/.bbscope.yaml` yourself.
- Config file mode `0644` warns; it does not fail closed (a CLI that would not start after `chmod` is worse than a warning).
- Custom webhook URLs are operator-configured. Loopback (`127.0.0.1`) is allowed for local ntfy/gotify. Cloud metadata addresses (`169.254.169.254`, link-local) are rejected.

## Review areas (no CODEOWNERS)

Route review by path, not GitHub handles:

| Area | Paths |
|------|-------|
| Storage / SQL | `pkg/storage/` |
| Poll orchestration / daemon | `cmd/poll.go`, `cmd/poll_build.go`, `cmd/daemon.go` |
| Platform pollers | `pkg/platforms/`, `cmd/poll_*.go` |
| HTTP / proxy / TLS | `pkg/whttp/`, Bugcrowd’s private retry client |
| Credentials / config | `pkg/credentials/`, `cmd/config.go`, `cmd/root.go` |
| Notify | `pkg/notify/`, `cmd/notify.go` |
| AI | `pkg/ai/` |
| TUI | `pkg/tui/`, `cmd/tui.go` |
| Import / export / diff | `cmd/db_import.go`, `cmd/db_export.go`, `cmd/db_diff.go` |

## Subsystem checklists

### Storage

- [ ] Upsert and sync run in one transaction with `FOR UPDATE` on the program row.
- [ ] Empty incoming scope against existing targets returns `ErrAbortingScopeWipe`.
- [ ] `SyncPlatformPrograms` soft-disables; it does not delete targets.
- [ ] Partial listings cannot disable half or more of a platform (`shouldAbortPartialSync`; full wipe always aborted).
- [ ] Identity uses `NormalizeTarget` + `NormalizeCategory`. Duplicate keys in one payload are dropped with a warning, first entry wins.
- [ ] User-controlled `LIKE` patterns are escaped (`%`, `_`, `\`).
- [ ] `--platform` filters accept both short names (`h1`) and long names (`hackerone`) via `platforms.MatchingNames`.

### Platform pollers

- [ ] Response-derived URLs (`links.next`, `redirect_to`, `targets_url`) stay on an allowlisted host and HTTPS where required.
- [ ] Parse/HTTP failures return errors, not empty success (empty success can look like “program now has zero scope”).
- [ ] Skips (404, compliance gate, missing id) log a warning.
- [ ] Handles interpolated into URL paths are `url.PathEscape`d.
- [ ] The `dev` poller is opt-in only (`poll dev` or `--platforms dev`).

### HTTP / proxy / TLS

- [ ] `SetupProxy` clones a client; it must not mutate the unproxied default transport.
- [ ] Platform HTTP keeps `InsecureSkipVerify == false`.
- [ ] `--debug-http` redacts `Authorization`, `Cookie`, `Set-Cookie`, and auth-shaped bodies.

### Credentials

- [ ] Subcommands: explicit flag overrides keychain.
- [ ] Aggregate `poll` / `daemon`: keychain then config (no per-platform flags on those commands).
- [ ] New credential keys are listed in `credentials.ListKeys` and `bbscope config` help.

### Notify

- [ ] Untrusted fields (platform, target, category, type, titles, usernames) are escaped for the destination markup.
- [ ] Discord `allowed_mentions.parse` is empty.
- [ ] Webhook URLs are `http`/`https` only; cloud metadata / link-local IPs are rejected. Loopback is allowed.

### AI

- [ ] `variantAllowed` rejects parent domains, bare eTLDs, and unrelated hosts.
- [ ] Response size is capped; `ai.insecure_skip_verify` is opt-in.

### Daemon / TUI

- [ ] Daemon authenticates pollers once at startup and reuses them; re-auth on auth failures only.
- [ ] Daemon merges `poll` persistent flags (`--db`, `--ai`, concurrency) before `runPollWithPollers`.
- [ ] TUI DB loads should not hang shutdown forever (today they use `context.Background()` — accepted Low unless you are wiring real polling).

### Import / export

- [ ] JSON and CSV parse by header name; `in_scope` / `is_bbp` are honored.
- [ ] Default import is merge-only; `--replace` can delete targets missing from the file.

## What CI does not catch

- Semantic wipe/sync tradeoffs.
- Host allowlists on new pagination fields.
- Markup injection in a newly added notifier field.
- Flag vs keychain precedence on a new command.
- Stale comments that claim `InsecureSkipVerify` is on whenever a proxy is set.

## gosec excludes

Documented in [`.golangci.yml`](../.golangci.yml):

| Rule | Why excluded |
|------|----------------|
| G104 | Errors not checked — audited case by case; many are deferred `Close` |
| G101 | `"credential"` constants are keychain *key names*, not secrets |
| G115 | Integer conversion noise (TOTP time step, status codes) |
| G201 | SQL string formatting — builders use parameterized placeholders only |
| G204 | `db shell` launches `psql` with a validated connection |
| G304 | File paths from user flags (`--pid-file`, import files) are expected |
| G402 | TLS skip is opt-in (`--proxy` / `ai.insecure_skip_verify`) |
| G505 | SHA-1 required for classic TOTP |
| G701 | `CREATE DATABASE` uses `validateDatabaseName` + `pgx.Identifier.Sanitize` |

Do not add a new exclude without a one-line rationale here and in `.golangci.yml`.

---

## Current findings (2026-08-30)

Review of `main` against this playbook, with high/medium items fixed in the accompanying PR.

### Open / follow-ups

| ID | Severity | Status | Notes |
|----|----------|--------|-------|
| F1 | Medium | Open | Optional unique indexes on *canonical* target/URL expressions after schema v2 proves clean on real databases |
| F2 | Low | Open | `OpenWithPool` auto-creates a missing database — surprising in some prod setups; leaving as-is to avoid breaking existing `Open` callers |
| F3 | Low | Open | Duplicated AI-variant reassignment SQL in `reassignProgramTargets` / `mergeDuplicateTargets` — DRY candidate, not a correctness bug |
| F4 | Low | Open | TUI polling is simulated; search is a placeholder (`docs/TUI_ARCHITECTURE.md`) |
| F5 | Low | Open | No live-network platform contract tests (all pollers are httptest). Refresh fixtures when HTML/RSC shapes change |
| F6 | Low | Open | No container image scan or secret scan in CI (`govulncheck` is non-blocking) |
| F7 | Low | Accepted | `config migrate` leaves plaintext YAML; documented above |
| F8 | Low | Accepted | Two-program platforms: a single genuine removal is allowed (`activeCount < 3` skips the 50% ratio; full wipe still aborted) |

### Fixed in this review

| Issue | Severity | Notes |
|-------|----------|-------|
| `whttp.SetupProxy` mutated the process-global transport | High | Clone + atomic swap; default client transport is immutable after init |
| Daemon rebuilt/authenticated pollers on every tick | High | Auth once at startup; rebuild only on auth-like errors |
| Platform aliases duplicated and `db --platform bugcrowd` missed `bc` rows | Medium | Shared `pkg/platforms` canonical map; storage filters expand aliases |
| Slack/Discord titles and usernames unescaped | Medium | Same escaping as payload fields |
| Custom webhook URL had no scheme/host policy | Medium | `http`/`https` only; reject cloud metadata and link-local; allow loopback |
| HackerOne / YesWeHack / Intigriti handles concatenated into URL paths | Medium | `url.PathEscape` |
| Bugcrowd 404 / Intigriti missing-id looked like empty success | Medium | Warning (BC 404 skip); missing Intigriti id is an error |
| Duplicate identity keys in one upsert payload dropped silently | Medium | First entry wins; warning logged |
| Stale “import is custom-only / CSV ignores in_scope” | — | Already implemented; marked stale |

### Stale (do not re-open)

- Import still custom-only; CSV ignores `in_scope` / `is_bbp` — false; see `cmd/db_import.go`.
- Docker runs as root — false; `Dockerfile` uses `USER bbscope` on Alpine 3.19.
- `pkg/validate.Platform` unused in production — now backed by the shared alias map and used for validation; storage/poll filters use the same map.

---

## Prior reviews

### 2026-07-28 quality-uplift (and follow-up commits)

Critical/High items from that pass are **Fixed**: keychain-first parent poll, H1/BC redirect allowlists, fake Bugcrowd 2FA targets, multi-platform continue-on-error, `db import` JSON fallback, email notifier empty `to:`, OTP digit clamp, `--debug-http` redaction, daemon stub, `db ignore` LIKE escape, soft-disable sync, upsert `FOR UPDATE`, identity normalization, AI `variantAllowed`, notify HTML escaping, Immunefi/YWH/IT empty-success on parse failure, daemon flag-merge, OTP base32 regression, Discord/Slack field escaping, bad `--proxy` fatal on poll-all, libpq DSN validation.

See git history around those PRs for the original write-up.
