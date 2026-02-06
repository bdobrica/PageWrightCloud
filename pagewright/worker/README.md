# PageWright Worker

Stateless worker service that executes site edit jobs using Codex.

## Architecture

- **Runner**: Main orchestrator (fetch → unpack → codex → pack → upload → callback)
- **Codex Executor**: Process wrapper for `codex exec` with kill capability
- **Storage Client**: HTTP client for Phase 1 storage service
- **Artifact Handler**: tar.gz pack/unpack and instructions patching
- **HTTP Server**: Status and control endpoints

## Configuration

All via `PAGEWRIGHT_*` environment variables:

- `PAGEWRIGHT_WORKER_PORT` - HTTP server port (default: 8082)
- `PAGEWRIGHT_WORK_DIR` - Working directory (default: /work)
- `PAGEWRIGHT_LLM_KEY` - OpenAI API key (required)
- `PAGEWRIGHT_LLM_URL` - LLM base URL (default: https://api.openai.com/v1)
- `PAGEWRIGHT_MANAGER_URL` - Manager service URL (default: http://localhost:8081)
- `PAGEWRIGHT_STORAGE_URL` - Storage service URL (default: http://localhost:8080)
- `PAGEWRIGHT_JOB` - Job JSON (required, set by manager)
- `PAGEWRIGHT_CODEX_BINARY` - Path to codex CLI (default: /usr/local/bin/codex)
- `PAGEWRIGHT_INSTRUCTIONS_PATH` - Codex instructions template (default: /.codex/instructions.md)

## API Endpoints

- `GET /health` - Health check
- `GET /status` - Current execution status and progress
- `POST /kill` - Terminate running codex process

## Workflow

1. Fetch artifact from storage (`GET /artifacts/{site_id}/{version_id}`)
2. Unpack to `/work/site/`
3. Patch `.codex/instructions.md` with container version
4. Execute `codex exec "<prompt>"`
5. Parse output for files changed and summary
6. Pack result to `output.tar.gz`
7. Upload artifact, manifest, and logs to storage
8. POST result to manager (`/jobs/{job_id}/result`)

## Testing

```bash
make test-unit          # Unit tests (config, artifact, codex mock)
make build              # Build runner binary
make docker-build       # Build container image
```

## Docker Image

Multi-stage build with:
- Go 1.22 builder
- Alpine runtime
- Mock codex binary (replace with real one)
- Pre-installed instructions template

## Codex Integration

The worker wraps `codex exec` execution:
- Captures stdout/stderr
- Monitors process state
- Supports cancellation via `/kill` endpoint
- Parses structured output (FILES_CHANGED, SUMMARY)

## Instructions Patching

On each run, the worker replaces `.codex/instructions.md` in the unpacked artifact with the version from the container. This allows updating AI instructions without re-uploading every artifact.
