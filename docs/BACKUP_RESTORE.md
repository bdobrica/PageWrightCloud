# Coordinated offline backup and restore

M4.11 provides a **maintenance-window backup**, not live snapshots, HA, point-in-time
recovery or automated remote deployment. Run `scripts/pilot_backup.py` from a
reviewed checkout as a trusted operator with local Docker access. Python 3.9+ and
Docker Compose 2.24.4+ are required. The tool never reads Compose secret files.

## Keep these components together

| Member | Preserved state |
| --- | --- |
| `postgres.dump` | Accounts/password hashes, ownership, bootstrap bytes, version/job history, deployment sequences/pointers, admission/spending reservations, migrations |
| `redis_data.tar.gz` | Entire cold Redis directory: multi-part AOF, job/commit reservations and fence counters |
| `nfs_data.tar.gz` | Immutable archives, commit manifests and private execution logs |
| `www_data.tar.gz` | Extracted versions, independent live/preview symlinks and private deployment receipts |
| `nginx_config.tar.gz` | Generated routing/configuration |
| `pagewright_config.tar.gz` | Serving maintenance configuration/page |
| `manifest.json` | Source project/volumes, operator-supplied revision, four exact image IDs, schema version and per-file size/SHA-256 |

PostgreSQL uses a logical custom-format dump; restore runs in one transaction
without source ownership/ACL commands. Global roles and cluster configuration are
not included. Consult PostgreSQL 15's [pg_dump](https://www.postgresql.org/docs/15/app-pgdump.html)
and [pg_restore](https://www.postgresql.org/docs/15/app-pgrestore.html) contracts.
Redis is authoritative duplicate-execution evidence, not a disposable cache.
After checking idle work, the tool gracefully stops Redis and copies **all** its
persistence files. Never copy only one incremental AOF or reconstruct reservations
from artifacts. See [Redis persistence](https://redis.io/docs/latest/operate/oss_and_stack/management/persistence/).

Only named, project-owned local volumes at the root Compose paths are supported.
External/NFS drivers, bind mounts, replicas and custom paths need a separate
reviewed procedure. Unsupported links/special files, pending serving receipts and
Nginx recovery journals are rejected rather than silently omitted.

## Preparation and security

- Appoint one operator; prohibit competing deployments, CLI writes, container
  restarts and backups throughout the window. Pre/post checks detect many races,
  but Docker labels are not a fence against another privileged operator.
- Retain the deployed checkout and exact service/worker images in private recovery
  storage. Export images if necessary: mutable tags and rebuilds do not guarantee
  identical IDs. `--revision` is an operator attestation, not proof of image origin.
- Choose a trusted private backup parent **outside the repository and serving
  roots**, with room for both backup and a full independent restore. Each member
  is limited to 100 GiB and each archive to one million entries.
- Encrypt and retain an access-controlled off-host copy with a separately stored
  recovery key. Bundles contain password hashes, private prompts and logs. 0700/0600
  permissions and checksums are **not encryption or authentication**. The tool
  performs no uploads, encryption-key management or old-backup deletion.
- Separately protect `.env.pilot`, image inventory, reviewed Compose/worker runtime
  configuration and PageWright-specific host TLS/controller state. These host files
  are **not in this bundle**. See [HTTP-01 operations](PILOT_HTTP01.md) for the
  dedicated controller/units, `/etc/pagewright-tls.env` and `/var/lib/pagewright-tls`.
  Do not copy or replace the shared host's unrelated blog/certificate lineages.

## Backup during an approved maintenance window

These are operator instructions, not commands M4.11 ran on the remote pilot.

```sh
cd /opt/pagewright-pilot
pilot_compose() {
  docker compose --env-file /opt/pagewright-pilot/.env.pilot \
    -p pagewright-pilot -f docker-compose.yaml -f docker-compose.pilot.yaml "$@"
}
```

Block public admission to the PageWright app/API vhosts using the reviewed host
maintenance procedure; do not stop shared Nginx or the blog. Drain existing work
while manager and gateway's internal provider remain available. Resolve uncertain
submissions, deployments and provider slots; never clear reservations to bypass a
backup check. Suspend dedicated controller/operator tasks accessing PostgreSQL.

After the stack is idle:

```sh
pilot_compose stop --timeout 65 ui gateway manager nginx serving storage themes
python3 -B scripts/pilot_backup.py backup --project pagewright-pilot --quiesced \
  --revision FULL_REVIEWED_DEPLOYED_GIT_SHA \
  --directory /YOUR_PRIVATE_BACKUP_PARENT/NEW_BACKUP_NAME
python3 -B scripts/pilot_backup.py verify \
  --directory /YOUR_PRIVATE_BACKUP_PARENT/NEW_BACKUP_NAME
```

Initially only PostgreSQL and Redis may run. Backup checks DB client connections,
active/uncertain work and live project workers, then stops Redis itself.
PostgreSQL stays running for its dump; every other service remains stopped.
Backup mounts are read-only. Short-lived archive helpers have no network/socket,
a read-only root, no-new-privileges and resource limits; they are not privileged
containers. Restore adds only file ownership/permission capabilities. Worker
isolation is unchanged.

The directory must be new. Failed output is retained without a valid completion
manifest; inspect privately and retry into a new directory. Nothing is resumed
automatically, even after failure. After verification and safe off-host retention,
a backup-only window may resume the **original** Redis/storage, serving, manager,
gateway and edge/UI/themes through Compose health checks. Check content before
re-enabling controller/tasks and public admission. Do not restore into that project.

## Fresh-target restore and validation

1. Decrypt a trusted bundle into a real private directory. Valid hashes do not
   make an untrusted dump safe: SQL and Nginx configuration are executable input.
   `--trust-backup` explicitly acknowledges that boundary; do not edit checksums.
2. Prepare a **new** explicit Compose project with fresh volumes, matching database
   name/user, recorded service image IDs and a reviewed recovery env file. Use the
   root Compose plus `docker-compose.restore.yaml`, not the fixed pilot override.
   The restore overlay removes public ports, blocks gateway/manager startup,
   removes manager's Docker socket, and uses `volume.nocopy` so Docker does not
   pre-populate target volumes. Supply a separate reviewed image-only override
   mapping PostgreSQL/Redis/storage/serving to the manifest's exact local image IDs.
3. With that same Compose configuration throughout, `create postgres redis storage
   serving nginx`, then `up -d --wait postgres`. Do not start Redis or applications
   yet; gateway startup would create schema and run recovery.
4. Run the matching revision's CLI:

```sh
python3 -B scripts/pilot_backup.py restore --project YOUR_NEW_RESTORE_PROJECT \
  --trust-backup --revision RECORDED_FULL_GIT_SHA \
  --directory /YOUR_PRIVATE_BACKUP_PARENT/VERIFIED_BACKUP_NAME
```

All files are checked before contacting the target. Restore rejects the source
project, shared source volumes, mismatched images, existing DB objects and any
nonempty target volume. There is no force/clean/delete/overwrite switch. A failed
restore may leave partial files: retain the stopped target for inspection and use
another fresh project. The multi-volume operation is not an atomic transaction.

5. Start only Redis, storage, serving and private Nginx. Check readiness and Nginx
   configuration. Verify Redis reservation/fence values, DB owners/history and
   live/preview pointers, matching serving receipts, and both hosts' actual HTML,
   CSS, JS and nested pages. Keep manager/provider access disabled during the drill.
6. Before a real cutover, fence the old host and its workers. Never run two
   authoritative copies. Review work and spending **since the snapshot**: restoring
   old allowance is not a refund. Account for later provider charges/reservations
   before choosing a safe remaining cap and enabling provider access.
7. Separately review host TLS, origins, ports and worker runtime; remove rehearsal
   guards only with cutover approval. Start recovery/admission, switch routing and
   validate trusted HTTPS. If validation fails, keep ingress closed; never mix
   old PostgreSQL with new Redis or arbitrary volume snapshots. Returning to the
   old host is safe only if no post-cutover writes require reconciliation.

## Repeatable drill and ongoing policy

```sh
make test-backup-unit
make test-backup-restore
```

The drill uses two generated projects with no app `.env`, public ports, Docker
socket or paid provider. Explicit synthetic artifacts (not an AI build) are
published as v1 live/v2 preview through real serving/deployment APIs. After backup,
the original test volumes are removed before restoration. Checks cover restored
ownership/password hash, database/storage version history and cross-owner denial,
artifact checksums, private serving receipts, spending reservations,
Redis job/commit/fence records, exact HTML/CSS/JS/nested-page bytes,
and an increasing subsequent deployment sequence. A repeated restore into the
populated target must fail. Only generated projects/volumes are cleaned up;
private fixture evidence is retained at the printed temporary path.

Take a verified pilot backup before upgrades and at an agreed daily maintenance
window. Retain encrypted off-host recovery points and periodically rehearse a real
backup restore under operator approval. Choose retention/access policy for private
user content; do not delete the last independently verified recovery point.
M4.11 installs no remote schedule and does not claim a production restore, public
TLS, AI or browser acceptance. Record snapshot time, off-host verification, restore
duration and results. RPO is the age of the last retained completed snapshot; RTO
includes restore, validation and cutover, not just container startup.
