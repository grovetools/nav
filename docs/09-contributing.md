# Contributing to gmux

This guide explains how to set up a development environment, run tests, follow code style conventions, and submit pull requests. It also summarizes the CI/CD pipeline.

## Development setup

Prerequisites:
- Go 1.24.x (the CI uses 1.24.4)
- Git
- tmux (recommended for running end-to-end tests locally)

Steps:
1. Clone and enter the repository
   ```bash
   git clone https://github.com/mattsolo1/grove-tmux.git
   cd grove-tmux
   ```
2. Build
   ```bash
   make clean        # recommended when switching branches
   make build        # binary is written to ./bin/gmux
   ```
3. Run the CLI
   - Directly:
     ```bash
     ./bin/gmux version
     ```
   - Or via the Makefile:
     ```bash
     make run ARGS="version"
     ```
4. Optional development build with race detector
   ```bash
   make dev
   ```

Notes:
- Binaries are created in ./bin and are managed by the Grove tooling. Do not copy them elsewhere on your PATH.
- Version information is injected at build time via LDFLAGS.

## Running tests

- Unit tests
  ```bash
  make test
  ```
- End-to-end (E2E) tests
  ```bash
  make test-e2e
  ```
  The E2E runner is built to ./bin/tend and will execute scenarios defined under tests/e2e. Tmux-based scenarios are skipped automatically if tmux is not installed.

Useful commands (from the local E2E runner and tooling):
- List available E2E scenarios:
  ```bash
  ./bin/tend list
  ```
- Run a specific scenario interactively, e.g.:
  ```bash
  make test-e2e ARGS="run -i gmux-launch"
  ```

## Code style and quality

- Format code:
  ```bash
  make fmt
  ```
- Static checks:
  ```bash
  make vet
  ```
- Lint (runs only if golangci-lint is installed):
  ```bash
  make lint
  # Install if needed:
  # go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
  ```
- Run all checks:
  ```bash
  make check   # fmt + vet + lint + test
  ```

## Pull request process

1. Fork the repository and create a feature branch:
   ```bash
   git checkout -b feat/your-topic
   ```
2. Make changes with clear, scoped commits.
3. Keep dependencies tidy if you change them:
   ```bash
   go mod tidy
   ```
4. Run the test suite locally:
   ```bash
   make test
   make test-e2e
   ```
5. Open a pull request against the main branch. Describe the motivation, approach, and any user-facing changes. Link related issues if applicable.

Tips:
- Keep PRs focused and reasonably sized.
- Include tests for new behavior where practical.
- If your changes affect documentation or CLI behavior, update docs accordingly.

## Continuous integration (CI)

The CI workflow (.github/workflows/ci.yml) runs on pushes and pull requests to main. It:
- Checks out the code and disables Git LFS filters (to avoid smudge/clean issues)
- Sets up Go 1.24.4
- Configures Git for private modules (CI uses a token for GOPRIVATE dependencies)
- Refreshes dependencies:
  - Removes go.sum
  - Runs go mod download and go mod tidy
- Builds the project (make build)
- Builds the E2E test runner (make test-e2e-build)
- Runs E2E tests (make test-e2e)

Notes:
- Linting and unit tests are currently commented out in CI; you can still run them locally.
- Tmux-dependent E2E scenarios will be skipped automatically if tmux is not available on the runner.

## Release pipeline (for reference)

Tagging a version (vX.Y.Z) triggers the release workflow (.github/workflows/release.yml):
- Builds cross-platform binaries (darwin/amd64, darwin/arm64, linux/amd64, linux/arm64) via make build-all VERSION=<tag>
- Runs E2E tests
- Generates SHA-256 checksums
- Creates a GitHub Release and uploads artifacts

Contributors do not need to run this process; it is included here to understand how tagged releases are produced.