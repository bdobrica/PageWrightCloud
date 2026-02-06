# PageWright Storage Service

A RESTful storage service for managing site artifacts and version history, designed as part of the PageWright Cloud platform. This service provides a pluggable backend architecture with atomic write guarantees for safe concurrent operations.

## Overview

The storage service implements Phase 1 of the PageWright PoC, providing:

- **Artifact Storage**: Upload and download site build artifacts (tar.gz files)
- **Version Tracking**: Maintain a complete history of site builds
- **Atomic Operations**: All writes use atomic operations (write to temp + fsync + rename)
- **Pluggable Backends**: Support for multiple storage backends (NFS, S3, etc.)

## Architecture

```
pagewright/storage/
├── cmd/server/          # Application entry point
├── internal/
│   ├── api/            # HTTP handlers and routing
│   ├── config/         # Configuration management
│   └── storage/        # Storage backend interface and implementations
│       └── nfs/        # NFS backend plugin
├── test/
│   └── integration/    # Integration tests
├── Dockerfile          # Container image
├── docker-compose.yaml # Local development setup
└── Makefile           # Build and test automation
```

## Implementation

### Storage Backend Interface

All storage backends implement the `Backend` interface:

```go
type Backend interface {
    StoreArtifact(siteID, buildID string, reader io.Reader) error
    FetchArtifact(siteID, buildID string) (io.ReadCloser, error)
    WriteLogEntry(siteID string, entry *LogEntry) error
    ListVersions(siteID string) ([]*Version, error)
}
```

### NFS Directory Structure

```
/nfs/
└── sites/
    └── <site_id>/
        ├── artifacts/
        │   └── <build_id>.tar.gz
        ├── logs/
        │   └── <timestamp>-<build_id>.json
        └── meta.json
```

### API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| PUT | `/sites/{site_id}/artifacts/{build_id}` | Upload artifact |
| GET | `/sites/{site_id}/artifacts/{build_id}` | Download artifact |
| POST | `/sites/{site_id}/logs` | Write log entry |
| GET | `/sites/{site_id}/versions` | List all versions |

#### Example: Store Artifact

```bash
curl -X PUT \
  http://localhost:8080/sites/my-site/artifacts/build-123 \
  --data-binary @artifact.tar.gz
```

#### Example: Fetch Artifact

```bash
curl -X GET \
  http://localhost:8080/sites/my-site/artifacts/build-123 \
  -o downloaded.tar.gz
```

#### Example: Write Log Entry

```bash
curl -X POST \
  http://localhost:8080/sites/my-site/logs \
  -H "Content-Type: application/json" \
  -d '{
    "build_id": "build-123",
    "action": "build",
    "status": "success",
    "metadata": {
      "user": "alice",
      "branch": "main"
    }
  }'
```

#### Example: List Versions

```bash
curl -X GET http://localhost:8080/sites/my-site/versions
```

## Configuration

Configuration is managed via environment variables prefixed with `PAGEWRIGHT_`:

| Variable | Default | Description |
|----------|---------|-------------|
| `PAGEWRIGHT_PORT` | `8080` | HTTP server port |
| `PAGEWRIGHT_STORAGE_BACKEND` | `nfs` | Storage backend type |
| `PAGEWRIGHT_NFS_BASE_PATH` | `/nfs` | NFS mount point |

## Setup

### Prerequisites

- Go 1.22 or later
- Docker and Docker Compose (for local testing)
- Make (optional, for convenience)

### Quick Start

1. **Start the infrastructure:**

```bash
make docker-up
```

This starts:
- NFS server container
- Storage service container

2. **Verify service is running:**

```bash
curl http://localhost:8080/health
```

3. **Stop the infrastructure:**

```bash
make docker-down
```

### Local Development

Run the service locally without Docker:

```bash
make run
```

Or manually:

```bash
export PAGEWRIGHT_PORT=8080
export PAGEWRIGHT_STORAGE_BACKEND=nfs
export PAGEWRIGHT_NFS_BASE_PATH=/tmp/nfs-test
go run ./cmd/server/main.go
```

## Testing

### Run All Tests

```bash
make test
```

### Unit Tests Only

```bash
make test-unit
```

### Integration Tests

Integration tests require the Docker infrastructure to be running:

```bash
make docker-up
make test-integration
```

Or run the complete end-to-end test suite:

```bash
make e2e
```

### Test Coverage

Generate a coverage report:

```bash
make coverage
```

This creates `coverage.html` in the current directory.

## Building

### Build Binary

```bash
make build
```

### Build Docker Image

```bash
make docker-build
```

## Development Tools

### Format Code

```bash
make fmt
```

### Run Linter

```bash
make vet
```

### View Logs

```bash
make docker-logs
```

## Key Features

### Atomic Writes

All write operations are atomic to prevent partial writes:

1. Write data to `<path>.tmp`
2. Call `fsync()` to ensure data is on disk
3. Atomically rename temp file to final path

This ensures that concurrent operations never see partial writes and that data survives crashes.

### Log Entry Format

Each log entry is stored as a separate JSON file with the format:

```json
{
  "timestamp": "2026-02-06T10:30:45.123456Z",
  "build_id": "build-123",
  "site_id": "my-site",
  "action": "build",
  "status": "success",
  "metadata": {
    "user": "alice",
    "branch": "main"
  }
}
```

Filenames use the pattern: `<timestamp>-<build_id>.json`

This approach:
- Avoids JSONL append hazards
- Provides natural chronological sorting
- Ensures each build has a complete, atomic log entry

## Project Status

✅ **Implemented:**
- NFS storage backend with atomic writes
- REST API for artifacts, logs, and versions
- Complete unit test suite
- Integration tests with real NFS mount
- Docker and docker-compose setup
- Makefile for automation

See [TODO.md](TODO.md) for planned features and improvements.

## License

See [LICENSE](../../LICENSE) file in the repository root.
