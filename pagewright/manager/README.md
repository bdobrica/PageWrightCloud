# Manager Service

**Port**: 8081

Job queue and worker orchestration with Redis-based distributed locking.

M2.1 implements Docker create/start. See the [launch contract](../../docs/DOCKER_SPAWNER.md)
for required network/image setup, manager socket authority and ambiguous-outcome
handling. Queue dispatch/recovery and the real AI worker remain subsequent items.

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| POST | `/jobs` | Create and enqueue job |
| GET | `/jobs/{job_id}` | Get job status |
| POST | `/jobs/{job_id}/status` | Update job status (worker callback) |
| POST | `/jobs/{job_id}/result` | Worker completion callback |

## Request/Response Formats

### Create Job

**Request:**
```json
{
  "site_id": "blog-example-com",
  "build_id": "v1-20240101120000",
  "prompt": "Add a contact form"
}
```

**Response:**
```json
{
  "job_id": "uuid",
  "status": "queued",
  "site_id": "blog-example-com",
  "build_id": "v1-20240101120000",
  "lock_token": "token-123",
  "fencing_token": 42,
  "worker_id": "worker-uuid"
}
```

### Get Job Status

**Response:**
```json
{
  "job_id": "uuid",
  "status": "running",
  "site_id": "blog-example-com",
  "created_at": "2024-01-01T12:00:00Z",
  "updated_at": "2024-01-01T12:01:00Z"
}
```

### Update Job Status (Worker Callback)

**Request:**
```json
{
  "status": "running",
  "message": "Executing codex..."
}
```

### Worker Completion Callback

**Request:**
```json
{
  "status": "completed",
  "artifact_url": "http://storage:8080/sites/blog/artifacts/v1",
  "files_changed": 3,
  "summary": "Added contact form with validation"
}
```

## Job Lifecycle

1. **Created**: Job submitted via API
2. **Queued**: Added to Redis queue (`queue:jobs`)
3. **Running**: Lock acquired (`lock:site:<site_id>`), worker spawned
4. **Completed/Failed**: Worker reports back, lock released

## Distributed Locking

### Lock Keys
- Pattern: `lock:site:<site_id>`
- Acquire: `SET key token NX PX ttl`
- Release: Lua script for atomic check-and-delete
- Renew: Lua script for atomic TTL extension

### Fencing Tokens
- Counter key: `fence:site:<site_id>`
- Incremented on each lock acquisition: `INCR fence:site:<site_id>`
- Included in job context
- Validated by deployer to reject stale builds

## Redis Data Structures

### Job Queue
```
queue:jobs: LIST
  - LPUSH to enqueue
  - RPOP to dequeue
```

### Job Data
```
job:<job_id>: HASH
  - status: created|queued|running|completed|failed
  - site_id: string
  - build_id: string
  - created_at: timestamp
  - updated_at: timestamp
```

### Locks
```
lock:site:<site_id>: STRING (token)
  - PX: milliseconds TTL
  - Value: random lock token
```

### Fencing Counters
```
fence:site:<site_id>: STRING (integer)
  - Monotonic counter
```

## Worker Spawning

### Docker Spawner
- Uses Unix-socket Engine API v1.45 with an explicit image tag/digest.
- Injects only job/endpoints/workspace and separate worker-provider credentials.
- Retains containers and uncertain reservations for later reconciliation.

### Kubernetes Spawner
- Historical logging stub, outside the Docker MVP; not a functioning spawner.

## Configuration

Environment variables (all with `PAGEWRIGHT_` prefix):

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `PORT` | `8081` | No | HTTP server port |
| `QUEUE_BACKEND` | `redis` | No | Queue backend type |
| `WORKER_SPAWNER` | `docker` | No | Worker spawner (docker, kubernetes) |
| `REDIS_ADDR` | `localhost:6379` | Yes | Redis address |
| `REDIS_PASSWORD` | - | No | Redis password |
| `REDIS_DB` | `0` | No | Redis database number |
| `LOCK_TTL` | `5m` | No | Lock expiration time |
| `LOCK_RENEW_INTERVAL` | `1m` | No | Lock renewal frequency |
| `WORKER_IMAGE` | `pagewright-worker:m2.1` | No | Build/load selected worker image before submission |
| `WORKER_NETWORK` | - | Yes | Dedicated network; root Compose derives its project network |
| `WORKER_LLM_KEY` | - | No | Worker-only provider credential, not gateway's key |
| `WORKER_TIMEOUT` | `30m` | No | Worker timeout |
| `MANAGER_URL` | `http://localhost:8081` | Yes | Manager callback URL |

## Running

```bash
# Development
cd pagewright/manager
make run

# Docker Compose (includes Redis)
make docker-up

# Tests
make test
```
