# Contributing to OC Mirror Test Automation

Thank you for your interest in contributing to this project! This document provides guidelines and instructions for contributing.

## Code of Conduct

- Be respectful and inclusive
- Welcome newcomers and help them learn
- Focus on constructive feedback
- Respect different viewpoints and experiences

## Getting Started

1. **Fork the repository**
2. **Clone your fork**:
   ```bash
   git clone https://github.com/your-username/NGC-495.git
   cd NGC-495
   ```

3. **Create a branch**:
   ```bash
   git checkout -b feature/your-feature-name
   ```

4. **Set up development environment**:
   ```bash
   make install
   ```

## Development Workflow

### Before You Start

- Check existing issues and pull requests to avoid duplicate work
- For major changes, open an issue first to discuss the approach
- Ensure you can build and test the project locally

### Making Changes

1. **Write code** following the project's style guidelines
2. **Add tests** for new functionality
3. **Update documentation** as needed
4. **Run tests and checks**:
   ```bash
   make all
   ```

### Code Style

- Follow Go standard formatting (`go fmt`)
- Use meaningful variable and function names
- Add comments for exported functions and types
- Keep functions focused and small
- Handle errors explicitly

### Testing

- Write unit tests for new functionality
- Ensure all tests pass: `make test`
- Aim for good test coverage
- Test edge cases and error conditions

### Commit Messages

Use clear, descriptive commit messages:

```
Add v1 vs v2 comparison feature

- Implement comparison logic in runner package
- Add --compare-v1-v2 flag
- Update documentation
```

### Pull Request Process

1. **Update your branch**:
   ```bash
   git fetch origin
   git rebase origin/main
   ```

2. **Ensure all checks pass**:
   ```bash
   make all
   ```

3. **Push your changes**:
   ```bash
   git push origin feature/your-feature-name
   ```

4. **Create a Pull Request**:
   - Provide a clear description of changes
   - Reference related issues
   - Include screenshots/output if applicable
   - Ensure CI checks pass

### PR Checklist

- [ ] Code follows project style guidelines
- [ ] Tests added/updated and passing
- [ ] Documentation updated
- [ ] Commit messages are clear
- [ ] No merge conflicts
- [ ] All CI checks passing

## Project Structure

- `cmd/`: Application entry points
- `pkg/`: Public packages (can be imported by other projects)
- `internal/`: Private packages (internal use only)
- `bin/`: Built binaries (gitignored)
- `results/`: Test results (gitignored)

## Areas for Contribution

- **Bug fixes**: Fix issues reported in GitHub Issues
- **Features**: Implement features from the roadmap
- **Documentation**: Improve README, add examples, fix typos
- **Tests**: Increase test coverage
- **Performance**: Optimize code and improve metrics collection
- **CI/CD**: Improve build and test automation

## Reporting Issues

When reporting issues, please include:

- **Description**: Clear description of the issue
- **Steps to Reproduce**: Detailed steps to reproduce
- **Expected Behavior**: What should happen
- **Actual Behavior**: What actually happens
- **Environment**: OS, Go version, oc-mirror version
- **Logs**: Relevant log output or error messages
- **Screenshots**: If applicable

## Feature Requests

For feature requests:

- **Use Case**: Describe the use case
- **Proposed Solution**: How you envision it working
- **Alternatives**: Other solutions you've considered
- **Additional Context**: Any other relevant information

## Questions?

- Open an issue for questions or discussions
- Check existing issues and documentation first
- Be patient - maintainers are volunteers

## License

By contributing, you agree that your contributions will be licensed under the same license as the project (Apache License 2.0).

Thank you for contributing! 🎉
