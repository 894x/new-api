# Dynamic model-routing simulation

This harness compares the repository's static highest-priority, `weight + 10`
lottery with the production dynamic-routing controller. It does not alter the
gateway request path.

## Run the deterministic feedback sweep

```powershell
go run ./cmd/dynamic-routing-sim `
  -output tools/dynamic-routing-sim/results/local `
  -require-significant
```

The default seeds are fixed. The command writes:

- `report.json`: every parameter, scenario, seed, static result, dynamic
  result, and gate decision;
- `report.md`: concise rankings, acceptance gates, and every winning, neutral,
  and losing run;
- `ranking.csv`: machine-friendly parameter ranking.

`-require-significant` exits unsuccessfully unless the leading parameter set
passes both the composite improvement threshold and every dedicated acceptance
gate. Neutral and losing seeds are never filtered out.

## Simulation model

Requests arrive open-loop: an upstream queue cannot slow the arrival schedule.
Each channel has a time-varying TTFT/TPOT/failure profile and independent
concurrency slots. When all slots are busy, queueing time is added to TTFT, so
route decisions feed back into later user latency and multiple channels can
contribute aggregate capacity.

Built-in cases cover gradual degradation, sudden outage, capacity aggregation,
transient spike, stale candidate data, recovery without flapping, all channels
bad, low traffic, and healthy steady state. Jitter, long-tail events, 429, 503,
hard failure, and recovery are deterministic for a fixed seed.

p95 TTFT is the user-observed latency to either the first visible streamed
content or an error response. TPOT only includes successful responses with
output timing. Reports also include SLO violation rate/area, success throughput,
detection elapsed time and completed observations, mitigation elapsed time,
bad-channel exposure, dominant-allocation reversals, and probe cost.

The dedicated gates require:

- at least 15% aggregate degradation/outage p95 TTFT improvement;
- at least 20% aggregate SLO violation-area improvement;
- no capacity-aggregation throughput loss;
- at least 60% post-detection bad-channel exposure reduction;
- at most 5% healthy p95 TTFT and success regression;
- at most two dominant-allocation reversals in transient/recovery cases.

## Protocol-level SSE validation

The protocol tests use real `httptest` HTTP connections and real SSE framing,
with a controlled clock and event acknowledgements instead of timing sleeps.
They distinguish metadata-only first events from first visible content, measure
per-token delay, capture 429/503 response latency and incomplete streams, and
feed measured TTFT/TPOT back into a two-channel controller until traffic moves
from degraded A to healthy B.

```powershell
go test ./pkg/dynamicrouting/sim/protocol -count=1
```

Historical round directories are intentionally retained so failed parameter
sets and the feedback path remain auditable. `round-5b-gate-fix` is the first
round that passes the dedicated gates after correcting the capacity scenario's
semantics: an overloaded-but-still-useful channel contributes capacity and is
therefore assessed by the throughput gate, not the bad-channel ejection gate.
After aligning simulated 429/5xx/channel errors with production hard-failure
semantics, `round-11-final`
also passes all gates using the corrected full-drain throughput window. Its
leading setting is `probe-015-m3-age90` (1.5% probe, three samples, 90-second
window): 51.89% average composite improvement, 94.74% degradation/outage p95
TTFT improvement, 98.11% SLO-area improvement, 95.14% post-detection
bad-exposure reduction, 1.51% capacity-throughput improvement, and 0.53%
healthy p95 TTFT regression.
