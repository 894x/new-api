# Dynamic routing simulation

Significant improvement gate: 15.00%; maximum regression: 8.00%; minimum scenario win rate: 0.45.

## Ranking

| Rank | Parameters | Average improvement | Worst seed | Win rate | Significant | Regressed runs |
|---:|---|---:|---:|---:|:---:|---:|
| 1 | fast-recovery | 17.19% | -0.11% | 0.25 | false | 0 |
| 2 | stable-recovery | 16.38% | -0.11% | 0.25 | false | 0 |
| 3 | hard-failure-confirmed | 14.55% | -0.11% | 0.25 | false | 0 |
| 4 | balanced | 14.00% | -0.11% | 0.25 | false | 0 |
| 5 | short-window | 13.58% | -0.11% | 0.12 | false | 0 |
| 6 | aggressive | 12.85% | -0.11% | 0.25 | false | 0 |
| 7 | conservative | 9.97% | -0.12% | 0.12 | false | 1 |
| 8 | fast-low-probe | 5.99% | -0.12% | 0.12 | false | 0 |

## Every scenario and seed

| Parameters | Scenario | Seed | Static p95 TTFT | Dynamic p95 TTFT | Static p95 TPOT | Dynamic p95 TPOT | Static SLO violations | Dynamic SLO violations | Detection elapsed | Detection observations | Bad exposure | Mitigation elapsed | Route reversals | Probe cost | Improvement | Result |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| conservative | gradual_degradation | 11 | 2m18.05810847s | 2m16.790178167s | 111.638008ms | 111.638008ms | 0.738 | 0.735 | 2.4s | 8 | 447 | not detected | 9 | 0.008 | 1.12% | neutral |
| conservative | gradual_degradation | 29 | 2m17.230127598s | 2m16.401289205s | 111.973508ms | 111.973508ms | 0.737 | 0.733 | 2.6s | 7 | 448 | not detected | 7 | 0.007 | 0.71% | neutral |
| conservative | gradual_degradation | 47 | 2m18.543146187s | 2m17.031311386s | 112.017461ms | 111.887792ms | 0.743 | 0.738 | 2.4s | 6 | 447 | not detected | 9 | 0.008 | 1.23% | neutral |
| conservative | gradual_degradation | 71 | 2m18.842157616s | 2m17.860558027s | 112.679273ms | 112.679273ms | 0.742 | 0.738 | 2.4s | 7 | 448 | not detected | 9 | 0.008 | 0.88% | neutral |
| conservative | gradual_degradation | 101 | 2m17.896601353s | 2m17.896601353s | 112.288179ms | 112.288179ms | 0.740 | 0.742 | 2.2s | 7 | 450 | not detected | 3 | 0.003 | -0.00% | neutral |
| conservative | gradual_degradation | 131 | 2m18.257709744s | 2m16.347484138s | 112.50077ms | 112.50077ms | 0.742 | 0.738 | 2.2s | 7 | 447 | not detected | 9 | 0.008 | 1.44% | neutral |
| conservative | gradual_degradation | 173 | 2m18.511286942s | 2m17.075365079s | 112.241073ms | 112.241073ms | 0.740 | 0.735 | 1.4s | 4 | 447 | not detected | 9 | 0.008 | 1.17% | neutral |
| conservative | sudden_outage | 11 | 306.854065ms | 307.805481ms | 29.758413ms | 29.957571ms | 0.669 | 0.664 | 2.6s | 8 | 298 | not detected | 7 | 0.009 | 0.54% | neutral |
| conservative | sudden_outage | 29 | 305.736046ms | 308.17038ms | 29.586311ms | 29.828135ms | 0.667 | 0.664 | 2.2s | 6 | 299 | not detected | 5 | 0.007 | 0.21% | neutral |
| conservative | sudden_outage | 47 | 306.757116ms | 309.640816ms | 29.297237ms | 29.532321ms | 0.676 | 0.673 | 2.6s | 8 | 299 | not detected | 5 | 0.007 | 0.21% | neutral |
| conservative | sudden_outage | 71 | 306.766218ms | 309.938996ms | 29.362123ms | 29.657202ms | 0.673 | 0.671 | 2.2s | 5 | 299 | not detected | 7 | 0.009 | 0.19% | neutral |
| conservative | sudden_outage | 101 | 306.026121ms | 306.415917ms | 29.696734ms | 29.759224ms | 0.673 | 0.676 | 2.2s | 5 | 300 | not detected | 3 | 0.004 | -0.12% | neutral |
| conservative | sudden_outage | 131 | 308.156379ms | 309.774647ms | 29.458433ms | 29.677918ms | 0.676 | 0.673 | 2.2s | 6 | 299 | not detected | 5 | 0.007 | 0.23% | neutral |
| conservative | sudden_outage | 173 | 307.890143ms | 309.337213ms | 29.101676ms | 29.753827ms | 0.673 | 0.669 | 1.2s | 3 | 298 | not detected | 7 | 0.009 | 0.43% | neutral |
| conservative | capacity_aggregation | 11 | 1m19.876239452s | 26.059894661s | 25ms | 28ms | 0.988 | 0.944 | 0s | 0 | 332 | not detected | 338 | 0.011 | 68.32% | win |
| conservative | capacity_aggregation | 29 | 1m19.971533361s | 26.064231675s | 25ms | 28ms | 0.988 | 0.938 | 0s | 0 | 332 | not detected | 338 | 0.011 | 68.42% | win |
| conservative | capacity_aggregation | 47 | 1m19.89581372s | 26.024372271s | 25ms | 28ms | 0.988 | 0.942 | 0s | 0 | 333 | not detected | 338 | 0.011 | 68.35% | win |
| conservative | capacity_aggregation | 71 | 1m19.767790605s | 26.183350614s | 25ms | 28ms | 0.988 | 0.940 | 0s | 0 | 333 | not detected | 338 | 0.011 | 68.17% | win |
| conservative | capacity_aggregation | 101 | 1m20.040921681s | 26.251577689s | 25ms | 28ms | 0.990 | 0.943 | 0s | 0 | 333 | not detected | 338 | 0.011 | 68.18% | win |
| conservative | capacity_aggregation | 131 | 1m19.734245977s | 25.871312703s | 25ms | 28ms | 0.988 | 0.942 | 0s | 0 | 333 | not detected | 338 | 0.011 | 68.45% | win |
| conservative | capacity_aggregation | 173 | 1m19.865475022s | 26.032343815s | 25ms | 28ms | 0.988 | 0.943 | 0s | 0 | 332 | not detected | 338 | 0.011 | 68.33% | win |
| conservative | transient_spike | 11 | 12.6925426s | 11.760379494s | 90ms | 90ms | 0.396 | 0.376 | 4.4s | 5 | 298 | not detected | 7 | 0.009 | 7.08% | neutral |
| conservative | transient_spike | 29 | 12.307633594s | 12.307633594s | 90ms | 90ms | 0.384 | 0.380 | 4.4s | 5 | 299 | not detected | 5 | 0.007 | 0.33% | neutral |
| conservative | transient_spike | 47 | 12.854235214s | 11.986753307s | 90ms | 90ms | 0.402 | 0.382 | 4.6s | 6 | 298 | not detected | 7 | 0.009 | 6.19% | neutral |
| conservative | transient_spike | 71 | 12.659328791s | 12.659328791s | 90ms | 90ms | 0.400 | 0.393 | 4.4s | 4 | 299 | not detected | 7 | 0.009 | 0.17% | neutral |
| conservative | transient_spike | 101 | 12.607221347s | 12.607221347s | 90ms | 90ms | 0.398 | 0.400 | 4.2s | 4 | 300 | not detected | 3 | 0.004 | -0.04% | neutral |
| conservative | transient_spike | 131 | 12.512324827s | 11.766566305s | 90ms | 90ms | 0.398 | 0.376 | 4.4s | 5 | 298 | not detected | 7 | 0.009 | 5.83% | neutral |
| conservative | transient_spike | 173 | 12.652228169s | 12.652228169s | 90ms | 90ms | 0.398 | 0.393 | 1.2s | 3 | 299 | not detected | 5 | 0.007 | 0.34% | neutral |
| conservative | stale_candidate | 11 | 46.540000047s | 44.676666715s | 110ms | 110ms | 0.889 | 0.865 | 10.333333323s | 13 | 147 | not detected | 9 | 0.029 | 4.66% | neutral |
| conservative | stale_candidate | 29 | 51.736666713s | 48.873333378s | 110ms | 110ms | 0.883 | 0.860 | 9.99999999s | 11 | 147 | not detected | 9 | 0.029 | 5.93% | neutral |
| conservative | stale_candidate | 47 | 51.540000046s | 49.600000047s | 110ms | 110ms | 0.883 | 0.860 | 10.999999989s | 12 | 147 | not detected | 9 | 0.029 | 4.46% | neutral |
| conservative | stale_candidate | 71 | 50.403333377s | 48.873333378s | 110ms | 110ms | 0.889 | 0.871 | 11.333333322s | 13 | 147 | not detected | 9 | 0.029 | 4.29% | neutral |
| conservative | stale_candidate | 101 | 48.010000047s | 46.403333379s | 110ms | 110ms | 0.883 | 0.865 | 10.666666656s | 11 | 147 | not detected | 9 | 0.029 | 3.83% | neutral |
| conservative | stale_candidate | 131 | 47.873333381s | 46.206666714s | 110ms | 110ms | 0.883 | 0.860 | 10.333333323s | 12 | 147 | not detected | 9 | 0.029 | 3.62% | neutral |
| conservative | stale_candidate | 173 | 46.87333338s | 43.010000046s | 110ms | 110ms | 0.883 | 0.860 | 10.666666656s | 11 | 147 | not detected | 9 | 0.029 | 8.49% | regression |
| conservative | recovery_no_flap | 11 | 44.799011002s | 44.406413079s | 95ms | 95ms | 0.752 | 0.747 | 4.2s | 5 | 447 | not detected | 9 | 0.008 | 1.31% | neutral |
| conservative | recovery_no_flap | 29 | 44.937203373s | 44.336363542s | 95ms | 95ms | 0.750 | 0.747 | 4.2s | 5 | 448 | not detected | 7 | 0.007 | 1.55% | neutral |
| conservative | recovery_no_flap | 47 | 44.810457119s | 44.299181422s | 95ms | 95ms | 0.757 | 0.752 | 4.2s | 5 | 447 | not detected | 9 | 0.008 | 1.48% | neutral |
| conservative | recovery_no_flap | 71 | 44.958361864s | 44.264532893s | 95ms | 95ms | 0.755 | 0.752 | 4.2s | 4 | 448 | not detected | 9 | 0.008 | 1.59% | neutral |
| conservative | recovery_no_flap | 101 | 44.837077225s | 44.837077225s | 95ms | 95ms | 0.755 | 0.757 | 4.2s | 4 | 450 | not detected | 3 | 0.003 | -0.01% | neutral |
| conservative | recovery_no_flap | 131 | 44.901274352s | 44.249072613s | 95ms | 95ms | 0.757 | 0.752 | 4.2s | 5 | 447 | not detected | 9 | 0.008 | 1.89% | neutral |
| conservative | recovery_no_flap | 173 | 44.9015634s | 44.264579752s | 95ms | 95ms | 0.755 | 0.752 | 1.2s | 3 | 447 | not detected | 9 | 0.008 | 1.78% | neutral |
| conservative | all_channels_bad | 11 | 2m10.9s | 2m9.1s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 7 | 0.009 | 1.45% | neutral |
| conservative | all_channels_bad | 29 | 2m9.8s | 2m8.2s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 7 | 0.009 | 1.32% | neutral |
| conservative | all_channels_bad | 47 | 2m5.2s | 2m3.5s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 7 | 0.009 | 1.44% | neutral |
| conservative | all_channels_bad | 71 | 2m9.4s | 2m8.1s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 7 | 0.009 | 0.95% | neutral |
| conservative | all_channels_bad | 101 | 2m5.7s | 2m5.7s | 100ms | 100ms | 0.673 | 0.676 | 0s | 0 | 300 | not detected | 3 | 0.004 | -0.00% | neutral |
| conservative | all_channels_bad | 131 | 2m5.1s | 2m4.1s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 7 | 0.009 | 1.02% | neutral |
| conservative | all_channels_bad | 173 | 2m9.3s | 2m8.153516526s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 7 | 0.009 | 1.07% | neutral |
| conservative | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| conservative | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| conservative | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| conservative | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| conservative | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| conservative | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| conservative | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| balanced | gradual_degradation | 11 | 2m18.05810847s | 2m18.05810847s | 111.638008ms | 111.638008ms | 0.738 | 0.738 | 2.2s | 6 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | gradual_degradation | 29 | 2m17.230127598s | 2m17.230127598s | 111.973508ms | 111.973508ms | 0.737 | 0.737 | 2.4s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | gradual_degradation | 47 | 2m18.543146187s | 2m18.543146187s | 112.017461ms | 112.017461ms | 0.743 | 0.743 | 2.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | gradual_degradation | 71 | 2m18.842157616s | 2m18.842157616s | 112.679273ms | 112.679273ms | 0.742 | 0.742 | 2.4s | 7 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | gradual_degradation | 101 | 2m17.896601353s | 2m17.896601353s | 112.288179ms | 112.288179ms | 0.740 | 0.740 | 2.2s | 7 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | gradual_degradation | 131 | 2m18.257709744s | 2m18.257709744s | 112.50077ms | 112.50077ms | 0.742 | 0.742 | 2.2s | 7 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | gradual_degradation | 173 | 2m18.511286942s | 2m18.511286942s | 112.241073ms | 112.241073ms | 0.740 | 0.740 | 1.4s | 4 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | sudden_outage | 11 | 306.854065ms | 307.058392ms | 29.758413ms | 29.775736ms | 0.669 | 0.669 | 2s | 5 | 300 | not detected | 1 | 0.002 | -0.01% | neutral |
| balanced | sudden_outage | 29 | 305.736046ms | 305.736046ms | 29.586311ms | 29.812279ms | 0.667 | 0.667 | 2s | 5 | 300 | not detected | 1 | 0.002 | -0.05% | neutral |
| balanced | sudden_outage | 47 | 306.757116ms | 307.058184ms | 29.297237ms | 29.386626ms | 0.676 | 0.676 | 2.4s | 7 | 300 | not detected | 1 | 0.002 | -0.02% | neutral |
| balanced | sudden_outage | 71 | 306.766218ms | 308.348868ms | 29.362123ms | 29.436265ms | 0.673 | 0.673 | 2s | 4 | 300 | not detected | 1 | 0.002 | -0.04% | neutral |
| balanced | sudden_outage | 101 | 306.026121ms | 306.150606ms | 29.696734ms | 29.696734ms | 0.673 | 0.673 | 2s | 4 | 300 | not detected | 1 | 0.002 | -0.00% | neutral |
| balanced | sudden_outage | 131 | 308.156379ms | 308.545061ms | 29.458433ms | 29.570855ms | 0.676 | 0.676 | 2s | 5 | 300 | not detected | 1 | 0.002 | -0.03% | neutral |
| balanced | sudden_outage | 173 | 307.890143ms | 308.543702ms | 29.101676ms | 29.535836ms | 0.673 | 0.673 | 1.2s | 3 | 300 | not detected | 1 | 0.002 | -0.11% | neutral |
| balanced | capacity_aggregation | 11 | 1m19.876239452s | 36.488497014s | 25ms | 28ms | 0.988 | 0.939 | 0s | 0 | 382 | 3.25s | 50 | 0.007 | 58.28% | win |
| balanced | capacity_aggregation | 29 | 1m19.971533361s | 36.034880173s | 25ms | 28ms | 0.988 | 0.940 | 0s | 0 | 370 | 3.125s | 36 | 0.007 | 58.79% | win |
| balanced | capacity_aggregation | 47 | 1m19.89581372s | 36.476683949s | 25ms | 28ms | 0.988 | 0.939 | 0s | 0 | 379 | 3.125s | 50 | 0.007 | 58.33% | win |
| balanced | capacity_aggregation | 71 | 1m19.767790605s | 22.073610032s | 25ms | 28ms | 0.988 | 0.919 | 0s | 0 | 267 | 3.25s | 61 | 0.007 | 73.62% | win |
| balanced | capacity_aggregation | 101 | 1m20.040921681s | 18.925384926s | 25ms | 28ms | 0.990 | 0.926 | 0s | 0 | 300 | 3.25s | 59 | 0.007 | 76.63% | win |
| balanced | capacity_aggregation | 131 | 1m19.734245977s | 36.307986354s | 25ms | 28ms | 0.988 | 0.940 | 0s | 0 | 381 | 3.25s | 50 | 0.007 | 58.35% | win |
| balanced | capacity_aggregation | 173 | 1m19.865475022s | 36.348437938s | 25ms | 28ms | 0.988 | 0.942 | 0s | 0 | 381 | 3.125s | 50 | 0.007 | 58.38% | win |
| balanced | transient_spike | 11 | 12.6925426s | 12.6925426s | 90ms | 90ms | 0.396 | 0.396 | 4.4s | 5 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | transient_spike | 29 | 12.307633594s | 12.307633594s | 90ms | 90ms | 0.384 | 0.384 | 4.4s | 5 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | transient_spike | 47 | 12.854235214s | 12.854235214s | 90ms | 90ms | 0.402 | 0.402 | 4.6s | 6 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | transient_spike | 71 | 12.659328791s | 12.659328791s | 90ms | 90ms | 0.400 | 0.400 | 4.4s | 4 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | transient_spike | 101 | 12.607221347s | 12.607221347s | 90ms | 90ms | 0.398 | 0.398 | 4.2s | 4 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | transient_spike | 131 | 12.512324827s | 12.512324827s | 90ms | 90ms | 0.398 | 0.398 | 4.4s | 5 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | transient_spike | 173 | 12.652228169s | 12.652228169s | 90ms | 90ms | 0.398 | 0.398 | 1.2s | 3 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | stale_candidate | 11 | 46.540000047s | 27.830000028s | 110ms | 110ms | 0.889 | 0.579 | 9.99999999s | 12 | 96 | 36.333333297s | 14 | 0.035 | 43.64% | win |
| balanced | stale_candidate | 29 | 51.736666713s | 31.890000031s | 110ms | 110ms | 0.883 | 0.567 | 9.666666657s | 9 | 96 | 36.333333297s | 14 | 0.035 | 42.93% | win |
| balanced | stale_candidate | 47 | 51.540000046s | 31.693333362s | 110ms | 110ms | 0.883 | 0.567 | 9.333333324s | 9 | 96 | 36.333333297s | 14 | 0.035 | 43.06% | win |
| balanced | stale_candidate | 71 | 50.403333377s | 31.693333364s | 110ms | 110ms | 0.889 | 0.585 | 10.333333323s | 11 | 97 | 36.66666663s | 14 | 0.035 | 41.93% | win |
| balanced | stale_candidate | 101 | 48.010000047s | 28.496666694s | 110ms | 110ms | 0.883 | 0.567 | 9.99999999s | 9 | 94 | 35.666666631s | 14 | 0.035 | 44.61% | win |
| balanced | stale_candidate | 131 | 47.873333381s | 28.163333363s | 110ms | 110ms | 0.883 | 0.567 | 9.666666657s | 10 | 96 | 36.333333297s | 14 | 0.035 | 45.05% | win |
| balanced | stale_candidate | 173 | 46.87333338s | 29.693333364s | 110ms | 110ms | 0.883 | 0.591 | 9.99999999s | 9 | 98 | 36.999999963s | 14 | 0.035 | 40.25% | win |
| balanced | recovery_no_flap | 11 | 44.799011002s | 44.799011002s | 95ms | 95ms | 0.752 | 0.752 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | recovery_no_flap | 29 | 44.937203373s | 44.937203373s | 95ms | 95ms | 0.750 | 0.750 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | recovery_no_flap | 47 | 44.810457119s | 44.810457119s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | recovery_no_flap | 71 | 44.958361864s | 44.958361864s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | recovery_no_flap | 101 | 44.837077225s | 44.837077225s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | recovery_no_flap | 131 | 44.901274352s | 44.901274352s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | recovery_no_flap | 173 | 44.9015634s | 44.9015634s | 95ms | 95ms | 0.755 | 0.755 | 1.2s | 3 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | all_channels_bad | 11 | 2m10.9s | 2m10.9s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | all_channels_bad | 29 | 2m9.8s | 2m9.8s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | all_channels_bad | 47 | 2m5.2s | 2m5.2s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | all_channels_bad | 71 | 2m9.4s | 2m9.4s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | all_channels_bad | 101 | 2m5.7s | 2m5.7s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | all_channels_bad | 131 | 2m5.1s | 2m5.1s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | all_channels_bad | 173 | 2m9.3s | 2m9.3s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| balanced | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| balanced | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| balanced | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| balanced | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| balanced | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| balanced | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| aggressive | gradual_degradation | 11 | 2m18.05810847s | 2m18.05810847s | 111.638008ms | 111.638008ms | 0.738 | 0.738 | 400ms | 2 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | gradual_degradation | 29 | 2m17.230127598s | 2m17.230127598s | 111.973508ms | 111.973508ms | 0.737 | 0.737 | 2.4s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | gradual_degradation | 47 | 2m18.543146187s | 2m18.543146187s | 112.017461ms | 112.017461ms | 0.743 | 0.743 | 2.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | gradual_degradation | 71 | 2m18.842157616s | 2m18.842157616s | 112.679273ms | 112.679273ms | 0.742 | 0.742 | 2.4s | 7 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | gradual_degradation | 101 | 2m17.896601353s | 2m17.896601353s | 112.288179ms | 112.288179ms | 0.740 | 0.740 | 0s | 0 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | gradual_degradation | 131 | 2m18.257709744s | 2m18.257709744s | 112.50077ms | 112.50077ms | 0.742 | 0.742 | 2.2s | 7 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | gradual_degradation | 173 | 2m18.511286942s | 2m18.511286942s | 112.241073ms | 112.241073ms | 0.740 | 0.740 | 1.2s | 3 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | sudden_outage | 11 | 306.854065ms | 307.058392ms | 29.758413ms | 29.775736ms | 0.669 | 0.669 | 200ms | 2 | 300 | not detected | 1 | 0.002 | -0.01% | neutral |
| aggressive | sudden_outage | 29 | 305.736046ms | 306.900343ms | 29.586311ms | 29.812279ms | 0.667 | 0.667 | 2s | 5 | 300 | not detected | 1 | 0.002 | -0.07% | neutral |
| aggressive | sudden_outage | 47 | 306.757116ms | 307.058184ms | 29.297237ms | 29.386626ms | 0.676 | 0.676 | 2.4s | 7 | 300 | not detected | 1 | 0.002 | -0.02% | neutral |
| aggressive | sudden_outage | 71 | 306.766218ms | 308.348868ms | 29.362123ms | 29.436265ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | -0.04% | neutral |
| aggressive | sudden_outage | 101 | 306.026121ms | 306.150606ms | 29.696734ms | 29.751595ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | -0.01% | neutral |
| aggressive | sudden_outage | 131 | 308.156379ms | 308.545061ms | 29.458433ms | 29.570855ms | 0.676 | 0.676 | 2s | 5 | 300 | not detected | 1 | 0.002 | -0.03% | neutral |
| aggressive | sudden_outage | 173 | 307.890143ms | 308.543702ms | 29.101676ms | 29.535836ms | 0.673 | 0.673 | 800ms | 2 | 300 | not detected | 1 | 0.002 | -0.11% | neutral |
| aggressive | capacity_aggregation | 11 | 1m19.876239452s | 1m6.379054481s | 25ms | 28ms | 0.988 | 0.907 | 0s | 0 | 588 | not detected | 115 | 0.081 | 18.83% | win |
| aggressive | capacity_aggregation | 29 | 1m19.971533361s | 1m6.404914573s | 25ms | 28ms | 0.988 | 0.907 | 0s | 0 | 588 | not detected | 115 | 0.081 | 18.88% | win |
| aggressive | capacity_aggregation | 47 | 1m19.89581372s | 1m6.386169879s | 25ms | 28ms | 0.988 | 0.907 | 0s | 0 | 588 | not detected | 115 | 0.081 | 18.85% | win |
| aggressive | capacity_aggregation | 71 | 1m19.767790605s | 1m6.111167294s | 25ms | 28ms | 0.988 | 0.907 | 0s | 0 | 588 | not detected | 115 | 0.081 | 18.99% | win |
| aggressive | capacity_aggregation | 101 | 1m20.040921681s | 1m6.357418244s | 25ms | 28ms | 0.990 | 0.907 | 0s | 0 | 588 | not detected | 115 | 0.081 | 18.98% | win |
| aggressive | capacity_aggregation | 131 | 1m19.734245977s | 1m6.141091172s | 25ms | 28ms | 0.988 | 0.907 | 0s | 0 | 588 | not detected | 115 | 0.081 | 18.95% | win |
| aggressive | capacity_aggregation | 173 | 1m19.865475022s | 1m6.225737267s | 25ms | 28ms | 0.988 | 0.907 | 0s | 0 | 588 | not detected | 115 | 0.081 | 18.94% | win |
| aggressive | transient_spike | 11 | 12.6925426s | 12.6925426s | 90ms | 90ms | 0.396 | 0.396 | 200ms | 2 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | transient_spike | 29 | 12.307633594s | 12.307633594s | 90ms | 90ms | 0.384 | 0.384 | 4.4s | 5 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | transient_spike | 47 | 12.854235214s | 12.854235214s | 90ms | 90ms | 0.402 | 0.402 | 4.6s | 6 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | transient_spike | 71 | 12.659328791s | 12.659328791s | 90ms | 90ms | 0.400 | 0.400 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | transient_spike | 101 | 12.607221347s | 12.607221347s | 90ms | 90ms | 0.398 | 0.398 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | transient_spike | 131 | 12.512324827s | 12.512324827s | 90ms | 90ms | 0.398 | 0.398 | 4.4s | 5 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | transient_spike | 173 | 12.652228169s | 12.652228169s | 90ms | 90ms | 0.398 | 0.398 | 800ms | 2 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | stale_candidate | 11 | 46.540000047s | 7.393333341s | 110ms | 110ms | 0.889 | 0.251 | 9.333333324s | 10 | 35 | 15.333333318s | 8 | 0.023 | 83.82% | win |
| aggressive | stale_candidate | 29 | 51.736666713s | 10.453333343s | 110ms | 110ms | 0.883 | 0.257 | 9.333333324s | 8 | 35 | 15.333333318s | 8 | 0.023 | 81.42% | win |
| aggressive | stale_candidate | 47 | 51.540000046s | 9.923333341s | 110ms | 110ms | 0.883 | 0.216 | 9.333333324s | 9 | 35 | 15.333333318s | 8 | 0.023 | 82.29% | win |
| aggressive | stale_candidate | 71 | 50.403333377s | 21.573333354s | 110ms | 110ms | 0.889 | 0.427 | 9.333333324s | 10 | 69 | 27.666666639s | 14 | 0.041 | 61.18% | win |
| aggressive | stale_candidate | 101 | 48.010000047s | 13.31666668s | 110ms | 110ms | 0.883 | 0.310 | 9.666666657s | 8 | 47 | 19.666666647s | 10 | 0.029 | 74.47% | win |
| aggressive | stale_candidate | 131 | 47.873333381s | 8.39333334s | 110ms | 110ms | 0.883 | 0.222 | 9.333333324s | 9 | 35 | 15.333333318s | 8 | 0.023 | 83.00% | win |
| aggressive | stale_candidate | 173 | 46.87333338s | 9.590000008s | 110ms | 110ms | 0.883 | 0.240 | 9.666666657s | 9 | 35 | 15.333333318s | 8 | 0.023 | 80.68% | win |
| aggressive | recovery_no_flap | 11 | 44.799011002s | 44.799011002s | 95ms | 95ms | 0.752 | 0.752 | 200ms | 2 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | recovery_no_flap | 29 | 44.937203373s | 44.937203373s | 95ms | 95ms | 0.750 | 0.750 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | recovery_no_flap | 47 | 44.810457119s | 44.810457119s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | recovery_no_flap | 71 | 44.958361864s | 44.958361864s | 95ms | 95ms | 0.755 | 0.755 | 0s | 0 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | recovery_no_flap | 101 | 44.837077225s | 44.837077225s | 95ms | 95ms | 0.755 | 0.755 | 0s | 0 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | recovery_no_flap | 131 | 44.901274352s | 44.901274352s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | recovery_no_flap | 173 | 44.9015634s | 44.9015634s | 95ms | 95ms | 0.755 | 0.755 | 800ms | 2 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | all_channels_bad | 11 | 2m10.9s | 2m10.9s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | all_channels_bad | 29 | 2m9.8s | 2m9.8s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | all_channels_bad | 47 | 2m5.2s | 2m5.2s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | all_channels_bad | 71 | 2m9.4s | 2m9.4s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | all_channels_bad | 101 | 2m5.7s | 2m5.7s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | all_channels_bad | 131 | 2m5.1s | 2m5.1s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | all_channels_bad | 173 | 2m9.3s | 2m9.3s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| aggressive | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| aggressive | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| aggressive | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| aggressive | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| aggressive | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| aggressive | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| fast-low-probe | gradual_degradation | 11 | 2m18.05810847s | 2m18.05810847s | 111.638008ms | 111.638008ms | 0.738 | 0.738 | 400ms | 2 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | gradual_degradation | 29 | 2m17.230127598s | 2m17.230127598s | 111.973508ms | 111.973508ms | 0.737 | 0.737 | 2.4s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | gradual_degradation | 47 | 2m18.543146187s | 2m18.543146187s | 112.017461ms | 112.017461ms | 0.743 | 0.743 | 2.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | gradual_degradation | 71 | 2m18.842157616s | 2m18.842157616s | 112.679273ms | 112.679273ms | 0.742 | 0.742 | 2.4s | 7 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | gradual_degradation | 101 | 2m17.896601353s | 2m17.896601353s | 112.288179ms | 112.288179ms | 0.740 | 0.742 | 0s | 0 | 450 | not detected | 1 | 0.002 | -0.00% | neutral |
| fast-low-probe | gradual_degradation | 131 | 2m18.257709744s | 2m18.257709744s | 112.50077ms | 112.50077ms | 0.742 | 0.742 | 2.2s | 7 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | gradual_degradation | 173 | 2m18.511286942s | 2m18.511286942s | 112.241073ms | 112.241073ms | 0.740 | 0.740 | 1.2s | 3 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | sudden_outage | 11 | 306.854065ms | 307.058392ms | 29.758413ms | 29.775736ms | 0.669 | 0.669 | 200ms | 2 | 300 | not detected | 1 | 0.002 | -0.01% | neutral |
| fast-low-probe | sudden_outage | 29 | 305.736046ms | 306.900343ms | 29.586311ms | 29.812279ms | 0.667 | 0.667 | 2s | 5 | 300 | not detected | 1 | 0.002 | -0.07% | neutral |
| fast-low-probe | sudden_outage | 47 | 306.757116ms | 307.058184ms | 29.297237ms | 29.386626ms | 0.676 | 0.676 | 2.4s | 7 | 300 | not detected | 1 | 0.002 | -0.02% | neutral |
| fast-low-probe | sudden_outage | 71 | 306.766218ms | 308.348868ms | 29.362123ms | 29.436265ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | -0.04% | neutral |
| fast-low-probe | sudden_outage | 101 | 306.026121ms | 306.150606ms | 29.696734ms | 29.751595ms | 0.673 | 0.676 | 0s | 0 | 300 | not detected | 1 | 0.002 | -0.12% | neutral |
| fast-low-probe | sudden_outage | 131 | 308.156379ms | 308.545061ms | 29.458433ms | 29.570855ms | 0.676 | 0.676 | 2s | 5 | 300 | not detected | 1 | 0.002 | -0.03% | neutral |
| fast-low-probe | sudden_outage | 173 | 307.890143ms | 308.543702ms | 29.101676ms | 29.535836ms | 0.673 | 0.673 | 800ms | 2 | 300 | not detected | 1 | 0.002 | -0.11% | neutral |
| fast-low-probe | capacity_aggregation | 11 | 1m19.876239452s | 1m14.756929746s | 25ms | 25ms | 0.988 | 0.957 | 0s | 0 | 620 | not detected | 43 | 0.031 | 7.26% | neutral |
| fast-low-probe | capacity_aggregation | 29 | 1m19.971533361s | 1m14.801259226s | 25ms | 25ms | 0.988 | 0.957 | 0s | 0 | 620 | not detected | 43 | 0.031 | 7.28% | neutral |
| fast-low-probe | capacity_aggregation | 47 | 1m19.89581372s | 1m14.795953949s | 25ms | 25ms | 0.988 | 0.957 | 0s | 0 | 620 | not detected | 43 | 0.031 | 7.24% | neutral |
| fast-low-probe | capacity_aggregation | 71 | 1m19.767790605s | 1m14.700943311s | 25ms | 25ms | 0.988 | 0.957 | 0s | 0 | 620 | not detected | 43 | 0.031 | 7.21% | neutral |
| fast-low-probe | capacity_aggregation | 101 | 1m20.040921681s | 1m14.820794303s | 25ms | 25ms | 0.990 | 0.960 | 0s | 0 | 620 | not detected | 43 | 0.031 | 7.31% | neutral |
| fast-low-probe | capacity_aggregation | 131 | 1m19.734245977s | 1m14.674318061s | 25ms | 25ms | 0.988 | 0.957 | 0s | 0 | 620 | not detected | 43 | 0.031 | 7.21% | neutral |
| fast-low-probe | capacity_aggregation | 173 | 1m19.865475022s | 1m14.749705402s | 25ms | 25ms | 0.988 | 0.957 | 0s | 0 | 620 | not detected | 43 | 0.031 | 7.24% | neutral |
| fast-low-probe | transient_spike | 11 | 12.6925426s | 12.6925426s | 90ms | 90ms | 0.396 | 0.396 | 200ms | 2 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | transient_spike | 29 | 12.307633594s | 12.307633594s | 90ms | 90ms | 0.384 | 0.384 | 4.4s | 5 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | transient_spike | 47 | 12.854235214s | 12.854235214s | 90ms | 90ms | 0.402 | 0.402 | 4.6s | 6 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | transient_spike | 71 | 12.659328791s | 12.659328791s | 90ms | 90ms | 0.400 | 0.400 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | transient_spike | 101 | 12.607221347s | 12.607221347s | 90ms | 90ms | 0.398 | 0.400 | 0s | 0 | 300 | not detected | 1 | 0.002 | -0.04% | neutral |
| fast-low-probe | transient_spike | 131 | 12.512324827s | 12.512324827s | 90ms | 90ms | 0.398 | 0.398 | 4.4s | 5 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | transient_spike | 173 | 12.652228169s | 12.652228169s | 90ms | 90ms | 0.398 | 0.398 | 800ms | 2 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | stale_candidate | 11 | 46.540000047s | 29.496666695s | 110ms | 110ms | 0.889 | 0.596 | 9.333333324s | 10 | 97 | 35.999999964s | 8 | 0.023 | 40.67% | win |
| fast-low-probe | stale_candidate | 29 | 51.736666713s | 33.890000031s | 110ms | 110ms | 0.883 | 0.591 | 9.333333324s | 8 | 99 | 36.66666663s | 8 | 0.023 | 39.24% | win |
| fast-low-probe | stale_candidate | 47 | 51.540000046s | 32.89000003s | 110ms | 110ms | 0.883 | 0.579 | 9.333333324s | 9 | 98 | 36.333333297s | 8 | 0.023 | 40.71% | win |
| fast-low-probe | stale_candidate | 71 | 50.403333377s | 32.89000003s | 110ms | 110ms | 0.889 | 0.591 | 9.666666657s | 10 | 99 | 36.66666663s | 8 | 0.023 | 39.09% | win |
| fast-low-probe | stale_candidate | 101 | 48.010000047s | 30.360000028s | 110ms | 110ms | 0.883 | 0.596 | 9.666666657s | 8 | 97 | 35.999999964s | 8 | 0.023 | 40.66% | win |
| fast-low-probe | stale_candidate | 131 | 47.873333381s | 28.966666695s | 110ms | 110ms | 0.883 | 0.579 | 9.333333324s | 9 | 98 | 36.333333297s | 8 | 0.023 | 43.14% | win |
| fast-low-probe | stale_candidate | 173 | 46.87333338s | 29.163333362s | 110ms | 110ms | 0.883 | 0.579 | 9.666666657s | 8 | 98 | 36.333333297s | 8 | 0.023 | 41.44% | win |
| fast-low-probe | recovery_no_flap | 11 | 44.799011002s | 44.799011002s | 95ms | 95ms | 0.752 | 0.752 | 200ms | 2 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | recovery_no_flap | 29 | 44.937203373s | 44.937203373s | 95ms | 95ms | 0.750 | 0.750 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | recovery_no_flap | 47 | 44.810457119s | 44.810457119s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | recovery_no_flap | 71 | 44.958361864s | 44.958361864s | 95ms | 95ms | 0.755 | 0.755 | 0s | 0 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | recovery_no_flap | 101 | 44.837077225s | 44.837077225s | 95ms | 95ms | 0.755 | 0.757 | 0s | 0 | 450 | not detected | 1 | 0.002 | -0.01% | neutral |
| fast-low-probe | recovery_no_flap | 131 | 44.901274352s | 44.901274352s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | recovery_no_flap | 173 | 44.9015634s | 44.9015634s | 95ms | 95ms | 0.755 | 0.755 | 800ms | 2 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | all_channels_bad | 11 | 2m10.9s | 2m10.9s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | all_channels_bad | 29 | 2m9.8s | 2m9.8s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | all_channels_bad | 47 | 2m5.2s | 2m5.2s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | all_channels_bad | 71 | 2m9.4s | 2m9.4s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | all_channels_bad | 101 | 2m5.7s | 2m5.7s | 100ms | 100ms | 0.673 | 0.676 | 0s | 0 | 300 | not detected | 1 | 0.002 | -0.00% | neutral |
| fast-low-probe | all_channels_bad | 131 | 2m5.1s | 2m5.1s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | all_channels_bad | 173 | 2m9.3s | 2m9.3s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| fast-low-probe | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| fast-low-probe | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| fast-low-probe | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| fast-low-probe | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| fast-low-probe | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| fast-low-probe | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| short-window | gradual_degradation | 11 | 2m18.05810847s | 2m18.05810847s | 111.638008ms | 111.638008ms | 0.738 | 0.738 | 2.2s | 6 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | gradual_degradation | 29 | 2m17.230127598s | 2m17.230127598s | 111.973508ms | 111.973508ms | 0.737 | 0.737 | 2.4s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | gradual_degradation | 47 | 2m18.543146187s | 2m18.543146187s | 112.017461ms | 112.017461ms | 0.743 | 0.743 | 2.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | gradual_degradation | 71 | 2m18.842157616s | 2m18.842157616s | 112.679273ms | 112.679273ms | 0.742 | 0.742 | 2.4s | 7 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | gradual_degradation | 101 | 2m17.896601353s | 2m17.896601353s | 112.288179ms | 112.288179ms | 0.740 | 0.740 | 2.2s | 7 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | gradual_degradation | 131 | 2m18.257709744s | 2m18.257709744s | 112.50077ms | 112.50077ms | 0.742 | 0.742 | 2.2s | 7 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | gradual_degradation | 173 | 2m18.511286942s | 2m18.511286942s | 112.241073ms | 112.241073ms | 0.740 | 0.740 | 1.4s | 4 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | sudden_outage | 11 | 306.854065ms | 307.058392ms | 29.758413ms | 29.775736ms | 0.669 | 0.669 | 2s | 5 | 300 | not detected | 1 | 0.002 | -0.01% | neutral |
| short-window | sudden_outage | 29 | 305.736046ms | 306.900343ms | 29.586311ms | 29.812279ms | 0.667 | 0.667 | 2s | 5 | 300 | not detected | 1 | 0.002 | -0.07% | neutral |
| short-window | sudden_outage | 47 | 306.757116ms | 307.058184ms | 29.297237ms | 29.386626ms | 0.676 | 0.676 | 2.4s | 7 | 300 | not detected | 1 | 0.002 | -0.02% | neutral |
| short-window | sudden_outage | 71 | 306.766218ms | 308.348868ms | 29.362123ms | 29.436265ms | 0.673 | 0.673 | 2s | 4 | 300 | not detected | 1 | 0.002 | -0.04% | neutral |
| short-window | sudden_outage | 101 | 306.026121ms | 306.150606ms | 29.696734ms | 29.751595ms | 0.673 | 0.673 | 2s | 4 | 300 | not detected | 1 | 0.002 | -0.01% | neutral |
| short-window | sudden_outage | 131 | 308.156379ms | 308.545061ms | 29.458433ms | 29.570855ms | 0.676 | 0.676 | 2s | 5 | 300 | not detected | 1 | 0.002 | -0.03% | neutral |
| short-window | sudden_outage | 173 | 307.890143ms | 308.543702ms | 29.101676ms | 29.535836ms | 0.673 | 0.673 | 1.2s | 3 | 300 | not detected | 1 | 0.002 | -0.11% | neutral |
| short-window | capacity_aggregation | 11 | 1m19.876239452s | 7.330567273s | 25ms | 28ms | 0.988 | 0.863 | 0s | 0 | 284 | 1.125s | 116 | 0.006 | 89.52% | win |
| short-window | capacity_aggregation | 29 | 1m19.971533361s | 6.294367405s | 25ms | 28ms | 0.988 | 0.847 | 0s | 0 | 277 | 1.125s | 109 | 0.006 | 90.70% | win |
| short-window | capacity_aggregation | 47 | 1m19.89581372s | 6.315259432s | 25ms | 28ms | 0.988 | 0.839 | 0s | 0 | 280 | 1.125s | 105 | 0.006 | 90.71% | win |
| short-window | capacity_aggregation | 71 | 1m19.767790605s | 6.211246974s | 25ms | 28ms | 0.988 | 0.844 | 0s | 0 | 287 | 1.125s | 109 | 0.006 | 90.81% | win |
| short-window | capacity_aggregation | 101 | 1m20.040921681s | 7.429708966s | 25ms | 28ms | 0.990 | 0.878 | 0s | 0 | 285 | 1.125s | 121 | 0.006 | 89.31% | win |
| short-window | capacity_aggregation | 131 | 1m19.734245977s | 6.40461911s | 25ms | 28ms | 0.988 | 0.840 | 0s | 0 | 276 | 1.125s | 107 | 0.006 | 90.58% | win |
| short-window | capacity_aggregation | 173 | 1m19.865475022s | 6.35882459s | 25ms | 28ms | 0.988 | 0.840 | 0s | 0 | 283 | 1.125s | 108 | 0.006 | 90.66% | win |
| short-window | transient_spike | 11 | 12.6925426s | 12.6925426s | 90ms | 90ms | 0.396 | 0.396 | 4.4s | 5 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | transient_spike | 29 | 12.307633594s | 12.307633594s | 90ms | 90ms | 0.384 | 0.384 | 4.4s | 5 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | transient_spike | 47 | 12.854235214s | 12.854235214s | 90ms | 90ms | 0.402 | 0.402 | 4.6s | 6 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | transient_spike | 71 | 12.659328791s | 12.659328791s | 90ms | 90ms | 0.400 | 0.400 | 4.4s | 4 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | transient_spike | 101 | 12.607221347s | 12.607221347s | 90ms | 90ms | 0.398 | 0.398 | 4.2s | 4 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | transient_spike | 131 | 12.512324827s | 12.512324827s | 90ms | 90ms | 0.398 | 0.398 | 4.4s | 5 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | transient_spike | 173 | 12.652228169s | 12.652228169s | 90ms | 90ms | 0.398 | 0.398 | 1.2s | 3 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | stale_candidate | 11 | 46.540000047s | 40.676666713s | 110ms | 110ms | 0.889 | 0.836 | 9.99999999s | 12 | 142 | not detected | 19 | 0.058 | 13.23% | neutral |
| short-window | stale_candidate | 29 | 51.736666713s | 46.736666712s | 110ms | 110ms | 0.883 | 0.830 | 9.666666657s | 9 | 142 | not detected | 19 | 0.058 | 11.12% | neutral |
| short-window | stale_candidate | 47 | 51.540000046s | 45.873333379s | 110ms | 110ms | 0.883 | 0.830 | 9.333333324s | 9 | 142 | not detected | 19 | 0.058 | 11.37% | neutral |
| short-window | stale_candidate | 71 | 50.403333377s | 44.540000045s | 110ms | 110ms | 0.889 | 0.836 | 9.666666657s | 10 | 142 | not detected | 19 | 0.058 | 13.08% | neutral |
| short-window | stale_candidate | 101 | 48.010000047s | 41.010000046s | 110ms | 110ms | 0.883 | 0.830 | 9.666666657s | 8 | 142 | not detected | 19 | 0.058 | 14.79% | neutral |
| short-window | stale_candidate | 131 | 47.873333381s | 43.010000048s | 110ms | 110ms | 0.883 | 0.830 | 9.666666657s | 10 | 142 | not detected | 19 | 0.058 | 10.95% | neutral |
| short-window | stale_candidate | 173 | 46.87333338s | 41.146666712s | 110ms | 110ms | 0.883 | 0.830 | 9.666666657s | 8 | 142 | not detected | 19 | 0.058 | 13.27% | neutral |
| short-window | recovery_no_flap | 11 | 44.799011002s | 44.799011002s | 95ms | 95ms | 0.752 | 0.752 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | recovery_no_flap | 29 | 44.937203373s | 44.937203373s | 95ms | 95ms | 0.750 | 0.750 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | recovery_no_flap | 47 | 44.810457119s | 44.810457119s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | recovery_no_flap | 71 | 44.958361864s | 44.958361864s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | recovery_no_flap | 101 | 44.837077225s | 44.837077225s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | recovery_no_flap | 131 | 44.901274352s | 44.901274352s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | recovery_no_flap | 173 | 44.9015634s | 44.9015634s | 95ms | 95ms | 0.755 | 0.755 | 1.2s | 3 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | all_channels_bad | 11 | 2m10.9s | 2m10.9s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | all_channels_bad | 29 | 2m9.8s | 2m9.8s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | all_channels_bad | 47 | 2m5.2s | 2m5.2s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | all_channels_bad | 71 | 2m9.4s | 2m9.4s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | all_channels_bad | 101 | 2m5.7s | 2m5.7s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | all_channels_bad | 131 | 2m5.1s | 2m5.1s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | all_channels_bad | 173 | 2m9.3s | 2m9.3s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | not detected | 0 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| short-window | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | not detected | 0 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| short-window | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | not detected | 0 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| short-window | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | not detected | 0 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| short-window | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | not detected | 0 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| short-window | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | not detected | 0 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| short-window | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | not detected | 0 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| stable-recovery | gradual_degradation | 11 | 2m18.05810847s | 2m18.05810847s | 111.638008ms | 111.638008ms | 0.738 | 0.738 | 2.2s | 6 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | gradual_degradation | 29 | 2m17.230127598s | 2m17.230127598s | 111.973508ms | 111.973508ms | 0.737 | 0.737 | 2.6s | 7 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | gradual_degradation | 47 | 2m18.543146187s | 2m18.543146187s | 112.017461ms | 112.017461ms | 0.743 | 0.743 | 2.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | gradual_degradation | 71 | 2m18.842157616s | 2m18.842157616s | 112.679273ms | 112.679273ms | 0.742 | 0.742 | 2.4s | 7 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | gradual_degradation | 101 | 2m17.896601353s | 2m17.896601353s | 112.288179ms | 112.288179ms | 0.740 | 0.740 | 2.2s | 7 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| stable-recovery | gradual_degradation | 131 | 2m18.257709744s | 2m18.257709744s | 112.50077ms | 112.50077ms | 0.742 | 0.742 | 2.2s | 7 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | gradual_degradation | 173 | 2m18.511286942s | 2m18.511286942s | 112.241073ms | 112.241073ms | 0.740 | 0.740 | 1.4s | 4 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | sudden_outage | 11 | 306.854065ms | 307.058392ms | 29.758413ms | 29.775736ms | 0.669 | 0.669 | 2s | 5 | 300 | not detected | 1 | 0.002 | -0.01% | neutral |
| stable-recovery | sudden_outage | 29 | 305.736046ms | 305.736046ms | 29.586311ms | 29.812279ms | 0.667 | 0.667 | 2s | 5 | 300 | not detected | 1 | 0.002 | -0.05% | neutral |
| stable-recovery | sudden_outage | 47 | 306.757116ms | 307.058184ms | 29.297237ms | 29.386626ms | 0.676 | 0.676 | 2.4s | 7 | 300 | not detected | 1 | 0.002 | -0.02% | neutral |
| stable-recovery | sudden_outage | 71 | 306.766218ms | 308.348868ms | 29.362123ms | 29.436265ms | 0.673 | 0.673 | 2s | 4 | 300 | not detected | 1 | 0.002 | -0.04% | neutral |
| stable-recovery | sudden_outage | 101 | 306.026121ms | 306.150606ms | 29.696734ms | 29.751595ms | 0.673 | 0.673 | 2s | 4 | 300 | not detected | 3 | 0.004 | -0.02% | neutral |
| stable-recovery | sudden_outage | 131 | 308.156379ms | 308.545061ms | 29.458433ms | 29.570855ms | 0.676 | 0.676 | 2s | 5 | 300 | not detected | 1 | 0.002 | -0.03% | neutral |
| stable-recovery | sudden_outage | 173 | 307.890143ms | 308.543702ms | 29.101676ms | 29.535836ms | 0.673 | 0.673 | 1.2s | 3 | 300 | not detected | 1 | 0.002 | -0.11% | neutral |
| stable-recovery | capacity_aggregation | 11 | 1m19.876239452s | 11.608491214s | 25ms | 28ms | 0.988 | 0.910 | 0s | 0 | 266 | 3.125s | 60 | 0.007 | 84.13% | win |
| stable-recovery | capacity_aggregation | 29 | 1m19.971533361s | 11.641781135s | 25ms | 28ms | 0.988 | 0.906 | 0s | 0 | 265 | 3.125s | 64 | 0.007 | 84.17% | win |
| stable-recovery | capacity_aggregation | 47 | 1m19.89581372s | 11.553297966s | 25ms | 28ms | 0.988 | 0.907 | 0s | 0 | 263 | 3.125s | 64 | 0.007 | 84.22% | win |
| stable-recovery | capacity_aggregation | 71 | 1m19.767790605s | 11.583083191s | 25ms | 28ms | 0.988 | 0.907 | 0s | 0 | 265 | 3.125s | 60 | 0.007 | 84.16% | win |
| stable-recovery | capacity_aggregation | 101 | 1m20.040921681s | 11.604046773s | 25ms | 28ms | 0.990 | 0.912 | 0s | 0 | 267 | 3.125s | 62 | 0.007 | 84.16% | win |
| stable-recovery | capacity_aggregation | 131 | 1m19.734245977s | 11.596831874s | 25ms | 28ms | 0.988 | 0.910 | 0s | 0 | 265 | 3.125s | 62 | 0.007 | 84.12% | win |
| stable-recovery | capacity_aggregation | 173 | 1m19.865475022s | 11.644458259s | 25ms | 28ms | 0.988 | 0.906 | 0s | 0 | 267 | 3.125s | 64 | 0.007 | 84.13% | win |
| stable-recovery | transient_spike | 11 | 12.6925426s | 12.6925426s | 90ms | 90ms | 0.396 | 0.396 | 4.4s | 5 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | transient_spike | 29 | 12.307633594s | 12.307633594s | 90ms | 90ms | 0.384 | 0.384 | 4.4s | 5 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | transient_spike | 47 | 12.854235214s | 12.854235214s | 90ms | 90ms | 0.402 | 0.402 | 4.6s | 6 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | transient_spike | 71 | 12.659328791s | 12.659328791s | 90ms | 90ms | 0.400 | 0.400 | 4.4s | 4 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | transient_spike | 101 | 12.607221347s | 12.607221347s | 90ms | 90ms | 0.398 | 0.398 | 4.2s | 4 | 300 | not detected | 3 | 0.004 | -0.00% | neutral |
| stable-recovery | transient_spike | 131 | 12.512324827s | 12.512324827s | 90ms | 90ms | 0.398 | 0.398 | 4.4s | 5 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | transient_spike | 173 | 12.652228169s | 12.652228169s | 90ms | 90ms | 0.398 | 0.398 | 1.2s | 3 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | stale_candidate | 11 | 46.540000047s | 29.83000003s | 110ms | 110ms | 0.889 | 0.579 | 9.99999999s | 12 | 97 | 36.66666663s | 14 | 0.035 | 39.90% | win |
| stable-recovery | stale_candidate | 29 | 51.736666713s | 31.890000031s | 110ms | 110ms | 0.883 | 0.567 | 9.666666657s | 9 | 96 | 36.333333297s | 14 | 0.035 | 42.94% | win |
| stable-recovery | stale_candidate | 47 | 51.540000046s | 31.693333362s | 110ms | 110ms | 0.883 | 0.567 | 9.99999999s | 10 | 96 | 36.333333297s | 14 | 0.035 | 43.06% | win |
| stable-recovery | stale_candidate | 71 | 50.403333377s | 31.693333364s | 110ms | 110ms | 0.889 | 0.579 | 10.333333323s | 11 | 97 | 36.66666663s | 14 | 0.035 | 41.96% | win |
| stable-recovery | stale_candidate | 101 | 48.010000047s | 32.890000034s | 110ms | 110ms | 0.883 | 0.649 | 9.99999999s | 9 | 108 | 40.666666626s | 16 | 0.041 | 35.09% | win |
| stable-recovery | stale_candidate | 131 | 47.873333381s | 28.163333363s | 110ms | 110ms | 0.883 | 0.567 | 9.666666657s | 10 | 96 | 36.333333297s | 14 | 0.035 | 45.05% | win |
| stable-recovery | stale_candidate | 173 | 46.87333338s | 29.693333364s | 110ms | 110ms | 0.883 | 0.585 | 9.99999999s | 9 | 98 | 36.999999963s | 14 | 0.035 | 40.29% | win |
| stable-recovery | recovery_no_flap | 11 | 44.799011002s | 44.799011002s | 95ms | 95ms | 0.752 | 0.752 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | recovery_no_flap | 29 | 44.937203373s | 44.937203373s | 95ms | 95ms | 0.750 | 0.750 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | recovery_no_flap | 47 | 44.810457119s | 44.810457119s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | recovery_no_flap | 71 | 44.958361864s | 44.958361864s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | recovery_no_flap | 101 | 44.837077225s | 44.837077225s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 3 | 0.003 | -0.00% | neutral |
| stable-recovery | recovery_no_flap | 131 | 44.901274352s | 44.901274352s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | recovery_no_flap | 173 | 44.9015634s | 44.9015634s | 95ms | 95ms | 0.755 | 0.755 | 1.2s | 3 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | all_channels_bad | 11 | 2m10.9s | 2m10.9s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | all_channels_bad | 29 | 2m9.8s | 2m9.8s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | all_channels_bad | 47 | 2m5.2s | 2m5.2s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | all_channels_bad | 71 | 2m9.4s | 2m9.4s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | all_channels_bad | 101 | 2m5.7s | 2m5.7s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 3 | 0.004 | -0.00% | neutral |
| stable-recovery | all_channels_bad | 131 | 2m5.1s | 2m5.1s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | all_channels_bad | 173 | 2m9.3s | 2m9.3s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| stable-recovery | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| stable-recovery | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| stable-recovery | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| stable-recovery | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| stable-recovery | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| stable-recovery | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| fast-recovery | gradual_degradation | 11 | 2m18.05810847s | 2m18.05810847s | 111.638008ms | 111.638008ms | 0.738 | 0.738 | 2.2s | 6 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | gradual_degradation | 29 | 2m17.230127598s | 2m17.230127598s | 111.973508ms | 111.973508ms | 0.737 | 0.737 | 2.4s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | gradual_degradation | 47 | 2m18.543146187s | 2m18.543146187s | 112.017461ms | 112.017461ms | 0.743 | 0.743 | 2.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | gradual_degradation | 71 | 2m18.842157616s | 2m18.842157616s | 112.679273ms | 112.679273ms | 0.742 | 0.742 | 2.4s | 7 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | gradual_degradation | 101 | 2m17.896601353s | 2m17.896601353s | 112.288179ms | 112.288179ms | 0.740 | 0.740 | 2.2s | 7 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | gradual_degradation | 131 | 2m18.257709744s | 2m18.257709744s | 112.50077ms | 112.50077ms | 0.742 | 0.742 | 2.2s | 7 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | gradual_degradation | 173 | 2m18.511286942s | 2m18.511286942s | 112.241073ms | 112.241073ms | 0.740 | 0.740 | 1.4s | 4 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | sudden_outage | 11 | 306.854065ms | 307.058392ms | 29.758413ms | 29.775736ms | 0.669 | 0.669 | 2s | 5 | 300 | not detected | 1 | 0.002 | -0.01% | neutral |
| fast-recovery | sudden_outage | 29 | 305.736046ms | 306.900343ms | 29.586311ms | 29.812279ms | 0.667 | 0.667 | 2s | 5 | 300 | not detected | 1 | 0.002 | -0.07% | neutral |
| fast-recovery | sudden_outage | 47 | 306.757116ms | 307.058184ms | 29.297237ms | 29.386626ms | 0.676 | 0.676 | 2.4s | 7 | 300 | not detected | 1 | 0.002 | -0.02% | neutral |
| fast-recovery | sudden_outage | 71 | 306.766218ms | 308.348868ms | 29.362123ms | 29.436265ms | 0.673 | 0.673 | 2s | 4 | 300 | not detected | 1 | 0.002 | -0.04% | neutral |
| fast-recovery | sudden_outage | 101 | 306.026121ms | 306.150606ms | 29.696734ms | 29.751595ms | 0.673 | 0.673 | 2s | 4 | 300 | not detected | 1 | 0.002 | -0.01% | neutral |
| fast-recovery | sudden_outage | 131 | 308.156379ms | 308.545061ms | 29.458433ms | 29.570855ms | 0.676 | 0.676 | 2s | 5 | 300 | not detected | 1 | 0.002 | -0.03% | neutral |
| fast-recovery | sudden_outage | 173 | 307.890143ms | 308.543702ms | 29.101676ms | 29.535836ms | 0.673 | 0.673 | 1.2s | 3 | 300 | not detected | 1 | 0.002 | -0.11% | neutral |
| fast-recovery | capacity_aggregation | 11 | 1m19.876239452s | 29.146310126s | 25ms | 28ms | 0.988 | 0.942 | 0s | 0 | 291 | 1.125s | 24 | 0.006 | 65.16% | win |
| fast-recovery | capacity_aggregation | 29 | 1m19.971533361s | 29.931097787s | 25ms | 28ms | 0.988 | 0.940 | 0s | 0 | 314 | 1.125s | 64 | 0.006 | 64.88% | win |
| fast-recovery | capacity_aggregation | 47 | 1m19.89581372s | 29.244526898s | 25ms | 28ms | 0.988 | 0.940 | 0s | 0 | 310 | 1.125s | 63 | 0.006 | 65.47% | win |
| fast-recovery | capacity_aggregation | 71 | 1m19.767790605s | 29.096476904s | 25ms | 28ms | 0.988 | 0.939 | 0s | 0 | 291 | 1.125s | 24 | 0.006 | 65.21% | win |
| fast-recovery | capacity_aggregation | 101 | 1m20.040921681s | 29.812563477s | 25ms | 28ms | 0.990 | 0.944 | 0s | 0 | 314 | 1.125s | 64 | 0.006 | 65.04% | win |
| fast-recovery | capacity_aggregation | 131 | 1m19.734245977s | 29.698943803s | 25ms | 28ms | 0.988 | 0.943 | 0s | 0 | 314 | 1.125s | 64 | 0.006 | 64.96% | win |
| fast-recovery | capacity_aggregation | 173 | 1m19.865475022s | 29.731350675s | 25ms | 28ms | 0.988 | 0.940 | 0s | 0 | 316 | 1.125s | 65 | 0.006 | 65.02% | win |
| fast-recovery | transient_spike | 11 | 12.6925426s | 12.6925426s | 90ms | 90ms | 0.396 | 0.396 | 4.4s | 5 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | transient_spike | 29 | 12.307633594s | 12.307633594s | 90ms | 90ms | 0.384 | 0.384 | 4.4s | 5 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | transient_spike | 47 | 12.854235214s | 12.854235214s | 90ms | 90ms | 0.402 | 0.402 | 4.6s | 6 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | transient_spike | 71 | 12.659328791s | 12.659328791s | 90ms | 90ms | 0.400 | 0.400 | 4.4s | 4 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | transient_spike | 101 | 12.607221347s | 12.607221347s | 90ms | 90ms | 0.398 | 0.398 | 4.2s | 4 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | transient_spike | 131 | 12.512324827s | 12.512324827s | 90ms | 90ms | 0.398 | 0.398 | 4.4s | 5 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | transient_spike | 173 | 12.652228169s | 12.652228169s | 90ms | 90ms | 0.398 | 0.398 | 1.2s | 3 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | stale_candidate | 11 | 46.540000047s | 17.376666683s | 110ms | 110ms | 0.889 | 0.386 | 9.99999999s | 12 | 63 | 24.999999975s | 10 | 0.029 | 65.57% | win |
| fast-recovery | stale_candidate | 29 | 51.736666713s | 19.376666683s | 110ms | 110ms | 0.883 | 0.368 | 9.666666657s | 9 | 61 | 24.333333309s | 10 | 0.029 | 66.32% | win |
| fast-recovery | stale_candidate | 47 | 51.540000046s | 19.04333335s | 110ms | 110ms | 0.883 | 0.398 | 9.333333324s | 9 | 64 | 25.333333308s | 12 | 0.029 | 66.51% | win |
| fast-recovery | stale_candidate | 71 | 50.403333377s | 17.376666683s | 110ms | 110ms | 0.889 | 0.380 | 9.666666657s | 10 | 63 | 24.999999975s | 10 | 0.029 | 68.42% | win |
| fast-recovery | stale_candidate | 101 | 48.010000047s | 17.846666684s | 110ms | 110ms | 0.883 | 0.398 | 9.666666657s | 8 | 64 | 24.999999975s | 12 | 0.029 | 65.87% | win |
| fast-recovery | stale_candidate | 131 | 47.873333381s | 16.710000017s | 110ms | 110ms | 0.883 | 0.392 | 9.666666657s | 10 | 63 | 24.999999975s | 10 | 0.029 | 67.44% | win |
| fast-recovery | stale_candidate | 173 | 46.87333338s | 16.846666685s | 110ms | 110ms | 0.883 | 0.392 | 9.666666657s | 8 | 64 | 24.999999975s | 12 | 0.029 | 66.64% | win |
| fast-recovery | recovery_no_flap | 11 | 44.799011002s | 44.799011002s | 95ms | 95ms | 0.752 | 0.752 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | recovery_no_flap | 29 | 44.937203373s | 44.937203373s | 95ms | 95ms | 0.750 | 0.750 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | recovery_no_flap | 47 | 44.810457119s | 44.810457119s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | recovery_no_flap | 71 | 44.958361864s | 44.958361864s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | recovery_no_flap | 101 | 44.837077225s | 44.837077225s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | recovery_no_flap | 131 | 44.901274352s | 44.901274352s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | recovery_no_flap | 173 | 44.9015634s | 44.9015634s | 95ms | 95ms | 0.755 | 0.755 | 1.2s | 3 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | all_channels_bad | 11 | 2m10.9s | 2m10.9s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | all_channels_bad | 29 | 2m9.8s | 2m9.8s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | all_channels_bad | 47 | 2m5.2s | 2m5.2s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | all_channels_bad | 71 | 2m9.4s | 2m9.4s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | all_channels_bad | 101 | 2m5.7s | 2m5.7s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | all_channels_bad | 131 | 2m5.1s | 2m5.1s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | all_channels_bad | 173 | 2m9.3s | 2m9.3s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| fast-recovery | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| fast-recovery | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| fast-recovery | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| fast-recovery | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| fast-recovery | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| fast-recovery | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| hard-failure-confirmed | gradual_degradation | 11 | 2m18.05810847s | 2m18.05810847s | 111.638008ms | 111.638008ms | 0.738 | 0.738 | 2.2s | 6 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | gradual_degradation | 29 | 2m17.230127598s | 2m17.230127598s | 111.973508ms | 111.973508ms | 0.737 | 0.737 | 2.4s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | gradual_degradation | 47 | 2m18.543146187s | 2m18.543146187s | 112.017461ms | 112.017461ms | 0.743 | 0.743 | 2.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | gradual_degradation | 71 | 2m18.842157616s | 2m18.842157616s | 112.679273ms | 112.679273ms | 0.742 | 0.742 | 2.4s | 7 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | gradual_degradation | 101 | 2m17.896601353s | 2m17.896601353s | 112.288179ms | 112.288179ms | 0.740 | 0.740 | 2.2s | 7 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | gradual_degradation | 131 | 2m18.257709744s | 2m18.257709744s | 112.50077ms | 112.50077ms | 0.742 | 0.742 | 2.2s | 7 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | gradual_degradation | 173 | 2m18.511286942s | 2m18.511286942s | 112.241073ms | 112.241073ms | 0.740 | 0.740 | 1.4s | 4 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | sudden_outage | 11 | 306.854065ms | 307.058392ms | 29.758413ms | 29.775736ms | 0.669 | 0.669 | 2.6s | 8 | 300 | not detected | 1 | 0.002 | -0.01% | neutral |
| hard-failure-confirmed | sudden_outage | 29 | 305.736046ms | 305.736046ms | 29.586311ms | 29.812279ms | 0.667 | 0.667 | 2.2s | 6 | 300 | not detected | 1 | 0.002 | -0.05% | neutral |
| hard-failure-confirmed | sudden_outage | 47 | 306.757116ms | 307.058184ms | 29.297237ms | 29.386626ms | 0.676 | 0.676 | 2.6s | 8 | 300 | not detected | 1 | 0.002 | -0.02% | neutral |
| hard-failure-confirmed | sudden_outage | 71 | 306.766218ms | 308.348868ms | 29.362123ms | 29.436265ms | 0.673 | 0.673 | 2.2s | 5 | 300 | not detected | 1 | 0.002 | -0.04% | neutral |
| hard-failure-confirmed | sudden_outage | 101 | 306.026121ms | 306.150606ms | 29.696734ms | 29.696734ms | 0.673 | 0.673 | 2.2s | 5 | 300 | not detected | 1 | 0.002 | -0.00% | neutral |
| hard-failure-confirmed | sudden_outage | 131 | 308.156379ms | 308.545061ms | 29.458433ms | 29.570855ms | 0.676 | 0.676 | 2.2s | 6 | 300 | not detected | 1 | 0.002 | -0.03% | neutral |
| hard-failure-confirmed | sudden_outage | 173 | 307.890143ms | 308.543702ms | 29.101676ms | 29.535836ms | 0.673 | 0.673 | 1.2s | 3 | 300 | not detected | 1 | 0.002 | -0.11% | neutral |
| hard-failure-confirmed | capacity_aggregation | 11 | 1m19.876239452s | 36.900507488s | 25ms | 28ms | 0.988 | 0.943 | 0s | 0 | 360 | 1.125s | 28 | 0.006 | 57.72% | win |
| hard-failure-confirmed | capacity_aggregation | 29 | 1m19.971533361s | 37.053290928s | 25ms | 28ms | 0.988 | 0.944 | 0s | 0 | 361 | 1.125s | 26 | 0.006 | 57.55% | win |
| hard-failure-confirmed | capacity_aggregation | 47 | 1m19.89581372s | 37.042298517s | 25ms | 28ms | 0.988 | 0.944 | 0s | 0 | 360 | 1.125s | 28 | 0.006 | 57.60% | win |
| hard-failure-confirmed | capacity_aggregation | 71 | 1m19.767790605s | 35.544563155s | 25ms | 28ms | 0.988 | 0.944 | 0s | 0 | 347 | 1.125s | 22 | 0.006 | 59.06% | win |
| hard-failure-confirmed | capacity_aggregation | 101 | 1m20.040921681s | 35.166703648s | 25ms | 28ms | 0.990 | 0.946 | 0s | 0 | 342 | 1.125s | 22 | 0.006 | 59.58% | win |
| hard-failure-confirmed | capacity_aggregation | 131 | 1m19.734245977s | 31.202906034s | 25ms | 28ms | 0.988 | 0.943 | 0s | 0 | 308 | 1.125s | 22 | 0.006 | 63.28% | win |
| hard-failure-confirmed | capacity_aggregation | 173 | 1m19.865475022s | 36.718494834s | 25ms | 28ms | 0.988 | 0.944 | 0s | 0 | 360 | 1.125s | 28 | 0.006 | 57.85% | win |
| hard-failure-confirmed | transient_spike | 11 | 12.6925426s | 12.6925426s | 90ms | 90ms | 0.396 | 0.396 | 4.4s | 5 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | transient_spike | 29 | 12.307633594s | 12.307633594s | 90ms | 90ms | 0.384 | 0.384 | 4.4s | 5 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | transient_spike | 47 | 12.854235214s | 12.854235214s | 90ms | 90ms | 0.402 | 0.402 | 4.6s | 6 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | transient_spike | 71 | 12.659328791s | 12.659328791s | 90ms | 90ms | 0.400 | 0.400 | 4.4s | 4 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | transient_spike | 101 | 12.607221347s | 12.607221347s | 90ms | 90ms | 0.398 | 0.398 | 4.2s | 4 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | transient_spike | 131 | 12.512324827s | 12.512324827s | 90ms | 90ms | 0.398 | 0.398 | 4.4s | 5 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | transient_spike | 173 | 12.652228169s | 12.652228169s | 90ms | 90ms | 0.398 | 0.398 | 1.2s | 3 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | stale_candidate | 11 | 46.540000047s | 23.966666696s | 110ms | 110ms | 0.889 | 0.474 | 9.99999999s | 12 | 78 | 29.99999997s | 12 | 0.029 | 52.73% | win |
| hard-failure-confirmed | stale_candidate | 29 | 51.736666713s | 24.966666689s | 110ms | 110ms | 0.883 | 0.474 | 9.666666657s | 9 | 78 | 29.666666637s | 12 | 0.029 | 56.01% | win |
| hard-failure-confirmed | stale_candidate | 47 | 51.540000046s | 23.770000023s | 110ms | 110ms | 0.883 | 0.468 | 9.333333324s | 9 | 78 | 29.666666637s | 12 | 0.029 | 57.91% | win |
| hard-failure-confirmed | stale_candidate | 71 | 50.403333377s | 38.283333369s | 110ms | 110ms | 0.889 | 0.690 | 9.666666657s | 10 | 116 | 43.33333329s | 16 | 0.041 | 27.85% | win |
| hard-failure-confirmed | stale_candidate | 101 | 48.010000047s | 22.633333362s | 110ms | 110ms | 0.883 | 0.462 | 9.99999999s | 9 | 76 | 28.999999971s | 12 | 0.029 | 56.67% | win |
| hard-failure-confirmed | stale_candidate | 131 | 47.873333381s | 22.770000022s | 110ms | 110ms | 0.883 | 0.480 | 9.666666657s | 10 | 78 | 29.666666637s | 12 | 0.029 | 56.04% | win |
| hard-failure-confirmed | stale_candidate | 173 | 46.87333338s | 23.103333357s | 110ms | 110ms | 0.883 | 0.468 | 9.99999999s | 9 | 78 | 29.99999997s | 12 | 0.029 | 54.53% | win |
| hard-failure-confirmed | recovery_no_flap | 11 | 44.799011002s | 44.799011002s | 95ms | 95ms | 0.752 | 0.752 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | recovery_no_flap | 29 | 44.937203373s | 44.937203373s | 95ms | 95ms | 0.750 | 0.750 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | recovery_no_flap | 47 | 44.810457119s | 44.810457119s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | recovery_no_flap | 71 | 44.958361864s | 44.958361864s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | recovery_no_flap | 101 | 44.837077225s | 44.837077225s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | recovery_no_flap | 131 | 44.901274352s | 44.901274352s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | recovery_no_flap | 173 | 44.9015634s | 44.9015634s | 95ms | 95ms | 0.755 | 0.755 | 1.2s | 3 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | all_channels_bad | 11 | 2m10.9s | 2m10.9s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | all_channels_bad | 29 | 2m9.8s | 2m9.8s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | all_channels_bad | 47 | 2m5.2s | 2m5.2s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | all_channels_bad | 71 | 2m9.4s | 2m9.4s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | all_channels_bad | 101 | 2m5.7s | 2m5.7s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | all_channels_bad | 131 | 2m5.1s | 2m5.1s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | all_channels_bad | 173 | 2m9.3s | 2m9.3s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| hard-failure-confirmed | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| hard-failure-confirmed | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| hard-failure-confirmed | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| hard-failure-confirmed | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| hard-failure-confirmed | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| hard-failure-confirmed | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
