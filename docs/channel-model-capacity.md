# Upstream channel-model capacity

Each upstream channel can set default RPM (admitted requests per minute) and
TPM (reserved tokens per minute). Defaults apply separately to each public model
served by that channel. They are not a shared budget across all models on the
channel. Different public aliases mapped to the same upstream model have separate
counters. All users, API keys, and groups share the same channel/public-model key.
User and API-key rate limits continue to apply independently.

## Configuration

Use Channels → Add/Edit → Routing Strategy for the channel defaults. The model
routing override editor, available from channel and model configuration, exposes
Priority, Weight, RPM, and TPM together. An empty model field inherits the channel
default; an explicit zero is unlimited. All existing channels remain unlimited.
Limits must be integers from zero through 9007199254740991. They are stored in
portable bigint columns through the existing GORM migrations and ability cache.
Old clients that omit RPM/TPM override fields preserve existing capacity overrides;
an explicit JSON null resets the corresponding override to inheritance.

## Admission and fallback

The window is aligned to Unix epoch minutes, not a rolling sixty-second interval.
Admission checks RPM and TPM atomically. Rejection consumes neither counter.
Redis deployments share counters and use Redis server time (Redis 3.2+); without
Redis, each process has its own counters. Redis errors fail closed.

Capacity uses the same eligible candidates as current routing: model matching,
request path, request parameter capabilities, asset replica constraints, status,
and group membership are checked first. Priority and weight select the preferred
channel. A full channel is skipped for this request and the next same-priority or
lower-priority candidate is tried without using an upstream retry. Local capacity
rejection preserves the current auto-group cursor and does not disable a channel
or clear its shared affinity mapping. A forced channel is never replaced.

If every eligible channel is full, the response is HTTP 429 with a safe
`channel_model_capacity_exhausted` error code for OpenAI-style responses (Claude
uses `rate_limit_error`) and a `Retry-After` header until the next minute boundary.

For text generation, TPM reserves estimated input plus the maximum output before
dispatch. Missing output limits reserve 8192 output tokens; explicit zero reserves
input only. Reservations use the final provider body after system prompts,
conversion and parameter policies, including multiple choices and Gemini batches.
Input-only routes reserve input tokens. Oversized reservations cannot wrap into a
small or negative count. This estimate leaves billing and user TPM metadata intact.

Reservations are conservative: actual shorter completions and later local failures
do not refund the channel window. Synchronous text, embedding, reranking, image,
and audio relays participate. Async task relays, OpenAI Realtime, and channel tests
do not. No adaptive TTFT/TPOT routing is introduced.

## Validation

Regression tests cover storage inheritance, explicit zero/null, legacy PATCH,
atomic limits, shared Redis time, output reservation, parameter-filtered spillover,
auto-group cursor restoration, and HTTP dispatch/429 behavior with local upstreams.
Live provider calls are not required for these gateway policy contracts.

The reproducible process E2E runner is `tools/channel-capacity-e2e/run.py`. Build
`web/dist` and the root Go executable, then pass `--binary <path>` and
`--output <new directory>`. Repeat with `--memory-cache false` to exercise DB
selection. It creates isolated SQLite storage (WAL, one connection), sets up real
users/tokens/channels through HTTP, and starts two local simulated upstreams.
Host performance admission is disabled only in that isolated fixture. Channel
creation is followed by an inheritance PATCH to refresh the existing periodic
channel cache. It verifies 13 scenarios, including cross-user/key/group limits,
forced channels, output reservations, transformed payloads, override persistence,
concurrent admission, streaming, user TPM, Auto groups, and actual Retry-After
recovery. It writes a JSON report and stops all spawned servers on exit. No real
provider credentials or paid calls are used.
