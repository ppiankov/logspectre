# Contributing to logspectre

## Prerequisites

- Go 1.24+
- golangci-lint

## Development

make build    # Build binary
make test     # Run tests with -race
make lint     # Run linter
make fmt      # Format code

## Pull Requests

- Use conventional commits (feat:, fix:, docs:, test:, refactor:, chore:)
- All tests must pass with -race
- Lint must be clean
