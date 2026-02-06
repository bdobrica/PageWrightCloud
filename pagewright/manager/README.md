# PageWright Manager Service

Job queue and worker management service for the PageWright Cloud platform. Handles job queueing, distributed locking, and worker spawning for site build operations.

## Overview

The manager service implements Phase 2 of the PageWright PoC, providing:

- **Job Queue**: Redis-based job queue with pluggable backend support
- **Distributed Locking**: Safe per-site locking with fencing tokens
- **Worker Management**: Spawn workers via Docker or Kubernetes
- **Status Tracking**: Track job lifecycle from creation to completion

## Architecture

```
pagewright/manager/
├── cmd/
│   ├── server/          # Manager service
│   └── worker/          # Stub worker
├── internal/
│   ├── api/            # HTTP handlers
│   ├── config/         # Configuration
│   ├── types/          # Shared types
│   ├── queue/          # Queue backend interface
│   │   └── redis/      # Redis implementation
│   ├── lock/           # Lock manager interface
│   │   └── redis/      # Redis implementation with Lua scripts
│   └── spawner/        # Worker spawner interface
│       ├── docker/     # Docker spawner
│       └── kubernetes/ # Kubernetes spawner
└── test/integration/   # E2E tests
```

## Key Features

### Distributed Locking

Per-site locking ensures only one worker can modify a site at a time:

- Lock key: `lock:site:<site_id>`
- Acquire with `SET key token NX PX ttl`
- Lua scripts for safe renew/release
- Fencing tokens prevent stale writes

### Fencing Tokens

Monotonic counter per site (`INCR fence:site:<site_id>`):
- Included in job context
- Deployer validates tokens to reject stale builds
- Prevents race conditions if locks expire

### Job Lifecycle

1. **Created**: Job submitted via API
2. **Queued**: Added to Redis queue
3. **Running**: Lock acquired, worker spawned
4. **Completed/Failed**: Worker reports back, lock released

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| POST | `/jobs` | Create and queue a job |
| GET | `/jobs/{job_id}` | Get job status |
| POST | `/jobs/{job_id}/status` | Update job status (worker callback) |

### Create Job

```bash
curl -X POST http://localhost:8081/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "site_id": "my-site",
    "prompt": "Update homepage title to Welcome",
    "source_version": "v1.0.0",
    "target_version": "v1.0.1"
  }'
```

Response:
```json
{
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "site_id": "my-site",
  "prompt": "Update homepage title to Welcome",
  "source_version": "v1.0.0",
  "target_version": "v1.0.1",
  "status": "running",
  "lock_token": "a1b2c3...",
  "fencing_token": 42,
  "created_at": "2026-02-06T16:00:00Z",
  "updated_at": "2026-02-06T16:00:00Z",
  "worker_id": "660f9511-f39c-52e5-b827-557766551111"
}
```

### Get Job Status

```bash
curl http://localhost:8081/jobs/550e8400-e29b-41d4-a716-446655440000
```

### Worker Callback (Internal)

Workers call this to update their status:

```bash
curl -X POST http://localhost:8081/jobs/550e8400-e29b-41d4-a716-446655440000/status \
  -H "Content-Type: application/json" \
  -d '{
    "status": "completed",
    "result": "Successfully updated homepage title"
  }'
```

## Configuration

All configuration via environment variables prefixed with `PAGEWRIGHT_`:

| Variable | Default | Description |
|----------|---------|-------------|
| `PAGEWRIGHT_PORT` | `8081` | HTTP server port |
| `PAGEWRIGHT_QUEUE_BACKEND` | `redis` | Queue backend (redis) |
| `PAGEWRIGHT_WORKER_SPAWNER` | `docker` | Worker spawner (docker, kubernetes) |
| `PAGEWRIGHT_REDIS_ADDR` | `localhost:6379` | Redis address |
| `PAGEWRIGHT_REDIS_PASSWORD` | `` | Redis password |
| `PAGEWRIGHT_REDIS_DB` | `0` | Redis database |
| `PAGEWRIGHT_LOCK_TTL` | `5m` | Lock TTL duration |
| `PAGEWRIGHT_LOCK_RENEW_INTERVAL` | `1m` | Lock renewal interval |
| `PAGEWRIGHT_WORKER_IMAGE` | `pagewright-worker:latest` | Worker container image |
| `PAGEWRIGHT_WORKER_TIMEOUT` | `30m` | Worker timeout |
| `PAGEWRIGHT_MANAGER_URL` | `http://localhost:8081` | Manager callback URL |

## Setup

### Prerequisites

- Go 1.22 or later
- Docker and Docker Compose
- Redis (or use docker-compose)

### Quick Start

1. **Start the infrastructure:**

```bash
make docker-up
```

This starts:
- Redis container
- Manager service container

2. **Verify service:**

```bash
curl http://localhost:8081/health
```

3. **Create a job:**

```bash
curl -X POST http://localhost:8081/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "site_id": "test-site",
    "prompt": "Add a contact page"
  }'
```

4. **Stop:**

```bash
make docker-down
```

### Local Development

Run the manager service locally:

```bash
# Start Redis
docker run -d -p 6379:6379 redis:7-alpine

# Run manager
make run
```

## Testing

### Unit Tests

```bash
make test-unit
```

### Integration Tests

Requires Docker services running:

```bash
make docker-up
make test-integration
```

### Full E2E Suite

```bash
make e2e
```

## Worker Implementation

The stub worker demonstrates the expected worker behavior:

1. Read job from `PAGEWRIGHT_JOB` environment variable
2. Read manager URL from `PAGEWRIGHT_MANAGER_URL`
3. Process the job (fetch artifact, run AI, create new artifact)
4. Call back to manager with status update
5. Exit

Real workers (Phase 3) will:
- Fetch artifacts from storage service
- Run AI coding agent (Codex)
- Perform browser checks
- Upload result artifacts
- Report completion

## Plugin Architecture

### Queue Backends

Implement `queue.Backend` interface:

```go
type Backend interface {
    Push(ctx context.Context, job *types.Job) error
    Pop(ctx context.Context) (*types.Job, error)
    GetJob(ctx context.Context, jobID string) (*types.Job, error)
    UpdateJob(ctx context.Context, job *types.Job) error
    Close() error
}
```

### Worker Spawners

Implement `spawner.Spawner` interface:

```go
type Spawner interface {
    Spawn(ctx context.Context, job *types.Job, managerURL string) (workerID string, err error)
    Close() error
}
```

## Locking Details

### Acquire

```lua
-- Set lock with NX (only if not exists) and PX (milliseconds TTL)
SET lock:site:<site_id> <token> NX PX <ttl_ms>
-- Increment fencing token
INCR fence:site:<site_id>
```

### Renew

```lua
-- Only renew if token matches (prevents stealing)
if redis.call("get", KEYS[1]) == ARGV[1] then
    return redis.call("pexpire", KEYS[1], ARGV[2])
else
    return 0
end
```

### Release

```lua
-- Only release if token matches
if redis.call("get", KEYS[1]) == ARGV[1] then
    return redis.call("del", KEYS[1])
else
    return 0
end
```

## Redis Data Structure

```
# Job queue (list)
pagewright:queue: [job-id-1, job-id-2, ...]

# Job data (strings with JSON)
pagewright:job:<job-id>: {...}

# Site locks (strings)
lock:site:<site-id>: <token>

# Fencing tokens (integers)
fence:site:<site-id>: 42
```

## Development

### Building

```bash
# Manager binary
make build

# Worker binary
make build-worker

# Docker images
make docker-build
```

### Code Quality

```bash
# Format
make fmt

# Lint
make vet

# Coverage report
make coverage
```

## Troubleshooting

### Lock Already Held

If a site's lock is stuck (worker died without releasing):

```bash
# Manually release lock (use with caution)
redis-cli DEL lock:site:my-site
```

### Job Stuck in Queue

```bash
# Check queue length
redis-cli LLEN pagewright:queue

# View pending jobs
redis-cli LRANGE pagewright:queue 0 -1

# View specific job
redis-cli GET pagewright:job:<job-id>
```

## Project Status

✅ **Implemented:**
- Redis queue and locking
- Fencing tokens
- Docker & Kubernetes spawner plugins
- REST API with job lifecycle
- Stub worker for testing
- Complete test suite

See [TODO.md](TODO.md) for planned enhancements.

## License

See [LICENSE](../../LICENSE) file in the repository root.
