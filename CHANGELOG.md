# Changelog

All notable changes to PageWrightCloud will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial project structure with 6 microservices
- Gateway service with authentication and site management
- Manager service with Redis job queue and worker orchestration
- Storage service with NFS backend
- Worker service with Codex integration
- Serving service with nginx-based hosting
- React/TypeScript UI foundation

### Changed
- Expanded runtime composition to include themes service and UI container in root docker-compose setup.
- Added root Makefile targets for compiler build/test/coverage integration.
- Fixed root UI compose configuration to use correct Vite build-time variables (`VITE_PAGEWRIGHT_*`).

### Implemented
- UI pages for dashboard, chat, profile, forgot/reset password, and site creation.
- UI components for site cards, version actions/listing, alias management, chat messages, and file attachments.
- WebSocket client hook in UI and authenticated `/ws` endpoint in gateway for real-time job updates.
- Gateway site listing pagination (`page`, `page_size`) with bounded page size.
- Added local-domain compose overlay (`docker-compose.local-domain.yaml`) for `pagewright.io` testing.
- Added Make targets for local-domain startup/shutdown and host-routing verification.
- Added strict local-domain verification target that validates auth + site creation + host-based `200` serving.

### Known Gaps
- Compiler has no test files yet.
- UI has no automated test suite yet.
- Serving service still assumes in-container nginx reload by default; split-container nginx setups need explicit external reload strategy.

## How to Update

When creating a new release:
1. Create a git tag: `git tag -a v0.1.0 -m "Release v0.1.0"`
2. Add release notes under `## [0.1.0] - YYYY-MM-DD`
3. Move items from `[Unreleased]` to the new version section
4. Push tag: `git push origin v0.1.0`
