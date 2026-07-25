# AGENTS.md

## Cursor Cloud specific instructions

`bbscope` is a single Go 1.24 CLI (module `github.com/cozyGarage/bbscope/v2`). There is no long-running server; you build one binary and run subcommands. Standard commands live in the `Makefile`, `README.md`, and `docs/CONTRIBUTING.md` — use those; notes below only cover non-obvious caveats.

### Build / test / lint / run
- Build: `go build -o bbscope .` (or `make build`).
- Test: `go test ./...` (or `make test` for `-race -cover`). Note: `pkg/whttp` tests take ~3 minutes because they exercise real retry/backoff timeouts — this is expected, not a hang.
- Lint: `golangci-lint` is installed to `~/go/bin`, which is not on `PATH` by default — run `PATH=$PATH:~/go/bin golangci-lint run ./...` (or `make lint`). Lint currently reports pre-existing findings in the repo; a nonzero exit does not mean your environment is broken.
- Run: `./bbscope --help`. The hidden `./bbscope poll dev` command emits deterministic sample scope with no network or credentials — use it to exercise polling and the `--db` path end to end. Real platform polling (`poll h1|bc|it|ywh|immunefi`) needs internet egress and, except Immunefi, credentials configured via `bbscope config` / `~/.bbscope.yaml` / flags.

### Database (optional features: `--db`, `db ...`, `daemon`)
- DB features require PostgreSQL. It is installed via apt but the service is NOT started automatically on boot. Start it with: `sudo pg_ctlcluster 16 main start`.
- A local role/db is provisioned: role `bbscope` / password `devpass` / database `bbscope`. Connection is over TCP (`127.0.0.1:5432`), not the default peer-auth socket.
- Config lives at `~/.bbscope.yaml` with `db_url: "postgres://bbscope:devpass@127.0.0.1:5432/bbscope?sslmode=disable"`. Tables are auto-created on first `--db` run.
- The `dev` poller is deterministic, so a second `poll dev --db` reports no changes; use `db add`, `db get`, `db stats`, `db find` to inspect stored data.
