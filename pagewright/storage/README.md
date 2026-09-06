# Storage Service

**Port**: 8080

Artifact versioning and retrieval with pluggable storage backends.

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| PUT | `/sites/{site_id}/artifacts/{build_id}` | Upload artifact (tar.gz) |
| GET | `/sites/{site_id}/artifacts/{build_id}` | Download artifact |
| POST / GET | `/sites/{site_id}/artifacts/{build_id}/logs` | Persist/read private execution log JSON |
| POST / GET | `/sites/{site_id}/artifacts/{build_id}/manifest` | Commit/read manifest after artifact + log writes |
| POST | `/sites/{site_id}/logs` | Write event record (does not commit a version) |
| GET | `/sites/{site_id}/versions` | List committed versions only |

See the [metadata contract](../../docs/VERSION_METADATA.md) for schemas, limits,
legacy visibility changes and privacy boundaries. Storage has no service
authentication yet; do not expose its port to untrusted networks.

M2.7 requires a matching `X-Pagewright-Attempt` JSON header on non-bootstrap
artifact/log/manifest writes and a reachable `PAGEWRIGHT_MANAGER_URL` (default
`http://manager:8081`). See [fenced commits and coordinated upgrade requirements](../../docs/FENCED_COMMITS.md).
The legacy standalone Compose topology needs an explicitly reachable manager;
root Compose configures the supported service connection.

## Request/Response Formats

### Store Artifact

**Request:**
```bash
curl -X PUT http://localhost:8080/sites/my-site/artifacts/build-123 \
  --data-binary @artifact.tar.gz \
  -H "X-Pagewright-Attempt: $PAGEWRIGHT_ATTEMPT_JSON" \
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
    "files_changed": "3",
    "duration_ms": "45000"
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
      "timestamp": "2024-01-01T12:00:00Z",
      "action": "build",
      "status": "completed"
    }
  ],
  "count": 1
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
  ├── metadata/{build_id}/
  │   ├── execution.json
  │   └── manifest.json
  └── logs/{timestamp}-{build_id}.json
```

### Atomic Write Operations

Writes use a temporary-file pattern:
1. Write to temporary file
2. fsync() to ensure disk persistence
3. Publish without replacing an existing file; sync containing directories

Files use unique temporary files and no-replace hard-link publication, with
checked write/sync/close and directory sync. Exact-byte retries succeed;
different bytes conflict. See [immutable versions](../../docs/IMMUTABLE_VERSIONS.md)
for concurrency and filesystem limits. Version deletion is disabled.

### Pluggable Backend Interface

```go
type Backend interface {
    StoreArtifact(siteID, buildID string, reader io.Reader) error
    FetchArtifact(siteID, buildID string) (io.ReadCloser, error)
    WriteLogEntry(siteID string, entry *LogEntry) error
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
