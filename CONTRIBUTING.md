# Contributing to mewroute

Thank you for your interest in contributing!

## Requirements

- **Go 1.26+**
- Docker (optional, for container workflows)
- Node.js and **pnpm** (only for the Cloudflare Workers target in `worker/`)

## Development setup

```bash
git clone https://github.com/mewisme/mewroute.git
cd mewroute
go test ./...
go run ./cmd/mewroute
```

Set `ROOT_DIR` to point at `testdata/` or `data/` for local testing:

```bash
ROOT_DIR=./data PORT=8080 go run ./cmd/mewroute
```

### Worker

```bash
cd worker
pnpm install
pnpm typecheck
pnpm test
```

Routing behavior must stay in sync between Go (`internal/router`, `internal/config`) and TypeScript (`worker/src/resolve.ts`): when changing routing semantics, update both and add the case to both test suites.

## Pull request checklist

1. Fork the repository and create a feature branch
2. Run `gofmt -w .` on changed Go files
3. Add or update tests for behavior changes
4. Ensure `go test ./...` and `go vet ./...` pass (and `pnpm typecheck && pnpm test` in `worker/` if you touched it)
5. Update `CHANGELOG.md` under **Unreleased** when appropriate
6. Open a pull request with a clear description and test plan

## Commit messages

Use clear, imperative commit messages. Conventional prefixes are welcome (`feat:`, `fix:`, `docs:`, `test:`, `chore:`).

## Code style

- Keep changes focused and minimal
- Prefer stdlib; justify new dependencies in the PR
- Match existing package layout under `internal/`

## Security

Report security issues privately to the maintainers rather than opening a public issue.
