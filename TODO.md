# TODO

## State Snapshot (2026-03-01)

- Verified against code and local test runs.
- Updated checklist items below only where implementation is clearly present.
- Remaining unchecked items should be treated as active roadmap work.
- Added local-domain verification workflow (`docker-up-local-domain`, `docker-verify-local-domain`, `docker-verify-local-domain-strict`).
- Strict local-domain verification passed after using non-default storage host port due to local port conflict (`PAGEWRIGHT_STORAGE_PORT=18080`).
- Serving still assumes in-process nginx reload by default; split-container nginx deployments require explicit external reload strategy.

## Resume Plan (Next 2 Weeks, Security First)

### Week 1 — Critical Security Hardening

#### Day 1-2: CORS and WebSocket Origin Hardening
- [ ] Replace wildcard CORS with environment-driven allowlist in gateway
- [ ] Replace WebSocket `CheckOrigin: true` with allowlist validation
- [ ] Add tests for allowed vs denied origins (HTTP + WebSocket handshake)

#### Day 3: Gateway Rate Limiting
- [ ] Add middleware rate limiting per IP for unauthenticated routes
- [ ] Add stricter limits on auth endpoints (login/reset)
- [ ] Add higher, token-keyed limits for authenticated users

#### Day 4: Authentication Hardening
- [ ] Raise password requirements (length + complexity) for reset/update flows
- [ ] Add account lockout after repeated failed login attempts
- [ ] Normalize auth error responses to avoid user enumeration

#### Day 5: Secrets and Baseline Security Hygiene
- [ ] Remove insecure default secrets from docker-compose/.env.example
- [ ] Document required secret values and minimum complexity in README
- [ ] Add startup validation for missing critical secrets

### Week 2 — Reliability + Security Boundary Testing

#### Day 6-7: Gateway Reliability Improvements
- [ ] Configure DB pooling in gateway (`SetMaxOpenConns`, idle/lifetime limits)
- [ ] Add request timeout/cancellation coverage for downstream HTTP clients
- [ ] Add request size limits for JSON/file upload endpoints

#### Day 8-9: Compiler Test Foundation (Security Boundary)
- [ ] Add first compiler unit test suite (markdown, mdx parse, content discover)
- [ ] Add path traversal prevention tests for content/theme/assets handling
- [ ] Add malformed component props validation tests

#### Day 10: WebSocket and Integration Coverage
- [ ] Add gateway integration tests for `/ws` auth + subscription flow
- [ ] Verify job status broadcast flow from manager to connected UI client
- [ ] Document local test command sequence for websocket/integration tests

### Definition of Done for This 2-Week Block
- [ ] No wildcard origin policies remain in gateway HTTP/WebSocket paths
- [ ] Rate limiting is active and covered by tests
- [ ] Critical auth hardening (password + lockout) is implemented
- [ ] Gateway DB pooling and timeouts are configured
- [ ] Compiler has initial automated tests committed

## High Priority

### Compiler & Themes
- [ ] Add comprehensive unit tests for compiler (target 80%+)
- [x] Create Makefile for compiler (build, test, clean targets)
- [ ] Add incremental build support (hash-based caching)
- [ ] Create additional themes (blog, portfolio, docs)
- [ ] Add theme validation tool
- [ ] Support custom component props validation
- [ ] Add sitemap.xml and RSS generation
- [ ] Implement draft pages support

### UI Completion
- [x] Complete remaining React components
  - [x] Dashboard page with site cards
  - [x] Chat interface for build requests
  - [x] Version management modal
  - [x] Profile and password reset pages
- [x] Integrate WebSocket for real-time updates
- [ ] Add responsive mobile layouts
- [x] Docker deployment configuration

### Testing & Documentation
- [ ] Add integration tests for Gateway WebSocket endpoints
- [ ] Increase test coverage for Manager (target 80%+)
- [ ] Add end-to-end tests across all services
- [ ] Performance/load testing for production readiness

## Gateway Enhancements

### Authentication
- [ ] Refresh token implementation (currently 15-min expiry)
- [ ] Rate limiting per user/IP
- [ ] Account lockout after failed login attempts
- [ ] Email verification on registration
- [ ] Multi-factor authentication (2FA)

### API Features
- [x] Pagination for site listings (currently basic)
- [ ] Search/filter sites by template, status, date
- [ ] Bulk operations (delete multiple sites)
- [ ] API versioning strategy
- [ ] GraphQL endpoint option

### Monitoring
- [ ] Structured logging (replace fmt.Printf)
- [ ] Prometheus metrics endpoint
- [ ] Request tracing (OpenTelemetry)
- [ ] Error tracking integration (Sentry)

## Manager Enhancements

### Queue Backends
- [ ] NATS Streaming/JetStream backend
- [ ] Apache Kafka support
- [ ] RabbitMQ support
- [ ] Priority-based job processing
- [ ] Dead letter queue for failed jobs

### Lock Management
- [ ] Lock monitoring dashboard
- [ ] Auto-renewal background process
- [ ] Graceful lock release on shutdown
- [ ] Lock contention metrics

### Worker Management
- [ ] Worker health checks and auto-restart
- [ ] Stream worker logs to manager
- [ ] Job cancellation API
- [ ] Worker resource limits (CPU/memory)
- [ ] Pre-warmed worker pools
- [ ] Auto-scaling based on queue depth

### API Improvements
- [ ] Job listing with filtering
- [ ] Search jobs by site, status, date range
- [ ] Bulk job operations
- [ ] WebSocket for real-time updates
- [ ] Job history with retention policies

## Storage Enhancements

### Storage Backends
- [ ] S3-compatible backend (AWS, MinIO, DigitalOcean)
- [ ] Azure Blob Storage backend
- [ ] Google Cloud Storage backend
- [ ] RustFS backend (when available)

### Features
- [ ] Artifact metadata (size, checksum, content-type)
- [ ] Integrity validation on upload/download
- [ ] Configurable size limits
- [ ] Automatic retention/cleanup policies
- [ ] Compression options (gzip, zstd)
- [ ] Artifact deduplication

### API Improvements
- [ ] Pagination for version listings
- [ ] Filter by date range, status, action
- [ ] Batch operations (bulk delete, bulk download)
- [ ] Artifact search by metadata
- [ ] Chunked/multipart uploads for large files
- [ ] ETags for conditional requests

### Performance
- [ ] Cache version lists in Redis
- [ ] Connection pooling for S3
- [ ] Gzip compression for API responses
- [ ] Background async processing

## Worker Enhancements

### Execution
- [ ] Deploy real Codex binary (currently mock)
- [ ] Implement Playwright browser checks
- [ ] Add site build step (npm run build)
- [ ] Screenshot capture for previews
- [ ] Enhanced error handling and logging
- [ ] Timeout configuration per template

### Kubernetes
- [ ] Real Kubernetes spawner (client-go)
- [ ] Job vs Pod strategy
- [ ] RBAC configuration
- [ ] Resource quotas
- [ ] Pod affinity rules

### Docker
- [ ] Real Docker API integration (SDK)
- [ ] SSH tunnel support for remote Docker
- [ ] Container cleanup policies
- [ ] Volume mounting for artifacts
- [ ] Network configuration
- [ ] Resource constraints

## Serving Enhancements

### Features
- [ ] Custom SSL/TLS certificate management
- [ ] Let's Encrypt automatic certificates
- [ ] HTTP/2 and HTTP/3 support
- [ ] Brotli compression
- [ ] CloudFlare integration testing
- [ ] CDN cache invalidation

### Monitoring
- [ ] Access logs analysis
- [ ] Traffic metrics per site
- [ ] Error rate monitoring
- [ ] Response time tracking

### API
- [ ] Batch deployment operations
- [ ] Deployment rollback automation
- [ ] Health check per site
- [ ] Site migration between servers

## Security

### All Services
- [ ] mTLS between internal services
- [ ] Secret management (HashiCorp Vault)
- [ ] Security audit logging
- [ ] Input validation hardening
- [ ] Rate limiting per endpoint
- [ ] DDoS protection strategy

### Gateway Specific
- [ ] CSRF protection
- [ ] Content Security Policy headers
- [ ] Session management improvements
- [ ] OAuth provider expansion (GitHub, Microsoft)

## Operations

### Infrastructure
- [ ] Kubernetes Helm charts
- [ ] Terraform/IaC configuration
- [ ] Service mesh integration (Istio)
- [ ] Multi-region deployment
- [ ] Disaster recovery procedures

### Monitoring
- [ ] Grafana dashboards for all services
- [ ] Alert rules for critical conditions
- [ ] Distributed tracing setup
- [ ] Log aggregation (ELK/Loki)
- [ ] Performance baselines

### CI/CD
- [ ] GitHub Actions workflows
- [ ] Automated integration tests
- [ ] Container scanning (Trivy)
- [ ] Dependency vulnerability checks
- [ ] Automated changelog generation

## Documentation

- [ ] OpenAPI/Swagger specifications
- [ ] Architecture decision records (ADRs)
- [ ] Runbooks for common scenarios
- [ ] Performance tuning guides
- [ ] Troubleshooting guides
- [ ] Backend plugin development guide

## Future Features

### User Experience
- [ ] Site templates marketplace
- [ ] AI-powered SEO suggestions
- [ ] Analytics dashboard integration
- [ ] Collaboration features (team sites)
- [ ] Site cloning/forking

### Platform
- [ ] Multi-language Codex support
- [ ] Plugin system for custom templates
- [ ] Webhook notifications
- [ ] External API integrations
- [ ] White-label deployment option
