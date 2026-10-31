# Resource monitoring and bounded deployment work

Apply this gate to the manual blue/green workflow and the Compose helper.
Keeping the API running does not make a build, online dump, or read-only audit
resource-neutral. Capturing full container logs can exhaust memory even while
health probes still return HTTP 200. A small dump-client RSS also does not prove
that database or application memory stayed stable; distinguish observed OOM
victims and concurrent work from an established cause.

## Establish the baseline and budget

Before adding work, collect a short series of bounded, read-only samples on
every affected host. Include application nodes, the real database/cache host,
and any backup/build destination. Record timestamps and phase names; retain
UTC timestamps and use the user's timezone in the handoff.

- Host RAM and `MemAvailable`, swap usage and swap-in/out activity; use available
  memory rather than the `free` column alone. Record recent kernel OOM events.
- Process RSS and container/cgroup memory usage, effective limits, and OOM/failure
  counters. Account for Docker/cgroup version differences and disclose unavailable
  metrics. Identify large consumers without dumping full process environments.
- Memory/CPU/I/O pressure where available, CPU count and utilization, load trend,
  disk space and inodes on source, temporary, image, and backup filesystems.
- Exact serving/protected container IDs, restart counts and health; current
  request/task ownership and semantic health/API latency on relevant routes.

Use `/proc/meminfo`, `/proc/pressure/*`, bounded `vmstat` samples, `df`, selected
Docker inspect/stats fields, and narrow kernel-log windows as appropriate.
Avoid full journal/container-log captures as part of baseline collection.
Do not add monitoring agents, swap, or change live service limits implicitly.

Calculate the highest expected overlap: current and previous slots, green and
role-transition clones, database/cache, builders, dump/compression/transfer/
validation jobs, log readers, and OS headroom. Include database-side query and
I/O costs when a dump runs on another host. Use conservative measured estimates;
unknown job peaks require more headroom, isolation, or a different execution host.

Before each phase, record:

- Per-host memory reserve, warning/stop thresholds, pressure and latency limits,
  sampling cadence, and disk reserve needed for the job and rollback artifacts.
- Expected additional usage and concurrency, job limits, cancellation targets,
  and the controller responsible for supervising and stopping those jobs.
- Evidence that current readings and their trend permit the next phase.

Choose limits from actual host capacity and workload. A fixed free-memory floor,
load value, or container cap from a previous release is not a universal default.
Load needs CPU-count and I/O context. `GOMEMLIMIT` is a Go runtime memory target,
not a hard process/container RSS cap. For new slots, set appropriate limits and
read back their effective cgroup configuration; do not retune existing live
applications or data services without authorization.

If capacity is insufficient, offload to an authorized idle peer/local builder,
reduce job concurrency, or wait. Do not stop healthy applications, database,
Redis, proxy, workers, or scheduler to make room. Do not start a temporary clone
unless its overlap fits the budget without forcing existing requests to end.

## Supervise the entire operation

Start a low-overhead monitor before auxiliary work and verify its first sample.
For example, sample every 3–5 seconds, adjusted to the workload and cost. Collect
compact numeric metrics and semantic health status/latency; never capture full
HTTP bodies or secrets. Store restricted, size-bounded or rotated records outside
the checkout, aggregate incrementally, and bound the monitor's own resource use.

Supervise the monitor independently of the expensive job. At every gate verify
fresh heartbeats on all affected hosts and check alert state before proceeding.
Cover builds, backups, verification/decompression, transfers, image loading,
asset checks, every temporary slot, route/role changes, draining, retirement,
startup/shutdown log reads, and the final audit. Route acceptance is not the end
of monitoring.

If monitoring exits, stalls, or SSH disconnects, stop advancing the rollout.
Keep independent remote supervision active when possible. Rediscover jobs,
routes, slots, and resource state through a fresh connection or authorized peer
before retrying. Do not launch a duplicate dump/build after losing its SSH client.

Treat new OOM events, unexpected protected-service restarts, exhausted reserves,
sustained pressure or rapid memory decline, disk exhaustion, and health/latency
failures as stop conditions under the recorded policy. A warning should trigger
early reduction or cancellation before the emergency threshold is reached.

On an alert:

1. Block new phases and cancel the responsible owned auxiliary work. A marker
   file or monitor exit alone is insufficient. Record exact job/process-group
   ownership before launch; include dump/compression children, database client
   sessions, and server-side Docker build/exec work. Killing only an SSH shell or
   Docker CLI may leave the resource consumer running. Verify cancellation.
2. Keep the monitor running and preserve serving applications, data services,
   proxy, and in-flight requests. Do not use broad process-name kills, project-wide
   Compose commands, or forced connection termination. Cancel only identified
   auxiliary jobs; do not interrupt a route mutation without reconciling it.
3. If availability was lost after a route change, restore the recorded healthy
   route gracefully and verify APIs/frontend. If cancelling an auxiliary job
   restores headroom and the serving release is healthy, preserve that route
   while assessing the incident instead of adding rollback clones immediately.
4. Preserve the alert and readings. Inspect service health, roles, OOM/restart
   deltas, and all affected hosts. Require a stable observation interval and a
   revised capacity budget before explicitly resuming; never erase a failure
   marker or overwrite the baseline to turn the run into a pass.

## Bound every auxiliary producer and reader

Use online consistent native dumps and streaming file pipelines. Bound memory,
concurrency, compression, throughput, temporary disk usage, and wall-clock time
where appropriate. A transfer throttle alone does not bound source database
memory or query cost. Lowering priority on a Docker CLI does not establish
limits for the tool inside its container or the database backend.

Offload compression/validation to an authorized idle host when useful, while
monitoring source and destination. Use a validator compatible with the dump/RDB
format. If only the checker version is incompatible, keep the completed backup
and validate it with compatible tooling instead of repeating the full dump.
Never stop/pause/freeze a live service for a backup or integrity check.

Apply streaming chunked hashing, copying, and HTTP asset checks to large files.
Do not load whole database dumps, application binaries, archives, or
logs into memory merely to hash, transfer, search, or summarize them. Reuse
existing verified artifacts rather than repeatedly scanning or regenerating them.

For startup, shutdown, and incident logs:

- Select the exact container and a narrow `--since`/`--until` interval with a
  `--tail` limit. Inspect retired/history containers only for a specific need.
- Enforce an additional byte cap and wall-clock timeout in the streaming reader.
  A time window or row count cannot bound large individual records. An example
  starting budget is a 60-second startup/shutdown window, 2,000 records, and
  2 MiB per read; adjust explicitly when evidence requires more, within capacity.
- Do not use unbounded `check_output`, `capture_output`, `communicate`, whole-file
  reads, or an in-memory buffer followed by slicing. Read fixed-size chunks,
  bound stderr too, and terminate/reap the owned reader when its limit expires.
  Cancelling `docker logs` must stop the reader CLI, not the container or daemon.
- Record exit status, selected time interval, bytes/records inspected, timeout,
  truncation, and unavailable logs. Empty output is not proof of a clean startup;
  qualify the verification or obtain the missing bounded window safely. Do not
  claim complete log acceptance from a partial scan, and do not print raw secrets
  or customer request payloads in the report.

Apply these limits to helper commands as well as ad hoc scripts. Inspect their
actual output/cancellation behavior before use; a streaming database backup does
not guarantee bounded log inspection or a functioning resource guard.

## Close the monitoring window

After the last expensive check and historical-slot audit, observe stable
headroom and semantic health, verify current/previous identities and task roles,
and compare OOM/restart counters with the original baseline. Stop only the
monitor owned by this deployment after these checks complete.

The receipt must include per-host monitoring start/end and gaps, thresholds,
baseline/minimum available memory and observed usage peaks, health/latency
results, OOM/restart deltas, alerts, cancelled jobs, recovery evidence, and any
missing/truncated verification. Preserve alert history even when the final state
is healthy. Do not describe an interrupted or incomplete monitoring window as
continuous coverage or infer a cause solely from simultaneous measurements.
