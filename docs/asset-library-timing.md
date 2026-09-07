# Asset library timing diagnostics

Asset upload (8), deletion (9), update (10), group creation (11), and
synchronization (12) are independent usage log types with list badges and filters.
Group deletion/update use the corresponding deletion/update type; channel
configuration changes remain management audits. Existing records are not rewritten.

Administrators can open an asset operation in **Usage Logs → Details** to see
the same segmented request timeline used by chat requests. The longest measured
stage is highlighted. Expand **Stage Details** for individual calls, channel and
asset IDs, protocol, HTTP/business error codes, safe upstream request IDs, bytes,
and lock hold duration. Lock hold time overlaps work and is not stacked again.

Measured stages are source download (including reading the body into a temporary
file), media inspection, logical/replica persistence, channel lock queueing, and
upstream asset requests. Remaining request time is labeled other gateway
processing. This is not a browser-upload measurement: the asset API imports a
source URL. Download/inspection starts before the logical record is saved; a
failed attempt still has an ID and a diagnostic timeline.

Each replica also persists its own upload-attempt start, upstream acceptance,
first observed Active, last status check, last Processing observation, and poll
count. These milestones survive process restarts. A separate activation timeline
shows upload-to-acceptance and acceptance-to-observation. The latter includes
polling intervals, lock queues and status queries; it must not be interpreted as
the provider's pure processing duration. Old replicas have no inferred submission
time. A later query cannot replace the first Active timestamp.

The create audit is enriched after the request finishes, including when logical
creation succeeded but replication failed. Failed validation creates a diagnostic
operation instead. Automatic imports and manual synchronization also produce
diagnostic request records; admin synchronization retains its existing operator
audit as a separate record. Read-only queries (listing or viewing assets and groups) never add usage log
rows, including slow queries, failures, or observed status changes. They still
update replica milestones and emit server diagnostics. HTTP 200 preview fallback
does not hide a failed refresh from server diagnostics.

The server log emits `asset_library.stage_started`,
`asset_library.stage_completed`, `asset_library.channel_released`, and
`asset_library.request_completed`, correlated by request ID. For an in-flight
request, find its most recent started stage without a matching completed stage.
Each retained stage has a request-local ID. The log database retains at most 256
stage details and 256 replica snapshots per request; truncation is indicated.
The system log retains the individual stage events. Detailed system events are
not gated by debug mode, so normal server log retention should be configured.

Diagnostics live under `other.admin_info.asset_timing`, version 1, and are stripped
from ordinary users' log responses. They contain no source URLs, credentials,
provider bodies, or arbitrary error text. Codes and upstream IDs are bounded and
allow-listed. Milestone fields are excluded from ordinary asset/replica JSON.

This change adds nullable BIGINT columns through the existing asset replica
AutoMigrate path. Existing data remains valid. No billing fields or provider
request contracts are changed. No background polling worker is added: Active is
observed through existing UI/manual/automatic-import queries. A log is a snapshot
at the time of its request, not a live progress subscription.
