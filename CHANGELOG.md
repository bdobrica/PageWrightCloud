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

## How to Update

When creating a new release:
1. Create a git tag: `git tag -a v0.1.0 -m "Release v0.1.0"`
2. Add release notes under `## [0.1.0] - YYYY-MM-DD`
3. Move items from `[Unreleased]` to the new version section
4. Push tag: `git push origin v0.1.0`
