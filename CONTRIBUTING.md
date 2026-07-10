# Contributing

## Development setup

```bash
go build ./...      # verify compilation
go test ./...       # run all tests
```

A convenience Makefile is also available:

```bash
make test
make build
```

## Testing

Docs-only changes run CI because `TestSkillDocMentionsAllCommands` enforces command/skill
documentation consistency.

Tests must not contact a real WiiM device, a real Spotify API, or any other real network
service. Fake the WiiM/Spotify HTTP APIs with `httptest.NewServer`, and fake the Cast TLS
protocol with `net.Pipe` (see `cast_test.go` for the pattern). Table-driven tests are the
norm — most `_test.go` files follow `client_test.go`/`spotify_test.go` for the style to match.

```bash
go test ./...
go test ./... -run TestVolume -v   # narrow to one test while iterating
```

## Code style

Run `gofmt` before committing. The CI pipeline enforces it:

```bash
gofmt -l .          # list unformatted files
gofmt -w .          # fix all files
```

## Linting

This repo uses `golangci-lint` with `govet`, `staticcheck`, `errcheck`, `revive`, `misspell`,
and `gosec` enabled (see `.golangci.yml`). Run it before opening a PR:

```bash
golangci-lint run ./...
```

The one standing exception is `gosec` rule `G402` (TLS certificate verification) under
`internal/wiim/` — WiiM devices only ever present self-signed certificates on the LAN, so
skipping verification there is deliberate, not an oversight. Don't silence a new `gosec`
finding elsewhere without discussing it in the PR first.

## Before submitting a PR

1. Ensure all existing tests pass: `go test ./...`
2. Add tests for new functionality.
3. Run `gofmt -w .` to format your code.
4. Run `golangci-lint run ./...` and resolve any issues.
5. If you touched a WiiM/Linkplay API call, update `docs/api.md`.
6. If you touched credential storage, token handling, or `play-file`'s LAN exposure,
   update `docs/security.md`.

## Commit messages

Follow conventional commit style:

```
feat: add spotify device transfer
fix: handle empty host in config
docs: update API endpoint reference
```

## Review

This project has a single maintainer. PRs are reviewed in order of submission. Be patient—responses may take a few days.
