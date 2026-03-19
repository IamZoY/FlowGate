# Contributing to FlowGate

Thank you for your interest in contributing to FlowGate! This document provides guidelines and instructions for contributing.

## Code of Conduct

Be respectful, constructive, and inclusive. We are all here to build something useful together.

## Getting Started

### Prerequisites

- [Go 1.24+](https://go.dev/dl/)
- [Docker](https://docs.docker.com/get-docker/) (for integration testing)
- [MinIO Client (`mc`)](https://min.io/docs/minio/linux/reference/minio-mc.html) (optional, for manual testing)
- Git

### Development Setup

```bash
# Fork and clone
git clone https://github.com/<your-username>/flowgate.git
cd flowgate

# Install dependencies
go mod download

# Build
make build

# Run tests
make test

# Start the full dev environment (FlowGate + 2 MinIO instances)
docker compose up -d
```

### Project Structure

```
cmd/flowgate/         Entry point
internal/
  config/             YAML configuration loading
  server/             HTTP server, router, middleware
  webhook/            MinIO webhook event handling
  group/              Group/App models, AES encryption
  transfer/           Worker pool and job processing
  storage/            SQLite persistence, MinIO client
  dashboard/          REST API handlers
  hub/                WebSocket pub/sub
web/assets/           Embedded SPA (HTML/JS/CSS)
```

## How to Contribute

### Reporting Bugs

1. Check [existing issues](https://github.com/ali/flowgate/issues) to avoid duplicates.
2. Open a new issue with:
   - A clear, descriptive title
   - Steps to reproduce
   - Expected vs. actual behavior
   - FlowGate version (`flowgate --version` or git commit)
   - OS and architecture

### Suggesting Features

Open an issue with the `enhancement` label. Describe:
- The problem you're trying to solve
- Your proposed solution
- Any alternatives you've considered

### Submitting Code

1. **Fork** the repository and create a branch from `main`:
   ```bash
   git checkout -b feature/my-feature
   ```

2. **Write your code.** Follow the conventions below.

3. **Add tests** for new functionality.

4. **Run checks locally:**
   ```bash
   make test
   make build
   ```

5. **Commit** with a clear message:
   ```
   Add webhook retry with exponential backoff

   Implements configurable retry logic for failed webhook deliveries.
   Default: 3 attempts with 5s base backoff.
   ```

6. **Push** and open a Pull Request against `main`.

## Code Conventions

### Go

- Follow standard [Go conventions](https://go.dev/doc/effective_go) and `gofmt`.
- Use `slog` for all logging (no `fmt.Println` or `log.Println`).
- Keep packages focused: one responsibility per package.
- Public functions must have doc comments.
- Error messages should be lowercase, no trailing punctuation.

### Commits

- Use the imperative mood: "Add feature" not "Added feature".
- Keep the first line under 72 characters.
- Reference issues when applicable: `Fixes #42`.

### Pull Requests

- Keep PRs focused — one feature or fix per PR.
- Update documentation if your change affects user-facing behavior.
- Ensure CI passes before requesting review.
- Squash fixup commits before merging.

## Testing

```bash
# Run all tests
make test

# Run tests with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

### Writing Tests

- Place tests in `_test.go` files alongside the code they test.
- Use table-driven tests where appropriate.
- Test edge cases: empty inputs, nil pointers, concurrent access.
- Use `t.Helper()` in test helpers.

## Release Process

Releases are automated via GitHub Actions when a version tag is pushed:

```bash
# Bump version (updates VERSION file)
make bump-patch   # 0.1.0 → 0.1.1
make bump-minor   # 0.1.0 → 0.2.0
make bump-major   # 0.1.0 → 1.0.0

# Tag and push (triggers release workflow)
make tag
```

The release workflow:
1. Runs the full test suite
2. Cross-compiles for Linux (amd64/arm64), macOS (amd64/arm64), and Windows (amd64)
3. Generates SHA-256 checksums
4. Creates a GitHub Release with all artifacts

## Questions?

Open an issue or start a [Discussion](https://github.com/ali/flowgate/discussions). We're happy to help!
