# PageWrightCloud: AI-Driven Static Website Builder

## Overview

This project is a proof-of-concept platform for building and maintaining **simple, fast, static websites** using an AI coding agent. Users interact with the system through a chat-like UI to request changes (content or layout), preview them instantly, promote them to production, or roll back to previous versions.

The system is designed around **immutability, atomic deploys, and strong isolation** to keep AI-generated changes safe, auditable, and reversible.

## Project Status

✅ **Phase 1 - Storage Service**: Complete
- REST API for artifact storage and versioning
- NFS backend with atomic writes
- Comprehensive test suite
- Docker-based local development

🔨 **In Progress**: Phases 2-6 (see below)

---

## Core Concepts

* **Static hosting**: websites are served as static files (no database, no runtime code).
* **AI-assisted editing**: an AI coding agent performs scoped edits based on user instructions.
* **Immutable versions**: every change produces a new versioned artifact.
* **Preview & promote**: users can compare preview vs active versions instantly.
* **Safe by design**: workers never write directly to served directories.

---

## High-Level Architecture

### 1. UI / API Server

* Provides a chat-style interface for user requests.
* Handles authentication (OAuth planned).
* Displays version history and controls:

  * preview
  * promote
  * rollback
* Enqueues edit jobs to Redis.

### 2. Queue & Locking (Redis)

* Job queue for edit requests.
* Per-site locking ensures only one worker edits a site at a time.
* Locks include TTLs and fencing tokens to prevent stale writes.

### 3. Worker System

* Each job runs in an isolated container.
* Steps:

  1. Fetch the latest site version archive.
  2. Unpack into a temporary workspace.
  3. Run **OpenAI Codex (non-interactive mode)** with strict editing rules.
  4. Build static output if required.
  5. Run headless browser checks (Chrome / Playwright).
  6. Package a new immutable `tar.gz` artifact.
  7. Produce a manifest describing changes and checks.

Workers **never** modify live files.

### 4. Storage (NFS for PoC)

* Used only as a simple artifact store.
* Stores:

  * versioned `tar.gz` site artifacts
  * per-version JSON log entries
* No in-place mutation of version history.

### 5. Deployer

* Validates worker artifacts (security and correctness checks).
* Unpacks artifacts into release directories.
* Atomically switches symlinks for:

  * `/preview`
  * `/current`
* Handles promotion and rollback.

### 6. Serving Web Server (nginx)

* Serves static files only.
* Uses symlink-based releases:

  * `releases/<build_id>/`
  * `current -> releases/<build_id>`
  * `preview -> releases/<build_id>`

---

## Services

### Storage Service
**Location**: `pagewright/storage/`  
**Status**: ✅ Complete (Phase 1)

A RESTful service for managing site artifacts and version history with pluggable storage backends.

**Features:**
- Store and retrieve site artifacts (tar.gz files)
- Version tracking with complete history
- Atomic write operations (temp + fsync + rename)
- NFS backend (S3 and others planned)

**Quick Start:**
```bash
cd pagewright/storage
make docker-up
curl http://localhost:8080/health
```

See [`pagewright/storage/README.md`](pagewright/storage/README.md) for complete documentation.

---

## Versioning Model

* Every change creates a **new immutable version**.
* Versions are stored as `tar.gz` archives.
* Version history is append-only (no edits or deletes).
* Rollback is an O(1) symlink switch.

---

## Safety & Guardrails

* Strict separation between:

  * **editing**
  * **validation**
  * **serving**
* File allowlists / denylists during deploy.
* No server-side code execution allowed.
* Per-site concurrency locks.
* Deterministic, auditable builds.

---

## PoC Goals

* Create a site from a template.
* Request edits via chat.
* Preview changes instantly.
* Promote or roll back with one click.
* Run entirely without Kubernetes (SSH / Ansible friendly).

---

## Non-Goals (for PoC)

* Multi-region deployment.
* Full CMS features.
* User-supplied backend code.
* Complex design tooling.

---

## Future Directions

* Replace NFS with object storage (e.g. RustFS / S3).
* Add stronger diff visualization.
* Multi-worker scaling.
* Custom domains & automated TLS.
* More advanced AI validation loops.

---

## Philosophy

> **Make AI changes boring.**
> Immutable artifacts, atomic deploys, easy rollback, and zero trust in generated code.
