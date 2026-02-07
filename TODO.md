# TODO

## High Priority

### Compiler & Themes
- [ ] Add comprehensive unit tests for compiler (target 80%+)
- [ ] Create Makefile for compiler (build, test, clean targets)
- [ ] Add incremental build support (hash-based caching)
- [ ] Create additional themes (blog, portfolio, docs)
- [ ] Add theme validation tool
- [ ] Support custom component props validation
- [ ] Add sitemap.xml and RSS generation
- [ ] Implement draft pages support

### UI Completion
- [ ] Complete remaining React components
  - [ ] Dashboard page with site cards
  - [ ] Chat interface for build requests
  - [ ] Version management modal
  - [ ] Profile and password reset pages
- [ ] Integrate WebSocket for real-time updates
- [ ] Add responsive mobile layouts
- [ ] Docker deployment configuration

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
- [ ] Pagination for site listings (currently basic)
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
