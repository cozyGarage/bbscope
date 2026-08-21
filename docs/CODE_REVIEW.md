# Code Review Findings (2026-07-28)

Review of `bbscope` after the quality-uplift work on `main`. This note captures remaining issues found during review. High-impact items addressed in the accompanying PR are marked **Fixed**.

## Critical / High

| Issue | Status | Notes |
|-------|--------|-------|
| Parent `bbscope poll` ignored OS keychain credentials (`viper` only); subcommands used `credentials.Get` | **Fixed** | Keychain-first auth + `--proxy` for H1/IT |
| HackerOne `links.next` followed off-origin with Basic auth attached | **Fixed** | Same-host allowlist |
| Bugcrowd `redirect_to` followed without host allowlist | **Fixed** | `*.bugcrowd.com` https only |
| Bugcrowd invented `2FA_REQUIRED` / `NO_IN_SCOPE_TABLE` fake targets | **Fixed** | Skip + warn instead |
| Multi-platform poll aborted on first program fetch error (also risky for Sync wipe) | **Fixed** | Continue other platforms; skip Sync when fetches fail |
| `db import` JSON array fallback never worked (reader already consumed) | **Fixed** | Read-all + dual unmarshal |
| Email notifier panics when `to:` is empty | **Fixed** | Guard + skip in `LoadNotifiers` |
| OTP `digits` unbounded → overflow / panic at 32 | **Fixed** | Clamp to 6–8 |
| `--debug-http` dumped auth bodies and `Set-Cookie` | **Fixed** | Body + header redaction |
| `bbscope daemon` advertised as working but was a no-op stub | **Fixed** | Fail fast with clear error |
| `db ignore` LIKE metacharacters (`%`/`_`) could mark many programs | **Fixed** | Escape user pattern |

### Addressed in follow-up commits

| Issue | Status |
|-------|--------|
| `SyncPlatformPrograms` hard-delete → soft-disable + partial-list ratio guard | **Fixed** |
| Upsert read/diff/write TOCTOU (single tx + program `FOR UPDATE`) | **Fixed** |
| Identity key uses `NormalizeTarget` + `NormalizeCategory`; program URLs normalized on upsert/sync | **Fixed** (no DB migration yet; trailing-slash reuse on program lookup) |
| AI variants validated as derived-from-original | **Fixed** |
| Notify HTML/markup escaping (email/telegram/slack/discord) | **Fixed** |

### Addressed in later commits

| Issue | Status |
|-------|--------|
| Immunefi/YWH/IT empty-success on parse/HTTP failures | **Fixed** |
| YWH TOTP payload `json.Marshal` | **Fixed** |
| `--since` on poll filters printed changes | **Fixed** |
| `pkg/validate` wired for `db_url` + `db add` targets; platform aliases | **Fixed** |
| `IncludeDisabled` on `ListOptions` (default false) | **Fixed** |
| Schema v2 canonicalize migration for program/target duplicates | **Fixed** |

### Addressed later

| Issue | Status |
|-------|--------|
| Daemon polling via shared `buildPollersFromConfig` + `runPollWithPollers` | **Fixed** |
| AI proxy TLS verify on by default (`ai.insecure_skip_verify` opt-in) + response size cap | **Fixed** |
| Parent `poll` includes Immunefi | **Fixed** |
| `db print --platform` comma-separated list | **Fixed** |
| `GetStats` / `SearchTargets` disabled filtering + distinct target counts | **Fixed** |
| TUI 24h changes are time-filtered; errors rendered | **Fixed** |
| OTP honors otpauth `period` / SHA256/SHA512 | **Fixed** |

### Addressed in review-follow-up commits

| Issue | Status |
|-------|--------|
| `bbscope daemon` invoked `pollCmd` directly, bypassing cobra's flag-merge; `--db`/`--ai`/concurrency silently read as zero on every daemon tick | **Fixed** |
| OTP `decodeBase32Flexible` regressed: dropped padded base32hex attempt and stopped trimming trailing `=`, breaking some previously-valid TOTP secrets | **Fixed** |
| Notify HTML/markup escaping was incomplete on Discord/Slack (`Type`/`Category`/Slack `Target` left raw, incl. Slack `<!everyone>` mention-injection risk) | **Fixed** |
| `variantAllowed` accepted a bare TLD/eTLD as a valid "derived" AI variant (e.g. `com` for `example.com`) | **Fixed** |
| Poll-all / daemon path treated a bad `--proxy` as a warning instead of aborting, unlike single-platform poll subcommands | **Fixed** |
| `validate.DatabaseURL` (wired into `GetDBConnectionString`) rejected libpq keyword/value DSNs that `storage.Open` accepts, breaking non-URL `db_url` configs | **Fixed** |

### Still open / follow-ups

1. Optional DB unique indexes on canonical expressions after migration proves clean in the wild.
2. Import still custom-only; CSV ignores in_scope/is_bbp columns.
3. Shared `whttp` client `SetupProxy` still mutates global transport.
4. Daemon re-authenticates Bugcrowd/YesWeHack from scratch on every poll tick instead of once at startup.
5. Duplicated platform-alias table (`cmd/poll_build.go` vs `pkg/validate`) and duplicated AI-variant-reassignment SQL (`reassignProgramTargets`/`mergeDuplicateTargets`) — candidates for extraction, not correctness bugs.

## Medium

- Shared `whttp` client `SetupProxy` mutates global transport (races / permanent `InsecureSkipVerify` after `--proxy`).
- ~~AI path also forces `InsecureSkipVerify` when proxy set; unbounded response decode.~~ **Fixed**
- Import still custom-only; CSV ignores in_scope/is_bbp columns.
- ~~`db print --platform` docs say comma-separated; code is single equality.~~ **Fixed** — comma-separated platform filter.
- `GetStats` can inflate counts via AI join. *(default list/search/stats now exclude soft-disabled programs)*
- ~~TUI “Changes (24h)” is not time-filtered; errors never rendered.~~ **Fixed**
- ~~OTP ignores otpauth `period` / algorithm.~~ **Fixed**

## What’s solid

- Parameterized SQL, DB-name validation, connection-string redaction.
- Scope-wipe guard on empty upsert; worker-pool poll tests.
- Soft-disable sync retention + partial-poll guard.
- Keychain credentials package + config permission warnings.
- Platform poller interface consistency; H1/IT/YWH/BC/Immunefi httptest coverage.
- Wildcard extraction pipeline well tested.

## Suggested next work

All items originally listed here (`--since` wiring, `pkg/validate` usage, daemon
implementation) have been addressed — see "Addressed in later commits" above.
Remaining open follow-ups are tracked under "Still open / follow-ups".
