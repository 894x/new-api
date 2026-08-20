# Dynamic routing simulation

Significant improvement gate: 15.00%; maximum regression: 8.00%; minimum scenario win rate: 0.00.

## Ranking

| Rank | Parameters | Average improvement | Worst seed | Win rate | Acceptance gates | Significant | Regressed runs |
|---:|---|---:|---:|---:|:---:|:---:|---:|
| 1 | probe-025-m4-age90 | 38.79% | -3.11% | 0.49 | true | true | 0 |
| 2 | probe-020-m3-age60 | 32.57% | -1.26% | 0.44 | true | true | 0 |
| 3 | probe-020-slow-recovery | 30.29% | -1.26% | 0.38 | true | true | 0 |
| 4 | probe-020-m3-age90 | 30.25% | -1.26% | 0.38 | true | true | 0 |
| 5 | probe-015-m3-age60 | 28.96% | -1.07% | 0.41 | true | true | 0 |
| 6 | probe-020-m4-age90 | 28.20% | -1.26% | 0.33 | true | true | 0 |
| 7 | probe-025-m3-age60 | 38.68% | -37.25% | 0.52 | false | false | 1 |
| 8 | probe-025-critical-fast | 38.63% | -37.58% | 0.52 | false | false | 1 |
| 9 | probe-025-m3-age90 | 36.82% | -37.58% | 0.46 | false | false | 1 |
| 10 | probe-015-m3-age90 | 25.60% | -1.07% | 0.27 | false | false | 0 |
| 11 | probe-015-hard-confirmed | 25.58% | -1.07% | 0.27 | false | false | 0 |
| 12 | probe-010-m3-age90 | 21.54% | -0.49% | 0.27 | false | false | 0 |

## Acceptance gates

| Parameters | Gate | Actual | Required | Applicable | Passed |
|---|---|---:|---:|:---:|:---:|
| probe-025-m4-age90 | Degradation/outage p95 TTFT improvement | 71.26 | >= 15.00 | true | true |
| probe-025-m4-age90 | SLO violation area improvement | 72.17 | >= 20.00 | true | true |
| probe-025-m4-age90 | Capacity throughput change | 2.66 | >= 0.00 | true | true |
| probe-025-m4-age90 | Bad exposure reduction | 78.00 | >= 60.00 | true | true |
| probe-025-m4-age90 | Healthy p95 TTFT regression | 1.83 | <= 5.00 | true | true |
| probe-025-m4-age90 | Healthy success regression | 0.00 | <= 5.00 | true | true |
| probe-025-m4-age90 | Stability route reversals | 1.00 | <= 2.00 | true | true |
| probe-020-m3-age60 | Degradation/outage p95 TTFT improvement | 71.36 | >= 15.00 | true | true |
| probe-020-m3-age60 | SLO violation area improvement | 74.34 | >= 20.00 | true | true |
| probe-020-m3-age60 | Capacity throughput change | 2.01 | >= 0.00 | true | true |
| probe-020-m3-age60 | Bad exposure reduction | 70.63 | >= 60.00 | true | true |
| probe-020-m3-age60 | Healthy p95 TTFT regression | 0.73 | <= 5.00 | true | true |
| probe-020-m3-age60 | Healthy success regression | 0.00 | <= 5.00 | true | true |
| probe-020-m3-age60 | Stability route reversals | 1.00 | <= 2.00 | true | true |
| probe-020-slow-recovery | Degradation/outage p95 TTFT improvement | 63.99 | >= 15.00 | true | true |
| probe-020-slow-recovery | SLO violation area improvement | 65.35 | >= 20.00 | true | true |
| probe-020-slow-recovery | Capacity throughput change | 2.01 | >= 0.00 | true | true |
| probe-020-slow-recovery | Bad exposure reduction | 63.95 | >= 60.00 | true | true |
| probe-020-slow-recovery | Healthy p95 TTFT regression | 0.73 | <= 5.00 | true | true |
| probe-020-slow-recovery | Healthy success regression | 0.00 | <= 5.00 | true | true |
| probe-020-slow-recovery | Stability route reversals | 1.00 | <= 2.00 | true | true |
| probe-020-m3-age90 | Degradation/outage p95 TTFT improvement | 64.01 | >= 15.00 | true | true |
| probe-020-m3-age90 | SLO violation area improvement | 65.32 | >= 20.00 | true | true |
| probe-020-m3-age90 | Capacity throughput change | 2.01 | >= 0.00 | true | true |
| probe-020-m3-age90 | Bad exposure reduction | 63.98 | >= 60.00 | true | true |
| probe-020-m3-age90 | Healthy p95 TTFT regression | 0.73 | <= 5.00 | true | true |
| probe-020-m3-age90 | Healthy success regression | 0.00 | <= 5.00 | true | true |
| probe-020-m3-age90 | Stability route reversals | 1.00 | <= 2.00 | true | true |
| probe-015-m3-age60 | Degradation/outage p95 TTFT improvement | 53.81 | >= 15.00 | true | true |
| probe-015-m3-age60 | SLO violation area improvement | 56.66 | >= 20.00 | true | true |
| probe-015-m3-age60 | Capacity throughput change | 1.51 | >= 0.00 | true | true |
| probe-015-m3-age60 | Bad exposure reduction | 63.10 | >= 60.00 | true | true |
| probe-015-m3-age60 | Healthy p95 TTFT regression | 0.53 | <= 5.00 | true | true |
| probe-015-m3-age60 | Healthy success regression | 0.00 | <= 5.00 | true | true |
| probe-015-m3-age60 | Stability route reversals | 1.00 | <= 2.00 | true | true |
| probe-020-m4-age90 | Degradation/outage p95 TTFT improvement | 58.00 | >= 15.00 | true | true |
| probe-020-m4-age90 | SLO violation area improvement | 57.83 | >= 20.00 | true | true |
| probe-020-m4-age90 | Capacity throughput change | 2.01 | >= 0.00 | true | true |
| probe-020-m4-age90 | Bad exposure reduction | 64.22 | >= 60.00 | true | true |
| probe-020-m4-age90 | Healthy p95 TTFT regression | 0.73 | <= 5.00 | true | true |
| probe-020-m4-age90 | Healthy success regression | 0.00 | <= 5.00 | true | true |
| probe-020-m4-age90 | Stability route reversals | 1.00 | <= 2.00 | true | true |
| probe-025-m3-age60 | Degradation/outage p95 TTFT improvement | 77.95 | >= 15.00 | true | true |
| probe-025-m3-age60 | SLO violation area improvement | 82.89 | >= 20.00 | true | true |
| probe-025-m3-age60 | Capacity throughput change | 2.66 | >= 0.00 | true | true |
| probe-025-m3-age60 | Bad exposure reduction | 81.07 | >= 60.00 | true | true |
| probe-025-m3-age60 | Healthy p95 TTFT regression | 8.54 | <= 5.00 | true | false |
| probe-025-m3-age60 | Healthy success regression | 0.00 | <= 5.00 | true | true |
| probe-025-m3-age60 | Stability route reversals | 1.00 | <= 2.00 | true | true |
| probe-025-critical-fast | Degradation/outage p95 TTFT improvement | 78.18 | >= 15.00 | true | true |
| probe-025-critical-fast | SLO violation area improvement | 83.12 | >= 20.00 | true | true |
| probe-025-critical-fast | Capacity throughput change | 2.66 | >= 0.00 | true | true |
| probe-025-critical-fast | Bad exposure reduction | 81.19 | >= 60.00 | true | true |
| probe-025-critical-fast | Healthy p95 TTFT regression | 8.54 | <= 5.00 | true | false |
| probe-025-critical-fast | Healthy success regression | 0.00 | <= 5.00 | true | true |
| probe-025-critical-fast | Stability route reversals | 1.00 | <= 2.00 | true | true |
| probe-025-m3-age90 | Degradation/outage p95 TTFT improvement | 74.42 | >= 15.00 | true | true |
| probe-025-m3-age90 | SLO violation area improvement | 77.32 | >= 20.00 | true | true |
| probe-025-m3-age90 | Capacity throughput change | 2.66 | >= 0.00 | true | true |
| probe-025-m3-age90 | Bad exposure reduction | 75.60 | >= 60.00 | true | true |
| probe-025-m3-age90 | Healthy p95 TTFT regression | 8.54 | <= 5.00 | true | false |
| probe-025-m3-age90 | Healthy success regression | 0.00 | <= 5.00 | true | true |
| probe-025-m3-age90 | Stability route reversals | 1.00 | <= 2.00 | true | true |
| probe-015-m3-age90 | Degradation/outage p95 TTFT improvement | 44.51 | >= 15.00 | true | true |
| probe-015-m3-age90 | SLO violation area improvement | 46.75 | >= 20.00 | true | true |
| probe-015-m3-age90 | Capacity throughput change | 1.51 | >= 0.00 | true | true |
| probe-015-m3-age90 | Bad exposure reduction | 57.31 | >= 60.00 | true | false |
| probe-015-m3-age90 | Healthy p95 TTFT regression | 0.53 | <= 5.00 | true | true |
| probe-015-m3-age90 | Healthy success regression | 0.00 | <= 5.00 | true | true |
| probe-015-m3-age90 | Stability route reversals | 1.00 | <= 2.00 | true | true |
| probe-015-hard-confirmed | Degradation/outage p95 TTFT improvement | 44.51 | >= 15.00 | true | true |
| probe-015-hard-confirmed | SLO violation area improvement | 46.74 | >= 20.00 | true | true |
| probe-015-hard-confirmed | Capacity throughput change | 1.51 | >= 0.00 | true | true |
| probe-015-hard-confirmed | Bad exposure reduction | 57.24 | >= 60.00 | true | false |
| probe-015-hard-confirmed | Healthy p95 TTFT regression | 0.53 | <= 5.00 | true | true |
| probe-015-hard-confirmed | Healthy success regression | 0.00 | <= 5.00 | true | true |
| probe-015-hard-confirmed | Stability route reversals | 1.00 | <= 2.00 | true | true |
| probe-010-m3-age90 | Degradation/outage p95 TTFT improvement | 53.77 | >= 15.00 | true | true |
| probe-010-m3-age90 | SLO violation area improvement | 53.33 | >= 20.00 | true | true |
| probe-010-m3-age90 | Capacity throughput change | 1.00 | >= 0.00 | true | true |
| probe-010-m3-age90 | Bad exposure reduction | 53.69 | >= 60.00 | true | false |
| probe-010-m3-age90 | Healthy p95 TTFT regression | 0.35 | <= 5.00 | true | true |
| probe-010-m3-age90 | Healthy success regression | 0.00 | <= 5.00 | true | true |
| probe-010-m3-age90 | Stability route reversals | 1.00 | <= 2.00 | true | true |

## Every scenario and seed

p95 TTFT includes the user-observed latency to either the first streamed response or an error response; TPOT only covers successful responses with output timing.

| Parameters | Scenario | Seed | Static p95 TTFT | Dynamic p95 TTFT | Static p95 TPOT | Dynamic p95 TPOT | Static SLO violations | Dynamic SLO violations | Detection elapsed | Detection observations | Bad exposure after fault | Bad exposure after detection | Mitigation elapsed | Route reversals | Probe cost | Improvement | Result |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| probe-010-m3-age90 | gradual_degradation | 11 | 2m18.05810847s | 37.500215075s | 111.638008ms | 105.993363ms | 0.738 | 0.338 | 0s | 0 | 206 | 206 | 43.4s | 0 | 0.007 | 75.73% | win |
| probe-010-m3-age90 | gradual_degradation | 29 | 2m17.230127598s | 5.110630427s | 111.973508ms | 48.64668ms | 0.737 | 0.183 | 2.6s | 6 | 107 | 95 | 23.4s | 0 | 0.005 | 96.27% | win |
| probe-010-m3-age90 | gradual_degradation | 47 | 2m18.543146187s | 5.205174172s | 112.017461ms | 49.525019ms | 0.743 | 0.203 | 2.4s | 5 | 108 | 97 | 23.4s | 0 | 0.005 | 96.19% | win |
| probe-010-m3-age90 | gradual_degradation | 71 | 2m18.842157616s | 2m17.026957514s | 112.679273ms | 112.585528ms | 0.742 | 0.733 | 2.4s | 6 | 445 | 434 | not detected | 0 | 0.010 | 1.68% | neutral |
| probe-010-m3-age90 | gradual_degradation | 101 | 2m17.896601353s | 1m59.260849976s | 112.288179ms | 111.972821ms | 0.740 | 0.660 | 2.2s | 6 | 403 | 393 | 1m23.2s | 0 | 0.010 | 15.93% | win |
| probe-010-m3-age90 | gradual_degradation | 131 | 2m18.257709744s | 5.39725221s | 112.50077ms | 49.690321ms | 0.742 | 0.185 | 0s | 0 | 108 | 108 | 23.4s | 0 | 0.005 | 96.10% | win |
| probe-010-m3-age90 | gradual_degradation | 173 | 2m18.511286942s | 5.046547868s | 112.241073ms | 48.627455ms | 0.740 | 0.192 | 1.2s | 3 | 107 | 102 | 23.4s | 0 | 0.005 | 96.32% | win |
| probe-010-m3-age90 | sudden_outage | 11 | 15.6s | 477.877788ms | 29.758413ms | 40.036541ms | 0.669 | 0.038 | 0s | 0 | 10 | 10 | 3.8s | 0 | 0.004 | 93.77% | win |
| probe-010-m3-age90 | sudden_outage | 29 | 15.6s | 477.005412ms | 29.586311ms | 39.871299ms | 0.667 | 0.040 | 2.2s | 5 | 10 | 0 | 3.8s | 0 | 0.004 | 93.74% | win |
| probe-010-m3-age90 | sudden_outage | 47 | 15.6s | 1.062780158s | 29.297237ms | 40.163736ms | 0.676 | 0.056 | 2.2s | 5 | 10 | 0 | 3.8s | 0 | 0.004 | 90.94% | win |
| probe-010-m3-age90 | sudden_outage | 71 | 15.6s | 478.860123ms | 29.362123ms | 39.983305ms | 0.673 | 0.047 | 2.2s | 4 | 10 | 0 | 3.8s | 0 | 0.004 | 93.64% | win |
| probe-010-m3-age90 | sudden_outage | 101 | 15.6s | 479.387647ms | 29.696734ms | 40.248819ms | 0.673 | 0.049 | 2.4s | 5 | 11 | 0 | 4s | 0 | 0.004 | 93.52% | win |
| probe-010-m3-age90 | sudden_outage | 131 | 15.6s | 477.864046ms | 29.458433ms | 40.270961ms | 0.676 | 0.038 | 0s | 0 | 10 | 10 | 3.8s | 0 | 0.004 | 93.76% | win |
| probe-010-m3-age90 | sudden_outage | 173 | 15.6s | 1.332870167s | 29.101676ms | 39.998469ms | 0.673 | 0.060 | 800ms | 2 | 10 | 7 | 3.8s | 0 | 0.004 | 89.69% | win |
| probe-010-m3-age90 | capacity_aggregation | 11 | 1m19.876239452s | 1m18.170298138s | 25ms | 25ms | 0.988 | 0.978 | 0s | 0 | 634 | 634 | not detected | 0 | 0.010 | 2.40% | neutral |
| probe-010-m3-age90 | capacity_aggregation | 29 | 1m19.971533361s | 1m18.252199129s | 25ms | 25ms | 0.988 | 0.978 | 0s | 0 | 634 | 634 | not detected | 0 | 0.010 | 2.40% | neutral |
| probe-010-m3-age90 | capacity_aggregation | 47 | 1m19.89581372s | 1m18.241641406s | 25ms | 25ms | 0.988 | 0.978 | 0s | 0 | 634 | 634 | not detected | 0 | 0.010 | 2.35% | neutral |
| probe-010-m3-age90 | capacity_aggregation | 71 | 1m19.767790605s | 1m18.035468266s | 25ms | 25ms | 0.988 | 0.978 | 0s | 0 | 634 | 634 | not detected | 0 | 0.010 | 2.44% | neutral |
| probe-010-m3-age90 | capacity_aggregation | 101 | 1m20.040921681s | 1m18.281633912s | 25ms | 25ms | 0.990 | 0.981 | 0s | 0 | 634 | 634 | not detected | 0 | 0.010 | 2.44% | neutral |
| probe-010-m3-age90 | capacity_aggregation | 131 | 1m19.734245977s | 1m18.010178368s | 25ms | 25ms | 0.988 | 0.978 | 0s | 0 | 634 | 634 | not detected | 0 | 0.010 | 2.41% | neutral |
| probe-010-m3-age90 | capacity_aggregation | 173 | 1m19.865475022s | 1m18.067978103s | 25ms | 25ms | 0.988 | 0.978 | 0s | 0 | 634 | 634 | not detected | 0 | 0.010 | 2.47% | neutral |
| probe-010-m3-age90 | transient_spike | 11 | 12.6925426s | 11.775049428s | 90ms | 90ms | 0.396 | 0.238 | 0s | 0 | 220 | 220 | 23.2s | 1 | 0.007 | 11.49% | neutral |
| probe-010-m3-age90 | transient_spike | 29 | 12.307633594s | 12.014700554s | 90ms | 90ms | 0.384 | 0.380 | 4.8s | 6 | 297 | 274 | not detected | 0 | 0.009 | 2.10% | neutral |
| probe-010-m3-age90 | transient_spike | 47 | 12.854235214s | 11.955529444s | 90ms | 90ms | 0.402 | 0.247 | 4.4s | 5 | 218 | 197 | 23.2s | 1 | 0.007 | 11.14% | neutral |
| probe-010-m3-age90 | transient_spike | 71 | 12.659328791s | 11.842692919s | 90ms | 90ms | 0.400 | 0.376 | 4.6s | 4 | 297 | 275 | not detected | 0 | 0.009 | 6.45% | neutral |
| probe-010-m3-age90 | transient_spike | 101 | 12.607221347s | 12.049150513s | 90ms | 90ms | 0.398 | 0.380 | 4.6s | 4 | 297 | 275 | not detected | 0 | 0.009 | 4.53% | neutral |
| probe-010-m3-age90 | transient_spike | 131 | 12.512324827s | 11.879288542s | 90ms | 90ms | 0.398 | 0.253 | 0s | 0 | 216 | 216 | 23.4s | 1 | 0.007 | 9.33% | neutral |
| probe-010-m3-age90 | transient_spike | 173 | 12.652228169s | 11.844818372s | 90ms | 90ms | 0.398 | 0.378 | 800ms | 2 | 297 | 294 | not detected | 0 | 0.009 | 6.13% | neutral |
| probe-010-m3-age90 | stale_candidate | 11 | 46.403333381s | 45.010000046s | 110ms | 110ms | 0.889 | 0.877 | 4.666666662s | 2 | 149 | 135 | not detected | 0 | 0.012 | 2.49% | neutral |
| probe-010-m3-age90 | stale_candidate | 29 | 52.933333379s | 51.266666714s | 110ms | 110ms | 0.883 | 0.871 | 4.666666662s | 2 | 149 | 135 | not detected | 0 | 0.012 | 3.56% | neutral |
| probe-010-m3-age90 | stale_candidate | 47 | 51.540000046s | 50.933333379s | 110ms | 110ms | 0.883 | 0.871 | 4.666666662s | 2 | 149 | 135 | not detected | 0 | 0.012 | 1.94% | neutral |
| probe-010-m3-age90 | stale_candidate | 71 | 52.600000048s | 51.873333379s | 110ms | 110ms | 0.889 | 0.877 | 0s | 0 | 149 | 149 | not detected | 0 | 0.012 | 1.71% | neutral |
| probe-010-m3-age90 | stale_candidate | 101 | 49.070000047s | 47.010000046s | 110ms | 110ms | 0.883 | 0.871 | 4.666666662s | 1 | 149 | 135 | not detected | 0 | 0.012 | 3.96% | neutral |
| probe-010-m3-age90 | stale_candidate | 131 | 47.40333338s | 47.266666714s | 110ms | 110ms | 0.883 | 0.871 | 4.666666662s | 2 | 149 | 135 | not detected | 0 | 0.012 | 1.02% | neutral |
| probe-010-m3-age90 | stale_candidate | 173 | 46.87333338s | 45.343333379s | 110ms | 110ms | 0.883 | 0.871 | 2.999999997s | 0 | 149 | 140 | not detected | 0 | 0.012 | 3.70% | neutral |
| probe-010-m3-age90 | recovery_no_flap | 11 | 44.799011002s | 43.945s | 95ms | 95ms | 0.752 | 0.353 | 0s | 0 | 204 | 204 | 43s | 0 | 0.007 | 20.01% | win |
| probe-010-m3-age90 | recovery_no_flap | 29 | 44.937203373s | 43.945s | 95ms | 95ms | 0.750 | 0.355 | 4.4s | 5 | 205 | 184 | 43s | 0 | 0.007 | 20.18% | win |
| probe-010-m3-age90 | recovery_no_flap | 47 | 44.810457119s | 43.945s | 95ms | 95ms | 0.757 | 0.527 | 4.4s | 5 | 304 | 283 | 1m3s | 0 | 0.008 | 11.09% | neutral |
| probe-010-m3-age90 | recovery_no_flap | 71 | 44.958361864s | 43.945s | 95ms | 95ms | 0.755 | 0.747 | 4.4s | 4 | 445 | 424 | not detected | 0 | 0.010 | 2.63% | neutral |
| probe-010-m3-age90 | recovery_no_flap | 101 | 44.837077225s | 43.945s | 95ms | 95ms | 0.755 | 0.747 | 4.4s | 4 | 445 | 424 | not detected | 0 | 0.010 | 2.48% | neutral |
| probe-010-m3-age90 | recovery_no_flap | 131 | 44.901274352s | 43.945s | 95ms | 95ms | 0.757 | 0.360 | 0s | 0 | 206 | 206 | 43.2s | 0 | 0.007 | 20.09% | win |
| probe-010-m3-age90 | recovery_no_flap | 173 | 44.9015634s | 24.265s | 95ms | 95ms | 0.755 | 0.212 | 800ms | 2 | 105 | 102 | 23s | 0 | 0.005 | 58.36% | win |
| probe-010-m3-age90 | all_channels_bad | 11 | 2m10.9s | 2m8.7s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | 300 | not detected | 0 | 0.009 | 1.83% | neutral |
| probe-010-m3-age90 | all_channels_bad | 29 | 2m11.7s | 2m9.6s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | 300 | not detected | 0 | 0.009 | 1.76% | neutral |
| probe-010-m3-age90 | all_channels_bad | 47 | 2m7.1s | 2m5.1s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | 300 | not detected | 0 | 0.009 | 1.76% | neutral |
| probe-010-m3-age90 | all_channels_bad | 71 | 2m9.6s | 2m8.1s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.009 | 1.44% | neutral |
| probe-010-m3-age90 | all_channels_bad | 101 | 2m6.5s | 2m4.9s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.009 | 1.37% | neutral |
| probe-010-m3-age90 | all_channels_bad | 131 | 2m5.1s | 2m4.1s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | 300 | not detected | 0 | 0.009 | 0.91% | neutral |
| probe-010-m3-age90 | all_channels_bad | 173 | 2m9.753516526s | 2m7.6s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.009 | 1.82% | neutral |
| probe-010-m3-age90 | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-010-m3-age90 | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-010-m3-age90 | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-010-m3-age90 | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-010-m3-age90 | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-010-m3-age90 | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-010-m3-age90 | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-010-m3-age90 | healthy_steady_state | 11 | 306.901147ms | 307.561492ms | 29.560991ms | 29.675177ms | 0.015 | 0.015 | not detected | 0 | 0 | 0 | not detected | 0 | 0.010 | -0.30% | neutral |
| probe-010-m3-age90 | healthy_steady_state | 29 | 306.449512ms | 307.998771ms | 29.524996ms | 29.599932ms | 0.015 | 0.015 | not detected | 0 | 0 | 0 | not detected | 0 | 0.010 | -0.33% | neutral |
| probe-010-m3-age90 | healthy_steady_state | 47 | 306.757116ms | 308.034025ms | 29.42026ms | 29.509822ms | 0.028 | 0.028 | not detected | 0 | 0 | 0 | not detected | 0 | 0.010 | -0.31% | neutral |
| probe-010-m3-age90 | healthy_steady_state | 71 | 304.971872ms | 306.486755ms | 29.463197ms | 29.600085ms | 0.018 | 0.018 | not detected | 0 | 0 | 0 | not detected | 0 | 0.010 | -0.45% | neutral |
| probe-010-m3-age90 | healthy_steady_state | 101 | 306.026121ms | 306.826856ms | 29.656679ms | 29.71846ms | 0.023 | 0.023 | not detected | 0 | 0 | 0 | not detected | 0 | 0.010 | -0.21% | neutral |
| probe-010-m3-age90 | healthy_steady_state | 131 | 307.461992ms | 308.596636ms | 29.32466ms | 29.509705ms | 0.022 | 0.022 | not detected | 0 | 0 | 0 | not detected | 0 | 0.010 | -0.49% | neutral |
| probe-010-m3-age90 | healthy_steady_state | 173 | 306.762211ms | 307.385827ms | 29.635213ms | 29.710777ms | 0.022 | 0.022 | not detected | 0 | 0 | 0 | not detected | 0 | 0.010 | -0.28% | neutral |
| probe-015-m3-age90 | gradual_degradation | 11 | 2m18.05810847s | 2m15.063822362s | 111.638008ms | 111.638008ms | 0.738 | 0.727 | 0s | 0 | 443 | 443 | not detected | 0 | 0.015 | 2.59% | neutral |
| probe-015-m3-age90 | gradual_degradation | 29 | 2m17.230127598s | 3.992127673s | 111.973508ms | 47.134381ms | 0.737 | 0.157 | 2.4s | 5 | 90 | 78 | 19.8s | 0 | 0.007 | 97.03% | win |
| probe-015-m3-age90 | gradual_degradation | 47 | 2m18.543146187s | 1m30.466737215s | 112.017461ms | 110.791939ms | 0.743 | 0.563 | 2.2s | 5 | 335 | 324 | 1m9.6s | 0 | 0.012 | 38.68% | win |
| probe-015-m3-age90 | gradual_degradation | 71 | 2m18.842157616s | 43.801565167s | 112.679273ms | 108.802018ms | 0.742 | 0.378 | 2.4s | 7 | 220 | 208 | 46.2s | 0 | 0.010 | 71.82% | win |
| probe-015-m3-age90 | gradual_degradation | 101 | 2m17.896601353s | 2m15.028784667s | 112.288179ms | 112.288179ms | 0.740 | 0.732 | 2.2s | 7 | 443 | 432 | not detected | 0 | 0.015 | 2.51% | neutral |
| probe-015-m3-age90 | gradual_degradation | 131 | 2m18.257709744s | 980.556272ms | 112.50077ms | 40.752122ms | 0.742 | 0.050 | 0s | 0 | 24 | 24 | 6.4s | 0 | 0.005 | 99.03% | win |
| probe-015-m3-age90 | gradual_degradation | 173 | 2m18.511286942s | 44.501332635s | 112.241073ms | 108.888477ms | 0.740 | 0.368 | 1.2s | 3 | 221 | 215 | 46.4s | 0 | 0.010 | 71.35% | win |
| probe-015-m3-age90 | sudden_outage | 11 | 15.6s | 477.877788ms | 29.758413ms | 40.050908ms | 0.669 | 0.038 | 0s | 0 | 10 | 10 | 3.6s | 0 | 0.004 | 93.77% | win |
| probe-015-m3-age90 | sudden_outage | 29 | 15.6s | 477.005412ms | 29.586311ms | 39.871299ms | 0.667 | 0.040 | 2s | 5 | 10 | 0 | 3.6s | 0 | 0.004 | 93.74% | win |
| probe-015-m3-age90 | sudden_outage | 47 | 15.6s | 1.05460131s | 29.297237ms | 39.405096ms | 0.676 | 0.053 | 2.4s | 7 | 12 | 0 | 4s | 0 | 0.004 | 90.93% | win |
| probe-015-m3-age90 | sudden_outage | 71 | 15.6s | 478.860123ms | 29.362123ms | 39.983305ms | 0.673 | 0.047 | 2s | 4 | 10 | 0 | 3.6s | 0 | 0.004 | 93.64% | win |
| probe-015-m3-age90 | sudden_outage | 101 | 15.6s | 478.24933ms | 29.696734ms | 40.539913ms | 0.673 | 0.049 | 2s | 4 | 10 | 0 | 3.6s | 0 | 0.004 | 93.57% | win |
| probe-015-m3-age90 | sudden_outage | 131 | 15.6s | 477.864046ms | 29.458433ms | 40.270961ms | 0.676 | 0.038 | 0s | 0 | 10 | 10 | 3.6s | 0 | 0.004 | 93.76% | win |
| probe-015-m3-age90 | sudden_outage | 173 | 15.6s | 1.332870167s | 29.101676ms | 39.998469ms | 0.673 | 0.060 | 800ms | 2 | 10 | 6 | 3.6s | 0 | 0.004 | 89.69% | win |
| probe-015-m3-age90 | capacity_aggregation | 11 | 1m19.876239452s | 1m17.424416709s | 25ms | 25ms | 0.988 | 0.972 | 0s | 0 | 630 | 630 | not detected | 0 | 0.015 | 3.55% | neutral |
| probe-015-m3-age90 | capacity_aggregation | 29 | 1m19.971533361s | 1m17.403747547s | 25ms | 25ms | 0.988 | 0.972 | 0s | 0 | 630 | 630 | not detected | 0 | 0.015 | 3.65% | neutral |
| probe-015-m3-age90 | capacity_aggregation | 47 | 1m19.89581372s | 1m17.464650921s | 25ms | 25ms | 0.988 | 0.972 | 0s | 0 | 630 | 630 | not detected | 0 | 0.015 | 3.54% | neutral |
| probe-015-m3-age90 | capacity_aggregation | 71 | 1m19.767790605s | 1m17.268448036s | 25ms | 25ms | 0.988 | 0.972 | 0s | 0 | 630 | 630 | not detected | 0 | 0.015 | 3.60% | neutral |
| probe-015-m3-age90 | capacity_aggregation | 101 | 1m20.040921681s | 1m17.526615303s | 25ms | 25ms | 0.990 | 0.975 | 0s | 0 | 630 | 630 | not detected | 0 | 0.015 | 3.59% | neutral |
| probe-015-m3-age90 | capacity_aggregation | 131 | 1m19.734245977s | 1m17.170636114s | 25ms | 25ms | 0.988 | 0.972 | 0s | 0 | 630 | 630 | not detected | 0 | 0.015 | 3.67% | neutral |
| probe-015-m3-age90 | capacity_aggregation | 173 | 1m19.865475022s | 1m17.320114208s | 25ms | 25ms | 0.988 | 0.972 | 0s | 0 | 630 | 630 | not detected | 0 | 0.015 | 3.63% | neutral |
| probe-015-m3-age90 | transient_spike | 11 | 12.6925426s | 11.666680066s | 90ms | 90ms | 0.396 | 0.369 | 0s | 0 | 295 | 295 | not detected | 0 | 0.016 | 7.95% | neutral |
| probe-015-m3-age90 | transient_spike | 29 | 12.307633594s | 11.911495052s | 90ms | 90ms | 0.384 | 0.373 | 4.4s | 5 | 295 | 274 | not detected | 0 | 0.016 | 3.26% | neutral |
| probe-015-m3-age90 | transient_spike | 47 | 12.854235214s | 11.8145828s | 90ms | 90ms | 0.402 | 0.376 | 4.6s | 6 | 295 | 273 | not detected | 0 | 0.016 | 7.79% | neutral |
| probe-015-m3-age90 | transient_spike | 71 | 12.659328791s | 1.931534132s | 90ms | 90ms | 0.400 | 0.076 | 4.4s | 4 | 23 | 2 | 6.2s | 0 | 0.007 | 78.46% | win |
| probe-015-m3-age90 | transient_spike | 101 | 12.607221347s | 11.934856311s | 90ms | 90ms | 0.398 | 0.207 | 4.2s | 4 | 188 | 168 | 19.6s | 1 | 0.009 | 11.98% | neutral |
| probe-015-m3-age90 | transient_spike | 131 | 12.512324827s | 11.776701608s | 90ms | 90ms | 0.398 | 0.367 | 0s | 0 | 295 | 295 | not detected | 0 | 0.016 | 6.30% | neutral |
| probe-015-m3-age90 | transient_spike | 173 | 12.652228169s | 11.673458085s | 90ms | 90ms | 0.398 | 0.211 | 800ms | 2 | 189 | 185 | 19.6s | 1 | 0.009 | 13.70% | neutral |
| probe-015-m3-age90 | stale_candidate | 11 | 46.403333381s | 44.010000047s | 110ms | 110ms | 0.889 | 0.865 | 4.666666662s | 2 | 147 | 134 | not detected | 0 | 0.018 | 5.63% | neutral |
| probe-015-m3-age90 | stale_candidate | 29 | 52.933333379s | 50.736666712s | 110ms | 110ms | 0.883 | 0.860 | 4.666666662s | 2 | 147 | 134 | not detected | 0 | 0.018 | 5.20% | neutral |
| probe-015-m3-age90 | stale_candidate | 47 | 51.540000046s | 48.873333378s | 110ms | 110ms | 0.883 | 0.860 | 4.666666662s | 2 | 147 | 134 | not detected | 0 | 0.018 | 5.58% | neutral |
| probe-015-m3-age90 | stale_candidate | 71 | 52.600000048s | 50.266666713s | 110ms | 110ms | 0.889 | 0.865 | 0s | 0 | 147 | 147 | not detected | 0 | 0.018 | 5.07% | neutral |
| probe-015-m3-age90 | stale_candidate | 101 | 49.070000047s | 46.87333338s | 110ms | 110ms | 0.883 | 0.860 | 4.666666662s | 1 | 147 | 134 | not detected | 0 | 0.018 | 4.79% | neutral |
| probe-015-m3-age90 | stale_candidate | 131 | 47.40333338s | 45.813333378s | 110ms | 110ms | 0.883 | 0.860 | 4.666666662s | 2 | 147 | 134 | not detected | 0 | 0.018 | 4.07% | neutral |
| probe-015-m3-age90 | stale_candidate | 173 | 46.87333338s | 44.676666713s | 110ms | 110ms | 0.883 | 0.860 | 2.999999997s | 0 | 147 | 138 | not detected | 0 | 0.018 | 5.65% | neutral |
| probe-015-m3-age90 | recovery_no_flap | 11 | 44.799011002s | 1.358971675s | 95ms | 40.762979ms | 0.752 | 0.053 | 0s | 0 | 22 | 22 | 6.2s | 0 | 0.005 | 96.68% | win |
| probe-015-m3-age90 | recovery_no_flap | 29 | 44.937203373s | 1.418231073s | 95ms | 40.65075ms | 0.750 | 0.062 | 4.2s | 5 | 23 | 3 | 6.2s | 0 | 0.005 | 96.55% | win |
| probe-015-m3-age90 | recovery_no_flap | 47 | 44.810457119s | 43.745s | 95ms | 95ms | 0.757 | 0.745 | 4.2s | 5 | 443 | 423 | not detected | 0 | 0.015 | 3.19% | neutral |
| probe-015-m3-age90 | recovery_no_flap | 71 | 44.958361864s | 1.3590909s | 95ms | 40.858579ms | 0.755 | 0.065 | 4.2s | 4 | 22 | 2 | 6.2s | 0 | 0.005 | 96.63% | win |
| probe-015-m3-age90 | recovery_no_flap | 101 | 44.837077225s | 43.745s | 95ms | 95ms | 0.755 | 0.600 | 4.2s | 4 | 352 | 332 | 1m12.8s | 0 | 0.013 | 8.26% | neutral |
| probe-015-m3-age90 | recovery_no_flap | 131 | 44.901274352s | 43.745s | 95ms | 95ms | 0.757 | 0.600 | 0s | 0 | 352 | 352 | 1m12.8s | 0 | 0.013 | 8.34% | neutral |
| probe-015-m3-age90 | recovery_no_flap | 173 | 44.9015634s | 19.095s | 95ms | 95ms | 0.755 | 0.185 | 800ms | 2 | 89 | 85 | 19.6s | 0 | 0.007 | 66.98% | win |
| probe-015-m3-age90 | all_channels_bad | 11 | 2m10.9s | 2m7.5s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | 300 | not detected | 0 | 0.016 | 2.95% | neutral |
| probe-015-m3-age90 | all_channels_bad | 29 | 2m11.7s | 2m9s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | 300 | not detected | 0 | 0.016 | 2.54% | neutral |
| probe-015-m3-age90 | all_channels_bad | 47 | 2m7.1s | 2m4.1s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | 300 | not detected | 0 | 0.016 | 2.66% | neutral |
| probe-015-m3-age90 | all_channels_bad | 71 | 2m9.6s | 2m7.3s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.016 | 2.26% | neutral |
| probe-015-m3-age90 | all_channels_bad | 101 | 2m6.5s | 2m4s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.016 | 2.38% | neutral |
| probe-015-m3-age90 | all_channels_bad | 131 | 2m5.1s | 2m2.6s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | 300 | not detected | 0 | 0.016 | 2.40% | neutral |
| probe-015-m3-age90 | all_channels_bad | 173 | 2m9.753516526s | 2m6.8s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.016 | 2.72% | neutral |
| probe-015-m3-age90 | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-015-m3-age90 | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-015-m3-age90 | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-015-m3-age90 | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-015-m3-age90 | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-015-m3-age90 | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-015-m3-age90 | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-015-m3-age90 | healthy_steady_state | 11 | 306.901147ms | 307.699496ms | 29.560991ms | 29.775736ms | 0.015 | 0.015 | not detected | 0 | 0 | 0 | not detected | 0 | 0.015 | -0.52% | neutral |
| probe-015-m3-age90 | healthy_steady_state | 29 | 306.449512ms | 308.206465ms | 29.524996ms | 29.654816ms | 0.015 | 0.015 | not detected | 0 | 0 | 0 | not detected | 0 | 0.015 | -0.46% | neutral |
| probe-015-m3-age90 | healthy_steady_state | 47 | 306.757116ms | 309.133957ms | 29.42026ms | 29.519065ms | 0.028 | 0.028 | not detected | 0 | 0 | 0 | not detected | 0 | 0.015 | -0.46% | neutral |
| probe-015-m3-age90 | healthy_steady_state | 71 | 304.971872ms | 307.576474ms | 29.463197ms | 29.650924ms | 0.018 | 0.018 | not detected | 0 | 0 | 0 | not detected | 0 | 0.015 | -0.67% | neutral |
| probe-015-m3-age90 | healthy_steady_state | 101 | 306.026121ms | 306.947866ms | 29.656679ms | 29.751595ms | 0.023 | 0.025 | not detected | 0 | 0 | 0 | not detected | 0 | 0.015 | -0.99% | neutral |
| probe-015-m3-age90 | healthy_steady_state | 131 | 307.461992ms | 308.762844ms | 29.32466ms | 29.584412ms | 0.022 | 0.022 | not detected | 0 | 0 | 0 | not detected | 0 | 0.015 | -0.65% | neutral |
| probe-015-m3-age90 | healthy_steady_state | 173 | 306.762211ms | 308.445333ms | 29.635213ms | 29.753827ms | 0.022 | 0.023 | not detected | 0 | 0 | 0 | not detected | 0 | 0.015 | -1.07% | neutral |
| probe-020-m3-age90 | gradual_degradation | 11 | 2m18.05810847s | 1.172039208s | 111.638008ms | 40.770887ms | 0.738 | 0.057 | 0s | 0 | 33 | 33 | 8.2s | 0 | 0.007 | 98.91% | win |
| probe-020-m3-age90 | gradual_degradation | 29 | 2m17.230127598s | 1.345810991s | 111.973508ms | 40.974649ms | 0.737 | 0.065 | 2.4s | 5 | 33 | 21 | 8.4s | 0 | 0.007 | 98.79% | win |
| probe-020-m3-age90 | gradual_degradation | 47 | 2m18.543146187s | 1m6.716505117s | 112.017461ms | 109.072554ms | 0.743 | 0.473 | 2.2s | 5 | 277 | 266 | 58s | 0 | 0.015 | 56.08% | win |
| probe-020-m3-age90 | gradual_degradation | 71 | 2m18.842157616s | 1.338605405s | 112.679273ms | 40.955626ms | 0.742 | 0.070 | 2.4s | 7 | 33 | 21 | 8.4s | 0 | 0.007 | 98.80% | win |
| probe-020-m3-age90 | gradual_degradation | 101 | 2m17.896601353s | 7.727712464s | 112.288179ms | 90.348249ms | 0.740 | 0.237 | 2.2s | 7 | 130 | 119 | 28.2s | 0 | 0.010 | 94.14% | win |
| probe-020-m3-age90 | gradual_degradation | 131 | 2m18.257709744s | 665.66754ms | 112.50077ms | 40.492487ms | 0.742 | 0.033 | 0s | 0 | 11 | 11 | 3.8s | 0 | 0.005 | 99.23% | win |
| probe-020-m3-age90 | gradual_degradation | 173 | 2m18.511286942s | 2m14.168782883s | 112.241073ms | 112.196973ms | 0.740 | 0.725 | 1.2s | 3 | 441 | 435 | not detected | 0 | 0.020 | 3.60% | neutral |
| probe-020-m3-age90 | sudden_outage | 11 | 15.6s | 477.877788ms | 29.758413ms | 40.050908ms | 0.669 | 0.038 | 0s | 0 | 10 | 10 | 3.6s | 0 | 0.007 | 93.77% | win |
| probe-020-m3-age90 | sudden_outage | 29 | 15.6s | 477.005412ms | 29.586311ms | 39.871299ms | 0.667 | 0.040 | 2s | 5 | 10 | 0 | 3.6s | 0 | 0.007 | 93.74% | win |
| probe-020-m3-age90 | sudden_outage | 47 | 15.6s | 1.05460131s | 29.297237ms | 39.405096ms | 0.676 | 0.053 | 2.4s | 7 | 12 | 0 | 4s | 0 | 0.007 | 90.93% | win |
| probe-020-m3-age90 | sudden_outage | 71 | 15.6s | 478.860123ms | 29.362123ms | 39.983305ms | 0.673 | 0.047 | 2s | 4 | 10 | 0 | 3.6s | 0 | 0.007 | 93.64% | win |
| probe-020-m3-age90 | sudden_outage | 101 | 15.6s | 478.24933ms | 29.696734ms | 40.521742ms | 0.673 | 0.049 | 2s | 4 | 10 | 0 | 3.6s | 0 | 0.007 | 93.57% | win |
| probe-020-m3-age90 | sudden_outage | 131 | 15.6s | 477.864046ms | 29.458433ms | 40.270961ms | 0.676 | 0.038 | 0s | 0 | 10 | 10 | 3.6s | 0 | 0.007 | 93.76% | win |
| probe-020-m3-age90 | sudden_outage | 173 | 15.6s | 1.332870167s | 29.101676ms | 39.998469ms | 0.673 | 0.060 | 800ms | 2 | 10 | 6 | 3.6s | 0 | 0.007 | 89.69% | win |
| probe-020-m3-age90 | capacity_aggregation | 11 | 1m19.876239452s | 1m16.441213267s | 25ms | 25ms | 0.988 | 0.968 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.81% | neutral |
| probe-020-m3-age90 | capacity_aggregation | 29 | 1m19.971533361s | 1m16.483636642s | 25ms | 25ms | 0.988 | 0.968 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.84% | neutral |
| probe-020-m3-age90 | capacity_aggregation | 47 | 1m19.89581372s | 1m16.570113621s | 25ms | 25ms | 0.988 | 0.968 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.70% | neutral |
| probe-020-m3-age90 | capacity_aggregation | 71 | 1m19.767790605s | 1m16.420662792s | 25ms | 25ms | 0.988 | 0.968 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.77% | neutral |
| probe-020-m3-age90 | capacity_aggregation | 101 | 1m20.040921681s | 1m16.597949361s | 25ms | 25ms | 0.990 | 0.971 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.81% | neutral |
| probe-020-m3-age90 | capacity_aggregation | 131 | 1m19.734245977s | 1m16.213165556s | 25ms | 25ms | 0.988 | 0.968 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.88% | neutral |
| probe-020-m3-age90 | capacity_aggregation | 173 | 1m19.865475022s | 1m16.321864619s | 25ms | 25ms | 0.988 | 0.968 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.91% | neutral |
| probe-020-m3-age90 | transient_spike | 11 | 12.6925426s | 11.760379494s | 90ms | 90ms | 0.396 | 0.180 | 0s | 0 | 190 | 190 | 17.8s | 1 | 0.011 | 14.96% | neutral |
| probe-020-m3-age90 | transient_spike | 29 | 12.307633594s | 11.75348859s | 90ms | 90ms | 0.384 | 0.369 | 4.4s | 5 | 294 | 272 | not detected | 0 | 0.020 | 4.58% | neutral |
| probe-020-m3-age90 | transient_spike | 47 | 12.854235214s | 11.957757948s | 90ms | 90ms | 0.402 | 0.373 | 4.6s | 6 | 294 | 271 | not detected | 0 | 0.020 | 6.91% | neutral |
| probe-020-m3-age90 | transient_spike | 71 | 12.659328791s | 4.446377905s | 90ms | 90ms | 0.400 | 0.093 | 4.4s | 4 | 31 | 9 | 7.8s | 0 | 0.009 | 63.30% | win |
| probe-020-m3-age90 | transient_spike | 101 | 12.607221347s | 11.819956179s | 90ms | 90ms | 0.398 | 0.371 | 4.2s | 4 | 294 | 273 | not detected | 0 | 0.020 | 6.41% | neutral |
| probe-020-m3-age90 | transient_spike | 131 | 12.512324827s | 1.452576305s | 90ms | 40.997411ms | 0.398 | 0.064 | 0s | 0 | 22 | 22 | 6s | 0 | 0.007 | 85.72% | win |
| probe-020-m3-age90 | transient_spike | 173 | 12.652228169s | 4.578714635s | 90ms | 90ms | 0.398 | 0.104 | 800ms | 2 | 31 | 27 | 7.8s | 0 | 0.009 | 62.24% | win |
| probe-020-m3-age90 | stale_candidate | 11 | 46.403333381s | 44.34333338s | 110ms | 110ms | 0.889 | 0.877 | 4.666666662s | 2 | 148 | 135 | not detected | 0 | 0.018 | 4.11% | neutral |
| probe-020-m3-age90 | stale_candidate | 29 | 52.933333379s | 37.086666697s | 110ms | 110ms | 0.883 | 0.626 | 4.666666662s | 2 | 107 | 94 | 39.333333294s | 0 | 0.018 | 34.01% | win |
| probe-020-m3-age90 | stale_candidate | 47 | 51.540000046s | 50.266666713s | 110ms | 110ms | 0.883 | 0.865 | 4.666666662s | 2 | 148 | 135 | not detected | 0 | 0.018 | 2.78% | neutral |
| probe-020-m3-age90 | stale_candidate | 71 | 52.600000048s | 35.086666699s | 110ms | 110ms | 0.889 | 0.632 | 0s | 0 | 107 | 107 | 39.333333294s | 0 | 0.018 | 36.08% | win |
| probe-020-m3-age90 | stale_candidate | 101 | 49.070000047s | 34.556666699s | 110ms | 110ms | 0.883 | 0.649 | 4.666666662s | 1 | 107 | 94 | 39.333333294s | 0 | 0.018 | 33.45% | win |
| probe-020-m3-age90 | stale_candidate | 131 | 47.40333338s | 46.736666712s | 110ms | 110ms | 0.883 | 0.865 | 4.666666662s | 1 | 148 | 135 | not detected | 0 | 0.018 | 1.74% | neutral |
| probe-020-m3-age90 | stale_candidate | 173 | 46.87333338s | 46.343333378s | 110ms | 110ms | 0.883 | 0.865 | 2.999999997s | 0 | 148 | 140 | not detected | 0 | 0.018 | 1.94% | neutral |
| probe-020-m3-age90 | recovery_no_flap | 11 | 44.799011002s | 43.545s | 95ms | 95ms | 0.752 | 0.305 | 0s | 0 | 179 | 179 | 37.8s | 0 | 0.012 | 23.47% | win |
| probe-020-m3-age90 | recovery_no_flap | 29 | 44.937203373s | 43.545s | 95ms | 95ms | 0.750 | 0.645 | 4.2s | 5 | 383 | 362 | 1m19.4s | 0 | 0.018 | 7.13% | neutral |
| probe-020-m3-age90 | recovery_no_flap | 47 | 44.810457119s | 31.32s | 95ms | 95ms | 0.757 | 0.247 | 4.2s | 5 | 130 | 109 | 28s | 0 | 0.010 | 45.96% | win |
| probe-020-m3-age90 | recovery_no_flap | 71 | 44.958361864s | 43.545s | 95ms | 95ms | 0.755 | 0.742 | 4.2s | 4 | 441 | 420 | not detected | 0 | 0.020 | 4.25% | neutral |
| probe-020-m3-age90 | recovery_no_flap | 101 | 44.837077225s | 43.545s | 95ms | 95ms | 0.755 | 0.742 | 4.2s | 4 | 441 | 420 | not detected | 0 | 0.020 | 4.12% | neutral |
| probe-020-m3-age90 | recovery_no_flap | 131 | 44.901274352s | 1.014703488s | 95ms | 40.710898ms | 0.757 | 0.053 | 0s | 0 | 21 | 21 | 5.8s | 0 | 0.005 | 97.19% | win |
| probe-020-m3-age90 | recovery_no_flap | 173 | 44.9015634s | 1.9s | 95ms | 95ms | 0.755 | 0.082 | 800ms | 2 | 32 | 28 | 8s | 0 | 0.007 | 94.32% | win |
| probe-020-m3-age90 | all_channels_bad | 11 | 2m10.9s | 2m7.4s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | 300 | not detected | 0 | 0.020 | 2.92% | neutral |
| probe-020-m3-age90 | all_channels_bad | 29 | 2m11.7s | 2m8.4s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | 300 | not detected | 0 | 0.020 | 2.91% | neutral |
| probe-020-m3-age90 | all_channels_bad | 47 | 2m7.1s | 2m3s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | 300 | not detected | 0 | 0.020 | 3.49% | neutral |
| probe-020-m3-age90 | all_channels_bad | 71 | 2m9.6s | 2m6.5s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.020 | 3.00% | neutral |
| probe-020-m3-age90 | all_channels_bad | 101 | 2m6.5s | 2m2.5s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.020 | 3.59% | neutral |
| probe-020-m3-age90 | all_channels_bad | 131 | 2m5.1s | 1m33.245s | 100ms | 85ms | 0.676 | 0.676 | 0s | 0 | 300 | 300 | not detected | 0 | 0.007 | 24.70% | win |
| probe-020-m3-age90 | all_channels_bad | 173 | 2m9.753516526s | 2m5.9s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.020 | 3.43% | neutral |
| probe-020-m3-age90 | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-m3-age90 | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-m3-age90 | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-m3-age90 | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-m3-age90 | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-m3-age90 | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-m3-age90 | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-m3-age90 | healthy_steady_state | 11 | 306.901147ms | 308.407929ms | 29.560991ms | 29.803017ms | 0.015 | 0.015 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -0.66% | neutral |
| probe-020-m3-age90 | healthy_steady_state | 29 | 306.449512ms | 308.553302ms | 29.524996ms | 29.698343ms | 0.015 | 0.017 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -1.26% | neutral |
| probe-020-m3-age90 | healthy_steady_state | 47 | 306.757116ms | 309.640816ms | 29.42026ms | 29.602958ms | 0.028 | 0.028 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -0.73% | neutral |
| probe-020-m3-age90 | healthy_steady_state | 71 | 304.971872ms | 308.600594ms | 29.463197ms | 29.657202ms | 0.018 | 0.018 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -0.81% | neutral |
| probe-020-m3-age90 | healthy_steady_state | 101 | 306.026121ms | 307.505642ms | 29.656679ms | 29.753082ms | 0.023 | 0.023 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -0.42% | neutral |
| probe-020-m3-age90 | healthy_steady_state | 131 | 307.461992ms | 309.233413ms | 29.32466ms | 29.584412ms | 0.022 | 0.022 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -0.71% | neutral |
| probe-020-m3-age90 | healthy_steady_state | 173 | 306.762211ms | 308.944167ms | 29.635213ms | 29.788126ms | 0.022 | 0.022 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -0.55% | neutral |
| probe-025-m3-age90 | gradual_degradation | 11 | 2m18.05810847s | 3.210360149s | 111.638008ms | 46.768613ms | 0.738 | 0.130 | 1s | 5 | 76 | 71 | 17.2s | 0 | 0.010 | 97.55% | win |
| probe-025-m3-age90 | gradual_degradation | 29 | 2m17.230127598s | 759.731479ms | 111.973508ms | 40.315928ms | 0.737 | 0.032 | 2.4s | 5 | 13 | 1 | 4.2s | 0 | 0.007 | 99.18% | win |
| probe-025-m3-age90 | gradual_degradation | 47 | 2m18.543146187s | 31.165180374s | 112.017461ms | 105.687832ms | 0.743 | 0.338 | 2.2s | 5 | 192 | 181 | 41s | 0 | 0.015 | 79.77% | win |
| probe-025-m3-age90 | gradual_degradation | 71 | 2m18.842157616s | 673.700086ms | 112.679273ms | 40.551492ms | 0.742 | 0.035 | 2.4s | 7 | 12 | 0 | 4s | 0 | 0.007 | 99.23% | win |
| probe-025-m3-age90 | gradual_degradation | 101 | 2m17.896601353s | 3.157771695s | 112.288179ms | 48.220673ms | 0.740 | 0.145 | 2.2s | 7 | 75 | 64 | 17s | 0 | 0.010 | 97.54% | win |
| probe-025-m3-age90 | gradual_degradation | 131 | 2m18.257709744s | 665.66754ms | 112.50077ms | 40.492487ms | 0.742 | 0.033 | 0s | 0 | 11 | 11 | 3.8s | 0 | 0.007 | 99.23% | win |
| probe-025-m3-age90 | gradual_degradation | 173 | 2m18.511286942s | 47.296086363s | 112.241073ms | 108.888477ms | 0.740 | 0.385 | 1.2s | 3 | 232 | 226 | 49.2s | 0 | 0.017 | 69.31% | win |
| probe-025-m3-age90 | sudden_outage | 11 | 15.6s | 478.077759ms | 29.758413ms | 40.050908ms | 0.669 | 0.040 | 800ms | 4 | 10 | 6 | 3.6s | 0 | 0.009 | 93.73% | win |
| probe-025-m3-age90 | sudden_outage | 29 | 15.6s | 477.005412ms | 29.586311ms | 39.871299ms | 0.667 | 0.040 | 2s | 5 | 10 | 0 | 3.6s | 0 | 0.009 | 93.74% | win |
| probe-025-m3-age90 | sudden_outage | 47 | 15.6s | 1.05460131s | 29.297237ms | 39.405096ms | 0.676 | 0.053 | 2.4s | 7 | 12 | 0 | 4s | 0 | 0.009 | 90.93% | win |
| probe-025-m3-age90 | sudden_outage | 71 | 15.6s | 478.860123ms | 29.362123ms | 40.024115ms | 0.673 | 0.047 | 2s | 4 | 10 | 0 | 3.6s | 0 | 0.009 | 93.64% | win |
| probe-025-m3-age90 | sudden_outage | 101 | 15.6s | 478.24933ms | 29.696734ms | 40.539913ms | 0.673 | 0.049 | 2s | 4 | 10 | 0 | 3.6s | 0 | 0.009 | 93.57% | win |
| probe-025-m3-age90 | sudden_outage | 131 | 15.6s | 477.864046ms | 29.458433ms | 40.270961ms | 0.676 | 0.038 | 0s | 0 | 10 | 10 | 3.6s | 0 | 0.009 | 93.76% | win |
| probe-025-m3-age90 | sudden_outage | 173 | 15.6s | 1.332870167s | 29.101676ms | 39.998469ms | 0.673 | 0.060 | 800ms | 2 | 10 | 6 | 3.6s | 0 | 0.009 | 89.69% | win |
| probe-025-m3-age90 | capacity_aggregation | 11 | 1m19.876239452s | 1m15.654996317s | 25ms | 25ms | 0.988 | 0.963 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 5.99% | neutral |
| probe-025-m3-age90 | capacity_aggregation | 29 | 1m19.971533361s | 1m15.62998706s | 25ms | 25ms | 0.988 | 0.963 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 6.09% | neutral |
| probe-025-m3-age90 | capacity_aggregation | 47 | 1m19.89581372s | 1m15.673178206s | 25ms | 25ms | 0.988 | 0.963 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 5.99% | neutral |
| probe-025-m3-age90 | capacity_aggregation | 71 | 1m19.767790605s | 1m15.452652771s | 25ms | 25ms | 0.988 | 0.963 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 6.08% | neutral |
| probe-025-m3-age90 | capacity_aggregation | 101 | 1m20.040921681s | 1m15.76206927s | 25ms | 25ms | 0.990 | 0.965 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 6.02% | neutral |
| probe-025-m3-age90 | capacity_aggregation | 131 | 1m19.734245977s | 1m15.397872531s | 25ms | 25ms | 0.988 | 0.963 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 6.08% | neutral |
| probe-025-m3-age90 | capacity_aggregation | 173 | 1m19.865475022s | 1m15.50360035s | 25ms | 25ms | 0.988 | 0.963 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 6.09% | neutral |
| probe-025-m3-age90 | transient_spike | 11 | 12.6925426s | 11.810116783s | 90ms | 90ms | 0.396 | 0.169 | 800ms | 4 | 189 | 185 | 16.8s | 1 | 0.013 | 15.43% | win |
| probe-025-m3-age90 | transient_spike | 29 | 12.307633594s | 1.468531557s | 90ms | 40.913302ms | 0.384 | 0.064 | 4.4s | 5 | 22 | 0 | 6s | 0 | 0.009 | 85.41% | win |
| probe-025-m3-age90 | transient_spike | 47 | 12.854235214s | 11.888008178s | 90ms | 90ms | 0.402 | 0.373 | 4.6s | 6 | 293 | 270 | not detected | 0 | 0.024 | 7.41% | neutral |
| probe-025-m3-age90 | transient_spike | 71 | 12.659328791s | 1.46716421s | 90ms | 40.987696ms | 0.400 | 0.073 | 4.4s | 4 | 22 | 0 | 6s | 0 | 0.009 | 85.66% | win |
| probe-025-m3-age90 | transient_spike | 101 | 12.607221347s | 11.841084557s | 90ms | 90ms | 0.398 | 0.173 | 4.2s | 4 | 188 | 167 | 16.8s | 1 | 0.013 | 14.60% | neutral |
| probe-025-m3-age90 | transient_spike | 131 | 12.512324827s | 1.452576305s | 90ms | 40.997411ms | 0.398 | 0.064 | 0s | 0 | 22 | 22 | 6s | 0 | 0.009 | 85.72% | win |
| probe-025-m3-age90 | transient_spike | 173 | 12.652228169s | 11.652576073s | 90ms | 90ms | 0.398 | 0.367 | 800ms | 2 | 293 | 289 | not detected | 0 | 0.024 | 7.81% | neutral |
| probe-025-m3-age90 | stale_candidate | 11 | 46.403333381s | 42.87333338s | 110ms | 110ms | 0.889 | 0.865 | 4.999999995s | 3 | 147 | 133 | not detected | 0 | 0.023 | 7.27% | neutral |
| probe-025-m3-age90 | stale_candidate | 29 | 52.933333379s | 49.540000046s | 110ms | 110ms | 0.883 | 0.865 | 4.999999995s | 3 | 147 | 133 | not detected | 0 | 0.023 | 7.00% | neutral |
| probe-025-m3-age90 | stale_candidate | 47 | 51.540000046s | 49.266666712s | 110ms | 110ms | 0.883 | 0.860 | 4.999999995s | 2 | 147 | 133 | not detected | 0 | 0.023 | 5.08% | neutral |
| probe-025-m3-age90 | stale_candidate | 71 | 52.600000048s | 25.300000024s | 110ms | 110ms | 0.889 | 0.503 | 0s | 0 | 81 | 81 | 30.666666636s | 0 | 0.018 | 55.30% | win |
| probe-025-m3-age90 | stale_candidate | 101 | 49.070000047s | 24.770000024s | 110ms | 110ms | 0.883 | 0.491 | 4.999999995s | 2 | 81 | 67 | 30.666666636s | 0 | 0.018 | 53.26% | win |
| probe-025-m3-age90 | stale_candidate | 131 | 47.40333338s | 36.22333337s | 110ms | 110ms | 0.883 | 0.713 | 4.999999995s | 3 | 120 | 106 | 43.999999956s | 0 | 0.023 | 26.66% | win |
| probe-025-m3-age90 | stale_candidate | 173 | 46.87333338s | 44.403333379s | 110ms | 110ms | 0.883 | 0.865 | 2.999999997s | 0 | 147 | 139 | not detected | 0 | 0.023 | 6.34% | neutral |
| probe-025-m3-age90 | recovery_no_flap | 11 | 44.799011002s | 14.125s | 95ms | 95ms | 0.752 | 0.140 | 800ms | 4 | 75 | 71 | 17s | 0 | 0.010 | 74.97% | win |
| probe-025-m3-age90 | recovery_no_flap | 29 | 44.937203373s | 1.377540368s | 95ms | 40.476027ms | 0.750 | 0.058 | 4.2s | 5 | 21 | 0 | 5.8s | 0 | 0.007 | 96.65% | win |
| probe-025-m3-age90 | recovery_no_flap | 47 | 44.810457119s | 43.545s | 95ms | 95ms | 0.757 | 0.592 | 4.2s | 5 | 350 | 329 | 1m12.8s | 0 | 0.022 | 9.06% | neutral |
| probe-025-m3-age90 | recovery_no_flap | 71 | 44.958361864s | 1.327112504s | 95ms | 40.829322ms | 0.755 | 0.058 | 4.2s | 4 | 21 | 0 | 5.8s | 0 | 0.007 | 96.72% | win |
| probe-025-m3-age90 | recovery_no_flap | 101 | 44.837077225s | 1.9s | 95ms | 95ms | 0.755 | 0.085 | 4.2s | 4 | 35 | 14 | 8.8s | 0 | 0.008 | 94.21% | win |
| probe-025-m3-age90 | recovery_no_flap | 131 | 44.901274352s | 1.014703488s | 95ms | 40.710898ms | 0.757 | 0.053 | 0s | 0 | 21 | 21 | 5.8s | 0 | 0.007 | 97.19% | win |
| probe-025-m3-age90 | recovery_no_flap | 173 | 44.9015634s | 43.475304915s | 95ms | 95ms | 0.755 | 0.737 | 800ms | 2 | 439 | 435 | not detected | 0 | 0.025 | 4.68% | neutral |
| probe-025-m3-age90 | all_channels_bad | 11 | 2m10.9s | 2m6.2s | 100ms | 100ms | 0.669 | 0.671 | 0s | 0 | 300 | 300 | not detected | 0 | 0.024 | 3.95% | neutral |
| probe-025-m3-age90 | all_channels_bad | 29 | 2m11.7s | 1m35.2s | 100ms | 85ms | 0.667 | 0.667 | 0s | 0 | 300 | 300 | not detected | 0 | 0.009 | 27.55% | win |
| probe-025-m3-age90 | all_channels_bad | 47 | 2m7.1s | 2m3.8s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | 300 | not detected | 0 | 0.024 | 2.96% | neutral |
| probe-025-m3-age90 | all_channels_bad | 71 | 2m9.6s | 1m33.89s | 100ms | 85ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.009 | 27.28% | win |
| probe-025-m3-age90 | all_channels_bad | 101 | 2m6.5s | 2m2.7s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.024 | 3.23% | neutral |
| probe-025-m3-age90 | all_channels_bad | 131 | 2m5.1s | 1m33.245s | 100ms | 85ms | 0.676 | 0.676 | 0s | 0 | 300 | 300 | not detected | 0 | 0.009 | 24.70% | win |
| probe-025-m3-age90 | all_channels_bad | 173 | 2m9.753516526s | 2m5.1s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.024 | 4.09% | neutral |
| probe-025-m3-age90 | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-m3-age90 | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-m3-age90 | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-m3-age90 | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-m3-age90 | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-m3-age90 | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-m3-age90 | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-m3-age90 | healthy_steady_state | 11 | 306.901147ms | 309.073169ms | 29.560991ms | 29.830249ms | 0.015 | 0.017 | not detected | 0 | 0 | 0 | not detected | 0 | 0.025 | -1.45% | neutral |
| probe-025-m3-age90 | healthy_steady_state | 29 | 306.449512ms | 308.918614ms | 29.524996ms | 29.783151ms | 0.015 | 0.015 | not detected | 0 | 0 | 0 | not detected | 0 | 0.025 | -0.80% | neutral |
| probe-025-m3-age90 | healthy_steady_state | 47 | 306.757116ms | 473.807885ms | 29.42026ms | 37.992525ms | 0.028 | 0.033 | not detected | 0 | 0 | 0 | not detected | 0 | 0.018 | -37.58% | regression |
| probe-025-m3-age90 | healthy_steady_state | 71 | 304.971872ms | 309.412318ms | 29.463197ms | 29.758366ms | 0.018 | 0.018 | not detected | 0 | 0 | 0 | not detected | 0 | 0.025 | -1.10% | neutral |
| probe-025-m3-age90 | healthy_steady_state | 101 | 306.026121ms | 308.709968ms | 29.656679ms | 29.803427ms | 0.023 | 0.025 | not detected | 0 | 0 | 0 | not detected | 0 | 0.025 | -1.23% | neutral |
| probe-025-m3-age90 | healthy_steady_state | 131 | 307.461992ms | 309.447485ms | 29.32466ms | 29.631848ms | 0.022 | 0.022 | not detected | 0 | 0 | 0 | not detected | 0 | 0.025 | -0.82% | neutral |
| probe-025-m3-age90 | healthy_steady_state | 173 | 306.762211ms | 309.118873ms | 29.635213ms | 29.825093ms | 0.022 | 0.022 | not detected | 0 | 0 | 0 | not detected | 0 | 0.025 | -0.70% | neutral |
| probe-015-m3-age60 | gradual_degradation | 11 | 2m18.05810847s | 30.047235366s | 111.638008ms | 103.391768ms | 0.738 | 0.313 | 0s | 0 | 188 | 188 | 39.6s | 0 | 0.008 | 80.50% | win |
| probe-015-m3-age60 | gradual_degradation | 29 | 2m17.230127598s | 3.992127673s | 111.973508ms | 46.818743ms | 0.737 | 0.157 | 2.4s | 5 | 90 | 78 | 20s | 0 | 0.007 | 97.03% | win |
| probe-015-m3-age60 | gradual_degradation | 47 | 2m18.543146187s | 56.529703482s | 112.017461ms | 108.429002ms | 0.743 | 0.435 | 2.2s | 5 | 253 | 242 | 53s | 0 | 0.010 | 63.10% | win |
| probe-015-m3-age60 | gradual_degradation | 71 | 2m18.842157616s | 30.363718599s | 112.679273ms | 105.324538ms | 0.742 | 0.325 | 2.4s | 7 | 188 | 176 | 39.8s | 0 | 0.008 | 80.41% | win |
| probe-015-m3-age60 | gradual_degradation | 101 | 2m17.896601353s | 2m15.028784667s | 112.288179ms | 112.288179ms | 0.740 | 0.732 | 2.2s | 7 | 443 | 432 | not detected | 0 | 0.015 | 2.51% | neutral |
| probe-015-m3-age60 | gradual_degradation | 131 | 2m18.257709744s | 980.556272ms | 112.50077ms | 40.752122ms | 0.742 | 0.050 | 0s | 0 | 24 | 24 | 6.6s | 0 | 0.005 | 99.03% | win |
| probe-015-m3-age60 | gradual_degradation | 173 | 2m18.511286942s | 58.111514028s | 112.241073ms | 109.63414ms | 0.740 | 0.425 | 1.2s | 3 | 255 | 249 | 53.2s | 0 | 0.010 | 62.15% | win |
| probe-015-m3-age60 | sudden_outage | 11 | 15.6s | 477.877788ms | 29.758413ms | 40.050908ms | 0.669 | 0.038 | 0s | 0 | 10 | 10 | 3.6s | 0 | 0.004 | 93.77% | win |
| probe-015-m3-age60 | sudden_outage | 29 | 15.6s | 477.005412ms | 29.586311ms | 39.871299ms | 0.667 | 0.040 | 2s | 5 | 10 | 0 | 3.6s | 0 | 0.004 | 93.74% | win |
| probe-015-m3-age60 | sudden_outage | 47 | 15.6s | 1.05460131s | 29.297237ms | 39.405096ms | 0.676 | 0.053 | 2.4s | 7 | 12 | 0 | 4s | 0 | 0.004 | 90.93% | win |
| probe-015-m3-age60 | sudden_outage | 71 | 15.6s | 478.860123ms | 29.362123ms | 39.983305ms | 0.673 | 0.047 | 2s | 4 | 10 | 0 | 3.6s | 0 | 0.004 | 93.64% | win |
| probe-015-m3-age60 | sudden_outage | 101 | 15.6s | 478.24933ms | 29.696734ms | 40.539913ms | 0.673 | 0.049 | 2s | 4 | 10 | 0 | 3.6s | 0 | 0.004 | 93.57% | win |
| probe-015-m3-age60 | sudden_outage | 131 | 15.6s | 477.864046ms | 29.458433ms | 40.270961ms | 0.676 | 0.038 | 0s | 0 | 10 | 10 | 3.6s | 0 | 0.004 | 93.76% | win |
| probe-015-m3-age60 | sudden_outage | 173 | 15.6s | 1.332870167s | 29.101676ms | 39.998469ms | 0.673 | 0.060 | 800ms | 2 | 10 | 6 | 3.6s | 0 | 0.004 | 89.69% | win |
| probe-015-m3-age60 | capacity_aggregation | 11 | 1m19.876239452s | 1m17.424416709s | 25ms | 25ms | 0.988 | 0.972 | 0s | 0 | 630 | 630 | not detected | 0 | 0.015 | 3.55% | neutral |
| probe-015-m3-age60 | capacity_aggregation | 29 | 1m19.971533361s | 1m17.403747547s | 25ms | 25ms | 0.988 | 0.972 | 0s | 0 | 630 | 630 | not detected | 0 | 0.015 | 3.65% | neutral |
| probe-015-m3-age60 | capacity_aggregation | 47 | 1m19.89581372s | 1m17.464650921s | 25ms | 25ms | 0.988 | 0.972 | 0s | 0 | 630 | 630 | not detected | 0 | 0.015 | 3.54% | neutral |
| probe-015-m3-age60 | capacity_aggregation | 71 | 1m19.767790605s | 1m17.268448036s | 25ms | 25ms | 0.988 | 0.972 | 0s | 0 | 630 | 630 | not detected | 0 | 0.015 | 3.60% | neutral |
| probe-015-m3-age60 | capacity_aggregation | 101 | 1m20.040921681s | 1m17.526615303s | 25ms | 25ms | 0.990 | 0.975 | 0s | 0 | 630 | 630 | not detected | 0 | 0.015 | 3.59% | neutral |
| probe-015-m3-age60 | capacity_aggregation | 131 | 1m19.734245977s | 1m17.170636114s | 25ms | 25ms | 0.988 | 0.972 | 0s | 0 | 630 | 630 | not detected | 0 | 0.015 | 3.67% | neutral |
| probe-015-m3-age60 | capacity_aggregation | 173 | 1m19.865475022s | 1m17.320114208s | 25ms | 25ms | 0.988 | 0.972 | 0s | 0 | 630 | 630 | not detected | 0 | 0.015 | 3.63% | neutral |
| probe-015-m3-age60 | transient_spike | 11 | 12.6925426s | 11.666680066s | 90ms | 90ms | 0.396 | 0.369 | 0s | 0 | 295 | 295 | not detected | 0 | 0.016 | 7.95% | neutral |
| probe-015-m3-age60 | transient_spike | 29 | 12.307633594s | 11.911495052s | 90ms | 90ms | 0.384 | 0.373 | 4.4s | 5 | 295 | 274 | not detected | 0 | 0.016 | 3.26% | neutral |
| probe-015-m3-age60 | transient_spike | 47 | 12.854235214s | 11.8145828s | 90ms | 90ms | 0.402 | 0.376 | 4.6s | 6 | 295 | 273 | not detected | 0 | 0.016 | 7.79% | neutral |
| probe-015-m3-age60 | transient_spike | 71 | 12.659328791s | 1.931534132s | 90ms | 90ms | 0.400 | 0.076 | 4.4s | 4 | 23 | 2 | 6.4s | 0 | 0.007 | 78.45% | win |
| probe-015-m3-age60 | transient_spike | 101 | 12.607221347s | 11.934856311s | 90ms | 90ms | 0.398 | 0.207 | 4.2s | 4 | 182 | 162 | 19.8s | 1 | 0.009 | 11.97% | neutral |
| probe-015-m3-age60 | transient_spike | 131 | 12.512324827s | 11.776701608s | 90ms | 90ms | 0.398 | 0.367 | 0s | 0 | 295 | 295 | not detected | 0 | 0.016 | 6.30% | neutral |
| probe-015-m3-age60 | transient_spike | 173 | 12.652228169s | 11.673458085s | 90ms | 90ms | 0.398 | 0.211 | 800ms | 2 | 181 | 177 | 19.8s | 1 | 0.009 | 13.69% | neutral |
| probe-015-m3-age60 | stale_candidate | 11 | 46.403333381s | 44.480000046s | 110ms | 110ms | 0.889 | 0.877 | 9.333333324s | 10 | 149 | 121 | not detected | 0 | 0.012 | 4.00% | neutral |
| probe-015-m3-age60 | stale_candidate | 29 | 52.933333379s | 51.070000047s | 110ms | 110ms | 0.883 | 0.871 | 9.333333324s | 8 | 149 | 121 | not detected | 0 | 0.012 | 3.88% | neutral |
| probe-015-m3-age60 | stale_candidate | 47 | 51.540000046s | 50.933333381s | 110ms | 110ms | 0.883 | 0.871 | 9.333333324s | 9 | 149 | 121 | not detected | 0 | 0.012 | 2.00% | neutral |
| probe-015-m3-age60 | stale_candidate | 71 | 52.600000048s | 50.600000046s | 110ms | 110ms | 0.889 | 0.877 | 9.666666657s | 10 | 149 | 120 | not detected | 0 | 0.012 | 3.72% | neutral |
| probe-015-m3-age60 | stale_candidate | 101 | 49.070000047s | 47.146666714s | 110ms | 110ms | 0.883 | 0.871 | 9.666666657s | 8 | 149 | 120 | not detected | 0 | 0.012 | 3.83% | neutral |
| probe-015-m3-age60 | stale_candidate | 131 | 47.40333338s | 46.34333338s | 110ms | 110ms | 0.883 | 0.871 | 9.333333324s | 9 | 149 | 121 | not detected | 0 | 0.012 | 2.36% | neutral |
| probe-015-m3-age60 | stale_candidate | 173 | 46.87333338s | 47.070000047s | 110ms | 110ms | 0.883 | 0.871 | 9.666666657s | 8 | 149 | 120 | not detected | 0 | 0.012 | 0.94% | neutral |
| probe-015-m3-age60 | recovery_no_flap | 11 | 44.799011002s | 1.358971675s | 95ms | 40.762979ms | 0.752 | 0.053 | 0s | 0 | 22 | 22 | 6.2s | 0 | 0.005 | 96.68% | win |
| probe-015-m3-age60 | recovery_no_flap | 29 | 44.937203373s | 1.418231073s | 95ms | 40.65075ms | 0.750 | 0.062 | 4.2s | 5 | 23 | 3 | 6.4s | 0 | 0.005 | 96.55% | win |
| probe-015-m3-age60 | recovery_no_flap | 47 | 44.810457119s | 43.745s | 95ms | 95ms | 0.757 | 0.745 | 4.2s | 5 | 443 | 423 | not detected | 0 | 0.015 | 3.19% | neutral |
| probe-015-m3-age60 | recovery_no_flap | 71 | 44.958361864s | 1.3590909s | 95ms | 40.851936ms | 0.755 | 0.065 | 4.2s | 4 | 22 | 2 | 6.2s | 0 | 0.005 | 96.63% | win |
| probe-015-m3-age60 | recovery_no_flap | 101 | 44.837077225s | 43.745s | 95ms | 95ms | 0.755 | 0.448 | 4.2s | 4 | 254 | 234 | 53s | 0 | 0.010 | 15.83% | win |
| probe-015-m3-age60 | recovery_no_flap | 131 | 44.901274352s | 43.745s | 95ms | 95ms | 0.757 | 0.547 | 0s | 0 | 320 | 320 | 1m6.2s | 0 | 0.012 | 10.63% | neutral |
| probe-015-m3-age60 | recovery_no_flap | 173 | 44.9015634s | 19.095s | 95ms | 95ms | 0.755 | 0.185 | 800ms | 2 | 89 | 85 | 19.8s | 0 | 0.007 | 66.98% | win |
| probe-015-m3-age60 | all_channels_bad | 11 | 2m10.9s | 1m47.7s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | 300 | not detected | 0 | 0.013 | 20.12% | win |
| probe-015-m3-age60 | all_channels_bad | 29 | 2m11.7s | 1m49.1s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | 300 | not detected | 0 | 0.013 | 18.92% | win |
| probe-015-m3-age60 | all_channels_bad | 47 | 2m7.1s | 1m45.1s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | 300 | not detected | 0 | 0.013 | 18.87% | win |
| probe-015-m3-age60 | all_channels_bad | 71 | 2m9.6s | 1m49.4s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.013 | 17.44% | win |
| probe-015-m3-age60 | all_channels_bad | 101 | 2m6.5s | 1m43.1s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.013 | 20.35% | win |
| probe-015-m3-age60 | all_channels_bad | 131 | 2m5.1s | 1m44.5s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | 300 | not detected | 0 | 0.013 | 18.85% | win |
| probe-015-m3-age60 | all_channels_bad | 173 | 2m9.753516526s | 1m47.6s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.013 | 19.04% | win |
| probe-015-m3-age60 | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-015-m3-age60 | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-015-m3-age60 | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-015-m3-age60 | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-015-m3-age60 | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-015-m3-age60 | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-015-m3-age60 | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-015-m3-age60 | healthy_steady_state | 11 | 306.901147ms | 307.699496ms | 29.560991ms | 29.775736ms | 0.015 | 0.015 | not detected | 0 | 0 | 0 | not detected | 0 | 0.015 | -0.52% | neutral |
| probe-015-m3-age60 | healthy_steady_state | 29 | 306.449512ms | 308.206465ms | 29.524996ms | 29.654816ms | 0.015 | 0.015 | not detected | 0 | 0 | 0 | not detected | 0 | 0.015 | -0.46% | neutral |
| probe-015-m3-age60 | healthy_steady_state | 47 | 306.757116ms | 309.133957ms | 29.42026ms | 29.519065ms | 0.028 | 0.028 | not detected | 0 | 0 | 0 | not detected | 0 | 0.015 | -0.46% | neutral |
| probe-015-m3-age60 | healthy_steady_state | 71 | 304.971872ms | 307.576474ms | 29.463197ms | 29.650924ms | 0.018 | 0.018 | not detected | 0 | 0 | 0 | not detected | 0 | 0.015 | -0.67% | neutral |
| probe-015-m3-age60 | healthy_steady_state | 101 | 306.026121ms | 306.947866ms | 29.656679ms | 29.751595ms | 0.023 | 0.025 | not detected | 0 | 0 | 0 | not detected | 0 | 0.015 | -0.99% | neutral |
| probe-015-m3-age60 | healthy_steady_state | 131 | 307.461992ms | 308.762844ms | 29.32466ms | 29.584412ms | 0.022 | 0.022 | not detected | 0 | 0 | 0 | not detected | 0 | 0.015 | -0.65% | neutral |
| probe-015-m3-age60 | healthy_steady_state | 173 | 306.762211ms | 308.445333ms | 29.635213ms | 29.753827ms | 0.022 | 0.023 | not detected | 0 | 0 | 0 | not detected | 0 | 0.015 | -1.07% | neutral |
| probe-020-m3-age60 | gradual_degradation | 11 | 2m18.05810847s | 1.172039208s | 111.638008ms | 40.790574ms | 0.738 | 0.057 | 0s | 0 | 33 | 33 | 8.4s | 0 | 0.007 | 98.91% | win |
| probe-020-m3-age60 | gradual_degradation | 29 | 2m17.230127598s | 1.345810991s | 111.973508ms | 40.974649ms | 0.737 | 0.067 | 2.4s | 5 | 34 | 22 | 8.6s | 0 | 0.007 | 98.79% | win |
| probe-020-m3-age60 | gradual_degradation | 47 | 2m18.543146187s | 27.062857436s | 112.017461ms | 103.100327ms | 0.743 | 0.317 | 2.2s | 5 | 179 | 168 | 38.2s | 0 | 0.012 | 82.46% | win |
| probe-020-m3-age60 | gradual_degradation | 71 | 2m18.842157616s | 1.338605405s | 112.679273ms | 40.964311ms | 0.742 | 0.072 | 2.4s | 7 | 34 | 22 | 8.6s | 0 | 0.007 | 98.80% | win |
| probe-020-m3-age60 | gradual_degradation | 101 | 2m17.896601353s | 7.740108219s | 112.288179ms | 91.816152ms | 0.740 | 0.238 | 2.2s | 7 | 131 | 120 | 28.4s | 0 | 0.010 | 94.10% | win |
| probe-020-m3-age60 | gradual_degradation | 131 | 2m18.257709744s | 665.66754ms | 112.50077ms | 40.492487ms | 0.742 | 0.033 | 0s | 0 | 11 | 11 | 3.8s | 0 | 0.005 | 99.23% | win |
| probe-020-m3-age60 | gradual_degradation | 173 | 2m18.511286942s | 1m7.321871106s | 112.241073ms | 110.111629ms | 0.740 | 0.460 | 1.2s | 3 | 278 | 272 | 58.4s | 0 | 0.015 | 55.68% | win |
| probe-020-m3-age60 | sudden_outage | 11 | 15.6s | 477.877788ms | 29.758413ms | 40.050908ms | 0.669 | 0.038 | 0s | 0 | 10 | 10 | 3.6s | 0 | 0.007 | 93.77% | win |
| probe-020-m3-age60 | sudden_outage | 29 | 15.6s | 477.005412ms | 29.586311ms | 39.871299ms | 0.667 | 0.040 | 2s | 5 | 10 | 0 | 3.6s | 0 | 0.007 | 93.74% | win |
| probe-020-m3-age60 | sudden_outage | 47 | 15.6s | 1.05460131s | 29.297237ms | 39.405096ms | 0.676 | 0.053 | 2.4s | 7 | 12 | 0 | 4s | 0 | 0.007 | 90.93% | win |
| probe-020-m3-age60 | sudden_outage | 71 | 15.6s | 478.860123ms | 29.362123ms | 39.983305ms | 0.673 | 0.047 | 2s | 4 | 10 | 0 | 3.6s | 0 | 0.007 | 93.64% | win |
| probe-020-m3-age60 | sudden_outage | 101 | 15.6s | 478.24933ms | 29.696734ms | 40.521742ms | 0.673 | 0.049 | 2s | 4 | 10 | 0 | 3.6s | 0 | 0.007 | 93.57% | win |
| probe-020-m3-age60 | sudden_outage | 131 | 15.6s | 477.864046ms | 29.458433ms | 40.270961ms | 0.676 | 0.038 | 0s | 0 | 10 | 10 | 3.6s | 0 | 0.007 | 93.76% | win |
| probe-020-m3-age60 | sudden_outage | 173 | 15.6s | 1.332870167s | 29.101676ms | 39.998469ms | 0.673 | 0.060 | 800ms | 2 | 10 | 6 | 3.6s | 0 | 0.007 | 89.69% | win |
| probe-020-m3-age60 | capacity_aggregation | 11 | 1m19.876239452s | 1m16.441213267s | 25ms | 25ms | 0.988 | 0.968 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.81% | neutral |
| probe-020-m3-age60 | capacity_aggregation | 29 | 1m19.971533361s | 1m16.483636642s | 25ms | 25ms | 0.988 | 0.968 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.84% | neutral |
| probe-020-m3-age60 | capacity_aggregation | 47 | 1m19.89581372s | 1m16.570113621s | 25ms | 25ms | 0.988 | 0.968 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.70% | neutral |
| probe-020-m3-age60 | capacity_aggregation | 71 | 1m19.767790605s | 1m16.420662792s | 25ms | 25ms | 0.988 | 0.968 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.77% | neutral |
| probe-020-m3-age60 | capacity_aggregation | 101 | 1m20.040921681s | 1m16.597949361s | 25ms | 25ms | 0.990 | 0.971 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.81% | neutral |
| probe-020-m3-age60 | capacity_aggregation | 131 | 1m19.734245977s | 1m16.213165556s | 25ms | 25ms | 0.988 | 0.968 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.88% | neutral |
| probe-020-m3-age60 | capacity_aggregation | 173 | 1m19.865475022s | 1m16.321864619s | 25ms | 25ms | 0.988 | 0.968 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.91% | neutral |
| probe-020-m3-age60 | transient_spike | 11 | 12.6925426s | 11.760379494s | 90ms | 90ms | 0.396 | 0.180 | 0s | 0 | 186 | 186 | 18s | 1 | 0.011 | 14.95% | neutral |
| probe-020-m3-age60 | transient_spike | 29 | 12.307633594s | 11.75348859s | 90ms | 90ms | 0.384 | 0.369 | 4.4s | 5 | 294 | 272 | not detected | 0 | 0.020 | 4.58% | neutral |
| probe-020-m3-age60 | transient_spike | 47 | 12.854235214s | 11.957757948s | 90ms | 90ms | 0.402 | 0.373 | 4.6s | 6 | 294 | 271 | not detected | 0 | 0.020 | 6.91% | neutral |
| probe-020-m3-age60 | transient_spike | 71 | 12.659328791s | 4.446377905s | 90ms | 90ms | 0.400 | 0.093 | 4.4s | 4 | 31 | 9 | 8s | 0 | 0.009 | 63.25% | win |
| probe-020-m3-age60 | transient_spike | 101 | 12.607221347s | 11.819956179s | 90ms | 90ms | 0.398 | 0.371 | 4.2s | 4 | 294 | 273 | not detected | 0 | 0.020 | 6.41% | neutral |
| probe-020-m3-age60 | transient_spike | 131 | 12.512324827s | 1.452576305s | 90ms | 40.997411ms | 0.398 | 0.064 | 0s | 0 | 22 | 22 | 6s | 0 | 0.007 | 85.72% | win |
| probe-020-m3-age60 | transient_spike | 173 | 12.652228169s | 4.578714635s | 90ms | 90ms | 0.398 | 0.091 | 800ms | 2 | 31 | 27 | 8s | 0 | 0.009 | 62.44% | win |
| probe-020-m3-age60 | stale_candidate | 11 | 46.403333381s | 39.813333374s | 110ms | 110ms | 0.889 | 0.749 | 9.333333324s | 10 | 127 | 100 | 45.999999954s | 0 | 0.018 | 17.46% | win |
| probe-020-m3-age60 | stale_candidate | 29 | 52.933333379s | 44.010000039s | 110ms | 110ms | 0.883 | 0.749 | 9.333333324s | 8 | 127 | 100 | 45.999999954s | 0 | 0.018 | 18.92% | win |
| probe-020-m3-age60 | stale_candidate | 47 | 51.540000046s | 49.93333338s | 110ms | 110ms | 0.883 | 0.871 | 9.333333324s | 9 | 148 | 121 | not detected | 0 | 0.018 | 3.81% | neutral |
| probe-020-m3-age60 | stale_candidate | 71 | 52.600000048s | 43.343333373s | 110ms | 110ms | 0.889 | 0.749 | 9.666666657s | 10 | 127 | 99 | 45.999999954s | 0 | 0.018 | 19.77% | win |
| probe-020-m3-age60 | stale_candidate | 101 | 49.070000047s | 40.813333373s | 110ms | 110ms | 0.883 | 0.749 | 9.666666657s | 8 | 127 | 99 | 45.999999954s | 0 | 0.018 | 19.08% | win |
| probe-020-m3-age60 | stale_candidate | 131 | 47.40333338s | 39.480000039s | 110ms | 110ms | 0.883 | 0.749 | 9.333333324s | 9 | 127 | 100 | 45.999999954s | 0 | 0.018 | 19.37% | win |
| probe-020-m3-age60 | stale_candidate | 173 | 46.87333338s | 38.813333373s | 110ms | 110ms | 0.883 | 0.743 | 9.666666657s | 8 | 127 | 99 | 45.999999954s | 0 | 0.018 | 20.30% | win |
| probe-020-m3-age60 | recovery_no_flap | 11 | 44.799011002s | 43.545s | 95ms | 95ms | 0.752 | 0.305 | 0s | 0 | 179 | 179 | 38s | 0 | 0.012 | 23.46% | win |
| probe-020-m3-age60 | recovery_no_flap | 29 | 44.937203373s | 43.545s | 95ms | 95ms | 0.750 | 0.737 | 4.2s | 5 | 441 | 420 | not detected | 0 | 0.020 | 4.26% | neutral |
| probe-020-m3-age60 | recovery_no_flap | 47 | 44.810457119s | 31.32s | 95ms | 95ms | 0.757 | 0.245 | 4.2s | 5 | 130 | 109 | 28.2s | 0 | 0.010 | 45.97% | win |
| probe-020-m3-age60 | recovery_no_flap | 71 | 44.958361864s | 43.545s | 95ms | 95ms | 0.755 | 0.475 | 4.2s | 4 | 277 | 256 | 58s | 0 | 0.015 | 14.49% | neutral |
| probe-020-m3-age60 | recovery_no_flap | 101 | 44.837077225s | 43.545s | 95ms | 95ms | 0.755 | 0.480 | 4.2s | 4 | 280 | 259 | 58.2s | 0 | 0.015 | 14.26% | neutral |
| probe-020-m3-age60 | recovery_no_flap | 131 | 44.901274352s | 1.014703488s | 95ms | 40.710898ms | 0.757 | 0.053 | 0s | 0 | 21 | 21 | 5.8s | 0 | 0.005 | 97.19% | win |
| probe-020-m3-age60 | recovery_no_flap | 173 | 44.9015634s | 1.9s | 95ms | 95ms | 0.755 | 0.082 | 800ms | 2 | 32 | 28 | 8.2s | 0 | 0.007 | 94.32% | win |
| probe-020-m3-age60 | all_channels_bad | 11 | 2m10.9s | 1m58.3s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | 300 | not detected | 0 | 0.020 | 10.84% | neutral |
| probe-020-m3-age60 | all_channels_bad | 29 | 2m11.7s | 1m59.7s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | 300 | not detected | 0 | 0.020 | 10.27% | neutral |
| probe-020-m3-age60 | all_channels_bad | 47 | 2m7.1s | 1m55.4s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | 300 | not detected | 0 | 0.020 | 9.82% | neutral |
| probe-020-m3-age60 | all_channels_bad | 71 | 2m9.6s | 1m58.3s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.020 | 9.86% | neutral |
| probe-020-m3-age60 | all_channels_bad | 101 | 2m6.5s | 1m53.6s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.020 | 11.15% | neutral |
| probe-020-m3-age60 | all_channels_bad | 131 | 2m5.1s | 1m30.27s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | 300 | not detected | 0 | 0.007 | 27.28% | win |
| probe-020-m3-age60 | all_channels_bad | 173 | 2m9.753516526s | 1m57.9s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.020 | 10.32% | neutral |
| probe-020-m3-age60 | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-m3-age60 | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-m3-age60 | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-m3-age60 | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-m3-age60 | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-m3-age60 | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-m3-age60 | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-m3-age60 | healthy_steady_state | 11 | 306.901147ms | 308.407929ms | 29.560991ms | 29.803017ms | 0.015 | 0.015 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -0.66% | neutral |
| probe-020-m3-age60 | healthy_steady_state | 29 | 306.449512ms | 308.553302ms | 29.524996ms | 29.698343ms | 0.015 | 0.017 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -1.26% | neutral |
| probe-020-m3-age60 | healthy_steady_state | 47 | 306.757116ms | 309.640816ms | 29.42026ms | 29.602958ms | 0.028 | 0.028 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -0.73% | neutral |
| probe-020-m3-age60 | healthy_steady_state | 71 | 304.971872ms | 308.600594ms | 29.463197ms | 29.657202ms | 0.018 | 0.018 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -0.81% | neutral |
| probe-020-m3-age60 | healthy_steady_state | 101 | 306.026121ms | 307.505642ms | 29.656679ms | 29.753082ms | 0.023 | 0.023 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -0.42% | neutral |
| probe-020-m3-age60 | healthy_steady_state | 131 | 307.461992ms | 309.233413ms | 29.32466ms | 29.584412ms | 0.022 | 0.022 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -0.71% | neutral |
| probe-020-m3-age60 | healthy_steady_state | 173 | 306.762211ms | 308.944167ms | 29.635213ms | 29.788126ms | 0.022 | 0.022 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -0.55% | neutral |
| probe-025-m3-age60 | gradual_degradation | 11 | 2m18.05810847s | 3.286403992s | 111.638008ms | 46.768613ms | 0.738 | 0.132 | 1s | 5 | 77 | 72 | 17.2s | 0 | 0.010 | 97.50% | win |
| probe-025-m3-age60 | gradual_degradation | 29 | 2m17.230127598s | 759.731479ms | 111.973508ms | 40.315928ms | 0.737 | 0.032 | 2.4s | 5 | 13 | 1 | 4.2s | 0 | 0.007 | 99.18% | win |
| probe-025-m3-age60 | gradual_degradation | 47 | 2m18.543146187s | 33.117084115s | 112.017461ms | 105.687832ms | 0.743 | 0.340 | 2.2s | 5 | 193 | 182 | 41.2s | 0 | 0.015 | 78.69% | win |
| probe-025-m3-age60 | gradual_degradation | 71 | 2m18.842157616s | 673.700086ms | 112.679273ms | 40.551492ms | 0.742 | 0.035 | 2.4s | 7 | 12 | 0 | 4s | 0 | 0.007 | 99.23% | win |
| probe-025-m3-age60 | gradual_degradation | 101 | 2m17.896601353s | 3.179427337s | 112.288179ms | 48.353343ms | 0.740 | 0.147 | 2.2s | 7 | 76 | 65 | 17.2s | 0 | 0.010 | 97.52% | win |
| probe-025-m3-age60 | gradual_degradation | 131 | 2m18.257709744s | 665.66754ms | 112.50077ms | 40.492487ms | 0.742 | 0.033 | 0s | 0 | 11 | 11 | 3.8s | 0 | 0.007 | 99.23% | win |
| probe-025-m3-age60 | gradual_degradation | 173 | 2m18.511286942s | 23.977848752s | 112.241073ms | 104.259241ms | 0.740 | 0.298 | 1.2s | 3 | 173 | 167 | 37s | 0 | 0.013 | 84.35% | win |
| probe-025-m3-age60 | sudden_outage | 11 | 15.6s | 478.077759ms | 29.758413ms | 40.050908ms | 0.669 | 0.040 | 800ms | 4 | 10 | 6 | 3.6s | 0 | 0.009 | 93.73% | win |
| probe-025-m3-age60 | sudden_outage | 29 | 15.6s | 477.005412ms | 29.586311ms | 39.871299ms | 0.667 | 0.040 | 2s | 5 | 10 | 0 | 3.6s | 0 | 0.009 | 93.74% | win |
| probe-025-m3-age60 | sudden_outage | 47 | 15.6s | 1.05460131s | 29.297237ms | 39.405096ms | 0.676 | 0.053 | 2.4s | 7 | 12 | 0 | 4s | 0 | 0.009 | 90.93% | win |
| probe-025-m3-age60 | sudden_outage | 71 | 15.6s | 478.860123ms | 29.362123ms | 40.024115ms | 0.673 | 0.047 | 2s | 4 | 10 | 0 | 3.6s | 0 | 0.009 | 93.64% | win |
| probe-025-m3-age60 | sudden_outage | 101 | 15.6s | 478.24933ms | 29.696734ms | 40.539913ms | 0.673 | 0.049 | 2s | 4 | 10 | 0 | 3.6s | 0 | 0.009 | 93.57% | win |
| probe-025-m3-age60 | sudden_outage | 131 | 15.6s | 477.864046ms | 29.458433ms | 40.270961ms | 0.676 | 0.038 | 0s | 0 | 10 | 10 | 3.6s | 0 | 0.009 | 93.76% | win |
| probe-025-m3-age60 | sudden_outage | 173 | 15.6s | 1.332870167s | 29.101676ms | 39.998469ms | 0.673 | 0.060 | 800ms | 2 | 10 | 6 | 3.6s | 0 | 0.009 | 89.69% | win |
| probe-025-m3-age60 | capacity_aggregation | 11 | 1m19.876239452s | 1m15.654996317s | 25ms | 25ms | 0.988 | 0.963 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 5.99% | neutral |
| probe-025-m3-age60 | capacity_aggregation | 29 | 1m19.971533361s | 1m15.62998706s | 25ms | 25ms | 0.988 | 0.963 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 6.09% | neutral |
| probe-025-m3-age60 | capacity_aggregation | 47 | 1m19.89581372s | 1m15.673178206s | 25ms | 25ms | 0.988 | 0.963 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 5.99% | neutral |
| probe-025-m3-age60 | capacity_aggregation | 71 | 1m19.767790605s | 1m15.452652771s | 25ms | 25ms | 0.988 | 0.963 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 6.08% | neutral |
| probe-025-m3-age60 | capacity_aggregation | 101 | 1m20.040921681s | 1m15.76206927s | 25ms | 25ms | 0.990 | 0.965 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 6.02% | neutral |
| probe-025-m3-age60 | capacity_aggregation | 131 | 1m19.734245977s | 1m15.397872531s | 25ms | 25ms | 0.988 | 0.963 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 6.08% | neutral |
| probe-025-m3-age60 | capacity_aggregation | 173 | 1m19.865475022s | 1m15.50360035s | 25ms | 25ms | 0.988 | 0.963 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 6.09% | neutral |
| probe-025-m3-age60 | transient_spike | 11 | 12.6925426s | 11.810116783s | 90ms | 90ms | 0.396 | 0.173 | 800ms | 4 | 161 | 157 | 17s | 1 | 0.013 | 15.26% | win |
| probe-025-m3-age60 | transient_spike | 29 | 12.307633594s | 1.468531557s | 90ms | 40.913302ms | 0.384 | 0.064 | 4.4s | 5 | 22 | 0 | 6s | 0 | 0.009 | 85.41% | win |
| probe-025-m3-age60 | transient_spike | 47 | 12.854235214s | 11.888008178s | 90ms | 90ms | 0.402 | 0.373 | 4.6s | 6 | 293 | 270 | not detected | 0 | 0.024 | 7.41% | neutral |
| probe-025-m3-age60 | transient_spike | 71 | 12.659328791s | 1.46716421s | 90ms | 40.987696ms | 0.400 | 0.073 | 4.4s | 4 | 22 | 0 | 6s | 0 | 0.009 | 85.66% | win |
| probe-025-m3-age60 | transient_spike | 101 | 12.607221347s | 11.841084557s | 90ms | 90ms | 0.398 | 0.176 | 4.2s | 4 | 156 | 135 | 17s | 1 | 0.013 | 14.46% | neutral |
| probe-025-m3-age60 | transient_spike | 131 | 12.512324827s | 1.452576305s | 90ms | 40.997411ms | 0.398 | 0.064 | 0s | 0 | 22 | 22 | 6s | 0 | 0.009 | 85.72% | win |
| probe-025-m3-age60 | transient_spike | 173 | 12.652228169s | 11.652576073s | 90ms | 90ms | 0.398 | 0.367 | 800ms | 2 | 293 | 289 | not detected | 0 | 0.024 | 7.81% | neutral |
| probe-025-m3-age60 | stale_candidate | 11 | 46.403333381s | 31.496666697s | 110ms | 110ms | 0.889 | 0.608 | 9.333333324s | 10 | 102 | 75 | 37.666666629s | 0 | 0.018 | 36.05% | win |
| probe-025-m3-age60 | stale_candidate | 29 | 52.933333379s | 34.223333364s | 110ms | 110ms | 0.883 | 0.608 | 9.333333324s | 8 | 102 | 75 | 37.666666629s | 0 | 0.018 | 39.33% | win |
| probe-025-m3-age60 | stale_candidate | 47 | 51.540000046s | 34.36000003s | 110ms | 110ms | 0.883 | 0.602 | 9.333333324s | 9 | 102 | 75 | 37.666666629s | 0 | 0.018 | 38.33% | win |
| probe-025-m3-age60 | stale_candidate | 71 | 52.600000048s | 35.420000032s | 110ms | 110ms | 0.889 | 0.602 | 9.666666657s | 10 | 102 | 74 | 37.666666629s | 0 | 0.018 | 36.71% | win |
| probe-025-m3-age60 | stale_candidate | 101 | 49.070000047s | 45.873333379s | 110ms | 110ms | 0.883 | 0.865 | 9.666666657s | 8 | 147 | 119 | not detected | 0 | 0.023 | 7.26% | neutral |
| probe-025-m3-age60 | stale_candidate | 131 | 47.40333338s | 30.496666698s | 110ms | 110ms | 0.883 | 0.602 | 9.333333324s | 9 | 102 | 75 | 37.666666629s | 0 | 0.018 | 39.47% | win |
| probe-025-m3-age60 | stale_candidate | 173 | 46.87333338s | 31.556666698s | 110ms | 110ms | 0.883 | 0.620 | 9.666666657s | 8 | 103 | 75 | 37.999999962s | 0 | 0.018 | 36.28% | win |
| probe-025-m3-age60 | recovery_no_flap | 11 | 44.799011002s | 14.125s | 95ms | 95ms | 0.752 | 0.142 | 800ms | 4 | 76 | 72 | 17.2s | 0 | 0.010 | 74.91% | win |
| probe-025-m3-age60 | recovery_no_flap | 29 | 44.937203373s | 1.377540368s | 95ms | 40.476027ms | 0.750 | 0.058 | 4.2s | 5 | 21 | 0 | 5.8s | 0 | 0.007 | 96.65% | win |
| probe-025-m3-age60 | recovery_no_flap | 47 | 44.810457119s | 43.545s | 95ms | 95ms | 0.757 | 0.463 | 4.2s | 5 | 272 | 251 | 57s | 0 | 0.018 | 14.81% | neutral |
| probe-025-m3-age60 | recovery_no_flap | 71 | 44.958361864s | 1.327112504s | 95ms | 40.829322ms | 0.755 | 0.058 | 4.2s | 4 | 21 | 0 | 5.8s | 0 | 0.007 | 96.72% | win |
| probe-025-m3-age60 | recovery_no_flap | 101 | 44.837077225s | 1.9s | 95ms | 95ms | 0.755 | 0.093 | 4.2s | 4 | 36 | 15 | 9s | 0 | 0.008 | 94.15% | win |
| probe-025-m3-age60 | recovery_no_flap | 131 | 44.901274352s | 1.014703488s | 95ms | 40.710898ms | 0.757 | 0.053 | 0s | 0 | 21 | 21 | 5.8s | 0 | 0.007 | 97.19% | win |
| probe-025-m3-age60 | recovery_no_flap | 173 | 44.9015634s | 43.475304915s | 95ms | 95ms | 0.755 | 0.470 | 800ms | 2 | 272 | 268 | 57s | 0 | 0.018 | 15.15% | win |
| probe-025-m3-age60 | all_channels_bad | 11 | 2m10.9s | 2m4.2s | 100ms | 100ms | 0.669 | 0.671 | 0s | 0 | 300 | 300 | not detected | 0 | 0.024 | 5.43% | neutral |
| probe-025-m3-age60 | all_channels_bad | 29 | 2m11.7s | 1m31.89s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | 300 | not detected | 0 | 0.009 | 30.19% | win |
| probe-025-m3-age60 | all_channels_bad | 47 | 2m7.1s | 2m1.6s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | 300 | not detected | 0 | 0.024 | 4.61% | neutral |
| probe-025-m3-age60 | all_channels_bad | 71 | 2m9.6s | 1m30.535s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.009 | 29.98% | win |
| probe-025-m3-age60 | all_channels_bad | 101 | 2m6.5s | 2m0.5s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.024 | 4.87% | neutral |
| probe-025-m3-age60 | all_channels_bad | 131 | 2m5.1s | 1m30.18s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | 300 | not detected | 0 | 0.009 | 27.35% | win |
| probe-025-m3-age60 | all_channels_bad | 173 | 2m9.753516526s | 2m2.7s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.024 | 5.98% | neutral |
| probe-025-m3-age60 | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-m3-age60 | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-m3-age60 | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-m3-age60 | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-m3-age60 | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-m3-age60 | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-m3-age60 | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-m3-age60 | healthy_steady_state | 11 | 306.901147ms | 309.073169ms | 29.560991ms | 29.830249ms | 0.015 | 0.017 | not detected | 0 | 0 | 0 | not detected | 0 | 0.025 | -1.45% | neutral |
| probe-025-m3-age60 | healthy_steady_state | 29 | 306.449512ms | 308.918614ms | 29.524996ms | 29.783151ms | 0.015 | 0.015 | not detected | 0 | 0 | 0 | not detected | 0 | 0.025 | -0.80% | neutral |
| probe-025-m3-age60 | healthy_steady_state | 47 | 306.757116ms | 473.807885ms | 29.42026ms | 37.818674ms | 0.028 | 0.033 | not detected | 0 | 0 | 0 | not detected | 0 | 0.018 | -37.25% | regression |
| probe-025-m3-age60 | healthy_steady_state | 71 | 304.971872ms | 309.412318ms | 29.463197ms | 29.758366ms | 0.018 | 0.018 | not detected | 0 | 0 | 0 | not detected | 0 | 0.025 | -1.10% | neutral |
| probe-025-m3-age60 | healthy_steady_state | 101 | 306.026121ms | 308.709968ms | 29.656679ms | 29.803427ms | 0.023 | 0.025 | not detected | 0 | 0 | 0 | not detected | 0 | 0.025 | -1.23% | neutral |
| probe-025-m3-age60 | healthy_steady_state | 131 | 307.461992ms | 309.447485ms | 29.32466ms | 29.631848ms | 0.022 | 0.022 | not detected | 0 | 0 | 0 | not detected | 0 | 0.025 | -0.82% | neutral |
| probe-025-m3-age60 | healthy_steady_state | 173 | 306.762211ms | 309.118873ms | 29.635213ms | 29.825093ms | 0.022 | 0.022 | not detected | 0 | 0 | 0 | not detected | 0 | 0.025 | -0.70% | neutral |
| probe-020-m4-age90 | gradual_degradation | 11 | 2m18.05810847s | 1.197988475s | 111.638008ms | 40.823911ms | 0.738 | 0.058 | 2.2s | 6 | 34 | 23 | 8.4s | 0 | 0.007 | 98.89% | win |
| probe-020-m4-age90 | gradual_degradation | 29 | 2m17.230127598s | 1.382763471s | 111.973508ms | 40.977053ms | 0.737 | 0.067 | 2.4s | 5 | 34 | 22 | 8.6s | 0 | 0.007 | 98.77% | win |
| probe-020-m4-age90 | gradual_degradation | 47 | 2m18.543146187s | 2m14.667839241s | 112.017461ms | 111.887792ms | 0.743 | 0.730 | 2.2s | 5 | 441 | 430 | not detected | 0 | 0.020 | 3.38% | neutral |
| probe-020-m4-age90 | gradual_degradation | 71 | 2m18.842157616s | 1.350051235s | 112.679273ms | 40.964311ms | 0.742 | 0.072 | 2.4s | 7 | 34 | 22 | 8.6s | 0 | 0.007 | 98.79% | win |
| probe-020-m4-age90 | gradual_degradation | 101 | 2m17.896601353s | 27.5836692s | 112.288179ms | 103.703763ms | 0.740 | 0.315 | 2.2s | 7 | 179 | 168 | 38.2s | 0 | 0.012 | 82.10% | win |
| probe-020-m4-age90 | gradual_degradation | 131 | 2m18.257709744s | 1.212779413s | 112.50077ms | 40.856401ms | 0.742 | 0.067 | 0s | 0 | 33 | 33 | 8.4s | 0 | 0.007 | 98.87% | win |
| probe-020-m4-age90 | gradual_degradation | 173 | 2m18.511286942s | 1m29.013531829s | 112.241073ms | 110.965067ms | 0.740 | 0.540 | 1.4s | 4 | 328 | 321 | 1m8.4s | 0 | 0.017 | 40.08% | win |
| probe-020-m4-age90 | sudden_outage | 11 | 15.6s | 477.877788ms | 29.758413ms | 40.050908ms | 0.669 | 0.038 | 2s | 5 | 10 | 0 | 3.6s | 0 | 0.007 | 93.77% | win |
| probe-020-m4-age90 | sudden_outage | 29 | 15.6s | 477.005412ms | 29.586311ms | 39.871299ms | 0.667 | 0.040 | 2s | 5 | 10 | 0 | 3.6s | 0 | 0.007 | 93.74% | win |
| probe-020-m4-age90 | sudden_outage | 47 | 15.6s | 1.05460131s | 29.297237ms | 39.405096ms | 0.676 | 0.053 | 2.4s | 7 | 12 | 0 | 4s | 0 | 0.007 | 90.93% | win |
| probe-020-m4-age90 | sudden_outage | 71 | 15.6s | 478.860123ms | 29.362123ms | 39.983305ms | 0.673 | 0.047 | 2s | 4 | 10 | 0 | 3.6s | 0 | 0.007 | 93.64% | win |
| probe-020-m4-age90 | sudden_outage | 101 | 15.6s | 478.24933ms | 29.696734ms | 40.521742ms | 0.673 | 0.049 | 2s | 4 | 10 | 0 | 3.6s | 0 | 0.007 | 93.57% | win |
| probe-020-m4-age90 | sudden_outage | 131 | 15.6s | 477.864046ms | 29.458433ms | 40.270961ms | 0.676 | 0.038 | 0s | 0 | 10 | 10 | 3.6s | 0 | 0.007 | 93.76% | win |
| probe-020-m4-age90 | sudden_outage | 173 | 15.6s | 1.332870167s | 29.101676ms | 39.998469ms | 0.673 | 0.060 | 1.2s | 3 | 10 | 4 | 3.6s | 0 | 0.007 | 89.69% | win |
| probe-020-m4-age90 | capacity_aggregation | 11 | 1m19.876239452s | 1m16.441213267s | 25ms | 25ms | 0.988 | 0.968 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.81% | neutral |
| probe-020-m4-age90 | capacity_aggregation | 29 | 1m19.971533361s | 1m16.483636642s | 25ms | 25ms | 0.988 | 0.968 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.84% | neutral |
| probe-020-m4-age90 | capacity_aggregation | 47 | 1m19.89581372s | 1m16.570113621s | 25ms | 25ms | 0.988 | 0.968 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.70% | neutral |
| probe-020-m4-age90 | capacity_aggregation | 71 | 1m19.767790605s | 1m16.420662792s | 25ms | 25ms | 0.988 | 0.968 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.77% | neutral |
| probe-020-m4-age90 | capacity_aggregation | 101 | 1m20.040921681s | 1m16.597949361s | 25ms | 25ms | 0.990 | 0.971 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.81% | neutral |
| probe-020-m4-age90 | capacity_aggregation | 131 | 1m19.734245977s | 1m16.213165556s | 25ms | 25ms | 0.988 | 0.968 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.88% | neutral |
| probe-020-m4-age90 | capacity_aggregation | 173 | 1m19.865475022s | 1m16.321864619s | 25ms | 25ms | 0.988 | 0.968 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.91% | neutral |
| probe-020-m4-age90 | transient_spike | 11 | 12.6925426s | 4.667483167s | 90ms | 90ms | 0.396 | 0.080 | 4.4s | 5 | 31 | 9 | 8s | 0 | 0.009 | 62.28% | win |
| probe-020-m4-age90 | transient_spike | 29 | 12.307633594s | 11.75348859s | 90ms | 90ms | 0.384 | 0.369 | 4.4s | 5 | 294 | 272 | not detected | 0 | 0.020 | 4.58% | neutral |
| probe-020-m4-age90 | transient_spike | 47 | 12.854235214s | 4.666308557s | 90ms | 90ms | 0.402 | 0.087 | 4.6s | 6 | 31 | 8 | 8s | 0 | 0.009 | 62.55% | win |
| probe-020-m4-age90 | transient_spike | 71 | 12.659328791s | 11.708751046s | 90ms | 90ms | 0.400 | 0.191 | 4.4s | 4 | 176 | 154 | 18s | 1 | 0.011 | 14.89% | neutral |
| probe-020-m4-age90 | transient_spike | 101 | 12.607221347s | 11.819956179s | 90ms | 90ms | 0.398 | 0.371 | 4.2s | 4 | 294 | 273 | not detected | 0 | 0.020 | 6.41% | neutral |
| probe-020-m4-age90 | transient_spike | 131 | 12.512324827s | 11.738229088s | 90ms | 90ms | 0.398 | 0.364 | 0s | 0 | 294 | 294 | not detected | 0 | 0.020 | 6.68% | neutral |
| probe-020-m4-age90 | transient_spike | 173 | 12.652228169s | 4.578714635s | 90ms | 90ms | 0.398 | 0.091 | 1.2s | 3 | 31 | 25 | 8s | 0 | 0.009 | 62.44% | win |
| probe-020-m4-age90 | stale_candidate | 11 | 46.403333381s | 44.34333338s | 110ms | 110ms | 0.889 | 0.877 | 4.666666662s | 2 | 148 | 135 | not detected | 0 | 0.018 | 4.11% | neutral |
| probe-020-m4-age90 | stale_candidate | 29 | 52.933333379s | 51.266666712s | 110ms | 110ms | 0.883 | 0.865 | 4.666666662s | 2 | 148 | 135 | not detected | 0 | 0.018 | 3.88% | neutral |
| probe-020-m4-age90 | stale_candidate | 47 | 51.540000046s | 50.266666713s | 110ms | 110ms | 0.883 | 0.865 | 4.666666662s | 2 | 148 | 135 | not detected | 0 | 0.018 | 2.78% | neutral |
| probe-020-m4-age90 | stale_candidate | 71 | 52.600000048s | 50.600000046s | 110ms | 110ms | 0.889 | 0.871 | 0s | 0 | 148 | 148 | not detected | 0 | 0.018 | 4.02% | neutral |
| probe-020-m4-age90 | stale_candidate | 101 | 49.070000047s | 47.070000047s | 110ms | 110ms | 0.883 | 0.865 | 4.666666662s | 1 | 148 | 135 | not detected | 0 | 0.018 | 4.52% | neutral |
| probe-020-m4-age90 | stale_candidate | 131 | 47.40333338s | 46.736666712s | 110ms | 110ms | 0.883 | 0.865 | 4.666666662s | 1 | 148 | 135 | not detected | 0 | 0.018 | 1.74% | neutral |
| probe-020-m4-age90 | stale_candidate | 173 | 46.87333338s | 46.343333378s | 110ms | 110ms | 0.883 | 0.865 | 2.999999997s | 0 | 148 | 140 | not detected | 0 | 0.018 | 1.94% | neutral |
| probe-020-m4-age90 | recovery_no_flap | 11 | 44.799011002s | 31.32s | 95ms | 95ms | 0.752 | 0.232 | 4.2s | 5 | 131 | 110 | 28s | 0 | 0.010 | 45.97% | win |
| probe-020-m4-age90 | recovery_no_flap | 29 | 44.937203373s | 43.545s | 95ms | 95ms | 0.750 | 0.737 | 4.2s | 5 | 441 | 420 | not detected | 0 | 0.020 | 4.26% | neutral |
| probe-020-m4-age90 | recovery_no_flap | 47 | 44.810457119s | 31.32s | 95ms | 95ms | 0.757 | 0.245 | 4.2s | 5 | 130 | 109 | 28.2s | 0 | 0.010 | 45.96% | win |
| probe-020-m4-age90 | recovery_no_flap | 71 | 44.958361864s | 43.545s | 95ms | 95ms | 0.755 | 0.742 | 4.2s | 4 | 441 | 420 | not detected | 0 | 0.020 | 4.25% | neutral |
| probe-020-m4-age90 | recovery_no_flap | 101 | 44.837077225s | 43.545s | 95ms | 95ms | 0.755 | 0.318 | 4.2s | 4 | 180 | 159 | 38s | 0 | 0.012 | 23.54% | win |
| probe-020-m4-age90 | recovery_no_flap | 131 | 44.901274352s | 1.9s | 95ms | 95ms | 0.757 | 0.080 | 0s | 0 | 32 | 32 | 8.2s | 0 | 0.007 | 94.32% | win |
| probe-020-m4-age90 | recovery_no_flap | 173 | 44.9015634s | 1.9s | 95ms | 95ms | 0.755 | 0.082 | 1.2s | 3 | 32 | 26 | 8.2s | 0 | 0.007 | 94.32% | win |
| probe-020-m4-age90 | all_channels_bad | 11 | 2m10.9s | 2m7.4s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | 300 | not detected | 0 | 0.020 | 2.92% | neutral |
| probe-020-m4-age90 | all_channels_bad | 29 | 2m11.7s | 2m8.4s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | 300 | not detected | 0 | 0.020 | 2.91% | neutral |
| probe-020-m4-age90 | all_channels_bad | 47 | 2m7.1s | 2m3s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | 300 | not detected | 0 | 0.020 | 3.49% | neutral |
| probe-020-m4-age90 | all_channels_bad | 71 | 2m9.6s | 2m6.5s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.020 | 3.00% | neutral |
| probe-020-m4-age90 | all_channels_bad | 101 | 2m6.5s | 2m2.5s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.020 | 3.59% | neutral |
| probe-020-m4-age90 | all_channels_bad | 131 | 2m5.1s | 2m1.7s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | 300 | not detected | 0 | 0.020 | 3.26% | neutral |
| probe-020-m4-age90 | all_channels_bad | 173 | 2m9.753516526s | 2m5.9s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.020 | 3.43% | neutral |
| probe-020-m4-age90 | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-m4-age90 | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-m4-age90 | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-m4-age90 | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-m4-age90 | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-m4-age90 | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-m4-age90 | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-m4-age90 | healthy_steady_state | 11 | 306.901147ms | 308.407929ms | 29.560991ms | 29.803017ms | 0.015 | 0.015 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -0.66% | neutral |
| probe-020-m4-age90 | healthy_steady_state | 29 | 306.449512ms | 308.553302ms | 29.524996ms | 29.698343ms | 0.015 | 0.017 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -1.26% | neutral |
| probe-020-m4-age90 | healthy_steady_state | 47 | 306.757116ms | 309.640816ms | 29.42026ms | 29.602958ms | 0.028 | 0.028 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -0.73% | neutral |
| probe-020-m4-age90 | healthy_steady_state | 71 | 304.971872ms | 308.600594ms | 29.463197ms | 29.657202ms | 0.018 | 0.018 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -0.81% | neutral |
| probe-020-m4-age90 | healthy_steady_state | 101 | 306.026121ms | 307.505642ms | 29.656679ms | 29.753082ms | 0.023 | 0.023 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -0.42% | neutral |
| probe-020-m4-age90 | healthy_steady_state | 131 | 307.461992ms | 309.233413ms | 29.32466ms | 29.584412ms | 0.022 | 0.022 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -0.71% | neutral |
| probe-020-m4-age90 | healthy_steady_state | 173 | 306.762211ms | 308.944167ms | 29.635213ms | 29.788126ms | 0.022 | 0.022 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -0.55% | neutral |
| probe-025-m4-age90 | gradual_degradation | 11 | 2m18.05810847s | 33.224525445s | 111.638008ms | 104.259099ms | 0.738 | 0.325 | 2.2s | 6 | 195 | 184 | 41.4s | 0 | 0.015 | 78.53% | win |
| probe-025-m4-age90 | gradual_degradation | 29 | 2m17.230127598s | 759.731479ms | 111.973508ms | 40.302296ms | 0.737 | 0.032 | 2.4s | 5 | 14 | 2 | 4.2s | 0 | 0.007 | 99.18% | win |
| probe-025-m4-age90 | gradual_degradation | 47 | 2m18.543146187s | 861.279896ms | 112.017461ms | 40.269915ms | 0.743 | 0.045 | 2.2s | 5 | 13 | 2 | 4s | 0 | 0.007 | 99.11% | win |
| probe-025-m4-age90 | gradual_degradation | 71 | 2m18.842157616s | 676.163776ms | 112.679273ms | 40.566479ms | 0.742 | 0.035 | 0s | 0 | 13 | 13 | 4s | 0 | 0.007 | 99.22% | win |
| probe-025-m4-age90 | gradual_degradation | 101 | 2m17.896601353s | 3.179427337s | 112.288179ms | 48.220673ms | 0.740 | 0.147 | 2.2s | 7 | 76 | 65 | 17.2s | 0 | 0.010 | 97.52% | win |
| probe-025-m4-age90 | gradual_degradation | 131 | 2m18.257709744s | 678.23531ms | 112.50077ms | 40.537649ms | 0.742 | 0.035 | 0s | 0 | 14 | 14 | 4.2s | 0 | 0.007 | 99.22% | win |
| probe-025-m4-age90 | gradual_degradation | 173 | 2m18.511286942s | 47.406683658s | 112.241073ms | 108.888477ms | 0.740 | 0.387 | 1.4s | 4 | 233 | 226 | 49.4s | 0 | 0.017 | 69.20% | win |
| probe-025-m4-age90 | sudden_outage | 11 | 15.6s | 478.077759ms | 29.758413ms | 40.050908ms | 0.669 | 0.040 | 2s | 5 | 10 | 0 | 3.6s | 0 | 0.009 | 93.73% | win |
| probe-025-m4-age90 | sudden_outage | 29 | 15.6s | 477.005412ms | 29.586311ms | 39.871299ms | 0.667 | 0.040 | 2s | 5 | 10 | 0 | 3.6s | 0 | 0.009 | 93.74% | win |
| probe-025-m4-age90 | sudden_outage | 47 | 15.6s | 1.05460131s | 29.297237ms | 39.405096ms | 0.676 | 0.053 | 2.4s | 7 | 12 | 0 | 4s | 0 | 0.009 | 90.93% | win |
| probe-025-m4-age90 | sudden_outage | 71 | 15.6s | 478.860123ms | 29.362123ms | 40.024115ms | 0.673 | 0.047 | 2s | 4 | 10 | 0 | 3.6s | 0 | 0.009 | 93.64% | win |
| probe-025-m4-age90 | sudden_outage | 101 | 15.6s | 478.24933ms | 29.696734ms | 40.539913ms | 0.673 | 0.049 | 2s | 4 | 10 | 0 | 3.6s | 0 | 0.009 | 93.57% | win |
| probe-025-m4-age90 | sudden_outage | 131 | 15.6s | 477.864046ms | 29.458433ms | 40.270961ms | 0.676 | 0.038 | 0s | 0 | 10 | 10 | 3.6s | 0 | 0.009 | 93.76% | win |
| probe-025-m4-age90 | sudden_outage | 173 | 15.6s | 1.332870167s | 29.101676ms | 39.998469ms | 0.673 | 0.060 | 1.2s | 3 | 10 | 4 | 3.6s | 0 | 0.009 | 89.69% | win |
| probe-025-m4-age90 | capacity_aggregation | 11 | 1m19.876239452s | 1m15.654996317s | 25ms | 25ms | 0.988 | 0.963 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 5.99% | neutral |
| probe-025-m4-age90 | capacity_aggregation | 29 | 1m19.971533361s | 1m15.62998706s | 25ms | 25ms | 0.988 | 0.963 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 6.09% | neutral |
| probe-025-m4-age90 | capacity_aggregation | 47 | 1m19.89581372s | 1m15.673178206s | 25ms | 25ms | 0.988 | 0.963 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 5.99% | neutral |
| probe-025-m4-age90 | capacity_aggregation | 71 | 1m19.767790605s | 1m15.452652771s | 25ms | 25ms | 0.988 | 0.963 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 6.08% | neutral |
| probe-025-m4-age90 | capacity_aggregation | 101 | 1m20.040921681s | 1m15.76206927s | 25ms | 25ms | 0.990 | 0.965 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 6.02% | neutral |
| probe-025-m4-age90 | capacity_aggregation | 131 | 1m19.734245977s | 1m15.397872531s | 25ms | 25ms | 0.988 | 0.963 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 6.08% | neutral |
| probe-025-m4-age90 | capacity_aggregation | 173 | 1m19.865475022s | 1m15.50360035s | 25ms | 25ms | 0.988 | 0.963 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 6.09% | neutral |
| probe-025-m4-age90 | transient_spike | 11 | 12.6925426s | 11.810116783s | 90ms | 90ms | 0.396 | 0.173 | 4.4s | 5 | 149 | 127 | 17s | 1 | 0.013 | 15.25% | win |
| probe-025-m4-age90 | transient_spike | 29 | 12.307633594s | 1.468531557s | 90ms | 40.913302ms | 0.384 | 0.067 | 4.4s | 5 | 23 | 1 | 6s | 0 | 0.009 | 85.37% | win |
| probe-025-m4-age90 | transient_spike | 47 | 12.854235214s | 1.963686801s | 90ms | 90ms | 0.402 | 0.071 | 4.6s | 6 | 24 | 1 | 6.2s | 0 | 0.009 | 78.56% | win |
| probe-025-m4-age90 | transient_spike | 71 | 12.659328791s | 1.46716421s | 90ms | 40.987696ms | 0.400 | 0.076 | 4.4s | 4 | 23 | 1 | 6s | 0 | 0.009 | 85.62% | win |
| probe-025-m4-age90 | transient_spike | 101 | 12.607221347s | 11.841084557s | 90ms | 90ms | 0.398 | 0.176 | 4.2s | 4 | 139 | 118 | 17s | 1 | 0.013 | 14.46% | neutral |
| probe-025-m4-age90 | transient_spike | 131 | 12.512324827s | 1.904461265s | 90ms | 40.997411ms | 0.398 | 0.067 | 0s | 0 | 23 | 23 | 6s | 0 | 0.009 | 83.12% | win |
| probe-025-m4-age90 | transient_spike | 173 | 12.652228169s | 11.652576073s | 90ms | 90ms | 0.398 | 0.187 | 1.2s | 3 | 146 | 140 | 17s | 1 | 0.013 | 15.53% | win |
| probe-025-m4-age90 | stale_candidate | 11 | 46.403333381s | 42.87333338s | 110ms | 110ms | 0.889 | 0.865 | 4.999999995s | 3 | 147 | 133 | not detected | 0 | 0.023 | 7.27% | neutral |
| probe-025-m4-age90 | stale_candidate | 29 | 52.933333379s | 49.540000046s | 110ms | 110ms | 0.883 | 0.865 | 4.999999995s | 3 | 147 | 133 | not detected | 0 | 0.023 | 7.00% | neutral |
| probe-025-m4-age90 | stale_candidate | 47 | 51.540000046s | 49.266666712s | 110ms | 110ms | 0.883 | 0.860 | 4.999999995s | 2 | 147 | 133 | not detected | 0 | 0.023 | 5.08% | neutral |
| probe-025-m4-age90 | stale_candidate | 71 | 52.600000048s | 40.480000038s | 110ms | 110ms | 0.889 | 0.725 | 0s | 0 | 121 | 121 | 44.333333289s | 0 | 0.023 | 25.20% | win |
| probe-025-m4-age90 | stale_candidate | 101 | 49.070000047s | 46.146666713s | 110ms | 110ms | 0.883 | 0.860 | 4.999999995s | 2 | 147 | 133 | not detected | 0 | 0.023 | 6.77% | neutral |
| probe-025-m4-age90 | stale_candidate | 131 | 47.40333338s | 44.813333379s | 110ms | 110ms | 0.883 | 0.860 | 4.999999995s | 3 | 147 | 133 | not detected | 0 | 0.023 | 6.14% | neutral |
| probe-025-m4-age90 | stale_candidate | 173 | 46.87333338s | 44.403333379s | 110ms | 110ms | 0.883 | 0.865 | 2.999999997s | 0 | 147 | 139 | not detected | 0 | 0.023 | 6.34% | neutral |
| probe-025-m4-age90 | recovery_no_flap | 11 | 44.799011002s | 14.125s | 95ms | 95ms | 0.752 | 0.142 | 4.2s | 5 | 76 | 55 | 17.2s | 0 | 0.010 | 74.91% | win |
| probe-025-m4-age90 | recovery_no_flap | 29 | 44.937203373s | 1.3901921s | 95ms | 40.635981ms | 0.750 | 0.060 | 4.2s | 5 | 22 | 1 | 5.8s | 0 | 0.007 | 96.61% | win |
| probe-025-m4-age90 | recovery_no_flap | 47 | 44.810457119s | 1.438387173s | 95ms | 40.825291ms | 0.757 | 0.072 | 4.2s | 5 | 23 | 2 | 5.8s | 0 | 0.007 | 96.48% | win |
| probe-025-m4-age90 | recovery_no_flap | 71 | 44.958361864s | 1.3590909s | 95ms | 40.858579ms | 0.755 | 0.065 | 4.2s | 4 | 22 | 1 | 5.8s | 0 | 0.007 | 96.63% | win |
| probe-025-m4-age90 | recovery_no_flap | 101 | 44.837077225s | 14.125s | 95ms | 95ms | 0.755 | 0.150 | 4.2s | 4 | 75 | 54 | 17s | 0 | 0.010 | 74.97% | win |
| probe-025-m4-age90 | recovery_no_flap | 131 | 44.901274352s | 1.377752802s | 95ms | 40.842365ms | 0.757 | 0.063 | 0s | 0 | 22 | 22 | 5.8s | 0 | 0.007 | 96.61% | win |
| probe-025-m4-age90 | recovery_no_flap | 173 | 44.9015634s | 43.475304915s | 95ms | 95ms | 0.755 | 0.737 | 1.2s | 3 | 439 | 433 | not detected | 0 | 0.025 | 4.68% | neutral |
| probe-025-m4-age90 | all_channels_bad | 11 | 2m10.9s | 2m6.2s | 100ms | 100ms | 0.669 | 0.671 | 0s | 0 | 300 | 300 | not detected | 0 | 0.024 | 3.95% | neutral |
| probe-025-m4-age90 | all_channels_bad | 29 | 2m11.7s | 1m28.335s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | 300 | not detected | 0 | 0.009 | 32.80% | win |
| probe-025-m4-age90 | all_channels_bad | 47 | 2m7.1s | 1m27.38s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | 300 | not detected | 0 | 0.009 | 30.79% | win |
| probe-025-m4-age90 | all_channels_bad | 71 | 2m9.6s | 1m27.49s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.009 | 32.48% | win |
| probe-025-m4-age90 | all_channels_bad | 101 | 2m6.5s | 2m2.7s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.024 | 3.23% | neutral |
| probe-025-m4-age90 | all_channels_bad | 131 | 2m5.1s | 1m26.98s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | 300 | not detected | 0 | 0.009 | 29.92% | win |
| probe-025-m4-age90 | all_channels_bad | 173 | 2m9.753516526s | 2m5.1s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.024 | 4.09% | neutral |
| probe-025-m4-age90 | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-m4-age90 | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-m4-age90 | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-m4-age90 | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-m4-age90 | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-m4-age90 | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-m4-age90 | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-m4-age90 | healthy_steady_state | 11 | 306.901147ms | 309.073169ms | 29.560991ms | 29.830249ms | 0.015 | 0.017 | not detected | 0 | 0 | 0 | not detected | 0 | 0.025 | -1.45% | neutral |
| probe-025-m4-age90 | healthy_steady_state | 29 | 306.449512ms | 308.918614ms | 29.524996ms | 29.783151ms | 0.015 | 0.015 | not detected | 0 | 0 | 0 | not detected | 0 | 0.025 | -0.80% | neutral |
| probe-025-m4-age90 | healthy_steady_state | 47 | 306.757116ms | 330.006512ms | 29.42026ms | 29.634088ms | 0.028 | 0.028 | not detected | 0 | 0 | 0 | not detected | 0 | 0.025 | -3.11% | neutral |
| probe-025-m4-age90 | healthy_steady_state | 71 | 304.971872ms | 309.412318ms | 29.463197ms | 29.758366ms | 0.018 | 0.018 | not detected | 0 | 0 | 0 | not detected | 0 | 0.025 | -1.10% | neutral |
| probe-025-m4-age90 | healthy_steady_state | 101 | 306.026121ms | 308.709968ms | 29.656679ms | 29.803427ms | 0.023 | 0.025 | not detected | 0 | 0 | 0 | not detected | 0 | 0.025 | -1.23% | neutral |
| probe-025-m4-age90 | healthy_steady_state | 131 | 307.461992ms | 309.447485ms | 29.32466ms | 29.631848ms | 0.022 | 0.022 | not detected | 0 | 0 | 0 | not detected | 0 | 0.025 | -0.82% | neutral |
| probe-025-m4-age90 | healthy_steady_state | 173 | 306.762211ms | 309.118873ms | 29.635213ms | 29.825093ms | 0.022 | 0.022 | not detected | 0 | 0 | 0 | not detected | 0 | 0.025 | -0.70% | neutral |
| probe-025-critical-fast | gradual_degradation | 11 | 2m18.05810847s | 3.210360149s | 111.638008ms | 46.768613ms | 0.738 | 0.130 | 1s | 5 | 76 | 71 | 17.2s | 0 | 0.010 | 97.55% | win |
| probe-025-critical-fast | gradual_degradation | 29 | 2m17.230127598s | 759.731479ms | 111.973508ms | 40.315928ms | 0.737 | 0.032 | 2.4s | 5 | 13 | 1 | 4.2s | 0 | 0.007 | 99.18% | win |
| probe-025-critical-fast | gradual_degradation | 47 | 2m18.543146187s | 31.165180374s | 112.017461ms | 105.687832ms | 0.743 | 0.338 | 2.2s | 5 | 192 | 181 | 41s | 0 | 0.015 | 79.77% | win |
| probe-025-critical-fast | gradual_degradation | 71 | 2m18.842157616s | 673.700086ms | 112.679273ms | 40.551492ms | 0.742 | 0.035 | 2.4s | 7 | 12 | 0 | 4s | 0 | 0.007 | 99.23% | win |
| probe-025-critical-fast | gradual_degradation | 101 | 2m17.896601353s | 3.157771695s | 112.288179ms | 48.220673ms | 0.740 | 0.145 | 2.2s | 7 | 75 | 64 | 17s | 0 | 0.010 | 97.54% | win |
| probe-025-critical-fast | gradual_degradation | 131 | 2m18.257709744s | 665.66754ms | 112.50077ms | 40.492487ms | 0.742 | 0.033 | 0s | 0 | 11 | 11 | 3.8s | 0 | 0.007 | 99.23% | win |
| probe-025-critical-fast | gradual_degradation | 173 | 2m18.511286942s | 23.977848752s | 112.241073ms | 104.259241ms | 0.740 | 0.298 | 1.2s | 3 | 173 | 167 | 37s | 0 | 0.013 | 84.35% | win |
| probe-025-critical-fast | sudden_outage | 11 | 15.6s | 478.077759ms | 29.758413ms | 40.050908ms | 0.669 | 0.040 | 800ms | 4 | 10 | 6 | 3.6s | 0 | 0.009 | 93.73% | win |
| probe-025-critical-fast | sudden_outage | 29 | 15.6s | 477.005412ms | 29.586311ms | 39.871299ms | 0.667 | 0.040 | 2s | 5 | 10 | 0 | 3.6s | 0 | 0.009 | 93.74% | win |
| probe-025-critical-fast | sudden_outage | 47 | 15.6s | 1.05460131s | 29.297237ms | 39.405096ms | 0.676 | 0.053 | 2.4s | 7 | 12 | 0 | 4s | 0 | 0.009 | 90.93% | win |
| probe-025-critical-fast | sudden_outage | 71 | 15.6s | 478.860123ms | 29.362123ms | 40.024115ms | 0.673 | 0.047 | 2s | 4 | 10 | 0 | 3.6s | 0 | 0.009 | 93.64% | win |
| probe-025-critical-fast | sudden_outage | 101 | 15.6s | 478.24933ms | 29.696734ms | 40.539913ms | 0.673 | 0.049 | 2s | 4 | 10 | 0 | 3.6s | 0 | 0.009 | 93.57% | win |
| probe-025-critical-fast | sudden_outage | 131 | 15.6s | 477.864046ms | 29.458433ms | 40.270961ms | 0.676 | 0.038 | 0s | 0 | 10 | 10 | 3.6s | 0 | 0.009 | 93.76% | win |
| probe-025-critical-fast | sudden_outage | 173 | 15.6s | 1.332870167s | 29.101676ms | 39.998469ms | 0.673 | 0.060 | 800ms | 2 | 10 | 6 | 3.6s | 0 | 0.009 | 89.69% | win |
| probe-025-critical-fast | capacity_aggregation | 11 | 1m19.876239452s | 1m15.654996317s | 25ms | 25ms | 0.988 | 0.963 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 5.99% | neutral |
| probe-025-critical-fast | capacity_aggregation | 29 | 1m19.971533361s | 1m15.62998706s | 25ms | 25ms | 0.988 | 0.963 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 6.09% | neutral |
| probe-025-critical-fast | capacity_aggregation | 47 | 1m19.89581372s | 1m15.673178206s | 25ms | 25ms | 0.988 | 0.963 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 5.99% | neutral |
| probe-025-critical-fast | capacity_aggregation | 71 | 1m19.767790605s | 1m15.452652771s | 25ms | 25ms | 0.988 | 0.963 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 6.08% | neutral |
| probe-025-critical-fast | capacity_aggregation | 101 | 1m20.040921681s | 1m15.76206927s | 25ms | 25ms | 0.990 | 0.965 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 6.02% | neutral |
| probe-025-critical-fast | capacity_aggregation | 131 | 1m19.734245977s | 1m15.397872531s | 25ms | 25ms | 0.988 | 0.963 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 6.08% | neutral |
| probe-025-critical-fast | capacity_aggregation | 173 | 1m19.865475022s | 1m15.50360035s | 25ms | 25ms | 0.988 | 0.963 | 0s | 0 | 624 | 624 | not detected | 0 | 0.025 | 6.09% | neutral |
| probe-025-critical-fast | transient_spike | 11 | 12.6925426s | 11.810116783s | 90ms | 90ms | 0.396 | 0.169 | 800ms | 4 | 189 | 185 | 16.8s | 1 | 0.013 | 15.43% | win |
| probe-025-critical-fast | transient_spike | 29 | 12.307633594s | 1.468531557s | 90ms | 40.913302ms | 0.384 | 0.064 | 4.4s | 5 | 22 | 0 | 6s | 0 | 0.009 | 85.41% | win |
| probe-025-critical-fast | transient_spike | 47 | 12.854235214s | 11.888008178s | 90ms | 90ms | 0.402 | 0.373 | 4.6s | 6 | 293 | 270 | not detected | 0 | 0.024 | 7.41% | neutral |
| probe-025-critical-fast | transient_spike | 71 | 12.659328791s | 1.46716421s | 90ms | 40.987696ms | 0.400 | 0.073 | 4.4s | 4 | 22 | 0 | 6s | 0 | 0.009 | 85.66% | win |
| probe-025-critical-fast | transient_spike | 101 | 12.607221347s | 11.841084557s | 90ms | 90ms | 0.398 | 0.173 | 4.2s | 4 | 185 | 164 | 16.8s | 1 | 0.013 | 14.60% | neutral |
| probe-025-critical-fast | transient_spike | 131 | 12.512324827s | 1.452576305s | 90ms | 40.997411ms | 0.398 | 0.064 | 0s | 0 | 22 | 22 | 6s | 0 | 0.009 | 85.72% | win |
| probe-025-critical-fast | transient_spike | 173 | 12.652228169s | 11.652576073s | 90ms | 90ms | 0.398 | 0.367 | 800ms | 2 | 293 | 289 | not detected | 0 | 0.024 | 7.81% | neutral |
| probe-025-critical-fast | stale_candidate | 11 | 46.403333381s | 31.496666697s | 110ms | 110ms | 0.889 | 0.602 | 9.333333324s | 10 | 101 | 74 | 37.333333296s | 0 | 0.018 | 36.35% | win |
| probe-025-critical-fast | stale_candidate | 29 | 52.933333379s | 33.890000031s | 110ms | 110ms | 0.883 | 0.596 | 9.333333324s | 8 | 101 | 74 | 37.333333296s | 0 | 0.018 | 40.06% | win |
| probe-025-critical-fast | stale_candidate | 47 | 51.540000046s | 34.223333364s | 110ms | 110ms | 0.883 | 0.596 | 9.333333324s | 9 | 101 | 74 | 37.333333296s | 0 | 0.018 | 38.79% | win |
| probe-025-critical-fast | stale_candidate | 71 | 52.600000048s | 35.223333363s | 110ms | 110ms | 0.889 | 0.608 | 9.666666657s | 10 | 101 | 73 | 37.333333296s | 0 | 0.018 | 36.99% | win |
| probe-025-critical-fast | stale_candidate | 101 | 49.070000047s | 45.873333379s | 110ms | 110ms | 0.883 | 0.865 | 9.666666657s | 8 | 147 | 119 | not detected | 0 | 0.023 | 7.26% | neutral |
| probe-025-critical-fast | stale_candidate | 131 | 47.40333338s | 29.966666696s | 110ms | 110ms | 0.883 | 0.596 | 9.333333324s | 9 | 101 | 74 | 37.333333296s | 0 | 0.018 | 40.49% | win |
| probe-025-critical-fast | stale_candidate | 173 | 46.87333338s | 31.496666697s | 110ms | 110ms | 0.883 | 0.608 | 9.666666657s | 8 | 102 | 74 | 37.666666629s | 0 | 0.018 | 36.70% | win |
| probe-025-critical-fast | recovery_no_flap | 11 | 44.799011002s | 14.125s | 95ms | 95ms | 0.752 | 0.140 | 800ms | 4 | 75 | 71 | 17s | 0 | 0.010 | 74.97% | win |
| probe-025-critical-fast | recovery_no_flap | 29 | 44.937203373s | 1.377540368s | 95ms | 40.476027ms | 0.750 | 0.058 | 4.2s | 5 | 21 | 0 | 5.8s | 0 | 0.007 | 96.65% | win |
| probe-025-critical-fast | recovery_no_flap | 47 | 44.810457119s | 43.545s | 95ms | 95ms | 0.757 | 0.463 | 4.2s | 5 | 272 | 251 | 56.8s | 0 | 0.018 | 14.87% | neutral |
| probe-025-critical-fast | recovery_no_flap | 71 | 44.958361864s | 1.327112504s | 95ms | 40.829322ms | 0.755 | 0.058 | 4.2s | 4 | 21 | 0 | 5.8s | 0 | 0.007 | 96.72% | win |
| probe-025-critical-fast | recovery_no_flap | 101 | 44.837077225s | 1.9s | 95ms | 95ms | 0.755 | 0.085 | 4.2s | 4 | 35 | 14 | 8.8s | 0 | 0.008 | 94.21% | win |
| probe-025-critical-fast | recovery_no_flap | 131 | 44.901274352s | 1.014703488s | 95ms | 40.710898ms | 0.757 | 0.053 | 0s | 0 | 21 | 21 | 5.8s | 0 | 0.007 | 97.19% | win |
| probe-025-critical-fast | recovery_no_flap | 173 | 44.9015634s | 43.475304915s | 95ms | 95ms | 0.755 | 0.470 | 800ms | 2 | 272 | 268 | 56.8s | 0 | 0.018 | 15.21% | win |
| probe-025-critical-fast | all_channels_bad | 11 | 2m10.9s | 2m4.2s | 100ms | 100ms | 0.669 | 0.671 | 0s | 0 | 300 | 300 | not detected | 0 | 0.024 | 5.43% | neutral |
| probe-025-critical-fast | all_channels_bad | 29 | 2m11.7s | 1m35.2s | 100ms | 85ms | 0.667 | 0.667 | 0s | 0 | 300 | 300 | not detected | 0 | 0.009 | 27.55% | win |
| probe-025-critical-fast | all_channels_bad | 47 | 2m7.1s | 2m1.6s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | 300 | not detected | 0 | 0.024 | 4.61% | neutral |
| probe-025-critical-fast | all_channels_bad | 71 | 2m9.6s | 1m33.89s | 100ms | 85ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.009 | 27.28% | win |
| probe-025-critical-fast | all_channels_bad | 101 | 2m6.5s | 2m0.5s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.024 | 4.87% | neutral |
| probe-025-critical-fast | all_channels_bad | 131 | 2m5.1s | 1m33.245s | 100ms | 85ms | 0.676 | 0.676 | 0s | 0 | 300 | 300 | not detected | 0 | 0.009 | 24.70% | win |
| probe-025-critical-fast | all_channels_bad | 173 | 2m9.753516526s | 2m2.7s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.024 | 5.98% | neutral |
| probe-025-critical-fast | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-critical-fast | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-critical-fast | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-critical-fast | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-critical-fast | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-critical-fast | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-critical-fast | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-025-critical-fast | healthy_steady_state | 11 | 306.901147ms | 309.073169ms | 29.560991ms | 29.830249ms | 0.015 | 0.017 | not detected | 0 | 0 | 0 | not detected | 0 | 0.025 | -1.45% | neutral |
| probe-025-critical-fast | healthy_steady_state | 29 | 306.449512ms | 308.918614ms | 29.524996ms | 29.783151ms | 0.015 | 0.015 | not detected | 0 | 0 | 0 | not detected | 0 | 0.025 | -0.80% | neutral |
| probe-025-critical-fast | healthy_steady_state | 47 | 306.757116ms | 473.807885ms | 29.42026ms | 37.992525ms | 0.028 | 0.033 | not detected | 0 | 0 | 0 | not detected | 0 | 0.018 | -37.58% | regression |
| probe-025-critical-fast | healthy_steady_state | 71 | 304.971872ms | 309.412318ms | 29.463197ms | 29.758366ms | 0.018 | 0.018 | not detected | 0 | 0 | 0 | not detected | 0 | 0.025 | -1.10% | neutral |
| probe-025-critical-fast | healthy_steady_state | 101 | 306.026121ms | 308.709968ms | 29.656679ms | 29.803427ms | 0.023 | 0.025 | not detected | 0 | 0 | 0 | not detected | 0 | 0.025 | -1.23% | neutral |
| probe-025-critical-fast | healthy_steady_state | 131 | 307.461992ms | 309.447485ms | 29.32466ms | 29.631848ms | 0.022 | 0.022 | not detected | 0 | 0 | 0 | not detected | 0 | 0.025 | -0.82% | neutral |
| probe-025-critical-fast | healthy_steady_state | 173 | 306.762211ms | 309.118873ms | 29.635213ms | 29.825093ms | 0.022 | 0.022 | not detected | 0 | 0 | 0 | not detected | 0 | 0.025 | -0.70% | neutral |
| probe-020-slow-recovery | gradual_degradation | 11 | 2m18.05810847s | 1.172039208s | 111.638008ms | 40.790574ms | 0.738 | 0.057 | 0s | 0 | 33 | 33 | 8.4s | 0 | 0.007 | 98.91% | win |
| probe-020-slow-recovery | gradual_degradation | 29 | 2m17.230127598s | 1.345810991s | 111.973508ms | 40.974649ms | 0.737 | 0.067 | 2.4s | 5 | 34 | 22 | 8.6s | 0 | 0.007 | 98.79% | win |
| probe-020-slow-recovery | gradual_degradation | 47 | 2m18.543146187s | 1m6.716505117s | 112.017461ms | 109.072554ms | 0.743 | 0.475 | 2.2s | 5 | 277 | 266 | 58.2s | 0 | 0.015 | 56.07% | win |
| probe-020-slow-recovery | gradual_degradation | 71 | 2m18.842157616s | 1.338605405s | 112.679273ms | 40.964311ms | 0.742 | 0.072 | 2.4s | 7 | 34 | 22 | 8.6s | 0 | 0.007 | 98.80% | win |
| probe-020-slow-recovery | gradual_degradation | 101 | 2m17.896601353s | 7.740108219s | 112.288179ms | 91.816152ms | 0.740 | 0.238 | 2.2s | 7 | 131 | 120 | 28.4s | 0 | 0.010 | 94.10% | win |
| probe-020-slow-recovery | gradual_degradation | 131 | 2m18.257709744s | 665.66754ms | 112.50077ms | 40.492487ms | 0.742 | 0.033 | 0s | 0 | 11 | 11 | 3.8s | 0 | 0.005 | 99.23% | win |
| probe-020-slow-recovery | gradual_degradation | 173 | 2m18.511286942s | 2m14.168782883s | 112.241073ms | 112.196973ms | 0.740 | 0.725 | 1.2s | 3 | 441 | 435 | not detected | 0 | 0.020 | 3.60% | neutral |
| probe-020-slow-recovery | sudden_outage | 11 | 15.6s | 477.877788ms | 29.758413ms | 40.050908ms | 0.669 | 0.038 | 0s | 0 | 10 | 10 | 3.6s | 0 | 0.007 | 93.77% | win |
| probe-020-slow-recovery | sudden_outage | 29 | 15.6s | 477.005412ms | 29.586311ms | 39.871299ms | 0.667 | 0.040 | 2s | 5 | 10 | 0 | 3.6s | 0 | 0.007 | 93.74% | win |
| probe-020-slow-recovery | sudden_outage | 47 | 15.6s | 1.05460131s | 29.297237ms | 39.405096ms | 0.676 | 0.053 | 2.4s | 7 | 12 | 0 | 4s | 0 | 0.007 | 90.93% | win |
| probe-020-slow-recovery | sudden_outage | 71 | 15.6s | 478.860123ms | 29.362123ms | 39.983305ms | 0.673 | 0.047 | 2s | 4 | 10 | 0 | 3.6s | 0 | 0.007 | 93.64% | win |
| probe-020-slow-recovery | sudden_outage | 101 | 15.6s | 478.24933ms | 29.696734ms | 40.521742ms | 0.673 | 0.049 | 2s | 4 | 10 | 0 | 3.6s | 0 | 0.007 | 93.57% | win |
| probe-020-slow-recovery | sudden_outage | 131 | 15.6s | 477.864046ms | 29.458433ms | 40.270961ms | 0.676 | 0.038 | 0s | 0 | 10 | 10 | 3.6s | 0 | 0.007 | 93.76% | win |
| probe-020-slow-recovery | sudden_outage | 173 | 15.6s | 1.332870167s | 29.101676ms | 39.998469ms | 0.673 | 0.060 | 800ms | 2 | 10 | 6 | 3.6s | 0 | 0.007 | 89.69% | win |
| probe-020-slow-recovery | capacity_aggregation | 11 | 1m19.876239452s | 1m16.441213267s | 25ms | 25ms | 0.988 | 0.968 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.81% | neutral |
| probe-020-slow-recovery | capacity_aggregation | 29 | 1m19.971533361s | 1m16.483636642s | 25ms | 25ms | 0.988 | 0.968 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.84% | neutral |
| probe-020-slow-recovery | capacity_aggregation | 47 | 1m19.89581372s | 1m16.570113621s | 25ms | 25ms | 0.988 | 0.968 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.70% | neutral |
| probe-020-slow-recovery | capacity_aggregation | 71 | 1m19.767790605s | 1m16.420662792s | 25ms | 25ms | 0.988 | 0.968 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.77% | neutral |
| probe-020-slow-recovery | capacity_aggregation | 101 | 1m20.040921681s | 1m16.597949361s | 25ms | 25ms | 0.990 | 0.971 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.81% | neutral |
| probe-020-slow-recovery | capacity_aggregation | 131 | 1m19.734245977s | 1m16.213165556s | 25ms | 25ms | 0.988 | 0.968 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.88% | neutral |
| probe-020-slow-recovery | capacity_aggregation | 173 | 1m19.865475022s | 1m16.321864619s | 25ms | 25ms | 0.988 | 0.968 | 0s | 0 | 628 | 628 | not detected | 0 | 0.019 | 4.91% | neutral |
| probe-020-slow-recovery | transient_spike | 11 | 12.6925426s | 11.760379494s | 90ms | 90ms | 0.396 | 0.184 | 0s | 0 | 144 | 144 | 18s | 1 | 0.011 | 14.86% | neutral |
| probe-020-slow-recovery | transient_spike | 29 | 12.307633594s | 11.75348859s | 90ms | 90ms | 0.384 | 0.369 | 4.4s | 5 | 294 | 272 | not detected | 0 | 0.020 | 4.58% | neutral |
| probe-020-slow-recovery | transient_spike | 47 | 12.854235214s | 11.957757948s | 90ms | 90ms | 0.402 | 0.373 | 4.6s | 6 | 294 | 271 | not detected | 0 | 0.020 | 6.91% | neutral |
| probe-020-slow-recovery | transient_spike | 71 | 12.659328791s | 4.446377905s | 90ms | 90ms | 0.400 | 0.093 | 4.4s | 4 | 31 | 9 | 8s | 0 | 0.009 | 63.25% | win |
| probe-020-slow-recovery | transient_spike | 101 | 12.607221347s | 11.819956179s | 90ms | 90ms | 0.398 | 0.371 | 4.2s | 4 | 294 | 273 | not detected | 0 | 0.020 | 6.41% | neutral |
| probe-020-slow-recovery | transient_spike | 131 | 12.512324827s | 1.452576305s | 90ms | 40.997411ms | 0.398 | 0.064 | 0s | 0 | 22 | 22 | 6s | 0 | 0.007 | 85.72% | win |
| probe-020-slow-recovery | transient_spike | 173 | 12.652228169s | 4.578714635s | 90ms | 90ms | 0.398 | 0.091 | 800ms | 2 | 31 | 27 | 8s | 0 | 0.009 | 62.44% | win |
| probe-020-slow-recovery | stale_candidate | 11 | 46.403333381s | 44.34333338s | 110ms | 110ms | 0.889 | 0.877 | 4.666666662s | 2 | 148 | 135 | not detected | 0 | 0.018 | 4.11% | neutral |
| probe-020-slow-recovery | stale_candidate | 29 | 52.933333379s | 37.223333365s | 110ms | 110ms | 0.883 | 0.643 | 4.666666662s | 2 | 108 | 95 | 39.666666627s | 0 | 0.018 | 33.71% | win |
| probe-020-slow-recovery | stale_candidate | 47 | 51.540000046s | 50.266666713s | 110ms | 110ms | 0.883 | 0.865 | 4.666666662s | 2 | 148 | 135 | not detected | 0 | 0.018 | 2.78% | neutral |
| probe-020-slow-recovery | stale_candidate | 71 | 52.600000048s | 35.086666699s | 110ms | 110ms | 0.889 | 0.632 | 0s | 0 | 107 | 107 | 39.333333294s | 0 | 0.018 | 36.07% | win |
| probe-020-slow-recovery | stale_candidate | 101 | 49.070000047s | 34.693333365s | 110ms | 110ms | 0.883 | 0.643 | 4.666666662s | 1 | 107 | 94 | 39.333333294s | 0 | 0.018 | 33.50% | win |
| probe-020-slow-recovery | stale_candidate | 131 | 47.40333338s | 46.736666712s | 110ms | 110ms | 0.883 | 0.865 | 4.666666662s | 1 | 148 | 135 | not detected | 0 | 0.018 | 1.74% | neutral |
| probe-020-slow-recovery | stale_candidate | 173 | 46.87333338s | 46.343333378s | 110ms | 110ms | 0.883 | 0.865 | 2.999999997s | 0 | 148 | 140 | not detected | 0 | 0.018 | 1.94% | neutral |
| probe-020-slow-recovery | recovery_no_flap | 11 | 44.799011002s | 43.545s | 95ms | 95ms | 0.752 | 0.305 | 0s | 0 | 178 | 178 | 38s | 0 | 0.012 | 23.46% | win |
| probe-020-slow-recovery | recovery_no_flap | 29 | 44.937203373s | 43.545s | 95ms | 95ms | 0.750 | 0.645 | 4.2s | 5 | 383 | 362 | 1m19.4s | 0 | 0.018 | 7.12% | neutral |
| probe-020-slow-recovery | recovery_no_flap | 47 | 44.810457119s | 31.32s | 95ms | 95ms | 0.757 | 0.245 | 4.2s | 5 | 130 | 109 | 28.2s | 0 | 0.010 | 45.97% | win |
| probe-020-slow-recovery | recovery_no_flap | 71 | 44.958361864s | 43.545s | 95ms | 95ms | 0.755 | 0.742 | 4.2s | 4 | 441 | 420 | not detected | 0 | 0.020 | 4.25% | neutral |
| probe-020-slow-recovery | recovery_no_flap | 101 | 44.837077225s | 43.545s | 95ms | 95ms | 0.755 | 0.742 | 4.2s | 4 | 441 | 420 | not detected | 0 | 0.020 | 4.12% | neutral |
| probe-020-slow-recovery | recovery_no_flap | 131 | 44.901274352s | 1.014703488s | 95ms | 40.710898ms | 0.757 | 0.053 | 0s | 0 | 21 | 21 | 5.8s | 0 | 0.005 | 97.19% | win |
| probe-020-slow-recovery | recovery_no_flap | 173 | 44.9015634s | 1.9s | 95ms | 95ms | 0.755 | 0.082 | 800ms | 2 | 32 | 28 | 8.2s | 0 | 0.007 | 94.32% | win |
| probe-020-slow-recovery | all_channels_bad | 11 | 2m10.9s | 2m7.4s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | 300 | not detected | 0 | 0.020 | 2.92% | neutral |
| probe-020-slow-recovery | all_channels_bad | 29 | 2m11.7s | 2m8.4s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | 300 | not detected | 0 | 0.020 | 2.91% | neutral |
| probe-020-slow-recovery | all_channels_bad | 47 | 2m7.1s | 2m3s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | 300 | not detected | 0 | 0.020 | 3.49% | neutral |
| probe-020-slow-recovery | all_channels_bad | 71 | 2m9.6s | 2m6.5s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.020 | 3.00% | neutral |
| probe-020-slow-recovery | all_channels_bad | 101 | 2m6.5s | 2m2.5s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.020 | 3.59% | neutral |
| probe-020-slow-recovery | all_channels_bad | 131 | 2m5.1s | 1m30.27s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | 300 | not detected | 0 | 0.007 | 27.28% | win |
| probe-020-slow-recovery | all_channels_bad | 173 | 2m9.753516526s | 2m5.9s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.020 | 3.43% | neutral |
| probe-020-slow-recovery | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-slow-recovery | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-slow-recovery | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-slow-recovery | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-slow-recovery | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-slow-recovery | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-slow-recovery | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-020-slow-recovery | healthy_steady_state | 11 | 306.901147ms | 308.407929ms | 29.560991ms | 29.803017ms | 0.015 | 0.015 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -0.66% | neutral |
| probe-020-slow-recovery | healthy_steady_state | 29 | 306.449512ms | 308.553302ms | 29.524996ms | 29.698343ms | 0.015 | 0.017 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -1.26% | neutral |
| probe-020-slow-recovery | healthy_steady_state | 47 | 306.757116ms | 309.640816ms | 29.42026ms | 29.602958ms | 0.028 | 0.028 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -0.73% | neutral |
| probe-020-slow-recovery | healthy_steady_state | 71 | 304.971872ms | 308.600594ms | 29.463197ms | 29.657202ms | 0.018 | 0.018 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -0.81% | neutral |
| probe-020-slow-recovery | healthy_steady_state | 101 | 306.026121ms | 307.505642ms | 29.656679ms | 29.753082ms | 0.023 | 0.023 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -0.42% | neutral |
| probe-020-slow-recovery | healthy_steady_state | 131 | 307.461992ms | 309.233413ms | 29.32466ms | 29.584412ms | 0.022 | 0.022 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -0.71% | neutral |
| probe-020-slow-recovery | healthy_steady_state | 173 | 306.762211ms | 308.944167ms | 29.635213ms | 29.788126ms | 0.022 | 0.022 | not detected | 0 | 0 | 0 | not detected | 0 | 0.020 | -0.55% | neutral |
| probe-015-hard-confirmed | gradual_degradation | 11 | 2m18.05810847s | 2m15.063822362s | 111.638008ms | 111.638008ms | 0.738 | 0.727 | 0s | 0 | 443 | 443 | not detected | 0 | 0.015 | 2.59% | neutral |
| probe-015-hard-confirmed | gradual_degradation | 29 | 2m17.230127598s | 3.992127673s | 111.973508ms | 47.134381ms | 0.737 | 0.157 | 2.4s | 5 | 90 | 78 | 19.8s | 0 | 0.007 | 97.03% | win |
| probe-015-hard-confirmed | gradual_degradation | 47 | 2m18.543146187s | 1m30.466737215s | 112.017461ms | 110.791939ms | 0.743 | 0.563 | 2.2s | 5 | 335 | 324 | 1m9.6s | 0 | 0.012 | 38.68% | win |
| probe-015-hard-confirmed | gradual_degradation | 71 | 2m18.842157616s | 43.801565167s | 112.679273ms | 108.802018ms | 0.742 | 0.378 | 2.4s | 7 | 220 | 208 | 46.2s | 0 | 0.010 | 71.82% | win |
| probe-015-hard-confirmed | gradual_degradation | 101 | 2m17.896601353s | 2m15.028784667s | 112.288179ms | 112.288179ms | 0.740 | 0.732 | 2.2s | 7 | 443 | 432 | not detected | 0 | 0.015 | 2.51% | neutral |
| probe-015-hard-confirmed | gradual_degradation | 131 | 2m18.257709744s | 980.556272ms | 112.50077ms | 40.752122ms | 0.742 | 0.050 | 0s | 0 | 24 | 24 | 6.4s | 0 | 0.005 | 99.03% | win |
| probe-015-hard-confirmed | gradual_degradation | 173 | 2m18.511286942s | 44.501332635s | 112.241073ms | 108.888477ms | 0.740 | 0.368 | 1.2s | 3 | 221 | 215 | 46.4s | 0 | 0.010 | 71.35% | win |
| probe-015-hard-confirmed | sudden_outage | 11 | 15.6s | 478.596346ms | 29.758413ms | 39.982664ms | 0.669 | 0.044 | 0s | 0 | 13 | 13 | 4.2s | 0 | 0.004 | 93.47% | win |
| probe-015-hard-confirmed | sudden_outage | 29 | 15.6s | 475.527462ms | 29.586311ms | 40.071145ms | 0.667 | 0.038 | 2.2s | 6 | 11 | 0 | 3.8s | 0 | 0.004 | 93.71% | win |
| probe-015-hard-confirmed | sudden_outage | 47 | 15.6s | 479.581566ms | 29.297237ms | 39.815778ms | 0.676 | 0.049 | 2.6s | 8 | 13 | 0 | 4.2s | 0 | 0.004 | 93.43% | win |
| probe-015-hard-confirmed | sudden_outage | 71 | 15.6s | 1.066075324s | 29.362123ms | 40.078569ms | 0.673 | 0.056 | 2.2s | 5 | 11 | 0 | 3.8s | 0 | 0.004 | 90.86% | win |
| probe-015-hard-confirmed | sudden_outage | 101 | 15.6s | 479.387647ms | 29.696734ms | 40.249474ms | 0.673 | 0.049 | 2.2s | 5 | 11 | 0 | 3.8s | 0 | 0.004 | 93.52% | win |
| probe-015-hard-confirmed | sudden_outage | 131 | 15.6s | 1.328626655s | 29.458433ms | 40.174049ms | 0.676 | 0.060 | 0s | 0 | 11 | 11 | 3.8s | 0 | 0.004 | 89.64% | win |
| probe-015-hard-confirmed | sudden_outage | 173 | 15.6s | 479.497374ms | 29.101676ms | 40.039804ms | 0.673 | 0.047 | 800ms | 2 | 11 | 7 | 3.8s | 0 | 0.004 | 93.57% | win |
| probe-015-hard-confirmed | capacity_aggregation | 11 | 1m19.876239452s | 1m17.424416709s | 25ms | 25ms | 0.988 | 0.972 | 0s | 0 | 630 | 630 | not detected | 0 | 0.015 | 3.55% | neutral |
| probe-015-hard-confirmed | capacity_aggregation | 29 | 1m19.971533361s | 1m17.403747547s | 25ms | 25ms | 0.988 | 0.972 | 0s | 0 | 630 | 630 | not detected | 0 | 0.015 | 3.65% | neutral |
| probe-015-hard-confirmed | capacity_aggregation | 47 | 1m19.89581372s | 1m17.464650921s | 25ms | 25ms | 0.988 | 0.972 | 0s | 0 | 630 | 630 | not detected | 0 | 0.015 | 3.54% | neutral |
| probe-015-hard-confirmed | capacity_aggregation | 71 | 1m19.767790605s | 1m17.268448036s | 25ms | 25ms | 0.988 | 0.972 | 0s | 0 | 630 | 630 | not detected | 0 | 0.015 | 3.60% | neutral |
| probe-015-hard-confirmed | capacity_aggregation | 101 | 1m20.040921681s | 1m17.526615303s | 25ms | 25ms | 0.990 | 0.975 | 0s | 0 | 630 | 630 | not detected | 0 | 0.015 | 3.59% | neutral |
| probe-015-hard-confirmed | capacity_aggregation | 131 | 1m19.734245977s | 1m17.170636114s | 25ms | 25ms | 0.988 | 0.972 | 0s | 0 | 630 | 630 | not detected | 0 | 0.015 | 3.67% | neutral |
| probe-015-hard-confirmed | capacity_aggregation | 173 | 1m19.865475022s | 1m17.320114208s | 25ms | 25ms | 0.988 | 0.972 | 0s | 0 | 630 | 630 | not detected | 0 | 0.015 | 3.63% | neutral |
| probe-015-hard-confirmed | transient_spike | 11 | 12.6925426s | 11.666680066s | 90ms | 90ms | 0.396 | 0.369 | 0s | 0 | 295 | 295 | not detected | 0 | 0.016 | 7.95% | neutral |
| probe-015-hard-confirmed | transient_spike | 29 | 12.307633594s | 11.911495052s | 90ms | 90ms | 0.384 | 0.373 | 4.4s | 5 | 295 | 274 | not detected | 0 | 0.016 | 3.26% | neutral |
| probe-015-hard-confirmed | transient_spike | 47 | 12.854235214s | 11.8145828s | 90ms | 90ms | 0.402 | 0.376 | 4.6s | 6 | 295 | 273 | not detected | 0 | 0.016 | 7.79% | neutral |
| probe-015-hard-confirmed | transient_spike | 71 | 12.659328791s | 1.931534132s | 90ms | 90ms | 0.400 | 0.076 | 4.4s | 4 | 23 | 2 | 6.2s | 0 | 0.007 | 78.46% | win |
| probe-015-hard-confirmed | transient_spike | 101 | 12.607221347s | 11.934856311s | 90ms | 90ms | 0.398 | 0.207 | 4.2s | 4 | 188 | 168 | 19.6s | 1 | 0.009 | 11.98% | neutral |
| probe-015-hard-confirmed | transient_spike | 131 | 12.512324827s | 11.776701608s | 90ms | 90ms | 0.398 | 0.367 | 0s | 0 | 295 | 295 | not detected | 0 | 0.016 | 6.30% | neutral |
| probe-015-hard-confirmed | transient_spike | 173 | 12.652228169s | 11.673458085s | 90ms | 90ms | 0.398 | 0.211 | 800ms | 2 | 189 | 185 | 19.6s | 1 | 0.009 | 13.70% | neutral |
| probe-015-hard-confirmed | stale_candidate | 11 | 46.403333381s | 44.010000047s | 110ms | 110ms | 0.889 | 0.865 | 4.666666662s | 2 | 147 | 134 | not detected | 0 | 0.018 | 5.63% | neutral |
| probe-015-hard-confirmed | stale_candidate | 29 | 52.933333379s | 50.736666712s | 110ms | 110ms | 0.883 | 0.860 | 4.666666662s | 2 | 147 | 134 | not detected | 0 | 0.018 | 5.20% | neutral |
| probe-015-hard-confirmed | stale_candidate | 47 | 51.540000046s | 48.873333378s | 110ms | 110ms | 0.883 | 0.860 | 4.666666662s | 2 | 147 | 134 | not detected | 0 | 0.018 | 5.58% | neutral |
| probe-015-hard-confirmed | stale_candidate | 71 | 52.600000048s | 50.266666713s | 110ms | 110ms | 0.889 | 0.865 | 0s | 0 | 147 | 147 | not detected | 0 | 0.018 | 5.07% | neutral |
| probe-015-hard-confirmed | stale_candidate | 101 | 49.070000047s | 46.87333338s | 110ms | 110ms | 0.883 | 0.860 | 4.666666662s | 1 | 147 | 134 | not detected | 0 | 0.018 | 4.79% | neutral |
| probe-015-hard-confirmed | stale_candidate | 131 | 47.40333338s | 45.813333378s | 110ms | 110ms | 0.883 | 0.860 | 4.666666662s | 2 | 147 | 134 | not detected | 0 | 0.018 | 4.07% | neutral |
| probe-015-hard-confirmed | stale_candidate | 173 | 46.87333338s | 44.676666713s | 110ms | 110ms | 0.883 | 0.860 | 2.999999997s | 0 | 147 | 138 | not detected | 0 | 0.018 | 5.65% | neutral |
| probe-015-hard-confirmed | recovery_no_flap | 11 | 44.799011002s | 1.358971675s | 95ms | 40.762979ms | 0.752 | 0.053 | 0s | 0 | 22 | 22 | 6.2s | 0 | 0.005 | 96.68% | win |
| probe-015-hard-confirmed | recovery_no_flap | 29 | 44.937203373s | 1.418231073s | 95ms | 40.65075ms | 0.750 | 0.062 | 4.2s | 5 | 23 | 3 | 6.2s | 0 | 0.005 | 96.55% | win |
| probe-015-hard-confirmed | recovery_no_flap | 47 | 44.810457119s | 43.745s | 95ms | 95ms | 0.757 | 0.745 | 4.2s | 5 | 443 | 423 | not detected | 0 | 0.015 | 3.19% | neutral |
| probe-015-hard-confirmed | recovery_no_flap | 71 | 44.958361864s | 1.3590909s | 95ms | 40.858579ms | 0.755 | 0.065 | 4.2s | 4 | 22 | 2 | 6.2s | 0 | 0.005 | 96.63% | win |
| probe-015-hard-confirmed | recovery_no_flap | 101 | 44.837077225s | 43.745s | 95ms | 95ms | 0.755 | 0.600 | 4.2s | 4 | 352 | 332 | 1m12.8s | 0 | 0.013 | 8.26% | neutral |
| probe-015-hard-confirmed | recovery_no_flap | 131 | 44.901274352s | 43.745s | 95ms | 95ms | 0.757 | 0.600 | 0s | 0 | 352 | 352 | 1m12.8s | 0 | 0.013 | 8.34% | neutral |
| probe-015-hard-confirmed | recovery_no_flap | 173 | 44.9015634s | 19.095s | 95ms | 95ms | 0.755 | 0.185 | 800ms | 2 | 89 | 85 | 19.6s | 0 | 0.007 | 66.98% | win |
| probe-015-hard-confirmed | all_channels_bad | 11 | 2m10.9s | 2m7.5s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | 300 | not detected | 0 | 0.016 | 2.95% | neutral |
| probe-015-hard-confirmed | all_channels_bad | 29 | 2m11.7s | 2m9s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | 300 | not detected | 0 | 0.016 | 2.54% | neutral |
| probe-015-hard-confirmed | all_channels_bad | 47 | 2m7.1s | 2m4.1s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | 300 | not detected | 0 | 0.016 | 2.66% | neutral |
| probe-015-hard-confirmed | all_channels_bad | 71 | 2m9.6s | 2m7.3s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.016 | 2.26% | neutral |
| probe-015-hard-confirmed | all_channels_bad | 101 | 2m6.5s | 2m4s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.016 | 2.38% | neutral |
| probe-015-hard-confirmed | all_channels_bad | 131 | 2m5.1s | 2m2.6s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | 300 | not detected | 0 | 0.016 | 2.40% | neutral |
| probe-015-hard-confirmed | all_channels_bad | 173 | 2m9.753516526s | 2m6.8s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | 300 | not detected | 0 | 0.016 | 2.72% | neutral |
| probe-015-hard-confirmed | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-015-hard-confirmed | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-015-hard-confirmed | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-015-hard-confirmed | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-015-hard-confirmed | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-015-hard-confirmed | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-015-hard-confirmed | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | 7 | not detected | 0 | 0.000 | 0.00% | neutral |
| probe-015-hard-confirmed | healthy_steady_state | 11 | 306.901147ms | 307.699496ms | 29.560991ms | 29.775736ms | 0.015 | 0.015 | not detected | 0 | 0 | 0 | not detected | 0 | 0.015 | -0.52% | neutral |
| probe-015-hard-confirmed | healthy_steady_state | 29 | 306.449512ms | 308.206465ms | 29.524996ms | 29.654816ms | 0.015 | 0.015 | not detected | 0 | 0 | 0 | not detected | 0 | 0.015 | -0.46% | neutral |
| probe-015-hard-confirmed | healthy_steady_state | 47 | 306.757116ms | 309.133957ms | 29.42026ms | 29.519065ms | 0.028 | 0.028 | not detected | 0 | 0 | 0 | not detected | 0 | 0.015 | -0.46% | neutral |
| probe-015-hard-confirmed | healthy_steady_state | 71 | 304.971872ms | 307.576474ms | 29.463197ms | 29.650924ms | 0.018 | 0.018 | not detected | 0 | 0 | 0 | not detected | 0 | 0.015 | -0.67% | neutral |
| probe-015-hard-confirmed | healthy_steady_state | 101 | 306.026121ms | 306.947866ms | 29.656679ms | 29.751595ms | 0.023 | 0.025 | not detected | 0 | 0 | 0 | not detected | 0 | 0.015 | -0.99% | neutral |
| probe-015-hard-confirmed | healthy_steady_state | 131 | 307.461992ms | 308.762844ms | 29.32466ms | 29.584412ms | 0.022 | 0.022 | not detected | 0 | 0 | 0 | not detected | 0 | 0.015 | -0.65% | neutral |
| probe-015-hard-confirmed | healthy_steady_state | 173 | 306.762211ms | 308.445333ms | 29.635213ms | 29.753827ms | 0.022 | 0.023 | not detected | 0 | 0 | 0 | not detected | 0 | 0.015 | -1.07% | neutral |
