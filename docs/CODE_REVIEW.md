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

1. ~~**`SyncPlatformPrograms` hard-deletes targets**~~ **Fixed** — soft-disable only + partial-list abort (`ErrAbortingPartialSync`).
2. ~~**Upsert TOCTOU**~~ **Fixed** — single transaction with program/target `FOR UPDATE`.
3. ~~**Identity key ≠ `NormalizeTarget`**~~ **Fixed** — `identityKey` uses `NormalizeTarget` + `NormalizeCategory`; program URLs canonicalized on upsert/sync.
4. ~~**AI normalizer** accepts arbitrary LLM-invented variants~~ **Fixed** — `variantAllowed` + alternation expansion; TLS verify on by default; 10MiB response cap.
5. ~~**Notify HTML/markup injection**~~ **Fixed** — `pkg/notify/sanitize.go` + safe links in email/slack/discord/telegram.
6. ~~**Immunefi / some pollers** empty success~~ **Fixed** — Immunefi missing arrays error; IT/YWH non-2xx status errors.
7. **`--since` flag** declared but unused.
8. **`pkg/validate` unused** by cmd/storage paths.
9. ~~**YWH TOTP payload**~~ **Fixed** — `json.Marshal`.

## Medium

- Shared `whttp` client `SetupProxy` mutates global transport (races / permanent `InsecureSkipVerify` after `--proxy`).
- ~~AI path also forces `InsecureSkipVerify` when proxy set; unbounded response decode.~~ **Fixed**
- Import still custom-only; CSV ignores in_scope/is_bbp columns.
- `db print --platform` docs say comma-separated; code is single equality.
- `GetStats` can inflate counts via AI join. *(default list/search/stats now exclude soft-disabled programs)*
- TUI “Changes (24h)” is not time-filtered; errors never rendered.
- OTP ignores otpauth `period` / algorithm.

## What’s solid

- Parameterized SQL, DB-name validation, connection-string redaction.
- Scope-wipe guard on empty upsert; worker-pool poll tests.
- Soft-disable sync retention + partial-poll guard.
- Keychain credentials package + config permission warnings.
- Platform poller interface consistency; H1/IT/YWH/BC/Immunefi httptest coverage.
- Wildcard extraction pipeline well tested.

## Suggested next work

1. Wire `--since` into poll change printing.
2. Use `pkg/validate` on DB URL / `db add` inputs.
3. Optional unique indexes after identity migration proves clean.
4. Avoid global `whttp.SetupProxy` transport mutation.
5. Implement daemon by reusing `runPollWithPollers`, or keep hidden until ready.
