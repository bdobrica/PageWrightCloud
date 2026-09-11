# PageWright operator runbooks

These are local-code procedures, not actions performed on the hosted pilot.
M4.9 public acceptance and a reviewed candidate rollout remain open. One operator
owns each window and records UTC time, revision/image IDs, project, affected
job/site/version IDs, observations and outcome. Keep secrets and raw traces private.

## Select the target and inspect

For the existing pilot only, in a Bash operator session:

```sh
pilot_compose() {
  docker compose --env-file /opt/pagewright-pilot/.env.pilot \
    -p pagewright-pilot \
    -f /opt/pagewright-pilot/docker-compose.yaml \
    -f /opt/pagewright-pilot/docker-compose.pilot.yaml "$@"
}
pilot_compose config --quiet
pilot_compose ps
curl --fail --silent --show-error --max-time 10 https://api.pagewright.io/ready
```

Empty successful `config --quiet` output is expected. Never print expanded config
or whole container environments. For development use the root project's Compose;
never mix identities. Readiness does not prove writable disk, worker startup,
provider access or end-to-end publication.

Read bounded logs privately with `pilot_compose logs --since 15m --tail 100 gateway
manager storage serving`; historical logs can contain secrets or user content.
Record safe IDs/categories rather than arbitrary output in issues.

Before mutations obtain maintenance approval, prepare a verified recovery point,
block new PageWright app/API admission using the reviewed host procedure, and
prohibit competing operator writes. Preserve generated-site reads when safe. Do
not stop shared Nginx or change blog/apex vhosts. Keep gateway/provider and manager
available while existing work drains. Quarantine uncertain work; do not declare it idle.
There is no universal ingress-maintenance toggle installed by these runbooks. If
the host's PageWright-only admission block has not been reviewed, arrange that
change first; do not improvise by shutting down shared services.

## Failed or stuck job

1. Capture the owner's job ID, source/target version, error category and status
   through authenticated history. A browser timeout does not prove failed admission;
   refresh/re-authenticate and retain the original retry key.
2. Check readiness and bounded private logs. Distinguish terminal failure from
   missing/corrupt evidence, unavailable dependencies or occupied provider slots.
   Never expose internal ports to debug.
3. For confirmed terminal failure verify prior live/preview selections remain.
   A corrected request intentionally starts a new attempt and consumes quota;
   do not loop retries or increase allowance without approval.
4. For pending/running/uncertain work let fenced reconciliation act once dependencies
   recover. Never reset queues, lower fences, delete receipts, manually mark success
   or relaunch a container from a saved payload.
5. For missing/conflicting evidence pause admission, stop all dispatch/recovery
   writers, inventory and stop only identity-verified affected workers, and preserve
   the recovery set. Follow [the audit procedure](JOB_DURABILITY.md#operator-recovery-missing-or-corrupt-evidence).
   With writers already stopped, use the selected configuration:

   ```sh
   pilot_compose run --rm --no-deps --entrypoint /app/recovery-audit manager
   ```

   Exit 0 means no detected audit issue, 2 means issues, 1 means incomplete audit;
   none proves lost work never executed. Never release unknown reservations.
6. For exhausted allowance/occupied slots use the read-only queries and review in
   [pilot limits](PILOT_LIMITS.md#fail-closed-recovery-and-rollout). Provider work can
   outlive timeouts. Never refund reservations or clear unrelated active flags.

Resume only after durable outcomes/ownership are consistent, dependencies are
ready, workers accounted for and live/preview bytes verified. Otherwise quarantine
affected work and request a reviewed recovery decision.

## Disk pressure

Measure bytes and inodes on the actual volume and backup filesystems:

```sh
df -h /opt/pagewright-pilot /var/lib/docker
df -i /opt/pagewright-pilot /var/lib/docker
docker system df
docker volume ls --filter label=com.docker.compose.project=pagewright-pilot
```

If Docker has a different data root inspect that mount instead. As an initial
operator policy investigate at 80% bytes/inodes and pause new writes at 90%, or
earlier if space cannot cover the largest build plus backup/restore headroom.
These are proposed manual thresholds, not installed alerts or enforced limits.
Monitor trends and adjust to actual capacity.

Cleanup is limited: verified terminal workers are eligible after one hour,
abandoned uploads after seven days; serving-cache retention protects live/preview/
receipt pins and may exceed its soft limit. Canonical archives, immutable logs
and identity/fence records remain retained. See [worker retention](WORKER_RETENTION.md)
and [cache retention](adr/0021-architecture-decisions-atomic-activation.md).

Pause admission and drain writers before maintenance. Add capacity or move verified
encrypted backups off-host under the retention policy. Review exact unused image/
cache targets separately and preserve rollback images. Do not globally prune Docker,
delete application volumes, truncate AOF, expire deduplication, remove `.uploads.lock`,
manually delete versions or force-unpin output. Unknown staging files require offline
evidence review, not recursive cleanup. If persistence was damaged, restore into a
fresh target; freeing space is not repair.

Exit criteria: adequate bytes/inodes/headroom, healthy persistence, no unresolved
write errors, consistent receipts and verified live/preview reads.

## Content rollback or failed deployment

Inspect the owner's deployment status before another selection. `pending` means
unresolved, not rolled back; recovery retries the same persisted identity. A lost
response can follow successful activation.

Choose a known completed version in history and publish it to the intended target.
Rollback is a **new deployment sequence** of immutable content, not editing symlinks
or rewinding sequences. Rolling back live leaves preview unchanged (and vice versa).
A cached copy may need re-download; preserve its canonical archive. For uncertain
activation preserve intent/receipt and investigate instead of forcing a new selection.

Verify completed status, selected version, trusted HTTPS HTML, CSS/JS/nested pages
and the opposite target. The M4.13 fix sends `Cache-Control: no-store`; old fresh
browser caches may need hard refresh. The fix still needs approved pilot rollout.
See [deployment recovery](adr/0020-architecture-decisions-deployment-recovery.md).

## Application/configuration rollout and rollback

Record exact service/worker image IDs and protect prior config; a mutable tag or
Git checkout is not reproducible rollback. Make a [coordinated backup](BACKUP_RESTORE.md).
Drain work, stop writers, build/pin reviewed images and recreate coordinated services
without deleting volumes. Gateway runs migrations; old binaries refuse newer schema
versions. Never assume old images can use an upgraded DB or automatically run `.down.sql`.

For the outstanding M4.13 fix, root `nginx` mounts `pagewright/serving/edge.conf`.
Review deployed file/mount and candidate before recreating that project's edge.
This is separate from shared host Nginx/TLS; local tests do not authorize rollout.

For config-only failure before writes, restore matching protected config/images
after compatibility review, validate Compose quietly and recheck readiness. If schema
or persistent state changed, keep admission closed and use fresh-target coordinated
restore with matching images. Retain failed state and account for post-snapshot
writes/spending. Never combine old PostgreSQL with newer Redis/receipts or run two
authoritative stacks. TLS rollback follows [HTTP-01 operations](PILOT_HTTP01.md), not
replacement of shared host configuration.

Reopen only after readiness, login, ownership, history, live/preview, origin/TLS and
rollback checks pass. Real AI verification needs its own remaining authorization.
Record unresolved gates rather than treating recovery as sign-off. No remote rollout,
restore or fault injection was performed for M4.14.

## Backup restore and credential rotation

[Backup/restore](BACKUP_RESTORE.md) supplies exact backup/verify/fresh-target restore
commands, encrypted off-host retention, RPO/RTO and old-host fencing.
[Credential rotation](CREDENTIAL_ROTATION.md) supplies secret-specific sequencing.
Share these policies rather than duplicating destructive commands across runbooks.
