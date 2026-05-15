# Contributing to Go Bitcoin Paywall

Thank you for your interest in contributing to the Go Bitcoin Paywall project! This document provides guidelines and instructions for contributing.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Code Style Guidelines](#code-style-guidelines)
- [Testing Requirements](#testing-requirements)
- [Pull Request Process](#pull-request-process)
- [Security Disclosure Policy](#security-disclosure-policy)
- [Cryptocurrency Support Policy](#cryptocurrency-support-policy)

## Code of Conduct

We are committed to providing a welcoming and inclusive environment. Please be respectful and professional in all interactions.

## Getting Started

1. **Fork the repository** on GitHub
2. **Clone your fork** locally:
   ```bash
   git clone https://github.com/YOUR-USERNAME/paywall.git
   cd paywall
   ```
3. **Add upstream remote**:
   ```bash
   git remote add upstream https://github.com/opd-ai/paywall.git
   ```
4. **Create a feature branch**:
   ```bash
   git checkout -b feature/your-feature-name
   ```

## Development Setup

### Prerequisites

- Go 1.23.2 or later
- Git
- gofumpt (for code formatting)
- golangci-lint (for linting)

### Install Development Tools

```bash
# Install gofumpt
go install mvdan.cc/gofumpt@latest

# Install golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Install gosec (security scanner)
go install github.com/securego/gosec/v2/cmd/gosec@latest
```

### Build the Project

```bash
# Download dependencies
go mod download

# Build the project
go build -v ./...

# Build examples
cd example/bitcoin-only && go build -v .
```

### Run Tests

```bash
# Run all tests
go test ./...

# Run tests with race detection
go test -race ./...

# Run tests with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Code Style Guidelines

### Formatting

We use `gofumpt` for code formatting. Run before committing:

```bash
# Format all Go files
make fmt

# Or manually:
find . -name '*.go' -exec gofumpt -w -s -extra {} \;
```

### Code Quality Standards

- **Function length**: Keep functions under 50 lines when possible
- **Cyclomatic complexity**: Aim for complexity < 10 per function
- **Error handling**: Always wrap errors with context using `fmt.Errorf("context: %w", err)`
- **Comments**: Document all exported functions, types, and constants
- **Naming**: Follow standard Go naming conventions (CamelCase, short local variables)

### Linting

All code must pass `golangci-lint` and `go vet`:

```bash
# Run go vet
go vet ./...

# Run golangci-lint
golangci-lint run --timeout=5m
```

### Security Best Practices

- Use `crypto/rand` for all random number generation
- Never commit secrets, private keys, or credentials
- Validate all user inputs before processing
- Use constant-time comparison for sensitive data (use `subtle.ConstantTimeCompare`)
- Follow BIP standards for Bitcoin operations
- Always test with race detector enabled (`go test -race`)

## Testing Requirements

### Minimum Requirements

- **All new features** must include tests
- **Bug fixes** should include regression tests
- **Minimum coverage**: Maintain or improve overall coverage (target 70%+)
- **Race tests**: All tests must pass with `-race` flag
- **Table-driven tests**: Preferred for multiple test cases

### Test Structure

```go
func TestFeatureName(t *testing.T) {
    tests := []struct {
        name    string
        input   InputType
        want    OutputType
        wantErr bool
    }{
        {
            name:    "valid case",
            input:   validInput,
            want:    expectedOutput,
            wantErr: false,
        },
        {
            name:    "error case",
            input:   invalidInput,
            want:    OutputType{},
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := FunctionUnderTest(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("FunctionUnderTest() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("FunctionUnderTest() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Critical Test Areas

For security-critical changes (wallet operations, escrow, multisig, signatures):
- Test with multiple scenarios including edge cases
- Include negative tests (invalid inputs, attack scenarios)
- Test concurrent access patterns
- Verify proper error handling
- Test both testnet and mainnet configurations (where applicable)

## Pull Request Process

### Before Submitting

1. **Update your branch** with latest upstream:
   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

2. **Run all checks locally**:
   ```bash
   # Format code
   make fmt
   
   # Run tests
   go test -race ./...
   
   # Run linters
   go vet ./...
   golangci-lint run
   ```

3. **Update documentation** if needed:
   - Update README.md for new features
   - Update API documentation in docs/
   - Add comments to exported functions

4. **Write clear commit messages**:
   ```
   Short summary (50 chars or less)
   
   More detailed explanation if needed. Wrap at 72 characters.
   Explain what changed and why, not how.
   
   Fixes #123
   ```

### Submitting the PR

1. **Push to your fork**:
   ```bash
   git push origin feature/your-feature-name
   ```

2. **Create Pull Request** on GitHub with:
   - Clear description of changes
   - Reference to related issues
   - Test results and coverage impact
   - Screenshots/examples for UI changes

3. **PR Checklist**:
   - [ ] Code follows style guidelines
   - [ ] All tests pass (`go test -race ./...`)
   - [ ] New tests added for new features
   - [ ] Documentation updated
   - [ ] No sensitive information committed
   - [ ] Commits are logically organized
   - [ ] PR description is clear and complete

### Review Process

- Maintainers will review your PR within a few days
- Address feedback by pushing new commits
- Keep discussion constructive and focused on code quality
- Once approved, maintainers will merge your PR

## Security Disclosure Policy

**DO NOT** open public issues for security vulnerabilities.

For security issues:

1. **Email the maintainers** with:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact assessment
   - Suggested fix (if you have one)

2. **Wait for response** before public disclosure
3. **Coordinate disclosure timeline** with maintainers
4. **Credit** will be given in security advisories

Security researchers following responsible disclosure will be acknowledged in:
- Security advisories
- Release notes
- Project documentation

## Cryptocurrency Support Policy

From the project README:

> Re: support for other cryptocurrency, we will consider other currencies, but we consider Monero to be the only good cryptocurrency.
> 
> This is because Monero **is** the only good cryptocurrency.
> 
> Bitcoin is supported out of expediency, Ethereum may also be worth supporting.
> We're not going to focus on shitcoins.

When proposing cryptocurrency additions:
- **Monero**: Highest priority, always welcome
- **Bitcoin**: Improvements welcome (supported for compatibility)
- **Ethereum**: May be considered if well-justified
- **Other coins**: Unlikely to be accepted unless exceptional justification

Focus contributions on:
- Improving existing Bitcoin/Monero support
- Security enhancements
- Privacy improvements
- Usability and documentation

## Questions?

- Open a discussion on GitHub
- Check existing issues and documentation
- Review the [ROADMAP.md](ROADMAP.md) for planned features

Thank you for contributing to making cryptocurrency accessible to creators! 🚀
