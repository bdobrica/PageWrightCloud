# Architecture decision records

These numbered implementation records preserve decisions and contract details from
M1–M4. Numbers reflect this documentation migration, not original decision dates.
Existing acceptance evidence and compatibility constraints are retained; historical
future-tense statements do not override the [current release record](../RELEASE_ACCEPTANCE.md).
Use the [operations index](../README.md) for procedures. Add new numbered decisions
here with context, decision, consequences, status and links to implementation/tests.
Supersede a decision explicitly instead of silently rewriting its history.

- [MVP topology and implementation principles](0000-architecture-decisions-mvp-topology.md)
- [Job wire contract — M1.1 / M1.2 / M2.7](0001-architecture-decisions-job-contract.md)
- [Durable build submissions — M1.2](0002-architecture-decisions-build-submissions.md)
- [Artifact transport (M1.3)](0003-architecture-decisions-artifact-transport.md)
- [Version metadata and completion (M1.4)](0004-architecture-decisions-version-metadata.md)
- [Immutable versions and disabled deletion (M1.5)](0005-architecture-decisions-immutable-versions.md)
- [Initial site source and retry (M1.6)](0006-architecture-decisions-site-bootstrap.md)
- [Version archive layout (M1.7)](0007-architecture-decisions-archive-layout.md)
- [Compiler fixtures and boundaries (M1.8)](0008-architecture-decisions-compiler-contract.md)
- [Version and serving contracts (M1.9)](0009-architecture-decisions-version-api.md)
- [Docker worker launch contract (M2.1)](0010-architecture-decisions-docker-spawner.md)
- [Durable bounded dispatch (M2.2)](0011-architecture-decisions-queue-dispatch.md)
- [Worker CLI and sandbox contract (M2.3–M2.7)](0012-architecture-decisions-worker-cli.md)
- [Trusted worker builds (M2.4)](0013-architecture-decisions-worker-build.md)
- [Default build source (M2.5)](0014-architecture-decisions-build-source.md)
- [Site leases and fenced commits (M2.7)](0015-architecture-decisions-fenced-commits.md)
- [Bounded result delivery and recovery (M2.8)](0016-architecture-decisions-result-recovery.md)
- [Owner-scoped build history (M3.1)](0017-architecture-decisions-job-history-api.md)
- [Preview activation and hosting URLs (M3.4–M3.6, M4.9)](0018-architecture-decisions-preview-activation.md)
- [Supervised hosting lifecycle (M3.5)](0019-architecture-decisions-hosting-lifecycle.md)
- [Deployment consistency and recovery (M3.7)](0020-architecture-decisions-deployment-recovery.md)
- [Atomic activation and serving-cache retention (M3.8)](0021-architecture-decisions-atomic-activation.md)
- [Text-only MVP capabilities](0022-architecture-decisions-mvp-capabilities.md)
- [Draft recovery and publication feedback (M3.10)](0023-architecture-decisions-draft-recovery.md)
- [Identifier and filesystem boundaries (M4.4)](0024-architecture-decisions-identifier-security.md)
- [Request, archive and compiler limits (M4.5)](0025-architecture-decisions-resource-limits.md)
- [Cross-user access acceptance (M4.7)](0026-architecture-decisions-ownership-security.md)
- [Pilot DNS and HTTPS — M4.9 in progress](0027-architecture-decisions-pilot-https.md)
