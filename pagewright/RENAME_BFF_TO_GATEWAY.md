# BFF to Gateway Rename Summary

This document summarizes the renaming of the BFF (Backend for Frontend) service to Gateway.

## Changes Made

### 1. Directory Structure
- **Renamed**: `/pagewright/bff/` → `/pagewright/gateway/`
- **Renamed**: `/pagewright/gateway/cmd/bff/` → `/pagewright/gateway/cmd/gateway/`

### 2. Go Module
- **Updated**: `go.mod` module path from `github.com/PageWrightCloud/pagewright/bff` to `github.com/PageWrightCloud/pagewright/gateway`
- **Updated**: All import paths in `.go` files from `github.com/PageWrightCloud/pagewright/bff/internal/*` to `github.com/PageWrightCloud/pagewright/gateway/internal/*`

### 3. Build Configuration
- **Makefile**: Updated build target to create `bin/gateway` instead of `bin/bff`
- **Dockerfile**: Updated build command to create `/gateway` binary instead of `/bff`
- **Build command**: Now uses `cmd/gateway/main.go` instead of `cmd/bff/main.go`

### 4. Environment Variables
- **Changed**: `PAGEWRIGHT_BFF_PORT` → `PAGEWRIGHT_GATEWAY_PORT`
- **Location**: `internal/config/config.go`
- **Default**: Still 8085

### 5. Documentation Updates
- **gateway/README.md**:
  - Title changed from "PageWright BFF (Backend for Frontend)" to "PageWright Gateway"
  - All references to "BFF" replaced with "Gateway"
  - Updated all environment variable examples
  - Updated all command examples (go run, docker-compose logs)

- **gateway/IMPLEMENTATION_SUMMARY.md**:
  - Updated entry point reference to `cmd/gateway/main.go`
  - Updated build commands

- **UI_IMPLEMENTATION_STATUS.md**:
  - Updated all references from "BFF" to "Gateway"
  - Updated API types comment
  - Updated endpoint references
  - Updated docker-compose dependency notes

### 6. UI Configuration
- **ui/src/types/api.ts**: Updated comment from "API Types matching BFF" to "API Types matching Gateway"
- **ui/nginx.conf**: Updated proxy comment from "proxy BFF through nginx" to "proxy Gateway through nginx"

### 7. Alias Updates
- Internal alias `bffWebsocket` changed to `gatewayWebsocket` in handlers

## Files Modified

### Gateway Service (23 files)
1. `go.mod` - Module path
2. `Makefile` - Build target
3. `Dockerfile` - Binary name and path
4. `README.md` - Service name and references
5. `IMPLEMENTATION_SUMMARY.md` - Build commands
6. `internal/config/config.go` - Environment variable name
7. `internal/handlers/websocket.go` - Import alias and Client reference
8. `internal/handlers/auth.go` - Import path
9. `internal/handlers/sites.go` - Import path
10. `internal/handlers/build.go` - Import path
11. `internal/handlers/versions.go` - Import path
12. `internal/handlers/aliases.go` - Import path
13. `internal/middleware/auth.go` - Import path
14. `internal/database/users.go` - Import path
15. `internal/database/sites.go` - Import path
16. `internal/database/versions.go` - Import path
17. `internal/database/aliases.go` - Import path
18. `internal/database/password_reset.go` - Import path
19. `internal/websocket/hub.go` - Import path
20. `test/unit/auth_test.go` - Import paths
21. `test/integration/auth_integration_test.go` - Import paths
22. `cmd/gateway/main.go` - Import paths (all internal imports)

### UI Files (2 files)
1. `ui/src/types/api.ts` - Comment update
2. `ui/nginx.conf` - Comment update

### Documentation (1 file)
1. `UI_IMPLEMENTATION_STATUS.md` - All BFF references

## Build Verification

✅ Gateway service compiles successfully:
```bash
cd pagewright/gateway
go mod tidy
go build -o bin/gateway cmd/gateway/main.go
```

Binary created: `bin/gateway` (11MB)

## Migration Notes

### For Existing Deployments
1. Update environment variable from `PAGEWRIGHT_BFF_PORT` to `PAGEWRIGHT_GATEWAY_PORT`
2. Update any docker-compose files that reference the `bff` service name to `gateway`
3. Update any scripts or CI/CD pipelines that reference the old binary name
4. Update any documentation or runbooks that reference "BFF"

### For Development
1. Run `go mod tidy` in the gateway directory after pulling these changes
2. Rebuild the gateway binary
3. Update any local environment variable files (.env, shell configs)
4. No database migrations required - this is purely a naming change

## API Compatibility
✅ No API changes - all endpoints remain the same
✅ No breaking changes for UI or clients
✅ Port and configuration behavior unchanged (except env var name)
