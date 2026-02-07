# PageWrightCloud

**AI-powered static website builder for non-technical users**

PageWrightCloud helps professionals (lawyers, accountants, teachers, artists) create and manage simple static websites through natural language chat, powered by OpenAI Codex.

## Quick Start

### Prerequisites
- Docker & Docker Compose
- Go 1.22+
- Node.js 18+ (for UI development)

### Start All Services

```bash
# Clone repository
git clone https://github.com/PageWrightCloud/PageWrightCloud.git
cd PageWrightCloud

# Start infrastructure (Redis, PostgreSQL, NFS)
cd pagewright
docker-compose up -d

# Start individual services (in separate terminals)
cd gateway && make run     # Port 8085
cd manager && make run     # Port 8081
cd storage && make run     # Port 8080
cd serving && make run     # Port 8083
cd worker && make run      # Port 8082
cd ui && npm run dev       # Port 5173
```

### Verify Services

```bash
curl http://localhost:8085/health  # Gateway
curl http://localhost:8081/health  # Manager
curl http://localhost:8080/health  # Storage
curl http://localhost:8083/health  # Serving
```

## Architecture

PageWrightCloud consists of 6 microservices:

1. **Gateway** (8085) - User authentication, site management, REST API
2. **Manager** (8081) - Job queue & worker orchestration with Redis
3. **Storage** (8080) - Artifact versioning on NFS
4. **Worker** (8082) - Executes Codex AI edits in isolated containers
5. **Serving** (8083) - nginx-based static hosting with atomic deploys
6. **UI** (5173) - React/TypeScript chat interface

See [pagewright/README.md](pagewright/README.md) for detailed architecture diagram.

## Testing

### Run All Tests

```bash
# Individual service tests
cd pagewright/gateway && make test
cd pagewright/manager && make test
cd pagewright/storage && make test
cd pagewright/serving && make test
cd pagewright/worker && make test

# Integration tests (requires docker-compose up)
cd pagewright/gateway && make test-integration
cd pagewright/storage && make test-integration
```

### Coverage Reports

```bash
cd pagewright/<service>
make coverage
open coverage.html
```

## Current Status

| Service | Status | Coverage |
|---------|--------|----------|
| Gateway | ✅ Complete | 75%+ |
| Manager | ✅ Complete | 70%+ |
| Storage | ✅ Complete | 80%+ |
| Worker | ✅ Complete | 75%+ |
| Serving | ✅ Complete | 77%+ |
| UI | 🚧 In Progress | N/A |

## Core Concepts

- **Immutable Versions**: Every edit creates a new versioned artifact
- **Preview & Promote**: Test changes before going live
- **Atomic Deploys**: Zero-downtime symlink switches
- **AI-Assisted**: Natural language site editing via Codex
- **Safe by Design**: Workers never modify live files

## Key Features

- Email/password + Google OAuth authentication
- Multi-site management per user
- Custom domain aliases
- Real-time WebSocket updates
- Version history with rollback
- Chat-based build clarification loop

## Configuration

All services use environment variables with `PAGEWRIGHT_` prefix:

```bash
# Gateway
PAGEWRIGHT_GATEWAY_PORT=8085
PAGEWRIGHT_DB_HOST=localhost
PAGEWRIGHT_JWT_SECRET=your-secret

# Manager
PAGEWRIGHT_MANAGER_PORT=8081
PAGEWRIGHT_REDIS_ADDR=localhost:6379

# Storage
PAGEWRIGHT_STORAGE_PORT=8080
PAGEWRIGHT_NFS_BASE_PATH=/nfs

# Worker
PAGEWRIGHT_LLM_KEY=sk-...
PAGEWRIGHT_CODEX_BINARY=/usr/local/bin/codex

# Serving
PAGEWRIGHT_SERVING_PORT=8083
PAGEWRIGHT_WWW_ROOT=/var/www
```

## Documentation

- [Architecture Overview](pagewright/README.md)
- [Gateway API](pagewright/gateway/README.md)
- [Manager API](pagewright/manager/README.md)
- [Storage API](pagewright/storage/README.md)
- [Worker Deployment](pagewright/worker/README.md)
- [Serving API](pagewright/serving/README.md)
- [UI Implementation](pagewright/ui/README.md)
- [TODO List](TODO.md)

## License

See [LICENSE](LICENSE) file.
