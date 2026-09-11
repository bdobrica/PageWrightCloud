# Coordinated credential rotation

Use an approved window and the explicit project wrapper in
[operations](OPERATIONS.md#select-the-target-and-inspect). Inventory replicas,
workers and external consumers first. No rotation was performed for this milestone.

## Prepare

Privately retain deployed configuration/images and a verified [backup](BACKUP_RESTORE.md).
Stop admission and drain jobs/deployments while existing credentials work. For
compromise, revoke exposed external keys promptly and stop affected traffic/workers;
preserve uncertain attempts rather than waiting indefinitely or automatically retrying.
Do not routinely rotate a signing key underneath active callbacks.

Generate independent random secrets using configuration requirements; enter them
through a protected editor/secret mechanism. Never put values in arguments, tickets,
transcripts, expanded Compose output or Git. Env-file changes need coordinated
recreation, not only `restart`. Protect recovery secrets separately from data bundles.

## Rotation matrix

| Credential | Coordination and effect |
| --- | --- |
| PostgreSQL password | Stop gateway/other DB writers. In an authorized interactive psql admin session use `\password pagewright`; privately set matching `PAGEWRIGHT_POSTGRES_PASSWORD`. Initialized volumes ignore changed initialization passwords. Recreate PostgreSQL/gateway as reviewed and verify access; never reinitialize the database. |
| Redis password | Drain manager/workers, stop manager and Redis cleanly, update `PAGEWRIGHT_REDIS_PASSWORD`, recreate Redis/manager retaining the entire AOF volume. Never reset queues, locks or fences. |
| JWT secret | Change `PAGEWRIGHT_JWT_SECRET` across all gateways and recreate together. Users re-login; no dual-key grace period exists. Password reset alone does not immediately revoke existing JWTs. |
| Service/signing token | Rotate `PAGEWRIGHT_SERVICE_TOKEN` across gateway, manager, storage and serving together with no old workers active. Old worker capabilities/callbacks become invalid. Preserve attempt state; never give workers the master token. |
| Internal provider token | Rotate `PAGEWRIGHT_PROVIDER_TOKEN` across gateway and manager launch configuration; drain/stop old workers retaining the retired token. Preserve lifetime reservations. This is separate from service signing authority. |
| Real provider key | Change `PAGEWRIGHT_LLM_KEY` only in gateway private configuration, recreate gateway and revoke the retired upstream key through its operator console. Paid verification needs a separate approved cap; a new key does not replenish allowance. |
| SMTP credentials | Coordinate upstream and `PAGEWRIGHT_SMTP_USERNAME`/`PAGEWRIGHT_SMTP_PASSWORD`, recreate gateway. Retain mandatory TLS and verified sender/reset origin. Verify an operator-controlled inbox, one reset, rejected reuse and login; never record the link. |
| TLS credentials/state | Follow dedicated HTTP-01 operations. Never replace apex/blog lineages or regenerate unrelated host configuration. |

Requirements: [configuration](CONFIGURATION_SECURITY.md), [internal auth](INTERNAL_AUTH.md),
[allowance](PILOT_LIMITS.md), [reset email](PASSWORD_RESET.md), [TLS](PILOT_HTTP01.md).

## Verify and recover

Run `pilot_compose config --quiet`, recreate the reviewed coordinated services using
pinned images, then check readiness, authenticated login/internal operations,
rejection of retired credentials, retained history/receipts and unchanged live/preview.
Startup checks do not establish external provider/SMTP credentials valid; document
unperformed checks. Never expose private ports or print secrets to probe them.

On failure keep admission closed. Restore previous matched configuration **and actual
database/Redis credential state** only if secrets remain safe; env-file rollback
alone is insufficient. Never reactivate compromised/revoked credentials. Use a new
coordinated rotation or reviewed restore. Retain uncertain reservations/job outcomes
and reopen only after consistency is established.
