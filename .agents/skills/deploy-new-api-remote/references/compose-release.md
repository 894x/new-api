# Reusable Compose release helper

Run `scripts/compose_release.py` on the target Linux host with Python 3.11+,
Docker Compose, curl, tar, gzip, sha256sum and passwordless sudo for Nginx and
read-only filesystem backup commands. The script uses the standard library.
It does not connect over SSH, push Git refs, build an image, or request credentials.

Supported contract: one running service each named `new-api`, `postgres` and
`redis` per Compose project; PostgreSQL tools and Redis tools inside their
containers; app `SQL_DSN` and `REDIS_CONN_STRING` point to those services; one
persistent `/data` bind mount and optional `/app/logs` bind mount; direct Nginx hostname routing
to `127.0.0.1:<port>`; HTTPS is already configured. It discovers actual container
names and project directories using labels. External databases, TLS/ACL-only
Redis, extra secret/config mounts and other proxy layouts use the manual skill
workflow until their backup/verification contract is implemented and tested.
The app must expose only container port `3000/tcp`, bound to the manifest port
on host `127.0.0.1`. Data peers must share its network and app-side `getent hosts`
must resolve to the selected peer's address. Custom app network options, named
app volumes and Compose fields outside the supported profile require manual
verification. The helper fails before replacement rather than ignoring them.
For each hostname, Nginx must have exactly one HTTPS server with one `location /`
and its direct proxy target. Rewrites, additional locations and routing includes
are unsupported; TLS-only includes are inspected. A matching port elsewhere in
the file is insufficient evidence that the hostname reaches this instance.

Before using the helper, verify the intended Git ref, full SHA, clean source,
image ID/OCI revision, Linux amd64 binary hash and frontend entry asset. Keep the
release directory outside every instance checkout. An empty application VERSION
does not replace these identity checks.

## Manifest

Create a private `release.json` alongside the helper's receipts. Populate hashes,
image tag and target details from actual discovery; these example identities are
illustrative and cannot identify a deployable artifact.

```json
{
  "commit": "0123456789abcdef0123456789abcdef01234567",
  "image": "new-api-local:deploy-0123456789ab",
  "image_id": "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "frontend_asset": "static/js/index.0123456789.js",
  "backup_root": "/srv/new-api-backups",
  "page_path": "/",
  "protected_api_path": "/api/channel/",
  "targets": [
    {"project": "test", "hostname": "test.example.com", "port": 3001},
    {"project": "site", "hostname": "api.example.com", "port": 3002, "requires": ["test"]}
  ]
}
```

No password, token, environment content or private key belongs in this manifest.
Keep the release directory mode `0700`. The helper uses umask `077` for its own
files. Its protected backups contain configuration and database/cache state;
do not copy their contents into user-facing reports or source control.

## Commands and receipts

```sh
python3 compose_release.py --config /srv/releases/release.json inspect
python3 compose_release.py --config /srv/releases/release.json snapshot
python3 compose_release.py --config /srv/releases/release.json deploy test
python3 compose_release.py --config /srv/releases/release.json deploy site
python3 compose_release.py --config /srv/releases/release.json verify
```

`inspect` reads topology. `snapshot` records all container identities once; it
refuses to overwrite the baseline. `deploy` requires exactly one named target
and a verified prerequisite receipt, when configured. A previously accepted
target is reverified without replacement. `verify` checks deployed identity,
protected containers, health, frontend asset, authentication boundary and backup
checksums; an optional project limits acceptance to that target.

Before replacement, the helper compares the rendered Compose service with the
running image, complete environment, command, entrypoint, labels, user, working
directory, mounts, ports, networks, restart policy and healthcheck. It then
creates and validates a native PostgreSQL dump, authenticated Redis RDB, app-data
archive, Compose/env archive and resolved Compose backup. PostgreSQL backup uses
streaming files to avoid loading large databases into memory. The rollback
command uses the saved resolved configuration and a retained tag of the exact
previous image. Compose's dollar escaping is preserved and the saved configuration
must pass an independent render round-trip before any replacement. Only image,
pull policy and revision label may differ in the
rendered deployment config. Replacement is always `up -d --no-deps --no-build
new-api`.

On acceptance failure, the helper records `release-failed.json`, attempts to
restore the prior application image/configuration, and checks its identity,
health, HTTP behavior and protected containers. It never restores database or
Redis contents automatically. A failure blocks later rollout commands until the
operator reconciles actual runtime state and prepares a new attempt; do not
delete the marker or overwrite snapshots to conceal a failed check. Commands
interrupted before a receipt also require inspection before retrying.

Read `<project>-acceptance.json` for image IDs, revision, data counts, checks and
backup/rollback paths. A passed receipt covers infrastructure and unauthenticated
HTTP checks. It does not claim a logged-in administrator workflow or a paid
upstream API test. This profile checks that users/channels/tokens/options counts
stay equal during replacement; coordinate configuration writes during that short
window to avoid treating legitimate concurrent changes as a failed acceptance.

## Local validation

```sh
python3 -m unittest discover -s scripts/tests -v
python3 scripts/compose_release.py --help
```

Run from the skill directory. Tests replace external system boundaries with
explicit fixtures; they do not operate live containers. Do not use Python `-O`,
which disables assertions; the CLI rejects optimized execution.
