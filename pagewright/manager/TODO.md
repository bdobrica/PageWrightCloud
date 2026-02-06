# TODO

## Phase 2 Enhancements

### Queue Backends
- [ ] **NATS Backend**: Implement NATS Streaming/JetStream backend
- [ ] **Kafka Backend**: Support for Apache Kafka
- [ ] **RabbitMQ Backend**: Support for RabbitMQ
- [ ] **Queue Priorities**: Priority-based job processing
- [ ] **Dead Letter Queue**: Handle failed jobs with retry logic

### Lock Management
- [ ] **Lock Monitoring**: Track lock acquisition failures and timeouts
- [ ] **Auto-renewal**: Background goroutine to auto-renew locks for long-running jobs
- [ ] **Lock Debugging**: API endpoints to view current locks
- [ ] **Graceful Lock Release**: Ensure locks released on service shutdown
- [ ] **Lock Metrics**: Track lock contention and hold times

### Worker Management
- [ ] **Worker Health Checks**: Monitor worker health and restart if needed
- [ ] **Worker Logs**: Stream worker logs back to manager
- [ ] **Worker Cancellation**: API to cancel running jobs
- [ ] **Worker Resource Limits**: Configure CPU/memory limits
- [ ] **Worker Pools**: Pre-warm worker containers for faster startup
- [ ] **Worker Scaling**: Auto-scale based on queue depth

### Docker Spawner
- [ ] **Real Docker API Integration**: Use Docker SDK instead of stub
- [ ] **SSH Tunnel Support**: Connect to remote Docker hosts via SSH
- [ ] **Container Cleanup**: Remove stopped containers
- [ ] **Volume Mounting**: Mount shared volumes for artifacts
- [ ] **Network Configuration**: Custom network settings
- [ ] **Resource Constraints**: CPU/memory limits per container

### Kubernetes Spawner
- [ ] **Real Kubernetes Integration**: Use client-go library
- [ ] **Job vs Pod**: Use Kubernetes Jobs instead of bare Pods
- [ ] **RBAC Configuration**: Service account and role setup
- [ ] **Namespace Management**: Support multiple namespaces
- [ ] **Resource Quotas**: Enforce resource limits
- [ ] **Pod Affinity**: Schedule workers on specific nodes

### API Improvements
- [ ] **Job Listing**: List all jobs with filtering
- [ ] **Job Search**: Search jobs by site, status, date range
- [ ] **Pagination**: Paginate job lists
- [ ] **Job Cancellation**: Cancel pending or running jobs
- [ ] **Bulk Operations**: Batch create/cancel jobs
- [ ] **WebSocket Updates**: Real-time job status updates
- [ ] **Job History**: Long-term job history storage

### Status Tracking
- [ ] **Detailed Progress**: Worker reports progress percentage
- [ ] **Status Transitions**: Track all status changes with timestamps
- [ ] **Job Timeline**: Visual timeline of job execution
- [ ] **Failure Reasons**: Categorize failure types
- [ ] **Retry Logic**: Automatic retry for transient failures

### Observability
- [ ] **Structured Logging**: Replace fmt.Printf with proper logging
- [ ] **Metrics**: 
  - Prometheus metrics endpoint
  - Job creation/completion rates
  - Queue depth
  - Lock acquisition success/failure
  - Worker spawn times
- [ ] **Tracing**: OpenTelemetry integration
- [ ] **Alerts**: Alert on stuck jobs, lock timeouts, queue backlog

### Performance
- [ ] **Connection Pooling**: Redis connection pooling
- [ ] **Batch Operations**: Batch Redis operations where possible
- [ ] **Caching**: Cache frequently accessed job data
- [ ] **Async Processing**: Non-blocking job creation
- [ ] **Rate Limiting**: Prevent queue flooding

### Security
- [ ] **Authentication**: API key or JWT authentication
- [ ] **Authorization**: Per-site access control
- [ ] **Rate Limiting**: Per-user/site rate limits
- [ ] **Input Validation**: Strict validation of job parameters
- [ ] **Secret Management**: Secure worker credentials
- [ ] **Audit Logging**: Track all API operations

### Reliability
- [ ] **Circuit Breaker**: Prevent cascading failures
- [ ] **Graceful Shutdown**: Finish in-progress jobs before shutdown
- [ ] **Job Persistence**: Survive manager restarts
- [ ] **Redis Failover**: Support Redis Sentinel/Cluster
- [ ] **Backup Queue**: Secondary queue for high availability
- [ ] **Idempotency**: Prevent duplicate job processing

### Integration
- [ ] **Storage Service Client**: Call storage service for artifact management
- [ ] **Deployer Service Integration**: Coordinate with deployer
- [ ] **Event Notifications**: Publish events to message bus
- [ ] **Webhook Support**: HTTP webhooks for job events

## Testing
- [ ] **Load Testing**: Test under high job volume
- [ ] **Chaos Testing**: Simulate Redis failures, network issues
- [ ] **Lock Contention Tests**: Test high-contention scenarios
- [ ] **Worker Failure Tests**: Test worker crashes and timeouts
- [ ] **Benchmark Tests**: Performance benchmarks

## Operations
- [ ] **Kubernetes Deployment**: 
  - Deployment manifests
  - Service definitions
  - ConfigMaps/Secrets
- [ ] **Monitoring Dashboards**: Grafana dashboards
- [ ] **Alerting Rules**: Alert definitions
- [ ] **Runbooks**: Operational procedures
- [ ] **Backup/Restore**: Redis backup procedures
- [ ] **Migration Tools**: Data migration utilities

## Documentation
- [ ] **API Documentation**: OpenAPI/Swagger spec
- [ ] **Plugin Development Guide**: How to add new backends
- [ ] **Worker Development Guide**: How to build workers
- [ ] **Architecture Diagrams**: System architecture visuals
- [ ] **Deployment Guide**: Production deployment instructions

## Future Phases Integration
- [ ] **Phase 3 Worker**: Replace stub with real worker
- [ ] **Phase 4 Deployer**: Coordinate deployment
- [ ] **Phase 5 UI/API**: Frontend integration

## Nice to Have
- [ ] **Job Scheduling**: Schedule jobs for future execution
- [ ] **Recurring Jobs**: Cron-like scheduled jobs
- [ ] **Job Dependencies**: Chain jobs together
- [ ] **Parallel Workers**: Multiple workers per job
- [ ] **Job Templates**: Predefined job configurations
- [ ] **CLI Tool**: Command-line interface for job management
- [ ] **Admin Dashboard**: Web UI for monitoring/management
