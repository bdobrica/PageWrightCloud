# Contributing to PageWright Storage Service

Thank you for your interest in contributing to the PageWright Storage Service!

## Development Setup

1. **Prerequisites**
   - Go 1.22 or later
   - Docker and Docker Compose
   - Make (recommended)
   - Git

2. **Clone and Setup**
   ```bash
   git clone https://github.com/PageWrightCloud/PageWrightCloud.git
   cd PageWrightCloud/pagewright/storage
   go mod download
   ```

3. **Run Tests**
   ```bash
   make test-unit
   ```

4. **Start Local Environment**
   ```bash
   make docker-up
   ```

## Project Structure

```
pagewright/storage/
├── cmd/server/              # Application entry point
├── internal/
│   ├── api/                # HTTP handlers
│   ├── config/             # Configuration
│   └── storage/            # Storage backends
│       ├── backend.go      # Interface definition
│       └── nfs/            # NFS implementation
├── test/integration/       # Integration tests
├── examples/               # Usage examples
└── docs/                   # Additional documentation
```

## Adding a New Storage Backend

To add a new storage backend (e.g., S3):

1. Create a new package: `internal/storage/s3/`

2. Implement the `Backend` interface:
   ```go
   type Backend interface {
       StoreArtifact(siteID, buildID string, reader io.Reader) error
       FetchArtifact(siteID, buildID string) (io.ReadCloser, error)
       WriteLogEntry(siteID string, entry *LogEntry) error
       ListVersions(siteID string) ([]*Version, error)
   }
   ```

3. Add configuration in `internal/config/config.go`

4. Register in `cmd/server/main.go`:
   ```go
   case "s3":
       backend, err = s3.NewS3Backend(cfg)
   ```

5. Add tests in `internal/storage/s3/s3_test.go`

6. Update documentation

## Coding Standards

### Go Style

- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Use `gofmt` for formatting: `make fmt`
- Run `go vet`: `make vet`
- Write tests for all new code

### Testing

- **Unit tests**: Test individual components in isolation
- **Integration tests**: Test with real dependencies (NFS, etc.)
- Target: >80% code coverage
- Use table-driven tests where appropriate

### Commits

- Write clear, descriptive commit messages
- Format: `component: brief description`
- Examples:
  - `storage/nfs: add retry logic for transient failures`
  - `api: add pagination support for version listing`
  - `tests: improve integration test reliability`

### Pull Requests

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/my-feature`
3. Make your changes
4. Write/update tests
5. Ensure all tests pass: `make test`
6. Format code: `make fmt`
7. Commit your changes
8. Push and create a PR

## Testing Guidelines

### Writing Tests

```go
func TestMyFeature(t *testing.T) {
    // Arrange
    backend := setupTestBackend(t)
    
    // Act
    err := backend.DoSomething()
    
    // Assert
    assert.NoError(t, err)
}
```

### Running Tests

```bash
# Unit tests only
make test-unit

# Integration tests (requires docker-up)
make test-integration

# All tests
make test

# With coverage report
make coverage
```

## API Changes

When modifying the API:

1. Update the handler in `internal/api/handler.go`
2. Add/update tests in `internal/api/handler_test.go`
3. Update the README.md with new endpoint documentation
4. Add examples to `examples/demo.sh` if applicable
5. Consider backward compatibility

## Documentation

- Update README.md for user-facing changes
- Update TODO.md to track future work
- Add inline comments for complex logic
- Update QUICKSTART.md if setup changes

## Performance Considerations

- Atomic writes are required for all storage operations
- Avoid holding locks for extended periods
- Stream large files instead of loading into memory
- Consider adding metrics for monitoring

## Security

- Never log sensitive information
- Validate all input parameters
- Prevent path traversal attacks
- Use secure defaults

## Questions?

- Open an issue for discussion
- Check existing issues/PRs
- Read the main README.md

## License

By contributing, you agree that your contributions will be licensed under the same license as the project (see LICENSE file).
