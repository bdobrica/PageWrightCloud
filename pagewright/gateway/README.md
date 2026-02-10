# Gateway Service

**Port**: 8085

User-facing REST API for authentication, site management, and build orchestration.

## Database Schema

### Users Table
```sql
CREATE TABLE users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email VARCHAR(255) UNIQUE NOT NULL,
  password_hash VARCHAR(255),
  oauth_provider VARCHAR(50),
  oauth_id VARCHAR(255),
  created_at TIMESTAMP DEFAULT NOW(),
  UNIQUE(oauth_provider, oauth_id)
);
```

### Sites Table
```sql
CREATE TABLE sites (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  fqdn VARCHAR(255) UNIQUE NOT NULL,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  template_id VARCHAR(100) NOT NULL,
  live_version_id VARCHAR(100),
  preview_version_id VARCHAR(100),
  enabled BOOLEAN DEFAULT true,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW()
);
```

### Site Aliases Table
```sql
CREATE TABLE site_aliases (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  site_id UUID NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
  alias VARCHAR(255) UNIQUE NOT NULL,
  created_at TIMESTAMP DEFAULT NOW()
);
```

### Versions Table (Cache)
```sql
CREATE TABLE versions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  site_id UUID NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
  build_id VARCHAR(100) NOT NULL,
  status VARCHAR(50) NOT NULL,
  created_at TIMESTAMP DEFAULT NOW(),
  UNIQUE(site_id, build_id)
);
```

### Password Reset Tokens Table
```sql
CREATE TABLE password_reset_tokens (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token VARCHAR(255) UNIQUE NOT NULL,
  expires_at TIMESTAMP NOT NULL,
  created_at TIMESTAMP DEFAULT NOW()
);
```

## API Endpoints

### Authentication

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/auth/register` | Create account with email/password |
| POST | `/auth/login` | Login with credentials |
| GET | `/auth/google/login` | Initiate Google OAuth flow |
| GET | `/auth/google/callback` | OAuth callback handler |
| POST | `/auth/forgot-password` | Request password reset email |
| POST | `/auth/reset-password` | Reset password with token |
| POST | `/auth/update-password` | Change password (authenticated) |

### Sites

All require `Authorization: Bearer <token>` header.

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/sites` | Create new site |
| GET | `/sites` | List user's sites (paginated) |
| GET | `/sites/{fqdn}` | Get site details |
| DELETE | `/sites/{fqdn}` | Delete site and all data |
| POST | `/sites/{fqdn}/enable` | Enable site serving |
| POST | `/sites/{fqdn}/disable` | Disable site (maintenance) |

### Aliases

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/sites/{fqdn}/aliases` | List domain aliases |
| POST | `/sites/{fqdn}/aliases` | Add domain alias |
| DELETE | `/sites/{fqdn}/aliases/{alias}` | Remove alias |

### Versions

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/sites/{fqdn}/versions` | List all versions (from storage) |
| POST | `/sites/{fqdn}/versions/{version_id}/deploy` | Deploy to live/preview |
| DELETE | `/sites/{fqdn}/versions/{version_id}` | Delete version artifact |
| GET | `/sites/{fqdn}/versions/{version_id}/download` | Download tar.gz artifact |

### Build (Chat Interface)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/sites/{fqdn}/build` | Submit build request (may return clarification question) |
| GET | `/ws` | WebSocket for real-time updates |

### Build Request Format

```json
// Initial request
{
  "message": "Add a contact form with email validation"
}

// Follow-up (if clarification needed)
{
  "message": "Add it as a separate page",
  "conversation_id": "uuid-from-previous-response"
}
```

### Response Formats

**Clear Request (Job Enqueued)**
```json
{
  "job_id": "uuid",
  "status": "queued"
}
```

**Needs Clarification**
```json
{
  "question": "Where should I add the contact form?",
  "conversation_id": "uuid"
}
```

## Configuration

Environment variables (all with `PAGEWRIGHT_` prefix):

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `GATEWAY_PORT` | `8085` | No | HTTP server port |
| `DATABASE_URL` | - | Yes | PostgreSQL connection string |
| `STORAGE_URL` | - | Yes | Storage service URL |
| `MANAGER_URL` | - | Yes | Manager service URL |
| `SERVING_URL` | - | Yes | Serving service URL |
| `JWT_SECRET` | - | Yes | JWT signing key |
| `JWT_EXPIRATION` | `15m` | No | Token lifetime |
| `GOOGLE_CLIENT_ID` | - | Yes | OAuth client ID |
| `GOOGLE_CLIENT_SECRET` | - | Yes | OAuth client secret |
| `GOOGLE_REDIRECT_URL` | - | Yes | OAuth callback URL |
| `LLM_KEY` | - | Yes | OpenAI API key |
| `LLM_URL` | `https://api.openai.com/v1` | No | OpenAI base URL |
| `DEFAULT_PAGE_SIZE` | `25` | No | Pagination default |

## User Management CLI

A command-line tool is provided for managing users directly in the database. This is especially useful for:
- Initial setup and creating admin users
- Local development and testing
- User management in containerized environments

### Available Commands

- `create -email <email> -password <password>` - Create a new user
- `list` - List all users with details
- `delete -email <email>` - Delete a user by email

### Method 1: Using Docker Compose (Recommended for Testing)

Start the gateway service:

```bash
cd pagewright/gateway
docker compose up -d
```

Wait for services to be ready, then create a user:

```bash
docker compose exec bff ./users create -email admin@pagewright.local -password admin123
```

List all users:

```bash
docker compose exec bff ./users list
```

### Method 2: Local Development (Without Docker)

1. Start PostgreSQL (via docker compose or locally):
   ```bash
   docker compose up -d postgres
   ```

2. Set environment variable:
   ```bash
   export PAGEWRIGHT_DATABASE_URL="postgres://pagewright:pagewright@localhost:5432/pagewright?sslmode=disable"
   ```

3. Build and use the CLI:
   ```bash
   make build-cli
   ./bin/users create -email test@example.com -password testpass123
   ./bin/users list
   ```

### Common Use Cases

**Create admin user for development:**
```bash
docker compose exec bff ./users create -email admin@dev.local -password devpass
```

**Create multiple test users:**
```bash
docker compose exec bff ./users create -email user1@test.com -password pass1
docker compose exec bff ./users create -email user2@test.com -password pass2
docker compose exec bff ./users create -email user3@test.com -password pass3
```

**View all users:**
```bash
docker compose exec bff ./users list
```

Output example:
```
ID                                    EMAIL                AUTH METHOD  CREATED AT
----                                  -----                -----------  ----------
550e8400-e29b-41d4-a716-446655440000  admin@dev.local      password     2026-02-10 14:30:00
660e8400-e29b-41d4-a716-446655440001  user1@test.com       password     2026-02-10 14:35:00

Total: 2 user(s)
```

**Delete a test user:**
```bash
docker compose exec bff ./users delete -email user1@test.com
```

### Integration with Testing Scripts

You can use the CLI in your test setup scripts:

```bash
#!/bin/bash
# setup-test-env.sh

# Start services
docker compose up -d

# Wait for database to be ready
sleep 5

# Create test users
docker compose exec -T bff ./users create -email admin@test.local -password admin
docker compose exec -T bff ./users create -email user@test.local -password user

echo "Test environment ready!"
```

### Troubleshooting

**Error: "PAGEWRIGHT_DATABASE_URL environment variable is required"**
- Make sure the environment variable is set (in docker compose it's already configured)

**Error: "Failed to connect to database"**
- Ensure PostgreSQL is running: `docker compose ps`
- Check database URL is correct

**Error: "User with email X already exists"**
- User already created, use `list` to see existing users
- Or delete the user first: `./users delete -email X`

## Running

```bash
# Development
cd pagewright/gateway
make run

# Docker
make docker-build
docker run -p 8085:8085 --env-file .env gateway

# Tests
make test
make test-integration
```
