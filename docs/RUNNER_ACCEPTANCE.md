# Runner failure acceptance (M2.11)

Selected image: `pagewright-worker:m2.12`. Run `make test-integration` for the
race-enabled disposable service suite. It builds a test-only CLI fixture and the
real compiler; no provider calls or production data are involved.

`worker/cmd/runner/failures_integration_test.go` invokes the production `main`
inside subprocesses of the integration test binary. It verifies exit success or
failure, fenced callbacks, exact upload ordering, retry counts, redaction and
absence of premature completion. Each subprocess has a bounded test wait and is
reaped, including on assertion failure.

| Boundary | Required outcome |
| --- | --- |
| Valid source edit | Real compilation, artifact/log/manifest upload, completed callback, exit 0 |
| Instructions tampering | Validation fails; no upload; failed callback |
| Unknown compiler component | Real compiler fails; no upload; failed callback |
| Missing compiled asset reference | Output validation fails; no upload; failed callback |
| Artifact or private-log upload failure | No manifest/completion; failed callback |
| Manifest acknowledgement unavailable | Exit failure with no contradictory failure callback; manager reconciliation required |
| Completion callback unavailable | Four identical completion attempts, no failed callback, nonzero exit |
| Callback acknowledgement lost after acceptance | Authoritative lookup resolves completion after one POST; exit 0 |
| SIGTERM during executor sleep | Process group canceled, no upload, failed callback, bounded exit |
| Deadline during blocked source fetch | Wrapped deadline error; executor never starts |
| SIGKILL before/after manifest boundary | No invented terminal outcome or follow-up failure; durable evidence must decide recovery |

The deadline test invokes the same runner pipeline with a short caller-owned
context. Production still uses its signal context and 15-minute job deadline,
10-minute executor limit and 16-minute hard watchdog. There is no environment
switch to disable isolation or extend those limits. The integration build tag's
existing source-only executor path is not compiled into the production image.
Installed CLI sandbox and compiler checks remain separate:
`make test-worker-cli`, `make test-worker-compiler`, `make test-docker-spawner`.

## Restart evidence and limits

Restart is recovery, not replay of the killed worker. Manager Redis integration
tests reconnect with a new backend and reconciler, using only canonical jobs and
digest receipts. Complete materialized output recovers completion; missing bytes
fail conservatively. Both paths preserve receipts and release terminal capacity
and site reservations; repeated recovery cannot reopen the attempt. Dispatch
restart tests verify that existing started/uncertain launch intents never requeue.
`make smoke-stack` separately SIGKILLs/recreates real Redis/gateway/manager services
and verifies durable job/history recovery and preserved uncertain submissions.

These are layered tests: subprocess fault injection uses controlled HTTP peers;
receipt/restart tests use real Redis with controlled worker inspection/storage
responses; stack restart tests use disposable production services. They are not
one end-to-end paid-AI crash test, and do not prove disk-loss or multi-host HA
recovery. Worker containers retain restart policy `no`; do not manually rerun a
fenced attempt. See [result recovery](adr/0016-architecture-decisions-result-recovery.md),
[durability](JOB_DURABILITY.md) and [retention](WORKER_RETENTION.md).

Executor parsing tests were restored in M2.3 and cancellation/process-tree tests
in M2.6. They are enabled and remain regression gates. The two unrelated serving
skips remain tracked in M3.8/M4.12. [M2.12](PROVIDER_SMOKE.md) records the separately
authorized, cost-bounded real-provider acceptance; M2.11 does not deploy to a host or
claim the complete browser publish journey.
