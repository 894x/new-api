# Incremental upstream integration: rc26 to rc40

## Scope and preservation rules

- Start: fork commit `317081814da2c7c8096758ecdc0f76b330745e28`, based on upstream rc25.
- Integrate each upstream RC in order, verify it, and commit once before the next RC.
- Preserve fork functionality; ask the owner before resolving duplicated features.
- Work on `codex/rc26-to-rc40` in an isolated worktree. Original checkout changes remain untouched; reconcile its pending request-body/copy optimization separately before final handoff, with ownership and regression checks.
- Do not push, deploy, or migrate production databases as part of local integration.

## Confirmed feature decisions

- rc26: retain the existing group/model RPM/TPM configuration and inheritance semantics; merge upstream expanded request counts and overflow protection.
- rc27: converge on the JS task plugin architecture and migrate all existing custom task functionality, including Doubao, MiniMax, Wan, Seedance SLS, Tencent TokenHub, and asset-library integration. Do not retain two permanent task execution pipelines.

## Version ledger

### rc26 — integrated

Upstream tag: `8f6961c675932f406260ff0c218bc2aa0603e9b2`.

- Wallet/top-up range expands to JavaScript-safe 64-bit values; individual request billing still saturates at int32. Preserve durable quota writes, quota versions, Redis cache fences, payment snapshots, and idempotent callbacks.
- Enforce wallet credit ceilings atomically, including WeChat and redemption; preserve CNY/custom-currency conversion.
- Expand request counts in the existing group/model limiter and frontend; keep TPM boundaries and existing config formats.
- Include upstream vLLM thinking-token field, log-filter autofill, and Bun CI/container changes.
- Repair verified pre-existing checks: ES2022-incompatible array method, response-only asset-library field classification, and stale hourly analytics expectations (explicit five-minute fixture).

Verification: root `go test -p 2 ./...` and `go build -p 2 ./...`; independent relaykit `GOWORK=off go build ./...` and `go test -p 2 ./...`; frontend typecheck, changed-file lint, 80 related tests, and production build passed. Local toolchain: Go 1.26.5, Bun 1.3.9. CI/container Bun 1.4 was not executed locally. No browser acceptance or live MySQL/PostgreSQL migration was performed.

Deployment prerequisite: inspect and explicitly migrate existing MySQL/PostgreSQL `users.quota`, `used_quota`, `aff_quota`, and `aff_history` to BIGINT before startup. The upstream startup guard intentionally rejects legacy 32-bit columns. Do not bypass the guard as a migration substitute.

### rc27 through rc40 — pending

For each RC, record merged changes, custom-feature mapping, confirmed duplication decisions, test evidence, and remaining deployment limitations here before committing.
