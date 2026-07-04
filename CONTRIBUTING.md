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

## Code style

Run `gofmt` before committing. The CI pipeline enforces it:

```bash
gofmt -l .          # list unformatted files
gofmt -w .          # fix all files
```

## Linting

This repo uses `golangci-lint`. Run it before opening a PR:

```bash
golangci-lint run ./...
```

The CI pipeline runs golangci-lint with the project configuration in `.golangci.yml`.

## Before submitting a PR

1. Ensure all existing tests pass: `go test ./...`
2. Add tests for new functionality.
3. Run `gofmt -w .` to format your code.
4. Run `golangci-lint run ./...` and resolve any issues.
5. Tests must not contact a real WiiM device or any real web APIs; use `httptest`, `net.Pipe`, or similar fakes.

## Commit messages

Follow conventional commit style:

```
feat: add spotify device transfer
fix: handle empty host in config
docs: update API endpoint reference
```

## Review

This project has a single maintainer. PRs are reviewed in order of submission. Be patient—responses may take a few days.
