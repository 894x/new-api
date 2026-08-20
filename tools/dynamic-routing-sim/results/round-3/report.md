# Dynamic routing simulation

Significant improvement gate: 15.00%; maximum regression: 8.00%; minimum scenario win rate: 0.45.

## Ranking

| Rank | Parameters | Average improvement | Worst seed | Win rate | Significant | Regressed runs |
|---:|---|---:|---:|---:|:---:|---:|
| 1 | fast-recovery | 24.53% | -0.06% | 0.33 | false | 0 |
| 2 | stable-recovery | 24.04% | -0.68% | 0.33 | false | 0 |
| 3 | hard-failure-confirmed | 23.28% | -0.68% | 0.33 | false | 0 |
| 4 | balanced | 21.17% | -0.68% | 0.33 | false | 0 |
| 5 | short-window | 20.97% | -0.06% | 0.22 | false | 0 |
| 6 | aggressive | 19.76% | -0.06% | 0.33 | false | 0 |
| 7 | conservative | 17.89% | -0.74% | 0.22 | false | 1 |
| 8 | fast-low-probe | 15.39% | -0.68% | 0.22 | false | 0 |

## Every scenario and seed

| Parameters | Scenario | Seed | Static p95 TTFT | Dynamic p95 TTFT | Static p95 TPOT | Dynamic p95 TPOT | Static SLO violations | Dynamic SLO violations | Detection elapsed | Detection observations | Bad exposure | Mitigation elapsed | Route reversals | Probe cost | Improvement | Result |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| conservative | gradual_degradation | 11 | 2m18.05810847s | 2m16.790178167s | 111.638008ms | 111.638008ms | 0.738 | 0.735 | 2.4s | 8 | 447 | not detected | 0 | 0.008 | 1.12% | neutral |
| conservative | gradual_degradation | 29 | 2m17.230127598s | 2m16.401289205s | 111.973508ms | 111.973508ms | 0.737 | 0.733 | 2.6s | 7 | 448 | not detected | 0 | 0.007 | 0.71% | neutral |
| conservative | gradual_degradation | 47 | 2m18.543146187s | 2m17.031311386s | 112.017461ms | 111.887792ms | 0.743 | 0.738 | 2.4s | 6 | 447 | not detected | 0 | 0.008 | 1.23% | neutral |
| conservative | gradual_degradation | 71 | 2m18.842157616s | 2m17.860558027s | 112.679273ms | 112.679273ms | 0.742 | 0.738 | 2.4s | 7 | 448 | not detected | 0 | 0.008 | 0.88% | neutral |
| conservative | gradual_degradation | 101 | 2m17.896601353s | 2m17.896601353s | 112.288179ms | 112.288179ms | 0.740 | 0.742 | 2.2s | 7 | 450 | not detected | 0 | 0.003 | -0.00% | neutral |
| conservative | gradual_degradation | 131 | 2m18.257709744s | 2m16.347484138s | 112.50077ms | 112.50077ms | 0.742 | 0.738 | 2.2s | 7 | 447 | not detected | 0 | 0.008 | 1.44% | neutral |
| conservative | gradual_degradation | 173 | 2m18.511286942s | 2m17.075365079s | 112.241073ms | 112.241073ms | 0.740 | 0.735 | 1.4s | 4 | 447 | not detected | 0 | 0.008 | 1.17% | neutral |
| conservative | sudden_outage | 11 | 306.854065ms | 473.499055ms | 29.758413ms | 39.982664ms | 0.669 | 0.044 | 2.6s | 8 | 13 | 4.2s | 0 | 0.004 | 80.19% | win |
| conservative | sudden_outage | 29 | 305.736046ms | 471.487188ms | 29.586311ms | 40.071145ms | 0.667 | 0.038 | 2.2s | 6 | 11 | 3.8s | 0 | 0.004 | 80.86% | win |
| conservative | sudden_outage | 47 | 306.757116ms | 473.153942ms | 29.297237ms | 39.815778ms | 0.676 | 0.049 | 2.6s | 8 | 13 | 4.2s | 0 | 0.004 | 80.09% | win |
| conservative | sudden_outage | 71 | 306.766218ms | 472.534043ms | 29.362123ms | 40.078569ms | 0.673 | 0.056 | 2.2s | 5 | 11 | 3.8s | 0 | 0.007 | 80.11% | win |
| conservative | sudden_outage | 101 | 306.026121ms | 478.535184ms | 29.696734ms | 40.316143ms | 0.673 | 0.069 | 2.2s | 5 | 16 | 3.8s | 0 | 0.004 | 78.40% | win |
| conservative | sudden_outage | 131 | 308.156379ms | 477.080917ms | 29.458433ms | 40.174049ms | 0.676 | 0.060 | 2.2s | 6 | 11 | 3.8s | 0 | 0.004 | 79.87% | win |
| conservative | sudden_outage | 173 | 307.890143ms | 472.362573ms | 29.101676ms | 40.039804ms | 0.673 | 0.047 | 1.2s | 3 | 11 | 3.8s | 0 | 0.004 | 80.51% | win |
| conservative | capacity_aggregation | 11 | 1m19.876239452s | 26.065736129s | 25ms | 28ms | 0.988 | 0.942 | 0s | 0 | 333 | not detected | 0 | 0.011 | 68.30% | win |
| conservative | capacity_aggregation | 29 | 1m19.971533361s | 26.059618083s | 25ms | 28ms | 0.988 | 0.944 | 0s | 0 | 333 | not detected | 0 | 0.011 | 68.35% | win |
| conservative | capacity_aggregation | 47 | 1m19.89581372s | 26.05044383s | 25ms | 28ms | 0.988 | 0.942 | 0s | 0 | 333 | not detected | 0 | 0.011 | 68.32% | win |
| conservative | capacity_aggregation | 71 | 1m19.767790605s | 26.262468775s | 25ms | 28ms | 0.988 | 0.943 | 0s | 0 | 334 | not detected | 0 | 0.011 | 68.05% | win |
| conservative | capacity_aggregation | 101 | 1m20.040921681s | 26.245578616s | 25ms | 28ms | 0.990 | 0.943 | 0s | 0 | 333 | not detected | 0 | 0.011 | 68.19% | win |
| conservative | capacity_aggregation | 131 | 1m19.734245977s | 26.100518023s | 25ms | 28ms | 0.988 | 0.943 | 0s | 0 | 334 | not detected | 0 | 0.011 | 68.18% | win |
| conservative | capacity_aggregation | 173 | 1m19.865475022s | 26.354953554s | 25ms | 28ms | 0.988 | 0.943 | 0s | 0 | 334 | not detected | 0 | 0.011 | 67.97% | win |
| conservative | transient_spike | 11 | 12.6925426s | 11.760379494s | 90ms | 90ms | 0.396 | 0.376 | 4.4s | 5 | 298 | not detected | 0 | 0.009 | 7.08% | neutral |
| conservative | transient_spike | 29 | 12.307633594s | 12.307633594s | 90ms | 90ms | 0.384 | 0.380 | 4.4s | 5 | 299 | not detected | 0 | 0.007 | 0.33% | neutral |
| conservative | transient_spike | 47 | 12.854235214s | 11.986753307s | 90ms | 90ms | 0.402 | 0.382 | 4.6s | 6 | 298 | not detected | 0 | 0.009 | 6.19% | neutral |
| conservative | transient_spike | 71 | 12.659328791s | 12.659328791s | 90ms | 90ms | 0.400 | 0.393 | 4.4s | 4 | 299 | not detected | 0 | 0.009 | 0.17% | neutral |
| conservative | transient_spike | 101 | 12.607221347s | 12.607221347s | 90ms | 90ms | 0.398 | 0.400 | 4.2s | 4 | 300 | not detected | 0 | 0.004 | -0.04% | neutral |
| conservative | transient_spike | 131 | 12.512324827s | 11.766566305s | 90ms | 90ms | 0.398 | 0.376 | 4.4s | 5 | 298 | not detected | 0 | 0.009 | 5.83% | neutral |
| conservative | transient_spike | 173 | 12.652228169s | 12.652228169s | 90ms | 90ms | 0.398 | 0.393 | 1.2s | 3 | 299 | not detected | 0 | 0.007 | 0.34% | neutral |
| conservative | stale_candidate | 11 | 46.540000047s | 44.206666714s | 110ms | 110ms | 0.889 | 0.860 | 10.333333323s | 13 | 146 | not detected | 0 | 0.035 | 6.21% | neutral |
| conservative | stale_candidate | 29 | 51.736666713s | 47.403333378s | 110ms | 110ms | 0.883 | 0.854 | 9.99999999s | 11 | 146 | not detected | 0 | 0.035 | 8.99% | regression |
| conservative | stale_candidate | 47 | 51.540000046s | 48.736666714s | 110ms | 110ms | 0.883 | 0.854 | 10.999999989s | 12 | 146 | not detected | 0 | 0.035 | 6.42% | neutral |
| conservative | stale_candidate | 71 | 50.403333377s | 47.736666711s | 110ms | 110ms | 0.889 | 0.860 | 11.333333322s | 13 | 146 | not detected | 0 | 0.035 | 6.66% | neutral |
| conservative | stale_candidate | 101 | 48.010000047s | 44.010000045s | 110ms | 110ms | 0.883 | 0.860 | 10.666666656s | 11 | 146 | not detected | 0 | 0.035 | 8.34% | neutral |
| conservative | stale_candidate | 131 | 47.873333381s | 44.87333338s | 110ms | 110ms | 0.883 | 0.854 | 10.333333323s | 12 | 146 | not detected | 0 | 0.035 | 6.69% | neutral |
| conservative | stale_candidate | 173 | 46.87333338s | 45.010000046s | 110ms | 110ms | 0.883 | 0.854 | 10.666666656s | 11 | 146 | not detected | 0 | 0.035 | 4.83% | neutral |
| conservative | recovery_no_flap | 11 | 44.799011002s | 44.406413079s | 95ms | 95ms | 0.752 | 0.747 | 4.2s | 5 | 447 | not detected | 0 | 0.008 | 1.31% | neutral |
| conservative | recovery_no_flap | 29 | 44.937203373s | 44.336363542s | 95ms | 95ms | 0.750 | 0.747 | 4.2s | 5 | 448 | not detected | 0 | 0.007 | 1.55% | neutral |
| conservative | recovery_no_flap | 47 | 44.810457119s | 44.299181422s | 95ms | 95ms | 0.757 | 0.752 | 4.2s | 5 | 447 | not detected | 0 | 0.008 | 1.48% | neutral |
| conservative | recovery_no_flap | 71 | 44.958361864s | 44.264532893s | 95ms | 95ms | 0.755 | 0.752 | 4.2s | 4 | 448 | not detected | 0 | 0.008 | 1.59% | neutral |
| conservative | recovery_no_flap | 101 | 44.837077225s | 44.837077225s | 95ms | 95ms | 0.755 | 0.757 | 4.2s | 4 | 450 | not detected | 0 | 0.003 | -0.01% | neutral |
| conservative | recovery_no_flap | 131 | 44.901274352s | 44.249072613s | 95ms | 95ms | 0.757 | 0.752 | 4.2s | 5 | 447 | not detected | 0 | 0.008 | 1.89% | neutral |
| conservative | recovery_no_flap | 173 | 44.9015634s | 44.264579752s | 95ms | 95ms | 0.755 | 0.752 | 1.2s | 3 | 447 | not detected | 0 | 0.008 | 1.78% | neutral |
| conservative | all_channels_bad | 11 | 2m10.9s | 2m9.1s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 0 | 0.009 | 1.45% | neutral |
| conservative | all_channels_bad | 29 | 2m9.8s | 2m8.2s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 0 | 0.009 | 1.32% | neutral |
| conservative | all_channels_bad | 47 | 2m5.2s | 2m3.5s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.009 | 1.44% | neutral |
| conservative | all_channels_bad | 71 | 2m9.4s | 2m8.1s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.009 | 0.95% | neutral |
| conservative | all_channels_bad | 101 | 2m5.7s | 2m5.7s | 100ms | 100ms | 0.673 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.004 | -0.00% | neutral |
| conservative | all_channels_bad | 131 | 2m5.1s | 2m4.1s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.009 | 1.02% | neutral |
| conservative | all_channels_bad | 173 | 2m9.3s | 2m8.153516526s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.009 | 1.07% | neutral |
| conservative | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| conservative | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| conservative | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| conservative | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| conservative | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| conservative | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| conservative | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| conservative | healthy_steady_state | 11 | 306.901147ms | 307.144336ms | 29.560991ms | 29.597962ms | 0.015 | 0.015 | not detected | 0 | 0 | not detected | 0 | 0.005 | -0.10% | neutral |
| conservative | healthy_steady_state | 29 | 306.449512ms | 306.900343ms | 29.524996ms | 29.586311ms | 0.015 | 0.015 | not detected | 0 | 0 | not detected | 0 | 0.005 | -0.17% | neutral |
| conservative | healthy_steady_state | 47 | 306.757116ms | 307.697892ms | 29.42026ms | 29.450545ms | 0.028 | 0.028 | not detected | 0 | 0 | not detected | 0 | 0.005 | -0.16% | neutral |
| conservative | healthy_steady_state | 71 | 304.971872ms | 305.362102ms | 29.463197ms | 29.569943ms | 0.018 | 0.018 | not detected | 0 | 0 | not detected | 0 | 0.005 | -0.25% | neutral |
| conservative | healthy_steady_state | 101 | 306.026121ms | 306.150606ms | 29.656679ms | 29.696734ms | 0.023 | 0.025 | not detected | 0 | 0 | not detected | 0 | 0.003 | -0.74% | neutral |
| conservative | healthy_steady_state | 131 | 307.461992ms | 308.156379ms | 29.32466ms | 29.347201ms | 0.022 | 0.022 | not detected | 0 | 0 | not detected | 0 | 0.005 | -0.12% | neutral |
| conservative | healthy_steady_state | 173 | 306.762211ms | 306.901039ms | 29.635213ms | 29.673567ms | 0.022 | 0.022 | not detected | 0 | 0 | not detected | 0 | 0.005 | -0.09% | neutral |
| balanced | gradual_degradation | 11 | 2m18.05810847s | 2m18.05810847s | 111.638008ms | 111.638008ms | 0.738 | 0.738 | 2.2s | 6 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| balanced | gradual_degradation | 29 | 2m17.230127598s | 2m17.230127598s | 111.973508ms | 111.973508ms | 0.737 | 0.737 | 2.4s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| balanced | gradual_degradation | 47 | 2m18.543146187s | 2m18.543146187s | 112.017461ms | 112.017461ms | 0.743 | 0.743 | 2.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| balanced | gradual_degradation | 71 | 2m18.842157616s | 2m18.842157616s | 112.679273ms | 112.679273ms | 0.742 | 0.742 | 2.4s | 7 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| balanced | gradual_degradation | 101 | 2m17.896601353s | 2m17.896601353s | 112.288179ms | 112.288179ms | 0.740 | 0.740 | 2.2s | 7 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| balanced | gradual_degradation | 131 | 2m18.257709744s | 2m18.257709744s | 112.50077ms | 112.50077ms | 0.742 | 0.743 | 2.2s | 7 | 450 | not detected | 0 | 0.002 | -0.00% | neutral |
| balanced | gradual_degradation | 173 | 2m18.511286942s | 2m18.511286942s | 112.241073ms | 112.241073ms | 0.740 | 0.740 | 1.4s | 4 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| balanced | sudden_outage | 11 | 306.854065ms | 472.597002ms | 29.758413ms | 40.050908ms | 0.669 | 0.038 | 2s | 5 | 10 | 3.6s | 0 | 0.002 | 81.08% | win |
| balanced | sudden_outage | 29 | 305.736046ms | 468.508565ms | 29.586311ms | 39.871299ms | 0.667 | 0.040 | 2s | 5 | 10 | 3.6s | 0 | 0.002 | 81.04% | win |
| balanced | sudden_outage | 47 | 306.757116ms | 476.943647ms | 29.297237ms | 39.405096ms | 0.676 | 0.053 | 2.4s | 7 | 12 | 4s | 0 | 0.002 | 80.13% | win |
| balanced | sudden_outage | 71 | 306.766218ms | 473.760036ms | 29.362123ms | 39.983305ms | 0.673 | 0.047 | 2s | 4 | 10 | 3.6s | 0 | 0.002 | 80.72% | win |
| balanced | sudden_outage | 101 | 306.026121ms | 470.23154ms | 29.696734ms | 40.521742ms | 0.673 | 0.049 | 2s | 4 | 10 | 3.6s | 0 | 0.002 | 80.56% | win |
| balanced | sudden_outage | 131 | 308.156379ms | 473.945235ms | 29.458433ms | 40.270961ms | 0.676 | 0.040 | 2s | 5 | 10 | 3.6s | 0 | 0.002 | 80.99% | win |
| balanced | sudden_outage | 173 | 307.890143ms | 476.089995ms | 29.101676ms | 39.998469ms | 0.673 | 0.060 | 1.2s | 3 | 10 | 3.6s | 0 | 0.002 | 80.07% | win |
| balanced | capacity_aggregation | 11 | 1m19.876239452s | 36.421045246s | 25ms | 28ms | 0.988 | 0.938 | 0s | 0 | 383 | 3.375s | 2 | 0.007 | 58.36% | win |
| balanced | capacity_aggregation | 29 | 1m19.971533361s | 36.627172725s | 25ms | 28ms | 0.988 | 0.942 | 0s | 0 | 376 | 3.25s | 2 | 0.007 | 58.17% | win |
| balanced | capacity_aggregation | 47 | 1m19.89581372s | 36.770404785s | 25ms | 28ms | 0.988 | 0.939 | 0s | 0 | 383 | 3.375s | 2 | 0.007 | 58.05% | win |
| balanced | capacity_aggregation | 71 | 1m19.767790605s | 36.204382036s | 25ms | 28ms | 0.988 | 0.939 | 0s | 0 | 382 | 3.375s | 2 | 0.007 | 58.53% | win |
| balanced | capacity_aggregation | 101 | 1m20.040921681s | 36.446674146s | 25ms | 28ms | 0.990 | 0.943 | 0s | 0 | 382 | 3.375s | 2 | 0.007 | 58.43% | win |
| balanced | capacity_aggregation | 131 | 1m19.734245977s | 36.592243941s | 25ms | 28ms | 0.988 | 0.942 | 0s | 0 | 383 | 3.375s | 2 | 0.007 | 58.09% | win |
| balanced | capacity_aggregation | 173 | 1m19.865475022s | 36.250471072s | 25ms | 28ms | 0.988 | 0.940 | 0s | 0 | 381 | 3.25s | 2 | 0.007 | 58.50% | win |
| balanced | transient_spike | 11 | 12.6925426s | 12.6925426s | 90ms | 90ms | 0.396 | 0.396 | 4.4s | 5 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| balanced | transient_spike | 29 | 12.307633594s | 12.307633594s | 90ms | 90ms | 0.384 | 0.384 | 4.4s | 5 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| balanced | transient_spike | 47 | 12.854235214s | 12.854235214s | 90ms | 90ms | 0.402 | 0.402 | 4.6s | 6 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| balanced | transient_spike | 71 | 12.659328791s | 12.659328791s | 90ms | 90ms | 0.400 | 0.400 | 4.4s | 4 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| balanced | transient_spike | 101 | 12.607221347s | 12.607221347s | 90ms | 90ms | 0.398 | 0.398 | 4.2s | 4 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| balanced | transient_spike | 131 | 12.512324827s | 12.512324827s | 90ms | 90ms | 0.398 | 0.400 | 4.4s | 5 | 300 | not detected | 0 | 0.002 | -0.04% | neutral |
| balanced | transient_spike | 173 | 12.652228169s | 12.652228169s | 90ms | 90ms | 0.398 | 0.398 | 1.2s | 3 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| balanced | stale_candidate | 11 | 46.540000047s | 27.103333361s | 110ms | 110ms | 0.889 | 0.550 | 9.99999999s | 12 | 91 | 34.666666632s | 0 | 0.035 | 45.78% | win |
| balanced | stale_candidate | 29 | 51.736666713s | 29.496666693s | 110ms | 110ms | 0.883 | 0.538 | 9.666666657s | 9 | 90 | 34.333333299s | 0 | 0.035 | 47.47% | win |
| balanced | stale_candidate | 47 | 51.540000046s | 28.360000028s | 110ms | 110ms | 0.883 | 0.526 | 9.333333324s | 9 | 90 | 34.333333299s | 0 | 0.035 | 49.43% | win |
| balanced | stale_candidate | 71 | 50.403333377s | 34.753333368s | 110ms | 110ms | 0.889 | 0.667 | 10.333333323s | 11 | 109 | 40.999999959s | 0 | 0.041 | 35.01% | win |
| balanced | stale_candidate | 101 | 48.010000047s | 27.830000028s | 110ms | 110ms | 0.883 | 0.550 | 9.99999999s | 9 | 90 | 34.333333299s | 0 | 0.035 | 46.46% | win |
| balanced | stale_candidate | 131 | 47.873333381s | 25.966666694s | 110ms | 110ms | 0.883 | 0.538 | 9.666666657s | 10 | 90 | 34.333333299s | 0 | 0.035 | 49.38% | win |
| balanced | stale_candidate | 173 | 46.87333338s | 26.163333359s | 110ms | 110ms | 0.883 | 0.544 | 9.99999999s | 10 | 90 | 34.333333299s | 0 | 0.035 | 48.00% | win |
| balanced | recovery_no_flap | 11 | 44.799011002s | 44.799011002s | 95ms | 95ms | 0.752 | 0.752 | 4.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| balanced | recovery_no_flap | 29 | 44.937203373s | 44.937203373s | 95ms | 95ms | 0.750 | 0.750 | 4.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| balanced | recovery_no_flap | 47 | 44.810457119s | 44.810457119s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| balanced | recovery_no_flap | 71 | 44.958361864s | 44.958361864s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| balanced | recovery_no_flap | 101 | 44.837077225s | 44.837077225s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| balanced | recovery_no_flap | 131 | 44.901274352s | 44.901274352s | 95ms | 95ms | 0.757 | 0.758 | 4.2s | 5 | 450 | not detected | 0 | 0.002 | -0.01% | neutral |
| balanced | recovery_no_flap | 173 | 44.9015634s | 44.9015634s | 95ms | 95ms | 0.755 | 0.755 | 1.2s | 3 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| balanced | all_channels_bad | 11 | 2m10.9s | 2m10.9s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| balanced | all_channels_bad | 29 | 2m9.8s | 2m9.8s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| balanced | all_channels_bad | 47 | 2m5.2s | 2m5.2s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| balanced | all_channels_bad | 71 | 2m9.4s | 2m9.4s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| balanced | all_channels_bad | 101 | 2m5.7s | 2m5.7s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| balanced | all_channels_bad | 131 | 2m5.1s | 2m5.1s | 100ms | 100ms | 0.676 | 0.678 | 0s | 0 | 300 | not detected | 0 | 0.002 | -0.00% | neutral |
| balanced | all_channels_bad | 173 | 2m9.3s | 2m9.3s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| balanced | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| balanced | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| balanced | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| balanced | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| balanced | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| balanced | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| balanced | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| balanced | healthy_steady_state | 11 | 306.901147ms | 306.99182ms | 29.560991ms | 29.568674ms | 0.015 | 0.015 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.03% | neutral |
| balanced | healthy_steady_state | 29 | 306.449512ms | 306.872371ms | 29.524996ms | 29.53103ms | 0.015 | 0.015 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.06% | neutral |
| balanced | healthy_steady_state | 47 | 306.757116ms | 307.058184ms | 29.42026ms | 29.429223ms | 0.028 | 0.028 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.05% | neutral |
| balanced | healthy_steady_state | 71 | 304.971872ms | 305.036931ms | 29.463197ms | 29.476368ms | 0.018 | 0.018 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.03% | neutral |
| balanced | healthy_steady_state | 101 | 306.026121ms | 306.089522ms | 29.656679ms | 29.666591ms | 0.023 | 0.023 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.03% | neutral |
| balanced | healthy_steady_state | 131 | 307.461992ms | 307.655414ms | 29.32466ms | 29.32802ms | 0.022 | 0.023 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.68% | neutral |
| balanced | healthy_steady_state | 173 | 306.762211ms | 306.866751ms | 29.635213ms | 29.652251ms | 0.022 | 0.022 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.04% | neutral |
| aggressive | gradual_degradation | 11 | 2m18.05810847s | 2m18.05810847s | 111.638008ms | 111.638008ms | 0.738 | 0.738 | 400ms | 2 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | gradual_degradation | 29 | 2m17.230127598s | 2m17.230127598s | 111.973508ms | 111.973508ms | 0.737 | 0.737 | 2.4s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | gradual_degradation | 47 | 2m18.543146187s | 2m18.543146187s | 112.017461ms | 112.017461ms | 0.743 | 0.743 | 2.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | gradual_degradation | 71 | 2m18.842157616s | 2m18.842157616s | 112.679273ms | 112.679273ms | 0.742 | 0.742 | 2.4s | 7 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | gradual_degradation | 101 | 2m17.896601353s | 2m17.896601353s | 112.288179ms | 112.288179ms | 0.740 | 0.740 | 0s | 0 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | gradual_degradation | 131 | 2m18.257709744s | 2m18.257709744s | 112.50077ms | 112.50077ms | 0.742 | 0.742 | 2.2s | 7 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | gradual_degradation | 173 | 2m18.511286942s | 2m18.511286942s | 112.241073ms | 112.241073ms | 0.740 | 0.740 | 1.2s | 3 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | sudden_outage | 11 | 306.854065ms | 472.597002ms | 29.758413ms | 40.050908ms | 0.669 | 0.038 | 200ms | 2 | 10 | 3.6s | 0 | 0.002 | 81.08% | win |
| aggressive | sudden_outage | 29 | 305.736046ms | 468.508565ms | 29.586311ms | 39.871299ms | 0.667 | 0.040 | 2s | 5 | 10 | 3.6s | 0 | 0.002 | 81.04% | win |
| aggressive | sudden_outage | 47 | 306.757116ms | 476.943647ms | 29.297237ms | 39.556131ms | 0.676 | 0.053 | 2.4s | 7 | 12 | 4s | 0 | 0.002 | 80.09% | win |
| aggressive | sudden_outage | 71 | 306.766218ms | 473.760036ms | 29.362123ms | 39.983305ms | 0.673 | 0.047 | 0s | 0 | 10 | 3.6s | 0 | 0.002 | 80.72% | win |
| aggressive | sudden_outage | 101 | 306.026121ms | 470.23154ms | 29.696734ms | 40.521742ms | 0.673 | 0.049 | 0s | 0 | 10 | 3.6s | 0 | 0.002 | 80.56% | win |
| aggressive | sudden_outage | 131 | 308.156379ms | 473.89148ms | 29.458433ms | 40.270961ms | 0.676 | 0.038 | 2s | 5 | 10 | 3.6s | 0 | 0.002 | 81.09% | win |
| aggressive | sudden_outage | 173 | 307.890143ms | 476.089995ms | 29.101676ms | 40.048681ms | 0.673 | 0.060 | 800ms | 2 | 10 | 3.6s | 0 | 0.002 | 80.06% | win |
| aggressive | capacity_aggregation | 11 | 1m19.876239452s | 1m6.379054481s | 25ms | 28ms | 0.988 | 0.907 | 0s | 0 | 588 | not detected | 0 | 0.081 | 18.83% | win |
| aggressive | capacity_aggregation | 29 | 1m19.971533361s | 1m6.404914573s | 25ms | 28ms | 0.988 | 0.907 | 0s | 0 | 588 | not detected | 0 | 0.081 | 18.88% | win |
| aggressive | capacity_aggregation | 47 | 1m19.89581372s | 1m6.386169879s | 25ms | 28ms | 0.988 | 0.907 | 0s | 0 | 588 | not detected | 0 | 0.081 | 18.85% | win |
| aggressive | capacity_aggregation | 71 | 1m19.767790605s | 1m6.111167294s | 25ms | 28ms | 0.988 | 0.907 | 0s | 0 | 588 | not detected | 0 | 0.081 | 18.99% | win |
| aggressive | capacity_aggregation | 101 | 1m20.040921681s | 1m6.357418244s | 25ms | 28ms | 0.990 | 0.907 | 0s | 0 | 588 | not detected | 0 | 0.081 | 18.98% | win |
| aggressive | capacity_aggregation | 131 | 1m19.734245977s | 1m6.141091172s | 25ms | 28ms | 0.988 | 0.907 | 0s | 0 | 588 | not detected | 0 | 0.081 | 18.95% | win |
| aggressive | capacity_aggregation | 173 | 1m19.865475022s | 1m6.225737267s | 25ms | 28ms | 0.988 | 0.907 | 0s | 0 | 588 | not detected | 0 | 0.081 | 18.94% | win |
| aggressive | transient_spike | 11 | 12.6925426s | 12.6925426s | 90ms | 90ms | 0.396 | 0.396 | 200ms | 2 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | transient_spike | 29 | 12.307633594s | 12.307633594s | 90ms | 90ms | 0.384 | 0.384 | 4.4s | 5 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | transient_spike | 47 | 12.854235214s | 12.854235214s | 90ms | 90ms | 0.402 | 0.402 | 4.6s | 6 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | transient_spike | 71 | 12.659328791s | 12.659328791s | 90ms | 90ms | 0.400 | 0.400 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | transient_spike | 101 | 12.607221347s | 12.607221347s | 90ms | 90ms | 0.398 | 0.398 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | transient_spike | 131 | 12.512324827s | 12.512324827s | 90ms | 90ms | 0.398 | 0.398 | 4.4s | 5 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | transient_spike | 173 | 12.652228169s | 12.652228169s | 90ms | 90ms | 0.398 | 0.398 | 800ms | 2 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | stale_candidate | 11 | 46.540000047s | 7.590000008s | 110ms | 110ms | 0.889 | 0.211 | 9.333333324s | 9 | 32 | 14.333333319s | 0 | 0.023 | 83.90% | win |
| aggressive | stale_candidate | 29 | 51.736666713s | 8.92333334s | 110ms | 110ms | 0.883 | 0.240 | 9.333333324s | 8 | 32 | 14.333333319s | 0 | 0.023 | 83.68% | win |
| aggressive | stale_candidate | 47 | 51.540000046s | 8.39333334s | 110ms | 110ms | 0.883 | 0.205 | 9.333333324s | 8 | 32 | 14.333333319s | 0 | 0.023 | 84.62% | win |
| aggressive | stale_candidate | 71 | 50.403333377s | 7.060000006s | 110ms | 110ms | 0.889 | 0.234 | 9.333333324s | 10 | 32 | 14.333333319s | 0 | 0.023 | 85.95% | win |
| aggressive | stale_candidate | 101 | 48.010000047s | 25.633333361s | 110ms | 110ms | 0.883 | 0.550 | 9.333333324s | 8 | 89 | 34.999999965s | 0 | 0.053 | 50.03% | win |
| aggressive | stale_candidate | 131 | 47.873333381s | 8.590000007s | 110ms | 110ms | 0.883 | 0.211 | 9.333333324s | 8 | 32 | 14.333333319s | 0 | 0.023 | 82.93% | win |
| aggressive | stale_candidate | 173 | 46.87333338s | 30.890000034s | 110ms | 110ms | 0.883 | 0.673 | 9.333333324s | 9 | 112 | 43.33333329s | 0 | 0.064 | 36.52% | win |
| aggressive | recovery_no_flap | 11 | 44.799011002s | 44.799011002s | 95ms | 95ms | 0.752 | 0.752 | 200ms | 2 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | recovery_no_flap | 29 | 44.937203373s | 44.937203373s | 95ms | 95ms | 0.750 | 0.750 | 4.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | recovery_no_flap | 47 | 44.810457119s | 44.810457119s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | recovery_no_flap | 71 | 44.958361864s | 44.958361864s | 95ms | 95ms | 0.755 | 0.755 | 0s | 0 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | recovery_no_flap | 101 | 44.837077225s | 44.837077225s | 95ms | 95ms | 0.755 | 0.755 | 0s | 0 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | recovery_no_flap | 131 | 44.901274352s | 44.901274352s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | recovery_no_flap | 173 | 44.9015634s | 44.9015634s | 95ms | 95ms | 0.755 | 0.755 | 800ms | 2 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | all_channels_bad | 11 | 2m10.9s | 2m10.9s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | all_channels_bad | 29 | 2m9.8s | 2m9.8s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | all_channels_bad | 47 | 2m5.2s | 2m5.2s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | all_channels_bad | 71 | 2m9.4s | 2m9.4s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | all_channels_bad | 101 | 2m5.7s | 2m5.7s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | all_channels_bad | 131 | 2m5.1s | 2m5.1s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | all_channels_bad | 173 | 2m9.3s | 2m9.3s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| aggressive | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| aggressive | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| aggressive | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| aggressive | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| aggressive | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| aggressive | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| aggressive | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| aggressive | healthy_steady_state | 11 | 306.901147ms | 306.99182ms | 29.560991ms | 29.568674ms | 0.015 | 0.015 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.03% | neutral |
| aggressive | healthy_steady_state | 29 | 306.449512ms | 306.872371ms | 29.524996ms | 29.53103ms | 0.015 | 0.015 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.06% | neutral |
| aggressive | healthy_steady_state | 47 | 306.757116ms | 307.058184ms | 29.42026ms | 29.429223ms | 0.028 | 0.028 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.05% | neutral |
| aggressive | healthy_steady_state | 71 | 304.971872ms | 305.036931ms | 29.463197ms | 29.476368ms | 0.018 | 0.018 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.03% | neutral |
| aggressive | healthy_steady_state | 101 | 306.026121ms | 306.089522ms | 29.656679ms | 29.666591ms | 0.023 | 0.023 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.03% | neutral |
| aggressive | healthy_steady_state | 131 | 307.461992ms | 307.655414ms | 29.32466ms | 29.32802ms | 0.022 | 0.022 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.03% | neutral |
| aggressive | healthy_steady_state | 173 | 306.762211ms | 306.866751ms | 29.635213ms | 29.652251ms | 0.022 | 0.022 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.04% | neutral |
| fast-low-probe | gradual_degradation | 11 | 2m18.05810847s | 2m18.05810847s | 111.638008ms | 111.638008ms | 0.738 | 0.738 | 400ms | 2 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-low-probe | gradual_degradation | 29 | 2m17.230127598s | 2m17.230127598s | 111.973508ms | 111.973508ms | 0.737 | 0.737 | 2.4s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-low-probe | gradual_degradation | 47 | 2m18.543146187s | 2m18.543146187s | 112.017461ms | 112.017461ms | 0.743 | 0.743 | 2.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-low-probe | gradual_degradation | 71 | 2m18.842157616s | 2m18.842157616s | 112.679273ms | 112.679273ms | 0.742 | 0.742 | 2.4s | 7 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-low-probe | gradual_degradation | 101 | 2m17.896601353s | 2m17.896601353s | 112.288179ms | 112.288179ms | 0.740 | 0.742 | 0s | 0 | 450 | not detected | 0 | 0.002 | -0.00% | neutral |
| fast-low-probe | gradual_degradation | 131 | 2m18.257709744s | 2m18.257709744s | 112.50077ms | 112.50077ms | 0.742 | 0.742 | 2.2s | 7 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-low-probe | gradual_degradation | 173 | 2m18.511286942s | 2m18.511286942s | 112.241073ms | 112.241073ms | 0.740 | 0.740 | 1.2s | 3 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-low-probe | sudden_outage | 11 | 306.854065ms | 472.597002ms | 29.758413ms | 40.050908ms | 0.669 | 0.038 | 200ms | 2 | 10 | 3.6s | 0 | 0.002 | 81.08% | win |
| fast-low-probe | sudden_outage | 29 | 305.736046ms | 468.508565ms | 29.586311ms | 39.871299ms | 0.667 | 0.040 | 2s | 5 | 10 | 3.6s | 0 | 0.002 | 81.04% | win |
| fast-low-probe | sudden_outage | 47 | 306.757116ms | 476.943647ms | 29.297237ms | 39.405096ms | 0.676 | 0.053 | 2.4s | 7 | 12 | 4s | 0 | 0.002 | 80.13% | win |
| fast-low-probe | sudden_outage | 71 | 306.766218ms | 473.760036ms | 29.362123ms | 39.983305ms | 0.673 | 0.047 | 0s | 0 | 10 | 3.6s | 0 | 0.002 | 80.72% | win |
| fast-low-probe | sudden_outage | 101 | 306.026121ms | 470.691269ms | 29.696734ms | 40.521742ms | 0.673 | 0.051 | 0s | 0 | 10 | 3.6s | 0 | 0.002 | 80.45% | win |
| fast-low-probe | sudden_outage | 131 | 308.156379ms | 473.89148ms | 29.458433ms | 40.270961ms | 0.676 | 0.038 | 2s | 5 | 10 | 3.6s | 0 | 0.002 | 81.09% | win |
| fast-low-probe | sudden_outage | 173 | 307.890143ms | 476.089995ms | 29.101676ms | 39.998469ms | 0.673 | 0.060 | 800ms | 2 | 10 | 3.6s | 0 | 0.002 | 80.07% | win |
| fast-low-probe | capacity_aggregation | 11 | 1m19.876239452s | 1m14.778295051s | 25ms | 25ms | 0.988 | 0.957 | 0s | 0 | 620 | not detected | 0 | 0.031 | 7.23% | neutral |
| fast-low-probe | capacity_aggregation | 29 | 1m19.971533361s | 1m14.795383989s | 25ms | 25ms | 0.988 | 0.957 | 0s | 0 | 620 | not detected | 0 | 0.031 | 7.28% | neutral |
| fast-low-probe | capacity_aggregation | 47 | 1m19.89581372s | 1m14.823676803s | 25ms | 25ms | 0.988 | 0.957 | 0s | 0 | 620 | not detected | 0 | 0.031 | 7.21% | neutral |
| fast-low-probe | capacity_aggregation | 71 | 1m19.767790605s | 1m14.658041714s | 25ms | 25ms | 0.988 | 0.957 | 0s | 0 | 620 | not detected | 0 | 0.031 | 7.26% | neutral |
| fast-low-probe | capacity_aggregation | 101 | 1m20.040921681s | 1m14.811831075s | 25ms | 25ms | 0.990 | 0.960 | 0s | 0 | 620 | not detected | 0 | 0.031 | 7.33% | neutral |
| fast-low-probe | capacity_aggregation | 131 | 1m19.734245977s | 1m14.690853956s | 25ms | 25ms | 0.988 | 0.957 | 0s | 0 | 620 | not detected | 0 | 0.031 | 7.19% | neutral |
| fast-low-probe | capacity_aggregation | 173 | 1m19.865475022s | 1m14.710734424s | 25ms | 25ms | 0.988 | 0.957 | 0s | 0 | 620 | not detected | 0 | 0.031 | 7.28% | neutral |
| fast-low-probe | transient_spike | 11 | 12.6925426s | 12.6925426s | 90ms | 90ms | 0.396 | 0.396 | 200ms | 2 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-low-probe | transient_spike | 29 | 12.307633594s | 12.307633594s | 90ms | 90ms | 0.384 | 0.384 | 4.4s | 5 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-low-probe | transient_spike | 47 | 12.854235214s | 12.854235214s | 90ms | 90ms | 0.402 | 0.402 | 4.6s | 6 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-low-probe | transient_spike | 71 | 12.659328791s | 12.659328791s | 90ms | 90ms | 0.400 | 0.400 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-low-probe | transient_spike | 101 | 12.607221347s | 12.607221347s | 90ms | 90ms | 0.398 | 0.400 | 0s | 0 | 300 | not detected | 0 | 0.002 | -0.04% | neutral |
| fast-low-probe | transient_spike | 131 | 12.512324827s | 12.512324827s | 90ms | 90ms | 0.398 | 0.398 | 4.4s | 5 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-low-probe | transient_spike | 173 | 12.652228169s | 12.652228169s | 90ms | 90ms | 0.398 | 0.398 | 800ms | 2 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-low-probe | stale_candidate | 11 | 46.540000047s | 25.103333357s | 110ms | 110ms | 0.889 | 0.520 | 9.333333324s | 10 | 85 | 31.666666635s | 0 | 0.023 | 50.02% | win |
| fast-low-probe | stale_candidate | 29 | 51.736666713s | 27.496666691s | 110ms | 110ms | 0.883 | 0.503 | 9.333333324s | 8 | 84 | 31.666666635s | 0 | 0.023 | 51.39% | win |
| fast-low-probe | stale_candidate | 47 | 51.540000046s | 26.966666691s | 110ms | 110ms | 0.883 | 0.509 | 9.333333324s | 9 | 85 | 31.999999968s | 0 | 0.023 | 52.07% | win |
| fast-low-probe | stale_candidate | 71 | 50.403333377s | 27.103333359s | 110ms | 110ms | 0.889 | 0.509 | 9.666666657s | 10 | 84 | 31.666666635s | 0 | 0.023 | 51.08% | win |
| fast-low-probe | stale_candidate | 101 | 48.010000047s | 26.300000025s | 110ms | 110ms | 0.883 | 0.520 | 9.666666657s | 8 | 85 | 31.999999968s | 0 | 0.023 | 49.53% | win |
| fast-low-probe | stale_candidate | 131 | 47.873333381s | 24.633333358s | 110ms | 110ms | 0.883 | 0.509 | 9.333333324s | 9 | 84 | 31.666666635s | 0 | 0.023 | 52.36% | win |
| fast-low-probe | stale_candidate | 173 | 46.87333338s | 25.693333366s | 110ms | 110ms | 0.883 | 0.515 | 9.666666657s | 8 | 85 | 31.666666635s | 0 | 0.023 | 48.93% | win |
| fast-low-probe | recovery_no_flap | 11 | 44.799011002s | 44.799011002s | 95ms | 95ms | 0.752 | 0.752 | 200ms | 2 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-low-probe | recovery_no_flap | 29 | 44.937203373s | 44.937203373s | 95ms | 95ms | 0.750 | 0.750 | 4.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-low-probe | recovery_no_flap | 47 | 44.810457119s | 44.810457119s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-low-probe | recovery_no_flap | 71 | 44.958361864s | 44.958361864s | 95ms | 95ms | 0.755 | 0.755 | 0s | 0 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-low-probe | recovery_no_flap | 101 | 44.837077225s | 44.837077225s | 95ms | 95ms | 0.755 | 0.757 | 0s | 0 | 450 | not detected | 0 | 0.002 | -0.01% | neutral |
| fast-low-probe | recovery_no_flap | 131 | 44.901274352s | 44.901274352s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-low-probe | recovery_no_flap | 173 | 44.9015634s | 44.9015634s | 95ms | 95ms | 0.755 | 0.755 | 800ms | 2 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-low-probe | all_channels_bad | 11 | 2m10.9s | 2m10.9s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-low-probe | all_channels_bad | 29 | 2m9.8s | 2m9.8s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-low-probe | all_channels_bad | 47 | 2m5.2s | 2m5.2s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-low-probe | all_channels_bad | 71 | 2m9.4s | 2m9.4s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-low-probe | all_channels_bad | 101 | 2m5.7s | 2m5.7s | 100ms | 100ms | 0.673 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.002 | -0.00% | neutral |
| fast-low-probe | all_channels_bad | 131 | 2m5.1s | 2m5.1s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-low-probe | all_channels_bad | 173 | 2m9.3s | 2m9.3s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-low-probe | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| fast-low-probe | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| fast-low-probe | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| fast-low-probe | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| fast-low-probe | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| fast-low-probe | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| fast-low-probe | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| fast-low-probe | healthy_steady_state | 11 | 306.901147ms | 306.99182ms | 29.560991ms | 29.568674ms | 0.015 | 0.015 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.03% | neutral |
| fast-low-probe | healthy_steady_state | 29 | 306.449512ms | 306.872371ms | 29.524996ms | 29.53103ms | 0.015 | 0.015 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.06% | neutral |
| fast-low-probe | healthy_steady_state | 47 | 306.757116ms | 307.058184ms | 29.42026ms | 29.429223ms | 0.028 | 0.028 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.05% | neutral |
| fast-low-probe | healthy_steady_state | 71 | 304.971872ms | 305.036931ms | 29.463197ms | 29.476368ms | 0.018 | 0.018 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.03% | neutral |
| fast-low-probe | healthy_steady_state | 101 | 306.026121ms | 306.089522ms | 29.656679ms | 29.666591ms | 0.023 | 0.025 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.68% | neutral |
| fast-low-probe | healthy_steady_state | 131 | 307.461992ms | 307.655414ms | 29.32466ms | 29.32802ms | 0.022 | 0.022 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.03% | neutral |
| fast-low-probe | healthy_steady_state | 173 | 306.762211ms | 306.866751ms | 29.635213ms | 29.652251ms | 0.022 | 0.022 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.04% | neutral |
| short-window | gradual_degradation | 11 | 2m18.05810847s | 2m18.05810847s | 111.638008ms | 111.638008ms | 0.738 | 0.738 | 2.2s | 6 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | gradual_degradation | 29 | 2m17.230127598s | 2m17.230127598s | 111.973508ms | 111.973508ms | 0.737 | 0.737 | 2.4s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | gradual_degradation | 47 | 2m18.543146187s | 2m18.543146187s | 112.017461ms | 112.017461ms | 0.743 | 0.743 | 2.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | gradual_degradation | 71 | 2m18.842157616s | 2m18.842157616s | 112.679273ms | 112.679273ms | 0.742 | 0.742 | 2.4s | 7 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | gradual_degradation | 101 | 2m17.896601353s | 2m17.896601353s | 112.288179ms | 112.288179ms | 0.740 | 0.740 | 2.2s | 7 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | gradual_degradation | 131 | 2m18.257709744s | 2m18.257709744s | 112.50077ms | 112.50077ms | 0.742 | 0.742 | 2.2s | 7 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | gradual_degradation | 173 | 2m18.511286942s | 2m18.511286942s | 112.241073ms | 112.241073ms | 0.740 | 0.740 | 1.4s | 4 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | sudden_outage | 11 | 306.854065ms | 472.597002ms | 29.758413ms | 40.050908ms | 0.669 | 0.038 | 2s | 5 | 10 | 3.6s | 0 | 0.002 | 81.08% | win |
| short-window | sudden_outage | 29 | 305.736046ms | 468.508565ms | 29.586311ms | 39.871299ms | 0.667 | 0.040 | 2s | 5 | 10 | 3.6s | 0 | 0.002 | 81.04% | win |
| short-window | sudden_outage | 47 | 306.757116ms | 476.943647ms | 29.297237ms | 39.405096ms | 0.676 | 0.053 | 2.4s | 7 | 12 | 4s | 0 | 0.002 | 80.13% | win |
| short-window | sudden_outage | 71 | 306.766218ms | 473.760036ms | 29.362123ms | 39.983305ms | 0.673 | 0.047 | 2s | 4 | 10 | 3.6s | 0 | 0.002 | 80.72% | win |
| short-window | sudden_outage | 101 | 306.026121ms | 470.23154ms | 29.696734ms | 40.521742ms | 0.673 | 0.049 | 2s | 4 | 10 | 3.6s | 0 | 0.002 | 80.56% | win |
| short-window | sudden_outage | 131 | 308.156379ms | 473.89148ms | 29.458433ms | 40.270961ms | 0.676 | 0.038 | 2s | 5 | 10 | 3.6s | 0 | 0.002 | 81.09% | win |
| short-window | sudden_outage | 173 | 307.890143ms | 476.089995ms | 29.101676ms | 39.998469ms | 0.673 | 0.060 | 1.2s | 3 | 10 | 3.6s | 0 | 0.002 | 80.07% | win |
| short-window | capacity_aggregation | 11 | 1m19.876239452s | 7.330567273s | 25ms | 28ms | 0.988 | 0.863 | 0s | 0 | 284 | 1.125s | 8 | 0.006 | 89.52% | win |
| short-window | capacity_aggregation | 29 | 1m19.971533361s | 6.294367405s | 25ms | 28ms | 0.988 | 0.847 | 0s | 0 | 277 | 1.125s | 9 | 0.006 | 90.70% | win |
| short-window | capacity_aggregation | 47 | 1m19.89581372s | 6.315259432s | 25ms | 28ms | 0.988 | 0.839 | 0s | 0 | 280 | 1.125s | 9 | 0.006 | 90.71% | win |
| short-window | capacity_aggregation | 71 | 1m19.767790605s | 6.211246974s | 25ms | 28ms | 0.988 | 0.844 | 0s | 0 | 287 | 1.125s | 9 | 0.006 | 90.81% | win |
| short-window | capacity_aggregation | 101 | 1m20.040921681s | 7.429708966s | 25ms | 28ms | 0.990 | 0.878 | 0s | 0 | 285 | 1.125s | 7 | 0.006 | 89.31% | win |
| short-window | capacity_aggregation | 131 | 1m19.734245977s | 6.40461911s | 25ms | 28ms | 0.988 | 0.840 | 0s | 0 | 276 | 1.125s | 9 | 0.006 | 90.58% | win |
| short-window | capacity_aggregation | 173 | 1m19.865475022s | 6.35882459s | 25ms | 28ms | 0.988 | 0.840 | 0s | 0 | 283 | 1.125s | 9 | 0.006 | 90.66% | win |
| short-window | transient_spike | 11 | 12.6925426s | 12.6925426s | 90ms | 90ms | 0.396 | 0.396 | 4.4s | 5 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | transient_spike | 29 | 12.307633594s | 12.307633594s | 90ms | 90ms | 0.384 | 0.384 | 4.4s | 5 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | transient_spike | 47 | 12.854235214s | 12.854235214s | 90ms | 90ms | 0.402 | 0.402 | 4.6s | 6 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | transient_spike | 71 | 12.659328791s | 12.659328791s | 90ms | 90ms | 0.400 | 0.400 | 4.4s | 4 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | transient_spike | 101 | 12.607221347s | 12.607221347s | 90ms | 90ms | 0.398 | 0.398 | 4.2s | 4 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | transient_spike | 131 | 12.512324827s | 12.512324827s | 90ms | 90ms | 0.398 | 0.398 | 4.4s | 5 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | transient_spike | 173 | 12.652228169s | 12.652228169s | 90ms | 90ms | 0.398 | 0.398 | 1.2s | 3 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | stale_candidate | 11 | 46.540000047s | 43.146666714s | 110ms | 110ms | 0.889 | 0.836 | 9.666666657s | 9 | 142 | not detected | 0 | 0.058 | 7.83% | neutral |
| short-window | stale_candidate | 29 | 51.736666713s | 45.600000047s | 110ms | 110ms | 0.883 | 0.830 | 9.666666657s | 9 | 142 | not detected | 0 | 0.058 | 13.25% | neutral |
| short-window | stale_candidate | 47 | 51.540000046s | 44.736666712s | 110ms | 110ms | 0.883 | 0.830 | 9.333333324s | 9 | 142 | not detected | 0 | 0.058 | 14.19% | neutral |
| short-window | stale_candidate | 71 | 50.403333377s | 44.206666712s | 110ms | 110ms | 0.889 | 0.836 | 9.666666657s | 10 | 142 | not detected | 0 | 0.058 | 13.59% | neutral |
| short-window | stale_candidate | 101 | 48.010000047s | 42.343333378s | 110ms | 110ms | 0.883 | 0.830 | 9.666666657s | 8 | 142 | not detected | 0 | 0.058 | 12.41% | neutral |
| short-window | stale_candidate | 131 | 47.873333381s | 42.540000047s | 110ms | 110ms | 0.883 | 0.830 | 9.666666657s | 10 | 142 | not detected | 0 | 0.058 | 11.71% | neutral |
| short-window | stale_candidate | 173 | 46.87333338s | 42.010000045s | 110ms | 110ms | 0.883 | 0.830 | 9.666666657s | 9 | 142 | not detected | 0 | 0.058 | 11.18% | neutral |
| short-window | recovery_no_flap | 11 | 44.799011002s | 44.799011002s | 95ms | 95ms | 0.752 | 0.752 | 4.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | recovery_no_flap | 29 | 44.937203373s | 44.937203373s | 95ms | 95ms | 0.750 | 0.750 | 4.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | recovery_no_flap | 47 | 44.810457119s | 44.810457119s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | recovery_no_flap | 71 | 44.958361864s | 44.958361864s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | recovery_no_flap | 101 | 44.837077225s | 44.837077225s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | recovery_no_flap | 131 | 44.901274352s | 44.901274352s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | recovery_no_flap | 173 | 44.9015634s | 44.9015634s | 95ms | 95ms | 0.755 | 0.755 | 1.2s | 3 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | all_channels_bad | 11 | 2m10.9s | 2m10.9s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | all_channels_bad | 29 | 2m9.8s | 2m9.8s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | all_channels_bad | 47 | 2m5.2s | 2m5.2s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | all_channels_bad | 71 | 2m9.4s | 2m9.4s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | all_channels_bad | 101 | 2m5.7s | 2m5.7s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | all_channels_bad | 131 | 2m5.1s | 2m5.1s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | all_channels_bad | 173 | 2m9.3s | 2m9.3s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| short-window | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | not detected | 0 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| short-window | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | not detected | 0 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| short-window | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | not detected | 0 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| short-window | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | not detected | 0 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| short-window | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | not detected | 0 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| short-window | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | not detected | 0 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| short-window | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | not detected | 0 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| short-window | healthy_steady_state | 11 | 306.901147ms | 306.99182ms | 29.560991ms | 29.568674ms | 0.015 | 0.015 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.03% | neutral |
| short-window | healthy_steady_state | 29 | 306.449512ms | 306.872371ms | 29.524996ms | 29.53103ms | 0.015 | 0.015 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.06% | neutral |
| short-window | healthy_steady_state | 47 | 306.757116ms | 307.058184ms | 29.42026ms | 29.429223ms | 0.028 | 0.028 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.05% | neutral |
| short-window | healthy_steady_state | 71 | 304.971872ms | 305.036931ms | 29.463197ms | 29.476368ms | 0.018 | 0.018 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.03% | neutral |
| short-window | healthy_steady_state | 101 | 306.026121ms | 306.089522ms | 29.656679ms | 29.666591ms | 0.023 | 0.023 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.03% | neutral |
| short-window | healthy_steady_state | 131 | 307.461992ms | 307.655414ms | 29.32466ms | 29.32802ms | 0.022 | 0.022 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.03% | neutral |
| short-window | healthy_steady_state | 173 | 306.762211ms | 306.866751ms | 29.635213ms | 29.652251ms | 0.022 | 0.022 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.04% | neutral |
| stable-recovery | gradual_degradation | 11 | 2m18.05810847s | 2m18.05810847s | 111.638008ms | 111.638008ms | 0.738 | 0.738 | 2.2s | 6 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| stable-recovery | gradual_degradation | 29 | 2m17.230127598s | 2m17.230127598s | 111.973508ms | 111.973508ms | 0.737 | 0.737 | 2.6s | 7 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| stable-recovery | gradual_degradation | 47 | 2m18.543146187s | 2m18.543146187s | 112.017461ms | 112.017461ms | 0.743 | 0.743 | 2.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| stable-recovery | gradual_degradation | 71 | 2m18.842157616s | 2m18.842157616s | 112.679273ms | 112.679273ms | 0.742 | 0.742 | 2.4s | 7 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| stable-recovery | gradual_degradation | 101 | 2m17.896601353s | 2m17.896601353s | 112.288179ms | 112.288179ms | 0.740 | 0.740 | 2.2s | 7 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| stable-recovery | gradual_degradation | 131 | 2m18.257709744s | 2m18.257709744s | 112.50077ms | 112.50077ms | 0.742 | 0.743 | 2.2s | 7 | 450 | not detected | 0 | 0.002 | -0.00% | neutral |
| stable-recovery | gradual_degradation | 173 | 2m18.511286942s | 2m18.511286942s | 112.241073ms | 112.241073ms | 0.740 | 0.740 | 1.4s | 4 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| stable-recovery | sudden_outage | 11 | 306.854065ms | 472.597002ms | 29.758413ms | 40.050908ms | 0.669 | 0.038 | 2s | 5 | 10 | 3.6s | 0 | 0.002 | 81.08% | win |
| stable-recovery | sudden_outage | 29 | 305.736046ms | 468.508565ms | 29.586311ms | 39.871299ms | 0.667 | 0.040 | 2s | 5 | 10 | 3.6s | 0 | 0.002 | 81.04% | win |
| stable-recovery | sudden_outage | 47 | 306.757116ms | 476.943647ms | 29.297237ms | 39.405096ms | 0.676 | 0.053 | 2.4s | 7 | 12 | 4s | 0 | 0.002 | 80.13% | win |
| stable-recovery | sudden_outage | 71 | 306.766218ms | 473.760036ms | 29.362123ms | 39.983305ms | 0.673 | 0.047 | 2s | 4 | 10 | 3.6s | 0 | 0.002 | 80.72% | win |
| stable-recovery | sudden_outage | 101 | 306.026121ms | 470.23154ms | 29.696734ms | 40.521742ms | 0.673 | 0.049 | 2s | 4 | 10 | 3.6s | 0 | 0.002 | 80.56% | win |
| stable-recovery | sudden_outage | 131 | 308.156379ms | 473.945235ms | 29.458433ms | 40.270961ms | 0.676 | 0.040 | 2s | 5 | 10 | 3.6s | 0 | 0.002 | 80.99% | win |
| stable-recovery | sudden_outage | 173 | 307.890143ms | 476.089995ms | 29.101676ms | 39.998469ms | 0.673 | 0.060 | 1.2s | 3 | 10 | 3.6s | 0 | 0.002 | 80.07% | win |
| stable-recovery | capacity_aggregation | 11 | 1m19.876239452s | 11.563571524s | 25ms | 28ms | 0.988 | 0.908 | 0s | 0 | 266 | 3.25s | 4 | 0.007 | 84.18% | win |
| stable-recovery | capacity_aggregation | 29 | 1m19.971533361s | 11.618700883s | 25ms | 28ms | 0.988 | 0.906 | 0s | 0 | 266 | 3.125s | 4 | 0.007 | 84.18% | win |
| stable-recovery | capacity_aggregation | 47 | 1m19.89581372s | 11.58629394s | 25ms | 28ms | 0.988 | 0.907 | 0s | 0 | 264 | 3.25s | 4 | 0.007 | 84.20% | win |
| stable-recovery | capacity_aggregation | 71 | 1m19.767790605s | 11.551655032s | 25ms | 28ms | 0.988 | 0.906 | 0s | 0 | 267 | 3.25s | 4 | 0.007 | 84.19% | win |
| stable-recovery | capacity_aggregation | 101 | 1m20.040921681s | 11.490471015s | 25ms | 28ms | 0.990 | 0.914 | 0s | 0 | 265 | 3.25s | 4 | 0.007 | 84.27% | win |
| stable-recovery | capacity_aggregation | 131 | 1m19.734245977s | 11.610125239s | 25ms | 28ms | 0.988 | 0.908 | 0s | 0 | 268 | 3.25s | 4 | 0.007 | 84.10% | win |
| stable-recovery | capacity_aggregation | 173 | 1m19.865475022s | 11.605976302s | 25ms | 28ms | 0.988 | 0.908 | 0s | 0 | 266 | 3.125s | 4 | 0.007 | 84.15% | win |
| stable-recovery | transient_spike | 11 | 12.6925426s | 12.6925426s | 90ms | 90ms | 0.396 | 0.396 | 4.4s | 5 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| stable-recovery | transient_spike | 29 | 12.307633594s | 12.307633594s | 90ms | 90ms | 0.384 | 0.384 | 4.4s | 5 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| stable-recovery | transient_spike | 47 | 12.854235214s | 12.854235214s | 90ms | 90ms | 0.402 | 0.402 | 4.6s | 6 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| stable-recovery | transient_spike | 71 | 12.659328791s | 12.659328791s | 90ms | 90ms | 0.400 | 0.400 | 4.4s | 4 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| stable-recovery | transient_spike | 101 | 12.607221347s | 12.607221347s | 90ms | 90ms | 0.398 | 0.398 | 4.2s | 4 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| stable-recovery | transient_spike | 131 | 12.512324827s | 12.512324827s | 90ms | 90ms | 0.398 | 0.400 | 4.4s | 5 | 300 | not detected | 0 | 0.002 | -0.04% | neutral |
| stable-recovery | transient_spike | 173 | 12.652228169s | 12.652228169s | 90ms | 90ms | 0.398 | 0.398 | 1.2s | 3 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| stable-recovery | stale_candidate | 11 | 46.540000047s | 27.103333361s | 110ms | 110ms | 0.889 | 0.550 | 9.99999999s | 12 | 91 | 34.666666632s | 0 | 0.035 | 45.78% | win |
| stable-recovery | stale_candidate | 29 | 51.736666713s | 29.496666693s | 110ms | 110ms | 0.883 | 0.544 | 9.666666657s | 9 | 90 | 34.333333299s | 0 | 0.035 | 47.27% | win |
| stable-recovery | stale_candidate | 47 | 51.540000046s | 28.360000028s | 110ms | 110ms | 0.883 | 0.526 | 9.99999999s | 10 | 90 | 34.333333299s | 0 | 0.035 | 49.43% | win |
| stable-recovery | stale_candidate | 71 | 50.403333377s | 34.753333368s | 110ms | 110ms | 0.889 | 0.667 | 10.333333323s | 11 | 109 | 40.999999959s | 0 | 0.041 | 35.02% | win |
| stable-recovery | stale_candidate | 101 | 48.010000047s | 27.83000003s | 110ms | 110ms | 0.883 | 0.532 | 9.99999999s | 9 | 90 | 34.333333299s | 0 | 0.035 | 46.37% | win |
| stable-recovery | stale_candidate | 131 | 47.873333381s | 25.966666694s | 110ms | 110ms | 0.883 | 0.538 | 9.666666657s | 10 | 90 | 34.333333299s | 0 | 0.035 | 49.39% | win |
| stable-recovery | stale_candidate | 173 | 46.87333338s | 26.163333359s | 110ms | 110ms | 0.883 | 0.544 | 9.99999999s | 10 | 90 | 34.333333299s | 0 | 0.035 | 48.00% | win |
| stable-recovery | recovery_no_flap | 11 | 44.799011002s | 44.799011002s | 95ms | 95ms | 0.752 | 0.752 | 4.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| stable-recovery | recovery_no_flap | 29 | 44.937203373s | 44.937203373s | 95ms | 95ms | 0.750 | 0.750 | 4.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| stable-recovery | recovery_no_flap | 47 | 44.810457119s | 44.810457119s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| stable-recovery | recovery_no_flap | 71 | 44.958361864s | 44.958361864s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| stable-recovery | recovery_no_flap | 101 | 44.837077225s | 44.837077225s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| stable-recovery | recovery_no_flap | 131 | 44.901274352s | 44.901274352s | 95ms | 95ms | 0.757 | 0.758 | 4.2s | 5 | 450 | not detected | 0 | 0.002 | -0.01% | neutral |
| stable-recovery | recovery_no_flap | 173 | 44.9015634s | 44.9015634s | 95ms | 95ms | 0.755 | 0.755 | 1.2s | 3 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| stable-recovery | all_channels_bad | 11 | 2m10.9s | 2m10.9s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| stable-recovery | all_channels_bad | 29 | 2m9.8s | 2m9.8s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| stable-recovery | all_channels_bad | 47 | 2m5.2s | 2m5.2s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| stable-recovery | all_channels_bad | 71 | 2m9.4s | 2m9.4s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| stable-recovery | all_channels_bad | 101 | 2m5.7s | 2m5.7s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| stable-recovery | all_channels_bad | 131 | 2m5.1s | 2m5.1s | 100ms | 100ms | 0.676 | 0.678 | 0s | 0 | 300 | not detected | 0 | 0.002 | -0.00% | neutral |
| stable-recovery | all_channels_bad | 173 | 2m9.3s | 2m9.3s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| stable-recovery | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| stable-recovery | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| stable-recovery | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| stable-recovery | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| stable-recovery | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| stable-recovery | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| stable-recovery | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| stable-recovery | healthy_steady_state | 11 | 306.901147ms | 306.99182ms | 29.560991ms | 29.568674ms | 0.015 | 0.015 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.03% | neutral |
| stable-recovery | healthy_steady_state | 29 | 306.449512ms | 306.872371ms | 29.524996ms | 29.53103ms | 0.015 | 0.015 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.06% | neutral |
| stable-recovery | healthy_steady_state | 47 | 306.757116ms | 307.058184ms | 29.42026ms | 29.429223ms | 0.028 | 0.028 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.05% | neutral |
| stable-recovery | healthy_steady_state | 71 | 304.971872ms | 305.036931ms | 29.463197ms | 29.476368ms | 0.018 | 0.018 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.03% | neutral |
| stable-recovery | healthy_steady_state | 101 | 306.026121ms | 306.089522ms | 29.656679ms | 29.666591ms | 0.023 | 0.023 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.03% | neutral |
| stable-recovery | healthy_steady_state | 131 | 307.461992ms | 307.655414ms | 29.32466ms | 29.32802ms | 0.022 | 0.023 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.68% | neutral |
| stable-recovery | healthy_steady_state | 173 | 306.762211ms | 306.866751ms | 29.635213ms | 29.652251ms | 0.022 | 0.022 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.04% | neutral |
| fast-recovery | gradual_degradation | 11 | 2m18.05810847s | 2m18.05810847s | 111.638008ms | 111.638008ms | 0.738 | 0.738 | 2.2s | 6 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | gradual_degradation | 29 | 2m17.230127598s | 2m17.230127598s | 111.973508ms | 111.973508ms | 0.737 | 0.737 | 2.4s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | gradual_degradation | 47 | 2m18.543146187s | 2m18.543146187s | 112.017461ms | 112.017461ms | 0.743 | 0.743 | 2.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | gradual_degradation | 71 | 2m18.842157616s | 2m18.842157616s | 112.679273ms | 112.679273ms | 0.742 | 0.742 | 2.4s | 7 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | gradual_degradation | 101 | 2m17.896601353s | 2m17.896601353s | 112.288179ms | 112.288179ms | 0.740 | 0.740 | 2.2s | 7 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | gradual_degradation | 131 | 2m18.257709744s | 2m18.257709744s | 112.50077ms | 112.50077ms | 0.742 | 0.742 | 2.2s | 7 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | gradual_degradation | 173 | 2m18.511286942s | 2m18.511286942s | 112.241073ms | 112.241073ms | 0.740 | 0.740 | 1.4s | 4 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | sudden_outage | 11 | 306.854065ms | 472.597002ms | 29.758413ms | 40.050908ms | 0.669 | 0.038 | 2s | 5 | 10 | 3.6s | 0 | 0.002 | 81.08% | win |
| fast-recovery | sudden_outage | 29 | 305.736046ms | 468.508565ms | 29.586311ms | 39.871299ms | 0.667 | 0.040 | 2s | 5 | 10 | 3.6s | 0 | 0.002 | 81.04% | win |
| fast-recovery | sudden_outage | 47 | 306.757116ms | 476.943647ms | 29.297237ms | 39.405096ms | 0.676 | 0.053 | 2.4s | 7 | 12 | 4s | 0 | 0.002 | 80.13% | win |
| fast-recovery | sudden_outage | 71 | 306.766218ms | 473.760036ms | 29.362123ms | 39.983305ms | 0.673 | 0.047 | 2s | 4 | 10 | 3.6s | 0 | 0.002 | 80.72% | win |
| fast-recovery | sudden_outage | 101 | 306.026121ms | 470.23154ms | 29.696734ms | 40.521742ms | 0.673 | 0.049 | 2s | 4 | 10 | 3.6s | 0 | 0.002 | 80.56% | win |
| fast-recovery | sudden_outage | 131 | 308.156379ms | 473.89148ms | 29.458433ms | 40.270961ms | 0.676 | 0.038 | 2s | 5 | 10 | 3.6s | 0 | 0.002 | 81.09% | win |
| fast-recovery | sudden_outage | 173 | 307.890143ms | 476.089995ms | 29.101676ms | 39.998469ms | 0.673 | 0.060 | 1.2s | 3 | 10 | 3.6s | 0 | 0.002 | 80.07% | win |
| fast-recovery | capacity_aggregation | 11 | 1m19.876239452s | 29.146310126s | 25ms | 28ms | 0.988 | 0.942 | 0s | 0 | 291 | 1.125s | 2 | 0.006 | 65.16% | win |
| fast-recovery | capacity_aggregation | 29 | 1m19.971533361s | 29.931097787s | 25ms | 28ms | 0.988 | 0.940 | 0s | 0 | 314 | 1.125s | 2 | 0.006 | 64.88% | win |
| fast-recovery | capacity_aggregation | 47 | 1m19.89581372s | 29.244526898s | 25ms | 28ms | 0.988 | 0.940 | 0s | 0 | 310 | 1.125s | 2 | 0.006 | 65.47% | win |
| fast-recovery | capacity_aggregation | 71 | 1m19.767790605s | 29.096476904s | 25ms | 28ms | 0.988 | 0.939 | 0s | 0 | 291 | 1.125s | 2 | 0.006 | 65.21% | win |
| fast-recovery | capacity_aggregation | 101 | 1m20.040921681s | 29.812563477s | 25ms | 28ms | 0.990 | 0.944 | 0s | 0 | 314 | 1.125s | 2 | 0.006 | 65.04% | win |
| fast-recovery | capacity_aggregation | 131 | 1m19.734245977s | 29.698943803s | 25ms | 28ms | 0.988 | 0.943 | 0s | 0 | 314 | 1.125s | 2 | 0.006 | 64.96% | win |
| fast-recovery | capacity_aggregation | 173 | 1m19.865475022s | 29.731350675s | 25ms | 28ms | 0.988 | 0.940 | 0s | 0 | 316 | 1.125s | 2 | 0.006 | 65.02% | win |
| fast-recovery | transient_spike | 11 | 12.6925426s | 12.6925426s | 90ms | 90ms | 0.396 | 0.396 | 4.4s | 5 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | transient_spike | 29 | 12.307633594s | 12.307633594s | 90ms | 90ms | 0.384 | 0.384 | 4.4s | 5 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | transient_spike | 47 | 12.854235214s | 12.854235214s | 90ms | 90ms | 0.402 | 0.402 | 4.6s | 6 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | transient_spike | 71 | 12.659328791s | 12.659328791s | 90ms | 90ms | 0.400 | 0.400 | 4.4s | 4 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | transient_spike | 101 | 12.607221347s | 12.607221347s | 90ms | 90ms | 0.398 | 0.398 | 4.2s | 4 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | transient_spike | 131 | 12.512324827s | 12.512324827s | 90ms | 90ms | 0.398 | 0.398 | 4.4s | 5 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | transient_spike | 173 | 12.652228169s | 12.652228169s | 90ms | 90ms | 0.398 | 0.398 | 1.2s | 3 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | stale_candidate | 11 | 46.540000047s | 16.650000016s | 110ms | 110ms | 0.889 | 0.363 | 9.666666657s | 9 | 59 | 23.666666643s | 0 | 0.029 | 67.10% | win |
| fast-recovery | stale_candidate | 29 | 51.736666713s | 17.376666683s | 110ms | 110ms | 0.883 | 0.351 | 9.666666657s | 9 | 58 | 23.33333331s | 0 | 0.029 | 69.56% | win |
| fast-recovery | stale_candidate | 47 | 51.540000046s | 16.846666683s | 110ms | 110ms | 0.883 | 0.374 | 9.333333324s | 9 | 60 | 23.666666643s | 0 | 0.029 | 70.10% | win |
| fast-recovery | stale_candidate | 71 | 50.403333377s | 16.846666683s | 110ms | 110ms | 0.889 | 0.351 | 9.666666657s | 10 | 58 | 23.33333331s | 0 | 0.029 | 69.84% | win |
| fast-recovery | stale_candidate | 101 | 48.010000047s | 15.983333348s | 110ms | 110ms | 0.883 | 0.368 | 9.666666657s | 8 | 58 | 23.33333331s | 0 | 0.029 | 69.45% | win |
| fast-recovery | stale_candidate | 131 | 47.873333381s | 15.846666682s | 110ms | 110ms | 0.883 | 0.351 | 9.666666657s | 10 | 58 | 23.33333331s | 0 | 0.029 | 69.56% | win |
| fast-recovery | stale_candidate | 173 | 46.87333338s | 15.513333349s | 110ms | 110ms | 0.883 | 0.351 | 9.666666657s | 9 | 58 | 23.33333331s | 0 | 0.029 | 69.40% | win |
| fast-recovery | recovery_no_flap | 11 | 44.799011002s | 44.799011002s | 95ms | 95ms | 0.752 | 0.752 | 4.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | recovery_no_flap | 29 | 44.937203373s | 44.937203373s | 95ms | 95ms | 0.750 | 0.750 | 4.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | recovery_no_flap | 47 | 44.810457119s | 44.810457119s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | recovery_no_flap | 71 | 44.958361864s | 44.958361864s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | recovery_no_flap | 101 | 44.837077225s | 44.837077225s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | recovery_no_flap | 131 | 44.901274352s | 44.901274352s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | recovery_no_flap | 173 | 44.9015634s | 44.9015634s | 95ms | 95ms | 0.755 | 0.755 | 1.2s | 3 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | all_channels_bad | 11 | 2m10.9s | 2m10.9s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | all_channels_bad | 29 | 2m9.8s | 2m9.8s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | all_channels_bad | 47 | 2m5.2s | 2m5.2s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | all_channels_bad | 71 | 2m9.4s | 2m9.4s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | all_channels_bad | 101 | 2m5.7s | 2m5.7s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | all_channels_bad | 131 | 2m5.1s | 2m5.1s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | all_channels_bad | 173 | 2m9.3s | 2m9.3s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| fast-recovery | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| fast-recovery | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| fast-recovery | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| fast-recovery | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| fast-recovery | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| fast-recovery | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| fast-recovery | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| fast-recovery | healthy_steady_state | 11 | 306.901147ms | 306.99182ms | 29.560991ms | 29.568674ms | 0.015 | 0.015 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.03% | neutral |
| fast-recovery | healthy_steady_state | 29 | 306.449512ms | 306.872371ms | 29.524996ms | 29.53103ms | 0.015 | 0.015 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.06% | neutral |
| fast-recovery | healthy_steady_state | 47 | 306.757116ms | 307.058184ms | 29.42026ms | 29.429223ms | 0.028 | 0.028 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.05% | neutral |
| fast-recovery | healthy_steady_state | 71 | 304.971872ms | 305.036931ms | 29.463197ms | 29.476368ms | 0.018 | 0.018 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.03% | neutral |
| fast-recovery | healthy_steady_state | 101 | 306.026121ms | 306.089522ms | 29.656679ms | 29.666591ms | 0.023 | 0.023 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.03% | neutral |
| fast-recovery | healthy_steady_state | 131 | 307.461992ms | 307.655414ms | 29.32466ms | 29.32802ms | 0.022 | 0.022 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.03% | neutral |
| fast-recovery | healthy_steady_state | 173 | 306.762211ms | 306.866751ms | 29.635213ms | 29.652251ms | 0.022 | 0.022 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.04% | neutral |
| hard-failure-confirmed | gradual_degradation | 11 | 2m18.05810847s | 2m18.05810847s | 111.638008ms | 111.638008ms | 0.738 | 0.738 | 2.2s | 6 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | gradual_degradation | 29 | 2m17.230127598s | 2m17.230127598s | 111.973508ms | 111.973508ms | 0.737 | 0.737 | 2.4s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | gradual_degradation | 47 | 2m18.543146187s | 2m18.543146187s | 112.017461ms | 112.017461ms | 0.743 | 0.743 | 2.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | gradual_degradation | 71 | 2m18.842157616s | 2m18.842157616s | 112.679273ms | 112.679273ms | 0.742 | 0.742 | 2.4s | 7 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | gradual_degradation | 101 | 2m17.896601353s | 2m17.896601353s | 112.288179ms | 112.288179ms | 0.740 | 0.740 | 2.2s | 7 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | gradual_degradation | 131 | 2m18.257709744s | 2m18.257709744s | 112.50077ms | 112.50077ms | 0.742 | 0.743 | 2.2s | 7 | 450 | not detected | 0 | 0.002 | -0.00% | neutral |
| hard-failure-confirmed | gradual_degradation | 173 | 2m18.511286942s | 2m18.511286942s | 112.241073ms | 112.241073ms | 0.740 | 0.740 | 1.4s | 4 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | sudden_outage | 11 | 306.854065ms | 473.499055ms | 29.758413ms | 39.982664ms | 0.669 | 0.044 | 2.6s | 8 | 13 | 4.2s | 0 | 0.002 | 80.19% | win |
| hard-failure-confirmed | sudden_outage | 29 | 305.736046ms | 471.487188ms | 29.586311ms | 40.071145ms | 0.667 | 0.038 | 2.2s | 6 | 11 | 3.8s | 0 | 0.002 | 80.86% | win |
| hard-failure-confirmed | sudden_outage | 47 | 306.757116ms | 473.153942ms | 29.297237ms | 39.815778ms | 0.676 | 0.049 | 2.6s | 8 | 13 | 4.2s | 0 | 0.002 | 80.09% | win |
| hard-failure-confirmed | sudden_outage | 71 | 306.766218ms | 472.534043ms | 29.362123ms | 40.078569ms | 0.673 | 0.056 | 2.2s | 5 | 11 | 3.8s | 0 | 0.002 | 80.11% | win |
| hard-failure-confirmed | sudden_outage | 101 | 306.026121ms | 475.225328ms | 29.696734ms | 40.206552ms | 0.673 | 0.069 | 2.2s | 5 | 21 | 3.8s | 2 | 0.002 | 77.49% | win |
| hard-failure-confirmed | sudden_outage | 131 | 308.156379ms | 477.3212ms | 29.458433ms | 40.174049ms | 0.676 | 0.062 | 2.2s | 6 | 11 | 3.8s | 0 | 0.002 | 79.76% | win |
| hard-failure-confirmed | sudden_outage | 173 | 307.890143ms | 472.362573ms | 29.101676ms | 40.039804ms | 0.673 | 0.047 | 1.2s | 3 | 11 | 3.8s | 0 | 0.002 | 80.51% | win |
| hard-failure-confirmed | capacity_aggregation | 11 | 1m19.876239452s | 36.900507488s | 25ms | 28ms | 0.988 | 0.943 | 0s | 0 | 360 | 1.125s | 2 | 0.006 | 57.72% | win |
| hard-failure-confirmed | capacity_aggregation | 29 | 1m19.971533361s | 25.222714186s | 25ms | 28ms | 0.988 | 0.943 | 0s | 0 | 238 | 1.125s | 2 | 0.006 | 68.90% | win |
| hard-failure-confirmed | capacity_aggregation | 47 | 1m19.89581372s | 37.042298517s | 25ms | 28ms | 0.988 | 0.944 | 0s | 0 | 360 | 1.125s | 2 | 0.006 | 57.60% | win |
| hard-failure-confirmed | capacity_aggregation | 71 | 1m19.767790605s | 22.210940613s | 25ms | 28ms | 0.988 | 0.926 | 0s | 0 | 250 | 1.125s | 3 | 0.006 | 73.29% | win |
| hard-failure-confirmed | capacity_aggregation | 101 | 1m20.040921681s | 35.166703648s | 25ms | 28ms | 0.990 | 0.946 | 0s | 0 | 342 | 1.125s | 2 | 0.006 | 59.57% | win |
| hard-failure-confirmed | capacity_aggregation | 131 | 1m19.734245977s | 25.071434603s | 25ms | 28ms | 0.988 | 0.940 | 0s | 0 | 223 | 1.125s | 2 | 0.006 | 68.95% | win |
| hard-failure-confirmed | capacity_aggregation | 173 | 1m19.865475022s | 28.502344591s | 25ms | 28ms | 0.988 | 0.943 | 0s | 0 | 286 | 1.125s | 2 | 0.006 | 65.91% | win |
| hard-failure-confirmed | transient_spike | 11 | 12.6925426s | 12.6925426s | 90ms | 90ms | 0.396 | 0.396 | 4.4s | 5 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | transient_spike | 29 | 12.307633594s | 12.307633594s | 90ms | 90ms | 0.384 | 0.384 | 4.4s | 5 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | transient_spike | 47 | 12.854235214s | 12.854235214s | 90ms | 90ms | 0.402 | 0.402 | 4.6s | 6 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | transient_spike | 71 | 12.659328791s | 12.659328791s | 90ms | 90ms | 0.400 | 0.400 | 4.4s | 4 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | transient_spike | 101 | 12.607221347s | 12.607221347s | 90ms | 90ms | 0.398 | 0.398 | 4.2s | 4 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | transient_spike | 131 | 12.512324827s | 12.512324827s | 90ms | 90ms | 0.398 | 0.400 | 4.4s | 5 | 300 | not detected | 0 | 0.002 | -0.04% | neutral |
| hard-failure-confirmed | transient_spike | 173 | 12.652228169s | 12.652228169s | 90ms | 90ms | 0.398 | 0.398 | 1.2s | 3 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | stale_candidate | 11 | 46.540000047s | 19.906666687s | 110ms | 110ms | 0.889 | 0.433 | 9.99999999s | 12 | 71 | 27.333333306s | 0 | 0.029 | 60.42% | win |
| hard-failure-confirmed | stale_candidate | 29 | 51.736666713s | 22.573333353s | 110ms | 110ms | 0.883 | 0.444 | 9.666666657s | 9 | 71 | 27.333333306s | 0 | 0.029 | 60.36% | win |
| hard-failure-confirmed | stale_candidate | 47 | 51.540000046s | 21.573333354s | 110ms | 110ms | 0.883 | 0.427 | 9.333333324s | 9 | 71 | 27.333333306s | 0 | 0.029 | 62.17% | win |
| hard-failure-confirmed | stale_candidate | 71 | 50.403333377s | 27.830000028s | 110ms | 110ms | 0.889 | 0.573 | 9.666666657s | 10 | 90 | 33.999999966s | 0 | 0.035 | 49.00% | win |
| hard-failure-confirmed | stale_candidate | 101 | 48.010000047s | 19.770000027s | 110ms | 110ms | 0.883 | 0.433 | 9.99999999s | 9 | 71 | 27.333333306s | 0 | 0.029 | 62.06% | win |
| hard-failure-confirmed | stale_candidate | 131 | 47.873333381s | 19.906666687s | 110ms | 110ms | 0.883 | 0.433 | 9.666666657s | 10 | 71 | 27.333333306s | 0 | 0.029 | 61.66% | win |
| hard-failure-confirmed | stale_candidate | 173 | 46.87333338s | 20.103333362s | 110ms | 110ms | 0.883 | 0.439 | 9.99999999s | 10 | 72 | 27.666666639s | 0 | 0.029 | 60.36% | win |
| hard-failure-confirmed | recovery_no_flap | 11 | 44.799011002s | 44.799011002s | 95ms | 95ms | 0.752 | 0.752 | 4.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | recovery_no_flap | 29 | 44.937203373s | 44.937203373s | 95ms | 95ms | 0.750 | 0.750 | 4.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | recovery_no_flap | 47 | 44.810457119s | 44.810457119s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | recovery_no_flap | 71 | 44.958361864s | 44.958361864s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | recovery_no_flap | 101 | 44.837077225s | 44.837077225s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | recovery_no_flap | 131 | 44.901274352s | 44.901274352s | 95ms | 95ms | 0.757 | 0.758 | 4.2s | 5 | 450 | not detected | 0 | 0.002 | -0.01% | neutral |
| hard-failure-confirmed | recovery_no_flap | 173 | 44.9015634s | 44.9015634s | 95ms | 95ms | 0.755 | 0.755 | 1.2s | 3 | 450 | not detected | 0 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | all_channels_bad | 11 | 2m10.9s | 2m10.9s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | all_channels_bad | 29 | 2m9.8s | 2m9.8s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | all_channels_bad | 47 | 2m5.2s | 2m5.2s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | all_channels_bad | 71 | 2m9.4s | 2m9.4s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | all_channels_bad | 101 | 2m5.7s | 2m5.7s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | all_channels_bad | 131 | 2m5.1s | 2m5.1s | 100ms | 100ms | 0.676 | 0.678 | 0s | 0 | 300 | not detected | 0 | 0.002 | -0.00% | neutral |
| hard-failure-confirmed | all_channels_bad | 173 | 2m9.3s | 2m9.3s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| hard-failure-confirmed | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| hard-failure-confirmed | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| hard-failure-confirmed | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| hard-failure-confirmed | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| hard-failure-confirmed | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| hard-failure-confirmed | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| hard-failure-confirmed | healthy_steady_state | 11 | 306.901147ms | 306.99182ms | 29.560991ms | 29.568674ms | 0.015 | 0.015 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.03% | neutral |
| hard-failure-confirmed | healthy_steady_state | 29 | 306.449512ms | 306.872371ms | 29.524996ms | 29.53103ms | 0.015 | 0.015 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.06% | neutral |
| hard-failure-confirmed | healthy_steady_state | 47 | 306.757116ms | 307.058184ms | 29.42026ms | 29.429223ms | 0.028 | 0.028 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.05% | neutral |
| hard-failure-confirmed | healthy_steady_state | 71 | 304.971872ms | 305.036931ms | 29.463197ms | 29.476368ms | 0.018 | 0.018 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.03% | neutral |
| hard-failure-confirmed | healthy_steady_state | 101 | 306.026121ms | 306.089522ms | 29.656679ms | 29.666591ms | 0.023 | 0.023 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.03% | neutral |
| hard-failure-confirmed | healthy_steady_state | 131 | 307.461992ms | 307.655414ms | 29.32466ms | 29.32802ms | 0.022 | 0.023 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.68% | neutral |
| hard-failure-confirmed | healthy_steady_state | 173 | 306.762211ms | 306.866751ms | 29.635213ms | 29.652251ms | 0.022 | 0.022 | not detected | 0 | 0 | not detected | 0 | 0.002 | -0.04% | neutral |
