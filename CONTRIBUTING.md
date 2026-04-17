# Contributing to send.to

Thanks for your interest in contributing. The project is a small Go
file-sharing service with an Astro/React frontend.

## Development setup

You need:

- **Go** 1.24 or newer
- **Node.js** 20 or newer (for the web client)
- **Docker** (optional, for testing the container image)

Clone and bootstrap:

```bash
git clone https://github.com/sooua/send.to
cd send.to
go mod download
cd web && npm ci && cd ..
```

## Common tasks

The `Makefile` is the fastest way to run common workflows:

| Command            | What it does                                        |
| ------------------ | --------------------------------------------------- |
| `make build`       | Build the Go binary and the web bundle              |
| `make test`        | Run Go tests                                        |
| `make test-race`   | Run Go tests with the race detector (needs CGO/gcc) |
| `make coverage`    | Generate `coverage.out` and an HTML report          |
| `make lint`        | Run `golangci-lint` and (if present) the web linter |
| `make vuln`        | Run `govulncheck`                                   |
| `make dev`         | Run the Go server and the Astro dev server together |
| `make docker`      | Build the Docker image                              |
| `make pre-commit`  | `fmt` + `vet` + `lint` + `test`                     |

## Running locally

Backend only (default local storage):

```bash
go run . --provider local --basedir /tmp/send.to --listener :8080
```

Backend + web frontend:

```bash
make dev   # runs both in parallel
```

## Code style

- Go: standard `gofmt`. Keep functions small and prefer explicit error
  wrapping (`fmt.Errorf("context: %w", err)`).
- TypeScript / TSX: 2-space indent, no semicolons before tool config
  enforces them. Match surrounding style.
- Astro components: keep server-rendered markup in `.astro` and put
  client-only React behind island components (`client:load` /
  `client:visible`).

## Tests

- Add unit tests next to the code you change (`*_test.go`).
- Tests that need external services (S3, ClamAV, VirusTotal) should
  skip themselves with `t.Skip("requires X")` when the dependency is
  unavailable.
- Run `make pre-commit` before opening a PR.

## Pull request checklist

- [ ] `go build ./...` succeeds
- [ ] `make test` succeeds (and `make test-race` if you have gcc)
- [ ] `make lint` is clean
- [ ] `make vuln` is clean (or known issues are called out)
- [ ] Web changes: `cd web && npm run build` succeeds
- [ ] No secrets, credentials, or large binaries committed
- [ ] PR description explains the *why* — what user-visible problem
      does this fix or feature address?
- [ ] Security-sensitive changes are flagged in the PR description

## Commit messages

Short imperative subject line (≤ 70 chars), then an optional body
explaining the motivation. Don't reference the AI tool that helped you
write the patch — focus on the change itself.

## Reporting bugs

Use GitHub Issues for bugs that are **not** security-sensitive. For
security reports, see [SECURITY.md](./SECURITY.md).

When filing a bug include:

- send.to version (commit SHA or tag)
- Operating system and Go version
- Storage backend (`local`, `s3`, `gdrive`, `storj`)
- Minimal reproduction steps
- Expected vs. actual behaviour, including any logs

## License

By contributing you agree that your contribution will be released under
the project's [MIT License](./LICENSE).
