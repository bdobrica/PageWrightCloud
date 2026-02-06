# PageWright Serving Service

The Serving Service is an nginx-based static site hosting platform with an HTTP API for deployment, version management, and maintenance control.

## Architecture

- **nginx**: Serves static HTML/CSS/JS files
- **Runner**: HTTP API that controls nginx configuration and site deployment

## Features

- ✅ Artifact deployment from storage service
- ✅ Version management with symlinks (public/preview)
- ✅ Automatic cleanup of old versions (keep max N per site)
- ✅ Domain aliases
- ✅ Per-site enable/disable (503 mode)
- ✅ Global maintenance mode
- ✅ CloudFlare-ready (reverse proxy compatible)

## Configuration

Environment variables with `PAGEWRIGHT_` prefix:

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVING_PORT` | `8083` | HTTP API port |
| `WWW_ROOT` | `/var/www` | Root directory for sites |
| `NGINX_SITES_ENABLED` | `/etc/nginx/sites-enabled` | nginx config directory |
| `NGINX_RELOAD_COMMAND` | `nginx -s reload` | Command to reload nginx |
| `STORAGE_URL` | - | Storage service URL (Phase 1) |
| `MAX_VERSIONS_PER_SITE` | `10` | Max artifact versions to keep |
| `MAINTENANCE_PAGE_PATH` | `/etc/pagewright/503.html` | Path to 503 page |

## API Endpoints

All endpoints are internal-only (firewall protected, not exposed to internet).

### Deploy Artifact

```bash
POST /sites/{fqdn}/artifacts
Content-Type: application/json

{
  "site_id": "blog-example-com",
  "version_id": "v1-20240101120000"
}
```

Downloads artifact from storage service and unpacks to `/var/www/{domain}/{fqdn}/artifacts/{version}/`.

### Activate Version (Public)

```bash
POST /sites/{fqdn}/activate
Content-Type: application/json

{
  "version_id": "v1-20240101120000"
}
```

Updates the `public` symlink to point to specified version.

### Activate Version (Preview)

```bash
POST /sites/{fqdn}/preview
Content-Type: application/json

{
  "version_id": "v1-20240101120000"
}
```

Updates the `preview` symlink to point to specified version. Accessible at `https://{fqdn}/preview/`.

### Manage Aliases

```bash
POST /sites/{fqdn}/aliases
Content-Type: application/json

{
  "aliases": ["www.example.com", "example.com"]
}
```

Updates domain aliases for the site (nginx `server_name` directive).

### Disable Site

```bash
POST /sites/{fqdn}/disable
```

Configures nginx to return 503 for the site (maintenance mode per-site).

### Enable Site

```bash
POST /sites/{fqdn}/enable
```

Re-enables the site (removes 503 response).

### Remove Site

```bash
DELETE /sites/{fqdn}
```

Removes site directory and nginx configuration completely.

### Global Maintenance Mode

```bash
# Enable
POST /maintenance/enable

# Disable
POST /maintenance/disable
```

Creates/removes `000-maintenance` default_server config that catches all requests.

### Health Check

```bash
GET /health
```

Returns `{"status":"ok"}`.

## Directory Structure

```
/var/www/{domain}/{fqdn}/
├── artifacts/
│   ├── v1-20240101120000/
│   │   └── public/
│   │       └── index.html
│   └── v2-20240102130000/
│       └── public/
│           └── index.html
├── public -> artifacts/v2-20240102130000/public
└── preview -> artifacts/v1-20240101120000/public
```

Example for `blog.example.com`:
- Path: `/var/www/example.com/blog.example.com/`
- Public URL: `https://blog.example.com/`
- Preview URL: `https://blog.example.com/preview/`

## nginx Configuration

Each site gets a config file at `/etc/nginx/sites-enabled/{fqdn}`:

```nginx
server {
    listen 80;
    server_name blog.example.com www.example.com;
    
    root /var/www/example.com/blog.example.com/public;
    index index.html;
    
    location / {
        try_files $uri $uri/ /index.html;
    }
    
    location /preview/ {
        alias /var/www/example.com/blog.example.com/preview/;
        try_files $uri $uri/ /preview/index.html;
    }
    
    error_page 503 /503.html;
    location = /503.html {
        root /etc/pagewright;
        internal;
    }
    
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
}
```

## Version Cleanup

Cleanup runs after each deployment:
1. List all versions in `artifacts/` directory
2. Read `public` and `preview` symlinks to identify protected versions
3. Sort remaining versions by access time (newest first)
4. Keep up to `MAX_VERSIONS_PER_SITE` unprotected versions
5. Delete oldest excess versions

## Docker

### Build

```bash
make docker-build
```

### Run

```bash
make docker-up
```

Services:
- nginx: http://localhost:8084
- runner: http://localhost:8083

### Stop

```bash
make docker-down
```

## Testing

```bash
# Unit tests
make test-unit

# Integration tests (requires docker-compose)
make docker-up
make test-integration
```

## Development

```bash
# Build binary
make build

# Run locally
export PAGEWRIGHT_STORAGE_URL=http://localhost:8080
export PAGEWRIGHT_NGINX_RELOAD_COMMAND="echo reload"
./serving-runner
```

## Production Deployment

1. **Storage Service**: Ensure Phase 1 storage service is running and accessible
2. **Firewall**: Block port 8083 from internet, allow only from internal network
3. **CloudFlare**: Configure as reverse proxy in front of nginx (port 80/443)
4. **nginx**: Production nginx configuration with SSL/TLS certificates
5. **Monitoring**: Monitor disk usage in `/var/www`, nginx logs, runner logs

## Integration with Phase 2 (Manager)

The Manager service (Phase 2) should call these endpoints after job completion:

1. Worker finishes job → uploads artifact to storage
2. Manager calls serving API:
   - `POST /sites/{fqdn}/artifacts` - deploy artifact
   - `POST /sites/{fqdn}/activate` - activate as public (if requested)
3. Site is live at `https://{fqdn}/`

## Security

- **Internal API**: Runner HTTP API (port 8083) must NOT be exposed to internet
- **Firewall Rules**: Allow 8083 only from trusted internal IPs
- **nginx**: Only nginx (port 80/443) should be internet-facing
- **CloudFlare**: Acts as WAF, DDoS protection, and CDN
- **Static Content**: Only serves static files, no code execution

## Limitations

- Static sites only (HTML/CSS/JS)
- No server-side rendering
- No websockets (yet)
- No custom nginx modules

## Next Steps

- Phase 4: Deployer service (BFF + UI for end users)
- SSL/TLS certificate management (Let's Encrypt integration)
- Websocket support for real-time updates
- Custom domain verification
