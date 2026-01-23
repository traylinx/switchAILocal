# Contributing to switchAILocal

Thank you for your interest in contributing! We welcome help in improving this project.

## Development Setup

1. **Go Version**: Ensure you have Go 1.22+ installed.
2. **Clone the Repo**:
   ```bash
   git clone https://github.com/traylinx/switchAILocal.git
   cd switchAILocal
   ```
3. **Build**:
   ```bash
   go build ./cmd/switchAILocal
   ```

## Pull Request Process

1. Fork the repository and create your branch from `main`.
2. If you've added code that should be tested, add tests.
3. Ensure the test suite passes (`go test ./...`).
4. Format your code with `gofmt -s -w .`.
5. Update documentation if you are changing functionality.
6. Open a PR with a clear description of the changes.

## Security

Please do not report security vulnerabilities through public GitHub issues. See [SECURITY.md](SECURITY.md) for our reporting process.
