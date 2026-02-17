# Contributing to kube-freeze-operator

Thank you for your interest in contributing to kube-freeze-operator! We welcome contributions from the community.

## How to Contribute

### Reporting Bugs

If you find a bug, please open an issue on our issue tracker with:

- A clear description of the problem
- Steps to reproduce the issue
- Expected vs actual behavior
- Environment details (Kubernetes version, operator version, etc.)

### Suggesting Enhancements

We welcome feature suggestions! Please open an issue with:

- A clear description of the enhancement
- Use cases and benefits
- Any relevant examples or mockups

### Pull Requests

1. **Fork the repository** and create your branch from `main`
2. **Make your changes** following the code style guidelines
3. **Add tests** if applicable
4. **Update documentation** as needed
5. **Ensure tests pass**: `make test`
6. **Run linter**: `make lint-fix`
7. **Submit a pull request**

#### Pull Request Guidelines

- Keep changes focused and atomic
- Write clear commit messages
- Reference related issues in your PR description
- Ensure CI checks pass
- Update CHANGELOG.md if applicable

### Development Setup

```bash
# Clone the repository
git clone https://github.com/jamalshahverdiev/kube-freeze-operator.git
cd kube-freeze-operator

# Install dependencies
go mod download

# Run tests
make test

# Run linter
make lint

# Build locally
make build
```

### Code Style

- Follow standard Go formatting (`gofmt`, `goimports`)
- Use meaningful variable and function names
- Add comments for exported functions and complex logic
- Keep functions small and focused
- Write tests for new functionality

### Testing

- Unit tests: `make test`
- E2E tests: `make test-e2e` (requires Kind cluster)
- Coverage: `make test-cover`

### Commit Message Format

We follow conventional commit format:

```txt
<type>(<scope>): <subject>

<body>

<footer>
```

Types:

- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

Example:

```txt
feat(controller): add CronJob suspend/resume functionality

Implement automatic CronJob suspension during active freeze periods
with annotation-based state tracking and conflict prevention.

Closes #123
```

### Code Review Process

1. Maintainers will review your PR within a few days
2. Address any feedback or requested changes
3. Once approved, a maintainer will merge your PR

### Community

- Be respectful and inclusive
- Help others learn and grow
- Follow the [Code of Conduct](CODE_OF_CONDUCT.md)

## Development Workflow

### Working with CRDs

After modifying `*_types.go` files:

```bash
make manifests  # Regenerate CRDs and RBAC
make generate   # Update generated code
```

### Working with Controllers

After modifying controller logic:

```bash
make test       # Run unit tests
make lint-fix   # Auto-fix code style
```

### Local Testing

```bash
# Run operator locally (uses current kubeconfig)
make run

# Or deploy to cluster
make deploy IMG=<your-registry>/kube-freeze-operator:dev
```

## Questions?

Feel free to open an issue for questions or reach out to the maintainers.

Thank you for contributing! 🚀
