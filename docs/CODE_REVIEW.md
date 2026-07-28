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

### Still open (not in this PR)

1. **`SyncPlatformPrograms` hard-deletes targets** for programs missing from a (possibly partial) handle list. Only empty-list + count>10 is guarded. Prefer soft-disable / retention + stricter sync thresholds.
2. **Upsert TOCTOU**: read/diff outside the write transaction — concurrent polls can conflict.
3. **Identity key ≠ `NormalizeTarget` / case-sensitive UNIQUE** — can merge or duplicate rows incorrectly; program URLs not normalized on insert.
4. **AI normalizer** accepts arbitrary LLM-invented variant strings (prompt-only constraint).
5. **Notify HTML/markup injection** from untrusted scope fields (email worst).
6. **Immunefi / some pollers** treat parse/HTTP failures as empty success → sync risk for small DBs.
7. **`--since` flag** declared but unused.
8. **`pkg/validate` unused** by cmd/storage paths.
9. **YWH TOTP payload** built with `fmt.Sprintf` (prefer `json.Marshal`).

## Medium

- Shared `whttp` client `SetupProxy` mutates global transport (races / permanent `InsecureSkipVerify` after `--proxy`).
- AI path also forces `InsecureSkipVerify` when proxy set; unbounded response decode.
- Import still custom-only; CSV ignores in_scope/is_bbp columns.
- `db print --platform` docs say comma-separated; code is single equality.
- `GetStats` can inflate counts via AI join.
- TUI “Changes (24h)” is not time-filtered; errors never rendered.
- OTP ignores otpauth `period` / algorithm.

## What’s solid

- Parameterized SQL, DB-name validation, connection-string redaction.
- Scope-wipe guard on empty upsert; worker-pool poll tests.
- Keychain credentials package + config permission warnings.
- Platform poller interface consistency; H1/IT/YWH httptest coverage.
- Wildcard extraction pipeline well tested.

## Suggested next work

1. Soft-delete / safer `SyncPlatformPrograms` + partial-list thresholds.
2. Transactional upsert (read+write under one tx / row locks).
3. Align identity keys with normalized targets + case-insensitive uniqueness.
4. Escape all notify interpolations; validate AI variants against source URI.
5. Implement daemon by reusing `runPollWithPollers`, or keep hidden until ready.
