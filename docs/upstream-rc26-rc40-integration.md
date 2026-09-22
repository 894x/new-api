# Incremental upstream integration: rc26 to rc40

## Scope and preservation rules

- Start: fork commit `317081814da2c7c8096758ecdc0f76b330745e28`, based on upstream rc25.
- Integrate each upstream RC in order and verify it before advancing to the next RC. The owner additionally approved splitting the rc27 migration into shared-foundation commits and one verified migration commit per channel; do not batch all channel migrations into one large commit.
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

### rc27 — shared runtime foundation, channel migration pending

- Add the upstream Sobek runtime, plugin registry, immutable routing generations, metadata/usage validation, bounded hook execution, and fixture/CLI library support with their regression tests.
- Reserve custom channel type 105 for task plugins; keep existing custom channel IDs 100–104 and all existing task adaptors unchanged.
- This foundation does not activate the plugin router, register built-in providers, change billing, or delete a channel implementation. Host integration and each channel migration remain separate follow-up commits.
- Preserve the existing rc27 merge resolutions and untracked migration tests while splitting commits. Record the upstream rc27 parent (`eb48396d5fe97d27772d0cd5e3ca8aa5caa4f3e9`) for the final integration merge; intermediate commits do not imply rc27 is fully integrated.
- Each channel commit must include its custom-feature port, compatibility fixes, and request/query/ownership/billing regressions. Do not declare a migration complete solely because the old source was removed.

Verification: performed against a detached rc26 checkout containing only this foundation, not the unfinished rc27 working tree. `go test -p 2 ./...`, `go build -p 2 ./...`, and independent relaykit `GOWORK=off go build ./...` passed. Existing frontend build output is reused solely for Go embedding; no frontend changes or browser acceptance are included.

### rc28 through rc40 — pending

For each RC, record merged changes, custom-feature mapping, confirmed duplication decisions, test evidence, and remaining deployment limitations here before committing.
