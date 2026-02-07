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
