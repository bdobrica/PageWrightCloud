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

# Copy environment file and configure
cp .env.example .env
# Edit .env and add your GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, and LLM_KEY

# Start all services with docker-compose
make docker-up

# Check service health
make docker-ps
```

This starts:
- **PostgreSQL** - Gateway database (port 5432)
- **Redis** - Manager job queue (port 6379)
- **NFS Server** - Storage backend (port 2049)
- **Gateway** - API & auth (port 8085)
- **Manager** - Job orchestration (port 8081)
- **Storage** - Artifact storage (port 8080)
- **Serving** - Static hosting (port 8083)
- **Themes** - Theme registry & downloads (port 8086)
- **nginx** - Web server (port 8084)
- **UI** - React frontend (port 3000)

### Verify Services

```bash
# Check all services are running
make docker-ps

# View logs
make docker-logs

# View specific service logs
make docker-logs-gateway
make docker-logs-manager

# Test API
curl http://localhost:8085/health  # Gateway
curl http://localhost:8081/health  # Manager
curl http://localhost:8080/health  # Storage
curl http://localhost:8083/health  # Serving
curl http://localhost:8086/        # Themes (returns JSON index)
```

### Development Workflow

**Option 1: All services in Docker**
```bash
make docker-up
make docker-logs
```

**Option 2: Infrastructure in Docker, services locally**
```bash
# Start only infrastructure (PostgreSQL, Redis, NFS)
make docker-up-infra

# Run services locally (in separate terminals)
cd pagewright/gateway && make run
cd pagewright/manager && make run
cd pagewright/storage && make run
cd pagewright/serving && make run
cd pagewright/ui && npm run dev
```

### Stop Services

```bash
make docker-down
```

## Architecture

PageWrightCloud consists of 7 microservices:

1. **Gateway** (8085) - User authentication, site management, REST API
2. **Manager** (8081) - Job queue & worker orchestration with Redis
3. **Storage** (8080) - Artifact versioning on NFS
4. **Worker** (8082) - Executes Codex AI edits in isolated containers
5. **Serving** (8083) - nginx-based static hosting with atomic deploys
6. **UI** (5173) - React/TypeScript chat interface
7. **Compiler** - Standalone Go binary that transforms markdown + theme → static HTML
8. **Themes** (8086) - Theme registry serving zipped themes via HTTP

### Compiler & Themes

The compiler is a security boundary that limits what AI agents can modify:

- **AI agents CAN:** Edit markdown, modify site.json, add page assets
- **AI agents CANNOT:** Change base URLs, edit theme templates, run arbitrary code

Themes define the look and feel using Go templates, CSS tokens, and MDX components.

See [pagewright/compiler/README.md](pagewright/compiler/README.md) and [pagewright/themes/README.md](pagewright/themes/README.md) for details.

## Testing

### Run All Tests

```bash
# All services (requires infrastructure running)
make docker-up-infra
make test-all

# Individual service tests
make test-gateway
make test-manager
make test-storage
make test-worker
make test-serving

# Integration tests (requires docker-up-infra)
make test-integration
```

### Coverage Reports

```bash
# Generate coverage for all services
make coverage

# View individual coverage
cd pagewright/gateway && make coverage && open coverage.html
cd pagewright/manager && make coverage && open coverage.html
```

## Current Status

| Service | Status | Coverage |
|---------|--------|----------|
| Gateway | ✅ Complete | 75%+ |
| Manager | ✅ Complete | 70%+ |
| Storage | ✅ Complete | 80%+ |
| Worker | ✅ Complete | 75%+ |
| Serving | ✅ Complete | 77%+ |
| Compiler | ✅ Complete | 0% (needs tests) |
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
- [Compiler](pagewright/compiler/README.md)
- [Themes](pagewright/themes/README.md)
- [UI Implementation](pagewright/ui/README.md)
- [TODO List](TODO.md)

## License

See [LICENSE](LICENSE) file.
