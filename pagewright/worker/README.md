# Worker Service

**Port**: 8082

Stateless worker that executes AI-powered site edits using Codex in isolated containers.

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| GET | `/status` | Current execution status |
| POST | `/kill` | Terminate codex process |

## Execution Workflow

1. **Fetch Artifact**: Download from storage service
2. **Unpack**: Extract to `/work/site/`
3. **Patch Instructions**: Replace `.codex/instructions.md` with container version
4. **Execute Codex**: Run `codex exec "<prompt>"`
5. **Parse Output**: Extract files_changed and summary
6. **Pack Result**: Create `output.tar.gz`
7. **Upload**: Send artifact, manifest, and logs to storage
8. **Callback**: POST result to manager (`/jobs/{job_id}/result`)

## Status Response

```json
{
  "status": "running",
  "phase": "executing_codex",
  "files_changed": 3,
  "summary": "Added contact form component",
  "error": null
}
```

Possible statuses:
- `idle`: Not running
- `fetching_artifact`: Downloading from storage
- `unpacking`: Extracting tar.gz
- `patching_instructions`: Updating Codex instructions
- `executing_codex`: Running AI agent
- `packing_result`: Creating output artifact
- `uploading`: Sending to storage
- `completed`: Successfully finished
- `failed`: Error occurred

## Codex Integration

### Execution
```bash
codex exec "<user_prompt>"
```

### Environment Variables
- `CODEX_API_KEY`: OpenAI API key
- `CODEX_MODEL`: Model to use (default: gpt-4)

### Output Parsing
Worker captures stdout and parses structured output:
```
FILES_CHANGED: 3
SUMMARY: Added contact form with email validation
```

## Docker Deployment

### Build Image
```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /build
COPY . .
RUN go build -o runner cmd/runner/main.go

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /build/runner /usr/local/bin/runner
COPY --from=codex /usr/local/bin/codex /usr/local/bin/codex
COPY internal/.codex/instructions.md /.codex/instructions.md
ENTRYPOINT ["/usr/local/bin/runner"]
```

### Run Container
```bash
docker run \
  -e PAGEWRIGHT_WORKER_PORT=8082 \
  -e PAGEWRIGHT_LLM_KEY=sk-... \
  -e PAGEWRIGHT_MANAGER_URL=http://manager:8081 \
  -e PAGEWRIGHT_STORAGE_URL=http://storage:8080 \
  -e PAGEWRIGHT_JOB='{"job_id":"...","site_id":"...","prompt":"..."}' \
  pagewright-worker:latest
```

## Kubernetes Deployment

### Pod Spec
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: worker-{{.JobID}}
  labels:
    app: pagewright-worker
    job-id: {{.JobID}}
spec:
  restartPolicy: Never
  containers:
  - name: worker
    image: pagewright-worker:latest
    env:
    - name: PAGEWRIGHT_WORKER_PORT
      value: "8082"
    - name: PAGEWRIGHT_LLM_KEY
      valueFrom:
        secretKeyRef:
          name: openai-api-key
          key: api-key
    - name: PAGEWRIGHT_MANAGER_URL
      value: "http://manager-service:8081"
    - name: PAGEWRIGHT_STORAGE_URL
      value: "http://storage-service:8080"
    - name: PAGEWRIGHT_JOB
      value: '{{.JobJSON}}'
    resources:
      requests:
        memory: "512Mi"
        cpu: "500m"
      limits:
        memory: "2Gi"
        cpu: "2000m"
```

### Kubernetes Job (Alternative)
```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: worker-{{.JobID}}
spec:
  ttlSecondsAfterFinished: 3600
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: worker
        image: pagewright-worker:latest
        # ... same env as above
```

### RBAC Configuration
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: worker-spawner
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: worker-spawner
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["create", "get", "list", "delete"]
- apiGroups: ["batch"]
  resources: ["jobs"]
  verbs: ["create", "get", "list", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: worker-spawner
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: worker-spawner
subjects:
- kind: ServiceAccount
  name: worker-spawner
```

## Configuration

Environment variables (all with `PAGEWRIGHT_` prefix):

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `WORKER_PORT` | `8082` | No | HTTP server port |
| `WORK_DIR` | `/work` | No | Working directory |
| `LLM_KEY` | - | Yes | OpenAI API key |
| `LLM_URL` | `https://api.openai.com/v1` | No | LLM base URL |
| `MANAGER_URL` | `http://localhost:8081` | Yes | Manager callback URL |
| `STORAGE_URL` | `http://localhost:8080` | Yes | Storage service URL |
| `JOB` | - | Yes | Job JSON (set by manager) |
| `CODEX_BINARY` | `/usr/local/bin/codex` | No | Path to codex CLI |
| `INSTRUCTIONS_PATH` | `/.codex/instructions.md` | No | Codex instructions template |

## Running

```bash
# Development
cd pagewright/worker
export PAGEWRIGHT_JOB='{"job_id":"test","site_id":"site","prompt":"test"}'
make run

# Docker
make docker-build

# Tests
make test
```

## Instructions Patching

On each run, worker replaces `.codex/instructions.md` in the unpacked artifact with the version from the container image. This allows updating AI instructions without re-uploading artifacts.

**Container Path**: `/.codex/instructions.md`  
**Site Path**: `/work/site/.codex/instructions.md`

Template includes:
- File structure guidelines
- Allowed/forbidden operations
- Security constraints
- Output format requirements
