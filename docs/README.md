# Operations and verification

Start with [operator runbooks](OPERATIONS.md). Commands target only the explicitly
selected PageWright project; nothing here authorizes a production change.
The [release record](RELEASE_ACCEPTANCE.md) distinguishes tested local behavior
from remaining public-pilot gates. See [architecture decisions](adr/README.md)
for contracts, rationale and historical milestone detail.

## Operate

| Task | Procedure |
| --- | --- |
| Failed/stuck jobs, disk pressure, safe rollback | [Incident and change runbooks](OPERATIONS.md) |
| Rotate credentials | [Coordinated rotation](CREDENTIAL_ROTATION.md) |
| Backup, restore and cutover | [Backup/restore](BACKUP_RESTORE.md) |
| Configure secrets and database credentials | [Configuration security](CONFIGURATION_SECURITY.md) |
| Private ports, service keys and worker upgrades | [Internal authentication](INTERNAL_AUTH.md) |
| Provision accounts and approve AI allowance | [Pilot access/limits](PILOT_LIMITS.md) |
| Configure reset email and sessions | [Password reset](PASSWORD_RESET.md) |
| DNS, certificates and host Nginx | [Pilot HTTP-01](PILOT_HTTP01.md) |
| Browser origins and headers | [Origin security](ORIGIN_SECURITY.md) |
| Readiness and request timeouts | [Runtime limits](RUNTIME_LIMITS.md) |
| Audit durable job evidence | [Job recovery](JOB_DURABILITY.md) |
| Automatic cleanup and limits | [Worker/staging retention](WORKER_RETENTION.md) |
| Prepare a worker host | [Worker isolation](WORKER_ISOLATION.md) |

## Develop and verify

- [Development](DEVELOPMENT.md), [test coverage/CI](TESTING.md), [release acceptance](RELEASE_ACCEPTANCE.md).
- [Browser journey](BROWSER_ACCEPTANCE.md), [accessibility](UI_ACCESSIBILITY.md),
  [offline operator artifact](OFFLINE_ACCEPTANCE.md), [bounded provider smoke](PROVIDER_SMOKE.md).
- [Deterministic compilation](DETERMINISTIC_ROUNDTRIP.md), [runner failures](RUNNER_ACCEPTANCE.md),
  [historical host acceptance](M2_6_HOST_ACCEPTANCE.md).

## Documentation policy

Keep procedures, configuration references and dated verification evidence directly
in `docs/`. Put architecture/contract decisions in numbered
`docs/adr/xxxx-architecture-decisions-topic.md` records. Operational safety limits
may stay with a procedure; link to the decision instead of duplicating protocols.
Label historical observations and superseded proposals. Update relative links when
moving files and run `node scripts/check-doc-links.mjs`. TODO/PLAN retain milestone
evidence; historical future-tense notes do not override the current release record.
