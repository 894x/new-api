---
name: deploy-new-api-remote
description: Safely deliver a new-api change through commit, Git push, pinned-commit remote build, backup, targeted Compose application replacement, verification, and rollback preparation. Use when asked to deploy, publish, update, restart, or roll back a new-api test or production instance on a remote server, especially when only one instance or application service may change and databases, Redis, or neighboring instances must remain untouched.
---

# Deploy New API Remotely

Deploy one verified commit to one explicitly named instance. Treat source delivery, remote build, backup, application replacement, and verification as separate gates.

## Reusable Compose helper

For the Linux Docker Compose + Nginx + PostgreSQL + Redis layout, use
[`scripts/compose_release.py`](scripts/compose_release.py) after source and image
verification. Read [`references/compose-release.md`](references/compose-release.md)
for its manifest and commands. It discovers container names from Compose labels,
validates backups, replaces only `new-api`, verifies the release, and attempts an
application-only rollback on failed acceptance. It does not grant deployment
authorization; use the targets already authorized in the conversation.

For multiple requested instances, declare prerequisite targets in the manifest
and deploy them sequentially. Verify test acceptance before the next target.
Pass release identity and instance details as configuration; credentials are read
at runtime and kept out of source, terminal output, and acceptance reports.
Restricted rollback configuration/data backups necessarily retain runtime secrets.

Use the manual workflow below for other database/proxy layouts or additional
runtime mounts. A helper rejection identifies a contract that needs inspection;
do not remove its guard just to continue.

## Collect the deployment contract

Resolve these values before making external changes:

- local repository and source branch
- Git remote and destination branch
- exact target commit SHA
- SSH host and account
- environment and instance name
- remote checkout, Compose files, project, and application service
- image name/tag and build arguments
- health/API URL and expected response
- services and data that must remain unchanged

Discover missing runtime details read-only on the server. Ask the user only when the target or authorized scope remains materially ambiguous. Never store credentials in the repository, skill, scripts, shell history, logs, or reports.

## 1. Prepare and publish the source

1. Confirm the active branch and inspect the worktree.
2. Preserve unrelated tracked and untracked files. Stage only deployment-related changes.
3. Run validation proportional to the change and inspect the staged diff.
4. Commit the intended changes and record the full SHA.
5. Push the explicit local commit or branch to the requested Git remote and branch.
6. Verify the remote ref contains the exact SHA. Do not deploy an unpushed or ambiguous revision.

When the user asks for the latest `main`, fetch and resolve `origin/main` explicitly,
then pin that full SHA. Do not substitute the active feature branch. If a clean
build checkout is needed, create an isolated worktree and preserve existing work.

Do not use destructive Git commands to clean either checkout. If the remote checkout contains unexplained changes, stop before changing it.

## 2. Rediscover the remote topology

After connecting, inspect the live host rather than trusting remembered paths or container names:

- enumerate Compose projects, containers, labels, mounted config files, images, ports, and health states
- identify reverse-proxy routing for the requested hostname
- distinguish the target application from production, test, database, Redis, and neighboring projects
- capture target and protected container IDs, image IDs, restart counts, health, and deployed revision labels
- locate the actual checkout and Compose file set used by the target

Resolve the absolute target path before any recursive move, backup, or replacement. Report and stop if discovery conflicts with the user's requested target.

## 3. Fetch and build the pinned commit

1. Fetch the requested remote without rewriting local deployment configuration.
2. Verify that the target SHA exists and belongs to the expected remote ref.
3. Move the deployment checkout to that exact commit using a clean fast-forward or an explicit detached/pinned checkout. Verify `git rev-parse HEAD` equals the target SHA.
4. Build a uniquely tagged image from that checkout using the deployment's existing Dockerfile, Compose configuration, and build arguments.
5. Stamp or verify `org.opencontainers.image.revision=<full-sha>` when the build supports OCI labels.
6. Record the resulting image tag, image ID/digest, and revision label. Do not reuse a mutable tag as the only deployment identity.

Building is non-disruptive; do not replace the running application until the build succeeds.

If registry access fails and the required builder images are unavailable, use a
clean local checkout at the pinned SHA to build the frontend and Linux binary.
Match the Dockerfile's Go environment/build flags, verify ELF architecture and
`go version -m`, then compare local, transferred, image-contained and running
binary SHA-256 values. Build a runtime-only image from an explicitly verified
cached base and copy the pinned license files. On a legacy Docker builder, use
plain `COPY` followed by `RUN chmod`, since `COPY --chmod` needs BuildKit.
For long SSH sessions, enable server keepalives; after disconnects, rediscover
the deployment state before repeating any replacement command.

## 4. Create and validate the rollback point

Immediately before replacing the application:

- create a timestamped backup directory outside the checkout
- back up the database with its native dump tool when the instance owns persistent data
- back up Compose overrides, environment/configuration files, and other required runtime state without printing secrets
- record current container inspection data, image reference/ID, revision, mounts, networks, and restart count
- capture the exact command or configuration needed to restore the previous application image
- validate every material backup: check nonzero size, archive integrity, and database dump integrity such as `gzip -t`

Do not proceed if the backup is missing or unverifiable. A copied source tree alone is not a data backup.

## 5. Replace only the authorized application

1. Render and validate the effective Compose configuration.
2. Point the target application service at the newly built immutable image.
3. Recreate only that service. Prefer the equivalent of:

   `docker compose -f <base> -f <override> up -d --no-deps --no-build <app-service>`

4. Never run project-wide `down`, restart database/Redis, or recreate neighboring instances unless explicitly requested.
5. Keep the previous image available until acceptance is complete.

## 6. Verify the deployment

Confirm all of the following with current evidence:

- the target container is running and healthy
- its image ID and OCI revision match the requested commit
- startup logs contain no migration, connection, or panic errors
- the health endpoint and a representative API request return the expected semantic result
- reverse-proxy routing reaches the intended instance
- target database and Redis container IDs/restart counts are unchanged unless authorized
- every protected application, database, Redis, and neighboring instance is unchanged
- persistent data remains available

An HTTP response alone does not prove the correct commit is running. Verify both runtime identity and behavior.

## 7. Roll back on failure

If verification fails, stop further rollout and restore the previous application image/configuration using the recorded rollback point. Recreate only the target application and repeat health, API, and protected-service checks. Restore data only when data was actually changed and the user authorized that recovery step.

## Report the handoff

Return a concise deployment record containing:

- commit SHA and pushed remote ref
- target host, environment, Compose project, and service
- new and previous image identities
- backup path and validation result
- health/API verification result
- protected services confirmed unchanged
- rollback status or rollback command reference
- any verification not completed

Never include passwords, tokens, private keys, database credentials, or unredacted environment contents.
