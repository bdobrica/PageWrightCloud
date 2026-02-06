# TODO

## Phase 1 Enhancements

### Storage Backends
- [ ] **S3 Backend Plugin**: Implement S3-compatible storage backend
  - Support for AWS S3
  - Support for MinIO and other S3-compatible services
  - Credential management via environment variables
- [ ] **Azure Blob Storage Backend**: Support for Azure storage
- [ ] **Google Cloud Storage Backend**: Support for GCS
- [ ] **RustFS Backend**: When RustFS becomes available

### Features
- [ ] **Artifact Metadata**: Store and retrieve metadata without downloading full artifact
  - Size, checksum, content type
  - Creation timestamp, creator
- [ ] **Artifact Validation**: 
  - Verify tar.gz integrity on upload
  - Calculate and store checksums (SHA256)
  - Validate checksums on fetch
- [ ] **Size Limits**: Configurable max artifact size
- [ ] **Retention Policy**: Automatic cleanup of old versions
  - Keep last N versions
  - Keep versions within time window
  - Manual retention pinning
- [ ] **Compression Options**: Support different compression formats (gzip, zstd, etc.)

### API Improvements
- [ ] **Pagination**: Add pagination for version listing
- [ ] **Filtering**: Filter versions by date range, status, action
- [ ] **Batch Operations**: Bulk delete, bulk download
- [ ] **Artifact Search**: Search by metadata fields
- [ ] **Streaming Uploads**: Support chunked/multipart uploads for large files

### Observability
- [ ] **Structured Logging**: Replace print statements with proper logging (zerolog, zap)
- [ ] **Metrics**: 
  - Prometheus metrics endpoint
  - Request counts, durations, errors
  - Storage usage metrics
- [ ] **Tracing**: OpenTelemetry integration
- [ ] **Health Check Improvements**: 
  - Check backend connectivity
  - Check disk space
  - Dependency health

### Performance
- [ ] **Caching**: 
  - Cache version lists
  - Cache metadata
  - ETags for conditional requests
- [ ] **Compression**: Gzip compression for API responses
- [ ] **Connection Pooling**: For S3 and other network backends
- [ ] **Background Jobs**: Async processing for large operations

### Security
- [ ] **Authentication**: 
  - API key authentication
  - JWT token validation
  - Integration with PageWright auth service
- [ ] **Authorization**: Role-based access control (RBAC)
- [ ] **Rate Limiting**: Per-site and per-user rate limits
- [ ] **Input Validation**: 
  - Strict site_id and build_id validation
  - Prevent path traversal attacks
  - Sanitize metadata fields
- [ ] **Encryption**: 
  - Encryption at rest (for backends that support it)
  - TLS/HTTPS support

### Reliability
- [ ] **Retry Logic**: Automatic retries for transient failures
- [ ] **Circuit Breaker**: Prevent cascading failures
- [ ] **Graceful Degradation**: Fallback strategies when backend unavailable
- [ ] **Backup and Restore**: Tools for backing up and restoring storage
- [ ] **Data Integrity Checks**: Periodic verification of stored artifacts

## Testing
- [ ] **Load Testing**: Performance testing with realistic workloads
- [ ] **Chaos Testing**: Test behavior under failure conditions
- [ ] **End-to-End Tests**: Full workflow tests with other PageWright services
- [ ] **Benchmark Tests**: Performance regression detection

## Operations
- [ ] **Kubernetes Deployment**: 
  - Helm chart
  - Deployment manifests
  - Service discovery
- [ ] **Monitoring Dashboards**: Grafana dashboards for metrics
- [ ] **Alerting Rules**: Alert definitions for critical conditions
- [ ] **Runbooks**: Operational documentation for common scenarios
- [ ] **Migration Tools**: Tools for migrating data between backends

## Documentation
- [ ] **API Documentation**: OpenAPI/Swagger specification
- [ ] **Backend Plugin Guide**: How to implement new storage backends
- [ ] **Performance Tuning Guide**: Optimization recommendations
- [ ] **Troubleshooting Guide**: Common issues and solutions

## Future Phases Integration
- [ ] **Phase 2 Integration**: Coordinate with Redis locking service
- [ ] **Phase 3 Integration**: Support worker container artifact operations
- [ ] **Phase 4 Integration**: Deployer service integration
- [ ] **Phase 5 Integration**: UI/API integration

## Nice to Have
- [ ] **Artifact Deduplication**: Content-addressable storage to save space
- [ ] **Multi-region Support**: Geographic distribution of artifacts
- [ ] **Artifact Signing**: Cryptographic signatures for artifact integrity
- [ ] **Webhook Notifications**: Notify on artifact events
- [ ] **CLI Tool**: Command-line interface for storage operations
- [ ] **Admin API**: Administrative operations (cleanup, stats, etc.)
