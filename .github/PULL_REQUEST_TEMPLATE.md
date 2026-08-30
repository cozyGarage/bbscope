## Description
<!-- A clear and concise description of the change. -->

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update
- [ ] Chore / refactor / CI

## Testing
<!-- How was this tested? Include commands and results. -->
- [ ] `go build ./...`
- [ ] `go test ./...`
- [ ] `golangci-lint run ./...`

## Related Issues
<!-- e.g. Fixes #123 -->

## Checklist
- [ ] Code follows the project style (`gofmt` / `goimports`)
- [ ] New functionality includes tests
- [ ] Documentation updated where applicable

## Subsystem review
<!-- See docs/CODE_REVIEW.md. Check only the areas this PR touches. -->
- [ ] **Storage** — wipe/sync guards, identity keys, `LIKE` escaping, platform aliases
- [ ] **Pollers** — host allowlists, no empty-success on parse failure, path-escaped handles, `dev` poller stays opt-in
- [ ] **HTTP / proxy** — `SetupProxy` does not mutate the default client; `--debug-http` redacts secrets
- [ ] **Credentials** — flag > keychain on subcommands; no new plaintext secrets
- [ ] **Notify** — untrusted fields escaped; webhook URLs are not metadata/link-local
- [ ] **Daemon** — pollers reused across ticks; `--db`/`--ai` flags still resolve

Review playbook: [docs/CODE_REVIEW.md](../docs/CODE_REVIEW.md)
