# Storage Service

**Port**: 8080

Artifact versioning and retrieval with pluggable storage backends.

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| PUT | `/sites/{site_id}/artifacts/{build_id}` | Upload artifact (tar.gz) |
| GET | `/sites/{site_id}/artifacts/{build_id}` | Download artifact |
| POST | `/sites/{site_id}/logs` | Write log entry (JSON) |
| GET | `/sites/{site_id}/versions` | List all versions |

## Request/Response Formats

### Store Artifact

**Request:**
```bash
curl -X PUT http://localhost:8080/sites/my-site/artifacts/build-123 \
  --data-binary @artifact.tar.gz \
  -H "Content-Type: application/gzip"
```

**Response:**
```json
{
  "status": "success"
}
```

### Fetch Artifact

**Request:**
```bash
curl http://localhost:8080/sites/my-site/artifacts/build-123 -o artifact.tar.gz
```

Returns tar.gz binary stream.

### Write Log Entry

**Request:**
```json
{
  "build_id": "build-123",
  "action": "build",
  "status": "success",
  "metadata": {
    "files_changed": 3,
    "duration_ms": 45000
  }
}
```

**Response:**
```json
{
  "status": "success"
}
```

### List Versions

**Response:**
```json
{
  "site_id": "my-site",
  "versions": [
    {
      "build_id": "build-123",
      "size_bytes": 1024000,
      "created_at": "2024-01-01T12:00:00Z",
      "logs": [
        {
          "action": "build",
          "status": "success",
          "timestamp": "2024-01-01T12:00:00Z"
        }
      ]
    }
  ]
}
```

## Storage Backend

### NFS (Current Implementation)

Directory structure:
```
/nfs/sites/{site_id}/
  ├── artifacts/
  │   ├── {build_id}.tar.gz
  │   └── {build_id}.tar.gz
  └── logs/
      ├── {build_id}.json
      └── {build_id}.json
```

### Atomic Write Operations

All writes follow atomic pattern:
1. Write to temporary file
2. fsync() to ensure disk persistence
3. Rename to final location (atomic operation)

This guarantees no partial/corrupted artifacts even on crashes.

### Pluggable Backend Interface

```go
type Backend interface {
    StoreArtifact(siteID, buildID string, reader io.Reader) error
    FetchArtifact(siteID, buildID string) (io.ReadCloser, error)
    WriteLog(siteID string, entry LogEntry) error
    ListVersions(siteID string) ([]*Version, error)
}
```

Future backends:
- S3-compatible (AWS, MinIO, DigitalOcean Spaces)
- Azure Blob Storage
- Google Cloud Storage

## Configuration

Environment variables (all with `PAGEWRIGHT_` prefix):

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `PORT` | `8080` | No | HTTP server port |
| `STORAGE_BACKEND` | `nfs` | No | Backend type (nfs, s3, azure, gcs) |
| `NFS_BASE_PATH` | `/nfs` | Yes | NFS mount point |

## Running

```bash
# Development
cd pagewright/storage
make run

# Docker Compose (includes NFS server)
make docker-up

# Tests
make test
make test-integration
```

## Queue Usage

**Note**: The storage service does NOT use Redis or any queue system. It is a simple synchronous HTTP service.

Queue operations are handled by:
- **Manager Service**: Job queue management
- **Gateway Service**: Enqueues build requests to manager

Storage service only stores and retrieves artifacts on-demand.
