# PageWright Gateway

The Gateway service provides the user-facing REST API for the PageWright platform, handling authentication, site management, and orchestrating operations across storage, manager, and serving services.

## Features

- **Authentication**
  - Email/password registration and login with bcrypt password hashing
  - Google OAuth integration
  - JWT tokens with configurable expiration (default 15 minutes)
  
- **Site Management**
  - Create sites from templates with FQDN-based identification
  - Enable/disable sites
  - Full ownership validation
  - Paginated site listing
  
- **Domain Aliases**
  - Add custom domain aliases to sites
  - Automatic synchronization with serving service
  - Unique constraint enforcement
  
- **Version Management**
  - List all versions (from storage service)
  - Deploy versions to live or preview
  - Delete versions (GDPR compliance)
  - Download artifacts
  - Recent versions cache in PostgreSQL
  
- **Chat-Based Build System**
  - OpenAI-powered clarification loop
  - Three-template pattern:
    1. Evaluate if request is clear
    2. Generate clarification questions
    3. Generate final job instructions
  - Automatic job enqueueing to manager service

## Architecture

### Database Schema

**Users Table:**
```sql
- id: UUID (primary key)
- email: VARCHAR(255) UNIQUE
- password_hash: VARCHAR(255) (nullable for OAuth users)
- oauth_provider: VARCHAR(50) (nullable)
- oauth_id: VARCHAR(255) (nullable)
- created_at: TIMESTAMP
- UNIQUE(oauth_provider, oauth_id)
```

**Sites Table:**
```sql
- id: UUID (primary key)
- fqdn: VARCHAR(255) UNIQUE
- user_id: UUID (foreign key → users.id)
- template_id: VARCHAR(100)
- live_version_id: VARCHAR(100) (nullable)
- preview_version_id: VARCHAR(100) (nullable)
- enabled: BOOLEAN (default: true)
- created_at: TIMESTAMP
- updated_at: TIMESTAMP
```

**Site Aliases Table:**
```sql
- id: UUID (primary key)
- site_id: UUID (foreign key → sites.id)
- alias: VARCHAR(255) UNIQUE
- created_at: TIMESTAMP
```

**Versions Table:**
```sql
- id: UUID (primary key)
- site_id: UUID (foreign key → sites.id)
- build_id: VARCHAR(100)
- status: VARCHAR(50)
- created_at: TIMESTAMP
- UNIQUE(site_id, build_id)
```

### Service Communication

The Gateway communicates with other PageWright services via HTTP:

- **Storage Service** (port 8080): Artifact storage and retrieval
- **Manager Service** (port 8081): Job queue management
- **Serving Service** (port 8083): Site deployment and nginx configuration

**Note:** BFF does not access Redis directly. All caching and queue operations go through the appropriate services.

## API Endpoints

### Authentication

#### Register
```http
POST /auth/register
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "secure-password"
}
```

Response:
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": {
    "id": "uuid",
    "email": "user@example.com"
  }
}
```

#### Login
```http
POST /auth/login
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "secure-password"
}
```

Response: Same as register

#### Google OAuth
```http
GET /auth/google/login
```

Redirects to Google OAuth consent screen. After authorization, Google redirects to `/auth/google/callback` which returns the JWT token.

### Sites

All site endpoints require JWT authentication via `Authorization: Bearer <token>` header.

#### Create Site
```http
POST /sites
Authorization: Bearer <token>
Content-Type: application/json

{
  "fqdn": "mysite.example.com",
  "template_id": "template-1"
}
```

#### List Sites
```http
GET /sites?page=1&page_size=25
Authorization: Bearer <token>
```

Response:
```json
{
  "data": [...],
  "page": 1,
  "page_size": 25,
  "total": 42
}
```

#### Get Site
```http
GET /sites/{fqdn}
Authorization: Bearer <token>
```

#### Delete Site
```http
DELETE /sites/{fqdn}
Authorization: Bearer <token>
```

#### Enable/Disable Site
```http
POST /sites/{fqdn}/enable
POST /sites/{fqdn}/disable
Authorization: Bearer <token>
```

### Aliases

#### List Aliases
```http
GET /sites/{fqdn}/aliases
Authorization: Bearer <token>
```

#### Add Alias
```http
POST /sites/{fqdn}/aliases
Authorization: Bearer <token>
Content-Type: application/json

{
  "alias": "alias.example.com"
}
```

#### Delete Alias
```http
DELETE /sites/{fqdn}/aliases/{alias}
Authorization: Bearer <token>
```

### Versions

#### List Versions
```http
GET /sites/{fqdn}/versions?page=1&page_size=25
Authorization: Bearer <token>
```

#### Deploy Version
```http
POST /sites/{fqdn}/versions/{version_id}/deploy
Authorization: Bearer <token>
Content-Type: application/json

{
  "target": "live"  // or "preview"
}
```

#### Delete Version
```http
DELETE /sites/{fqdn}/versions/{version_id}
Authorization: Bearer <token>
```

#### Download Version
```http
GET /sites/{fqdn}/versions/{version_id}/download
Authorization: Bearer <token>
```

Returns tar.gz artifact.

### Build (Chat Interface)

#### Initiate Build
```http
POST /sites/{fqdn}/build
Authorization: Bearer <token>
Content-Type: application/json

{
  "message": "Add a contact form with email validation"
}
```

Response if clear:
```json
{
  "job_id": "job-uuid"
}
```

Response if needs clarification:
```json
{
  "question": "Where should I add the contact form? Header, footer, or separate page?",
  "conversation_id": "conversation-uuid"
}
```

#### Follow-up with Clarification
```http
POST /sites/{fqdn}/build
Authorization: Bearer <token>
Content-Type: application/json

{
  "message": "Add it as a separate page",
  "conversation_id": "conversation-uuid"
}
```

Response:
```json
{
  "job_id": "job-uuid"
}
```

## Configuration

All configuration is done via environment variables with the `PAGEWRIGHT_` prefix:

| Variable | Default | Description |
|----------|---------|-------------|
| `PAGEWRIGHT_BFF_PORT` | 8085 | HTTP server port |
| `PAGEWRIGHT_DATABASE_URL` | *(required)* | PostgreSQL connection string |
| `PAGEWRIGHT_STORAGE_URL` | *(required)* | Storage service URL |
| `PAGEWRIGHT_MANAGER_URL` | *(required)* | Manager service URL |
| `PAGEWRIGHT_SERVING_URL` | *(required)* | Serving service URL |
| `PAGEWRIGHT_JWT_SECRET` | *(required)* | JWT signing secret |
| `PAGEWRIGHT_JWT_EXPIRATION` | 15m | JWT token expiration |
| `PAGEWRIGHT_GOOGLE_CLIENT_ID` | *(required)* | Google OAuth client ID |
| `PAGEWRIGHT_GOOGLE_CLIENT_SECRET` | *(required)* | Google OAuth client secret |
| `PAGEWRIGHT_GOOGLE_REDIRECT_URL` | *(required)* | OAuth callback URL |
| `PAGEWRIGHT_LLM_KEY` | *(required)* | OpenAI API key |
| `PAGEWRIGHT_LLM_URL` | https://api.openai.com/v1 | OpenAI API base URL |
| `PAGEWRIGHT_DEFAULT_PAGE_SIZE` | 25 | Default pagination size |

## Development

### Prerequisites

- Go 1.22 or later
- PostgreSQL 15 or later
- Running instances of storage, manager, and serving services
- Google OAuth credentials
- OpenAI API key

### Setup

1. Clone the repository
2. Copy `.env.example` to `.env` and fill in values
3. Install dependencies:
   ```bash
   go mod download
   ```

### Running Locally

```bash
# Set environment variables
export PAGEWRIGHT_BFF_PORT=8085
export PAGEWRIGHT_DATABASE_URL="postgres://user:pass@localhost/pagewright?sslmode=disable"
export PAGEWRIGHT_STORAGE_URL="http://localhost:8080"
export PAGEWRIGHT_MANAGER_URL="http://localhost:8081"
export PAGEWRIGHT_SERVING_URL="http://localhost:8083"
export PAGEWRIGHT_JWT_SECRET="your-secret-key"
export PAGEWRIGHT_GOOGLE_CLIENT_ID="your-client-id"
export PAGEWRIGHT_GOOGLE_CLIENT_SECRET="your-client-secret"
export PAGEWRIGHT_GOOGLE_REDIRECT_URL="http://localhost:8085/auth/google/callback"
export PAGEWRIGHT_LLM_KEY="your-openai-key"

# Run migrations and start server
go run cmd/bff/main.go
```

### Using Docker Compose

```bash
# Create .env file with credentials
cat > .env << EOF
GOOGLE_CLIENT_ID=your-client-id
GOOGLE_CLIENT_SECRET=your-client-secret
OPENAI_API_KEY=your-openai-key
EOF

# Start all services
make docker-up

# View logs
docker-compose logs -f gateway

# Stop services
make docker-down
```

### Testing

```bash
# Run all tests
make test

# Run unit tests only
make test-unit

# Run integration tests only
make test-integration
```

### Building

```bash
# Build binary
make build

# Build Docker image
make docker-build
```

## Testing

### Unit Tests

Unit tests mock external dependencies (database, service clients, LLM) and test business logic in isolation.

Location: `internal/*/`

Run: `make test-unit`

### Integration Tests

Integration tests use a real PostgreSQL instance and test the full request/response cycle.

Location: `test/integration/`

Run: `make test-integration`

## Security Considerations

1. **JWT Secret**: Use a strong, random secret in production (minimum 256 bits)
2. **Password Hashing**: Uses bcrypt with cost factor 10
3. **OAuth State**: Validates state parameter to prevent CSRF attacks
4. **Ownership Validation**: All operations verify user owns the resource
5. **HTTPS**: Use HTTPS in production (configure reverse proxy)
6. **CORS**: Configure allowed origins in production
7. **Database**: Use connection pooling and prepared statements (sqlx)
8. **Rate Limiting**: Consider adding rate limiting for auth endpoints

## Deployment

### Environment Variables

Ensure all required environment variables are set:

```bash
# Required
PAGEWRIGHT_DATABASE_URL=postgres://...
PAGEWRIGHT_STORAGE_URL=http://...
PAGEWRIGHT_MANAGER_URL=http://...
PAGEWRIGHT_SERVING_URL=http://...
PAGEWRIGHT_JWT_SECRET=...
PAGEWRIGHT_GOOGLE_CLIENT_ID=...
PAGEWRIGHT_GOOGLE_CLIENT_SECRET=...
PAGEWRIGHT_GOOGLE_REDIRECT_URL=https://...
PAGEWRIGHT_LLM_KEY=...

# Optional
PAGEWRIGHT_BFF_PORT=8085
PAGEWRIGHT_JWT_EXPIRATION=15m
PAGEWRIGHT_LLM_URL=https://api.openai.com/v1
PAGEWRIGHT_DEFAULT_PAGE_SIZE=25
```

### Database Migrations

Migrations run automatically on startup. The service creates a `schema_migrations` table to track applied migrations.

### Health Check

```http
GET /health
```

Returns `200 OK` if service is running.

## Troubleshooting

### Database Connection Issues

- Verify PostgreSQL is running and accessible
- Check connection string format
- Ensure database exists or service has permission to create it

### Service Communication Issues

- Verify storage, manager, and serving services are running
- Check URLs are accessible from BFF container/host
- Review firewall rules

### Authentication Issues

- Verify JWT secret matches across restarts
- Check token expiration settings
- Ensure Google OAuth credentials are correct
- Verify redirect URL matches Google Console configuration

### OpenAI Issues

- Verify API key is valid and has credits
- Check LLM URL is correct
- Review rate limits

## License

[Your License Here]
