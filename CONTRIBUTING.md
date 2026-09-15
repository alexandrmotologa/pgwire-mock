# Contributing to PGWire-Mock

Thank you for contributing to PGWire-Mock. This guide explains how to set up your development environment, run tests, and propose changes.

## Development setup

### Prerequisites

- Go 1.22 or higher
- Git
- Optional: PostgreSQL client (`psql`) for live manual verification

### Clone and build

```bash
git clone https://github.com/alexandrmotologa/pgwire-mock.git
cd pgwire-mock
go build -v -o bin/pgwire-mock ./cmd/pgwire-mock
```

## Running tests

Run the complete test suite:

```bash
go test -v -race ./...
```

Run tests with coverage output:

```bash
go test -coverprofile=coverage.txt ./...
go tool cover -func=coverage.txt
```

Run linter checks:

```bash
go vet ./...
```

## Pull request workflow

1. Fork the repository on GitHub.
2. Create a feature branch with a descriptive name (`git checkout -b feature/my-feature`).
3. Write code with unit tests covering new protocol messages or mock behaviors.
4. Ensure all existing tests pass without regressions.
5. Commit your changes using conventional commit messages (e.g. `feat: add support for cancel request message`).
6. Push to your fork and submit a pull request against the `main` branch.

## Code style

- Follow standard Go conventions (`gofmt`, `go vet`).
- Keep binary protocol encoders and decoders covered by unit tests.
- Avoid external dependencies for core protocol functions. Use the Go standard library (`net`, `encoding/binary`, `bufio`) whenever possible.
