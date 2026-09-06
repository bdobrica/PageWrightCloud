# Serving Service

**Port**: 8083

nginx-based static site hosting with atomic deployments and version management.

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| POST | `/sites/{fqdn}/artifacts` | Deploy artifact from storage |
| POST | `/sites/{fqdn}/activate` | Activate version as public |
| POST | `/sites/{fqdn}/preview` | Activate version as preview |
| POST | `/sites/{fqdn}/aliases` | Update domain aliases |
| POST | `/sites/{fqdn}/disable` | Enable maintenance mode |
| POST | `/sites/{fqdn}/enable` | Disable maintenance mode |
| DELETE | `/sites/{fqdn}` | Remove site completely |
| POST | `/maintenance/enable` | Global maintenance mode |
| POST | `/maintenance/disable` | Disable global maintenance |

## Request/Response Formats

### Deploy Artifact

**Request:**
```json
{
  "site_id": "blog-example-com",
  "version_id": "v1-20240101120000"
}
```

Downloads from storage and unpacks to `/var/www/{domain}/{fqdn}/artifacts/{version}/`.

### Activate Version

**Request:**
```json
{
  "version_id": "v1-20240101120000"
}
```

Updates symlink:
- `/activate`: Updates `public` symlink
- `/preview`: Updates `preview` symlink

### Update Aliases

**Request:**
```json
{
  "aliases": ["www.example.com", "example.com"]
}
```

Updates nginx `server_name` directive and reloads nginx.

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

**Example for blog.example.com (default public hosting configuration):**

- Path: `/var/www/example.com/blog.example.com/`
- Public URL: `http://blog.example.com:8084/`
- Preview URL: `http://preview.blog.example.com:8084/`

See [hosting URL configuration, DNS/TLS and migration](../../docs/PREVIEW_ACTIVATION.md).

## nginx Configuration

### Per-Site Config

Generated at `/etc/nginx/sites-enabled/{fqdn}`:

```nginx
server {
    listen 80;
    absolute_redirect off;
    server_name blog.example.com www.blog.example.com;

    root /var/www/example.com/blog.example.com/public;
    index index.html;

    # Main site
    location / {
        try_files $uri $uri/ =404;
    }

    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
}

server {
    listen 80;
    absolute_redirect off;
    server_name preview.blog.example.com;
    root /var/www/example.com/blog.example.com/preview;
    index index.html;
    location / { try_files $uri $uri/ =404; }
    # Same security headers and disabled-site policy as live.
}
```

### Maintenance Mode (Per-Site)

When disabled, config returns 503:

```nginx
server {
    listen 80;
    server_name blog.example.com;
    return 503;
}
```

### Global Maintenance

Creates `000-maintenance` default_server:

```nginx
server {
    listen 80 default_server;
    server_name _;
    root /etc/pagewright;
    try_files /503.html =503;
}
```

## Version Cleanup

Automatic cleanup after each deployment:

1. Protect exact live/preview and receipt-pinned versions, plus a one-minute grace period.
2. Leave staging/private entries alone; refuse cleanup on uncertain pointer/receipt evidence.
3. Retain newest inactive entries within `PAGEWRIGHT_MAX_VERSIONS_PER_SITE` (default 10), a soft total budget.
4. Remove only excess serving-cache copies; canonical storage archives remain available for rollback.

Protected versions can exceed the budget and are never deleted. See [atomic activation,
retention, rollback and upgrade boundaries](../../docs/ATOMIC_ACTIVATION.md).

## nginx Reload

After configuration changes:
```bash
nginx -s reload
```

Zero-downtime updates via SIGHUP signal.

## Configuration

Environment variables (all with `PAGEWRIGHT_` prefix):

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `SERVING_PORT` | `8083` | No | HTTP API port |
| `WWW_ROOT` | `/var/www` | No | Site root directory |
| `NGINX_SITES_ENABLED` | `/etc/nginx/sites-enabled` | No | nginx config dir |
| `NGINX_RELOAD_COMMAND` | `nginx -s reload` | No | Reload command |
| `STORAGE_URL` | - | Yes | Storage service URL |
| `MAX_VERSIONS_PER_SITE` | `10` | No | Max versions to keep |
| `MAINTENANCE_PAGE_PATH` | `/etc/pagewright/503.html` | No | Maintenance page path |

## Running

```bash
# Development
cd pagewright/serving
make run

# Docker Compose (includes nginx)
make docker-up

# Tests
make test
```

## Docker Deployment

### docker-compose.yaml

```yaml
version: '3.8'

services:
  nginx:
    image: nginx:alpine
    ports:
      - "80:80"
    volumes:
      - ./var/www:/var/www:ro
      - ./etc/nginx/sites-enabled:/etc/nginx/sites-enabled:ro
      - ./etc/nginx/nginx.conf:/etc/nginx/nginx.conf:ro
      - ./etc/pagewright:/etc/pagewright:ro
    depends_on:
      - serving-runner

  serving-runner:
    build: .
    ports:
      - "8083:8083"
    environment:
      - PAGEWRIGHT_SERVING_PORT=8083
      - PAGEWRIGHT_STORAGE_URL=http://storage:8080
      - PAGEWRIGHT_WWW_ROOT=/var/www
      - PAGEWRIGHT_NGINX_SITES_ENABLED=/etc/nginx/sites-enabled
    volumes:
      - ./var/www:/var/www
      - ./etc/nginx/sites-enabled:/etc/nginx/sites-enabled
      - ./etc/pagewright:/etc/pagewright
```

## CloudFlare Integration

The serving service is designed to work behind CloudFlare (or any reverse proxy):

- nginx listens on port 80 (HTTP only)
- CloudFlare handles SSL/TLS termination
- CloudFlare caches static assets
- Use CloudFlare API to invalidate cache after deployments

No SSL certificates needed on serving host.
