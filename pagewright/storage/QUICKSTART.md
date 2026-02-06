# Quick Start Guide

This guide will help you get the PageWright Storage Service up and running in less than 5 minutes.

## Prerequisites

- Docker and Docker Compose installed
- curl and jq (optional, for testing)
- Make (optional, but recommended)

## Step 1: Start the Service

```bash
cd /home/bogdan/GitHub/PageWrightCloud/pagewright/storage
make docker-up
```

This will:
- Start an NFS server container
- Build and start the storage service container
- Expose the API on port 8080

## Step 2: Verify the Service

Check if the service is healthy:

```bash
curl http://localhost:8080/health
```

Expected response:
```json
{
  "status": "healthy",
  "time": "2026-02-06T15:30:00Z"
}
```

## Step 3: Try the Demo

Run the provided demo script to see all API endpoints in action:

```bash
./examples/demo.sh
```

This will:
- Create a test artifact
- Upload it to the storage service
- Write log entries
- List versions
- Download the artifact back

## Step 4: Manual Testing

### Upload an Artifact

```bash
# Create a test artifact
echo "Hello PageWright!" > test.txt
tar -czf test.tar.gz test.txt

# Upload it
curl -X PUT \
  http://localhost:8080/sites/my-site/artifacts/build-1 \
  --data-binary @test.tar.gz
```

### Write a Log Entry

```bash
curl -X POST \
  http://localhost:8080/sites/my-site/logs \
  -H "Content-Type: application/json" \
  -d '{
    "build_id": "build-1",
    "action": "build",
    "status": "success",
    "metadata": {
      "user": "alice"
    }
  }'
```

### List Versions

```bash
curl http://localhost:8080/sites/my-site/versions | jq .
```

### Download an Artifact

```bash
curl http://localhost:8080/sites/my-site/artifacts/build-1 \
  -o downloaded.tar.gz
```

## Step 5: Inspect the Storage

View files stored in the NFS volume:

```bash
# List all sites
docker exec pagewright-storage ls -la /nfs/sites/

# List artifacts for a specific site
docker exec pagewright-storage ls -la /nfs/sites/my-site/artifacts/

# List logs for a specific site
docker exec pagewright-storage ls -la /nfs/sites/my-site/logs/
```

## Step 6: View Logs

Monitor the service logs:

```bash
make docker-logs
```

Or:

```bash
docker-compose logs -f storage-service
```

## Step 7: Stop the Service

When you're done:

```bash
make docker-down
```

## Running Tests

### Unit Tests

```bash
make test-unit
```

### Integration Tests

```bash
make docker-up
make test-integration
make docker-down
```

### Full Test Suite

```bash
make e2e
```

## Troubleshooting

### Service won't start

1. Check if port 8080 is already in use:
   ```bash
   sudo lsof -i :8080
   ```

2. Check Docker logs:
   ```bash
   docker-compose logs storage-service
   ```

### NFS mount issues

If you see NFS-related errors, ensure the NFS container has proper privileges:

```bash
docker-compose down
docker-compose up -d
```

### Tests fail

Make sure the service is running before integration tests:

```bash
curl http://localhost:8080/health
```

## Next Steps

- Read the [README.md](README.md) for complete documentation
- Check [TODO.md](TODO.md) for planned features
- Explore the API with your own tools

## Configuration

You can customize the service with environment variables:

```bash
# In docker-compose.yaml, modify the environment section:
environment:
  - PAGEWRIGHT_PORT=9090
  - PAGEWRIGHT_STORAGE_BACKEND=nfs
  - PAGEWRIGHT_NFS_BASE_PATH=/custom/path
```

Then restart:

```bash
docker-compose down
docker-compose up -d
```

## Support

For issues or questions, check the [README.md](README.md) or open an issue in the repository.
