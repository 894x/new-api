# Blue/green release and historical-instance retirement

Use this workflow for Nginx upstream routing and manually created application
slots. The Compose helper supports application replacement within its declared
profile; it does not provide this retention stage. Keep credentials in memory
and restricted runtime backups, and preserve the original acceptance receipts.

## Identify the deployment lineage

- Resolve the current traffic slot from the loaded proxy configuration and its
  actual upstream port. Use its accepted release receipt to identify the previous
  version. Container creation dates and version-name prefixes are insufficient.
- Scope the lineage to the requested hostname and its deployment history. Shared
  Compose projects, mounts, authentication secrets, or a physical database are
  relationship evidence, not authorization to move or stop another hostname.
  Preserve a historical-looking container if it actively serves an out-of-scope
  hostname; include that exception in the handoff.
- Enumerate every hostname and path using each candidate for retirement,
  including variable upstream maps, old static assets, and HTTP-only aliases.
- Resolve SQL/Redis hosts inside each application's network. Match the resolved
  address to the actual database/cache container ID and verify the database name.
  Two isolated Compose projects can both use `postgres` and the same DSN text.
- Match shared data mounts and authentication secrets in memory without printing
  them. Do not merge unrelated deployments merely because image tags or service
  names look alike. Move aliases only when explicitly authorized. If a retirement
  would interrupt another hostname, keep its container and clarify the conflict.
- Record exact current/previous image IDs, revisions, container IDs, task roles,
  restart policies, mounts, networks, proxy configuration, and connection counts.

## Build, start, and accept the new slot

Follow the main skill's pinned-source and validated-backup gates. Start the new
application on an unused loopback port with a unique node name. Preserve runtime
configuration and health checks. Confirm image and binary identity, startup,
semantic health/authentication responses, and the relevant frontend resources.
Negotiate Docker's supported API version if using the Engine API directly.

Change only the authorized routes, write configurations atomically, run
`nginx -t`, and reload gracefully. Keep old containers and old Nginx workers
running until their requests finish. Observe all affected hostnames throughout
the handoff, and restore the saved routes if acceptance fails.

## Preserve pages already opened on old versions

Hashed frontend chunks can still be requested after the HTML entrypoint switches.
Do not stop their serving container before preserving those resources. A missing
JS path may return the SPA's HTML with HTTP 200; check content type and hashes.

Use the current build's resource manifest and the previous slot for ordinary
two-version routing. When retiring still older traffic slots, archive their
public static files or verified build artifacts before stopping them. Serve the
archived files from a restricted-to-public-assets directory through Nginx;
exclude runtime data, environment files, logs, and credentials. Validate both
new resources and old async chunks after routing changes. Keep static archives
and images available; stopping a version does not authorize deleting artifacts.

## Convert the previous slot to standby

Within the target lineage, only the accepted current version should run periodic
billing/task workers. Other authorized environments may share the database;
report their roles without modifying them or claiming a database-wide upgrade.
Configure the previous version with `NODE_TYPE=slave`, `UPDATE_TASK=false`, and a
unique node name. These settings require a new process.

Create a clone of the exact previous image and runtime configuration on a free
loopback port with only the task-role/node/port changes. Validate health and
background-role guards before moving the previous slot's static/rollback routes
to it. Drain the original previous container, then stop it and disable restart.
The steady state must contain only two running application slots: current master
and previous standby. Temporary clones are allowed during the handoff and must
be retired before reporting completion.

Refresh rollback commands after changing ports, aliases, or upstream includes.
Do not let a rollback script remove an upstream now used by another hostname.
Distinguish a traffic-only rollback from a complete runtime/task-role rollback.
For a full rollback, check the previous version can process outstanding tasks,
promote it safely, and demote/drain the newer master. Preserve ongoing requests;
do not force an incompatible old task worker to claim new-version tasks.

## Stop and persist retirement

- Confirm no loaded proxy route still needs the target container. Wait for its
  established client connections to drain and inspect owned background tasks.
  Wait for active work/leases to finish or reconcile them through supported
  application behavior; do not delete task rows or reset accounting state.
- Set each exact historical container's restart policy to `no`, then stop it
  gracefully. If the grace interval would expire with active work, postpone the
  stop instead of allowing a forced kill. Inspect application shutdown behavior
  and stop timeouts for the pinned version.
- Check for explicit startup automation or Compose definitions that would revive
  retired slots. Update only the authorized application startup configuration;
  preserve shared database/cache services. Record any remaining revival risk.
- Keep stopped containers, image IDs, configurations, and backups recoverable.
  Never run project-wide `down`, container/image pruning, or volume deletion.

## Final acceptance

Enumerate running applications connected to the resolved target database peer
and classify them by the requested hostname's deployment lineage. There must be
exactly the current and previous release slots for that lineage, with one task
master. Preserve out-of-scope hostname applications even if they share the peer.
Check stopped historical statuses and restart policies,
slot health/revision/binary identity, every affected hostname and old/new static
resource, database/cache identities, and independent environments. Record the
retired IDs, drain/task evidence, backup and refreshed rollback paths, HTTP
monitoring results, and any incomplete step. Claims about a single task master
apply only to the authorized lineage. Report other hostname masters separately;
do not describe their shared database as having only one master.
