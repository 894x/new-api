# Dynamic routing simulation

Significant improvement gate: 15.00%; maximum regression: 8.00%; minimum scenario win rate: 0.45.

## Ranking

| Rank | Parameters | Average improvement | Worst seed | Win rate | Significant | Regressed runs |
|---:|---|---:|---:|---:|:---:|---:|
| 1 | stable-recovery | 18.89% | -0.15% | 0.30 | false | 0 |
| 2 | hard-failure-confirmed | 18.60% | -0.14% | 0.30 | false | 0 |
| 3 | balanced | 18.47% | -0.14% | 0.30 | false | 0 |
| 4 | fast-recovery | 18.21% | -0.14% | 0.29 | false | 0 |
| 5 | short-window | 15.43% | -0.14% | 0.23 | false | 0 |
| 6 | aggressive | 12.51% | -0.14% | 0.25 | false | 0 |
| 7 | conservative | 11.24% | -0.17% | 0.14 | false | 0 |
| 8 | fast-low-probe | 6.22% | -0.14% | 0.09 | false | 0 |

## Every scenario and seed

| Parameters | Scenario | Seed | Static p95 TTFT | Dynamic p95 TTFT | Static p95 TPOT | Dynamic p95 TPOT | Static SLO violations | Dynamic SLO violations | Detection elapsed | Detection observations | Bad exposure | Mitigation elapsed | Route reversals | Probe cost | Improvement | Result |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| conservative | gradual_degradation | 11 | 2m18.05810847s | 2m15.165692071s | 111.638008ms | 111.638008ms | 0.738 | 0.730 | 2.2s | 6 | 444 | not detected | 19 | 0.017 | 2.52% | neutral |
| conservative | gradual_degradation | 29 | 2m17.230127598s | 2.092941142s | 111.973508ms | 49.325396ms | 0.737 | 0.132 | 2.6s | 6 | 82 | 14s | 62 | 0.013 | 98.15% | win |
| conservative | gradual_degradation | 47 | 2m18.543146187s | 2m17.60995586s | 112.017461ms | 111.887792ms | 0.743 | 0.740 | 2.4s | 6 | 448 | not detected | 5 | 0.005 | 0.77% | neutral |
| conservative | gradual_degradation | 71 | 2m18.842157616s | 2m17.665278025s | 112.679273ms | 112.679273ms | 0.742 | 0.737 | 2.4s | 7 | 447 | not detected | 15 | 0.013 | 1.14% | neutral |
| conservative | gradual_degradation | 101 | 2m17.896601353s | 2m15.055825918s | 112.288179ms | 112.058045ms | 0.740 | 0.732 | 2.2s | 6 | 445 | not detected | 17 | 0.015 | 2.40% | neutral |
| conservative | gradual_degradation | 131 | 2m18.257709744s | 2m17.531746925s | 112.50077ms | 112.375658ms | 0.742 | 0.740 | 2.2s | 7 | 449 | not detected | 7 | 0.007 | 0.59% | neutral |
| conservative | gradual_degradation | 173 | 2m18.511286942s | 2m14.967068042s | 112.241073ms | 112.196973ms | 0.740 | 0.730 | 1.4s | 4 | 444 | not detected | 19 | 0.017 | 2.87% | neutral |
| conservative | sudden_outage | 11 | 306.854065ms | 307.805481ms | 29.758413ms | 29.830249ms | 0.669 | 0.669 | 2.6s | 8 | 300 | not detected | 7 | 0.009 | -0.03% | neutral |
| conservative | sudden_outage | 29 | 305.736046ms | 309.0818ms | 29.586311ms | 29.943397ms | 0.667 | 0.667 | 2.2s | 5 | 300 | not detected | 13 | 0.016 | -0.12% | neutral |
| conservative | sudden_outage | 47 | 306.757116ms | 309.640816ms | 29.297237ms | 29.532321ms | 0.676 | 0.671 | 2.6s | 8 | 298 | not detected | 5 | 0.007 | 0.50% | neutral |
| conservative | sudden_outage | 71 | 306.766218ms | 336.815719ms | 29.362123ms | 29.669521ms | 0.673 | 0.671 | 2.2s | 5 | 299 | not detected | 11 | 0.013 | -0.17% | neutral |
| conservative | sudden_outage | 101 | 306.026121ms | 352.234552ms | 29.696734ms | 29.841869ms | 0.673 | 0.669 | 2.2s | 4 | 298 | not detected | 11 | 0.013 | -0.06% | neutral |
| conservative | sudden_outage | 131 | 308.156379ms | 333.789359ms | 29.458433ms | 29.702717ms | 0.676 | 0.673 | 2.2s | 6 | 299 | not detected | 7 | 0.009 | -0.10% | neutral |
| conservative | sudden_outage | 173 | 307.890143ms | 333.19714ms | 29.101676ms | 29.974344ms | 0.673 | 0.664 | 1.2s | 3 | 296 | not detected | 15 | 0.018 | 0.66% | neutral |
| conservative | capacity_aggregation | 11 | 1m19.876239452s | 16.810311676s | 25ms | 28ms | 0.988 | 0.932 | 0s | 0 | 327 | 15.625s | 230 | 0.011 | 78.86% | win |
| conservative | capacity_aggregation | 29 | 1m19.971533361s | 15.36947941s | 25ms | 28ms | 0.988 | 0.890 | 0s | 0 | 312 | 17.5s | 264 | 0.011 | 81.33% | win |
| conservative | capacity_aggregation | 47 | 1m19.89581372s | 45.092826656s | 25ms | 28ms | 0.988 | 0.956 | 0s | 0 | 440 | 45s | 166 | 0.011 | 45.29% | win |
| conservative | capacity_aggregation | 71 | 1m19.767790605s | 29.205508709s | 25ms | 28ms | 0.988 | 0.949 | 0s | 0 | 356 | 29.875s | 218 | 0.011 | 64.47% | win |
| conservative | capacity_aggregation | 101 | 1m20.040921681s | 30.140037073s | 25ms | 28ms | 0.990 | 0.942 | 0s | 0 | 357 | 34.375s | 227 | 0.011 | 63.60% | win |
| conservative | capacity_aggregation | 131 | 1m19.734245977s | 43.425388743s | 25ms | 28ms | 0.988 | 0.949 | 0s | 0 | 446 | 49s | 165 | 0.011 | 46.95% | win |
| conservative | capacity_aggregation | 173 | 1m19.865475022s | 30.48463064s | 25ms | 28ms | 0.988 | 0.939 | 0s | 0 | 365 | 30.875s | 232 | 0.011 | 63.07% | win |
| conservative | transient_spike | 11 | 12.6925426s | 12.6925426s | 90ms | 90ms | 0.396 | 0.387 | 4.4s | 5 | 298 | not detected | 11 | 0.013 | 0.32% | neutral |
| conservative | transient_spike | 29 | 12.307633594s | 12.307633594s | 90ms | 90ms | 0.384 | 0.384 | 4.4s | 4 | 300 | not detected | 13 | 0.016 | 0.00% | neutral |
| conservative | transient_spike | 47 | 12.854235214s | 12.689579021s | 90ms | 90ms | 0.402 | 0.398 | 4.6s | 6 | 299 | not detected | 3 | 0.004 | 1.28% | neutral |
| conservative | transient_spike | 71 | 12.659328791s | 12.659328791s | 90ms | 90ms | 0.400 | 0.389 | 4.4s | 4 | 298 | not detected | 13 | 0.016 | 0.50% | neutral |
| conservative | transient_spike | 101 | 12.607221347s | 12.607221347s | 90ms | 90ms | 0.398 | 0.384 | 4.2s | 3 | 297 | not detected | 13 | 0.016 | 0.42% | neutral |
| conservative | transient_spike | 131 | 12.512324827s | 12.512324827s | 90ms | 90ms | 0.398 | 0.389 | 4.4s | 5 | 299 | not detected | 7 | 0.009 | 0.33% | neutral |
| conservative | transient_spike | 173 | 12.652228169s | 12.652228169s | 90ms | 90ms | 0.398 | 0.391 | 1.2s | 3 | 297 | not detected | 13 | 0.016 | 0.30% | neutral |
| conservative | stale_candidate | 11 | 46.540000047s | 43.540000046s | 110ms | 110ms | 0.889 | 0.854 | 10.333333323s | 13 | 145 | not detected | 11 | 0.041 | 5.80% | neutral |
| conservative | stale_candidate | 29 | 51.736666713s | 50.266666713s | 110ms | 110ms | 0.883 | 0.854 | 10.666666656s | 10 | 146 | not detected | 15 | 0.047 | 3.66% | neutral |
| conservative | stale_candidate | 47 | 51.540000046s | 51.600000047s | 110ms | 110ms | 0.883 | 0.877 | 10.999999989s | 12 | 150 | not detected | 1 | 0.006 | 0.53% | neutral |
| conservative | stale_candidate | 71 | 50.403333377s | 48.73666671s | 110ms | 110ms | 0.889 | 0.865 | 11.666666655s | 13 | 147 | not detected | 9 | 0.029 | 4.59% | neutral |
| conservative | stale_candidate | 101 | 48.010000047s | 44.206666712s | 110ms | 110ms | 0.883 | 0.854 | 10.666666656s | 11 | 146 | not detected | 9 | 0.029 | 8.10% | neutral |
| conservative | stale_candidate | 131 | 47.873333381s | 47.070000047s | 110ms | 110ms | 0.883 | 0.865 | 10.333333323s | 11 | 148 | not detected | 7 | 0.023 | 2.12% | neutral |
| conservative | stale_candidate | 173 | 46.87333338s | 46.070000046s | 110ms | 110ms | 0.883 | 0.871 | 10.666666656s | 12 | 149 | not detected | 7 | 0.023 | 2.44% | neutral |
| conservative | recovery_no_flap | 11 | 44.799011002s | 44.144717487s | 95ms | 95ms | 0.752 | 0.742 | 4.2s | 5 | 444 | not detected | 19 | 0.017 | 1.93% | neutral |
| conservative | recovery_no_flap | 29 | 44.937203373s | 44.937203373s | 95ms | 95ms | 0.750 | 0.748 | 4.2s | 4 | 449 | not detected | 15 | 0.013 | 0.06% | neutral |
| conservative | recovery_no_flap | 47 | 44.810457119s | 44.299181422s | 95ms | 95ms | 0.757 | 0.752 | 4.2s | 5 | 447 | not detected | 7 | 0.007 | 1.46% | neutral |
| conservative | recovery_no_flap | 71 | 44.958361864s | 43.745s | 95ms | 95ms | 0.755 | 0.750 | 4.2s | 4 | 447 | not detected | 15 | 0.013 | 2.77% | neutral |
| conservative | recovery_no_flap | 101 | 44.837077225s | 43.945s | 95ms | 95ms | 0.755 | 0.748 | 4.2s | 3 | 445 | not detected | 17 | 0.015 | 2.63% | neutral |
| conservative | recovery_no_flap | 131 | 44.901274352s | 44.249072613s | 95ms | 95ms | 0.757 | 0.755 | 4.2s | 5 | 449 | not detected | 7 | 0.007 | 1.61% | neutral |
| conservative | recovery_no_flap | 173 | 44.9015634s | 43.945s | 95ms | 95ms | 0.755 | 0.745 | 1.2s | 3 | 444 | not detected | 19 | 0.017 | 2.74% | neutral |
| conservative | all_channels_bad | 11 | 2m10.9s | 2m9.2s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 13 | 0.016 | 1.38% | neutral |
| conservative | all_channels_bad | 29 | 2m9.8s | 2m9.8s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 13 | 0.016 | 0.00% | neutral |
| conservative | all_channels_bad | 47 | 2m5.2s | 2m3.2s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 5 | 0.007 | 1.44% | neutral |
| conservative | all_channels_bad | 71 | 2m9.4s | 2m7.6s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 13 | 0.016 | 1.46% | neutral |
| conservative | all_channels_bad | 101 | 2m5.7s | 2m3.7s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 13 | 0.016 | 1.81% | neutral |
| conservative | all_channels_bad | 131 | 2m5.1s | 2m4.5s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 7 | 0.009 | 0.57% | neutral |
| conservative | all_channels_bad | 173 | 2m9.3s | 2m5.2s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 15 | 0.018 | 3.14% | neutral |
| conservative | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| conservative | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.500 | 10s | 1 | 6 | not detected | 3 | 0.167 | 11.55% | neutral |
| conservative | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| conservative | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 1 | 0.083 | 0.00% | neutral |
| conservative | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| conservative | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| conservative | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 20s | 1 | 7 | not detected | 3 | 0.167 | 5.78% | neutral |
| balanced | gradual_degradation | 11 | 2m18.05810847s | 2m18.05810847s | 111.638008ms | 111.638008ms | 0.738 | 0.738 | 2.2s | 6 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| balanced | gradual_degradation | 29 | 2m17.230127598s | 1.088157485s | 111.973508ms | 40.775813ms | 0.737 | 0.053 | 2.4s | 5 | 27 | 6.8s | 14 | 0.010 | 98.96% | win |
| balanced | gradual_degradation | 47 | 2m18.543146187s | 2m18.543146187s | 112.017461ms | 112.017461ms | 0.743 | 0.743 | 2.2s | 5 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| balanced | gradual_degradation | 71 | 2m18.842157616s | 2m18.842157616s | 112.679273ms | 112.679273ms | 0.742 | 0.742 | 2.4s | 7 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| balanced | gradual_degradation | 101 | 2m17.896601353s | 2m17.896601353s | 112.288179ms | 112.288179ms | 0.740 | 0.740 | 2.2s | 7 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | gradual_degradation | 131 | 2m18.257709744s | 2m18.257709744s | 112.50077ms | 112.50077ms | 0.742 | 0.742 | 2.2s | 7 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| balanced | gradual_degradation | 173 | 2m18.511286942s | 2m18.511286942s | 112.241073ms | 112.241073ms | 0.740 | 0.740 | 1.4s | 4 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| balanced | sudden_outage | 11 | 306.854065ms | 307.297288ms | 29.758413ms | 29.775736ms | 0.669 | 0.669 | 2s | 5 | 300 | not detected | 3 | 0.004 | -0.01% | neutral |
| balanced | sudden_outage | 29 | 305.736046ms | 468.508565ms | 29.586311ms | 39.871299ms | 0.667 | 0.040 | 2s | 5 | 10 | 3.6s | 10 | 0.013 | 81.04% | win |
| balanced | sudden_outage | 47 | 306.757116ms | 307.844452ms | 29.297237ms | 29.386626ms | 0.676 | 0.676 | 2.4s | 7 | 300 | not detected | 3 | 0.004 | -0.03% | neutral |
| balanced | sudden_outage | 71 | 306.766218ms | 308.670003ms | 29.362123ms | 29.449668ms | 0.673 | 0.673 | 2s | 4 | 300 | not detected | 3 | 0.004 | -0.04% | neutral |
| balanced | sudden_outage | 101 | 306.026121ms | 306.150606ms | 29.696734ms | 29.751595ms | 0.673 | 0.673 | 2s | 4 | 300 | not detected | 1 | 0.002 | -0.01% | neutral |
| balanced | sudden_outage | 131 | 308.156379ms | 309.590947ms | 29.458433ms | 29.597748ms | 0.676 | 0.676 | 2s | 5 | 300 | not detected | 3 | 0.004 | -0.05% | neutral |
| balanced | sudden_outage | 173 | 307.890143ms | 308.721609ms | 29.101676ms | 29.690264ms | 0.673 | 0.673 | 1.2s | 3 | 300 | not detected | 3 | 0.004 | -0.14% | neutral |
| balanced | capacity_aggregation | 11 | 1m19.876239452s | 22.308354788s | 25ms | 28ms | 0.988 | 0.944 | 0s | 0 | 284 | 6.75s | 44 | 0.007 | 72.24% | win |
| balanced | capacity_aggregation | 29 | 1m19.971533361s | 29.060001888s | 25ms | 28ms | 0.988 | 0.912 | 0s | 0 | 251 | 1.125s | 47 | 0.008 | 66.26% | win |
| balanced | capacity_aggregation | 47 | 1m19.89581372s | 24.482078449s | 25ms | 28ms | 0.988 | 0.947 | 0s | 0 | 407 | 19.375s | 65 | 0.008 | 68.79% | win |
| balanced | capacity_aggregation | 71 | 1m19.767790605s | 38.517885813s | 25ms | 28ms | 0.988 | 0.931 | 0s | 0 | 384 | 1.125s | 54 | 0.007 | 55.94% | win |
| balanced | capacity_aggregation | 101 | 1m20.040921681s | 19.057314016s | 25ms | 28ms | 0.990 | 0.944 | 0s | 0 | 253 | 9.5s | 60 | 0.007 | 75.59% | win |
| balanced | capacity_aggregation | 131 | 1m19.734245977s | 20.776090546s | 25ms | 28ms | 0.988 | 0.928 | 0s | 0 | 346 | 18s | 63 | 0.007 | 73.53% | win |
| balanced | capacity_aggregation | 173 | 1m19.865475022s | 26.269997877s | 25ms | 28ms | 0.988 | 0.918 | 0s | 0 | 238 | 1.375s | 63 | 0.008 | 69.40% | win |
| balanced | transient_spike | 11 | 12.6925426s | 12.6925426s | 90ms | 90ms | 0.396 | 0.396 | 4.4s | 5 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| balanced | transient_spike | 29 | 12.307633594s | 2.262498242s | 90ms | 90ms | 0.384 | 0.078 | 4.4s | 5 | 27 | 6.8s | 14 | 0.013 | 75.45% | win |
| balanced | transient_spike | 47 | 12.854235214s | 12.854235214s | 90ms | 90ms | 0.402 | 0.402 | 4.6s | 6 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| balanced | transient_spike | 71 | 12.659328791s | 12.659328791s | 90ms | 90ms | 0.400 | 0.400 | 4.4s | 4 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| balanced | transient_spike | 101 | 12.607221347s | 12.607221347s | 90ms | 90ms | 0.398 | 0.398 | 4.2s | 4 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | transient_spike | 131 | 12.512324827s | 12.512324827s | 90ms | 90ms | 0.398 | 0.398 | 4.4s | 5 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| balanced | transient_spike | 173 | 12.652228169s | 12.652228169s | 90ms | 90ms | 0.398 | 0.398 | 1.2s | 3 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| balanced | stale_candidate | 11 | 46.540000047s | 31.556666698s | 110ms | 110ms | 0.889 | 0.655 | 9.99999999s | 12 | 107 | 39.666666627s | 12 | 0.047 | 35.41% | win |
| balanced | stale_candidate | 29 | 51.736666713s | 44.540000041s | 110ms | 110ms | 0.883 | 0.754 | 9.666666657s | 8 | 129 | 47.333333286s | 20 | 0.053 | 16.15% | win |
| balanced | stale_candidate | 47 | 51.540000046s | 51.600000047s | 110ms | 110ms | 0.883 | 0.871 | 9.333333324s | 9 | 149 | not detected | 3 | 0.012 | 0.44% | neutral |
| balanced | stale_candidate | 71 | 50.403333377s | 46.343333376s | 110ms | 110ms | 0.889 | 0.854 | 9.333333324s | 10 | 144 | not detected | 15 | 0.047 | 9.67% | neutral |
| balanced | stale_candidate | 101 | 48.010000047s | 39.086666707s | 110ms | 110ms | 0.883 | 0.743 | 9.99999999s | 9 | 127 | 46.66666662s | 12 | 0.035 | 21.04% | win |
| balanced | stale_candidate | 131 | 47.873333381s | 47.070000047s | 110ms | 110ms | 0.883 | 0.865 | 9.666666657s | 9 | 148 | not detected | 7 | 0.023 | 2.12% | neutral |
| balanced | stale_candidate | 173 | 46.87333338s | 27.360000037s | 110ms | 110ms | 0.883 | 0.567 | 9.99999999s | 10 | 95 | 35.666666631s | 16 | 0.041 | 44.79% | win |
| balanced | recovery_no_flap | 11 | 44.799011002s | 44.799011002s | 95ms | 95ms | 0.752 | 0.752 | 4.2s | 5 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| balanced | recovery_no_flap | 29 | 44.937203373s | 1.452259751s | 95ms | 40.82363ms | 0.750 | 0.068 | 4.2s | 5 | 27 | 6.8s | 14 | 0.010 | 96.39% | win |
| balanced | recovery_no_flap | 47 | 44.810457119s | 44.810457119s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| balanced | recovery_no_flap | 71 | 44.958361864s | 44.958361864s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| balanced | recovery_no_flap | 101 | 44.837077225s | 44.837077225s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | recovery_no_flap | 131 | 44.901274352s | 44.901274352s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| balanced | recovery_no_flap | 173 | 44.9015634s | 44.9015634s | 95ms | 95ms | 0.755 | 0.755 | 1.2s | 3 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| balanced | all_channels_bad | 11 | 2m10.9s | 2m10.9s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| balanced | all_channels_bad | 29 | 2m9.8s | 1m22.58s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 44 | 0.013 | 36.76% | win |
| balanced | all_channels_bad | 47 | 2m5.2s | 2m5.2s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| balanced | all_channels_bad | 71 | 2m9.4s | 2m9.4s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| balanced | all_channels_bad | 101 | 2m5.7s | 2m5.7s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| balanced | all_channels_bad | 131 | 2m5.1s | 2m5.1s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| balanced | all_channels_bad | 173 | 2m9.3s | 2m9.3s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| balanced | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.500 | 10s | 1 | 6 | not detected | 3 | 0.167 | 11.55% | neutral |
| balanced | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.417 | 10s | 1 | 5 | 1m10s | 4 | 0.333 | 17.33% | win |
| balanced | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| balanced | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 1 | 0.083 | 0.00% | neutral |
| balanced | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| balanced | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| balanced | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 20s | 1 | 7 | not detected | 3 | 0.167 | 5.78% | neutral |
| aggressive | gradual_degradation | 11 | 2m18.05810847s | 2m18.05810847s | 111.638008ms | 111.638008ms | 0.738 | 0.738 | 400ms | 2 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| aggressive | gradual_degradation | 29 | 2m17.230127598s | 2m17.230127598s | 111.973508ms | 111.973508ms | 0.737 | 0.737 | 2.4s | 5 | 450 | not detected | 5 | 0.005 | 0.00% | neutral |
| aggressive | gradual_degradation | 47 | 2m18.543146187s | 2m18.543146187s | 112.017461ms | 112.017461ms | 0.743 | 0.743 | 2.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | gradual_degradation | 71 | 2m18.842157616s | 2m18.842157616s | 112.679273ms | 112.679273ms | 0.742 | 0.742 | 2.4s | 7 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| aggressive | gradual_degradation | 101 | 2m17.896601353s | 2m17.896601353s | 112.288179ms | 112.288179ms | 0.740 | 0.740 | 0s | 0 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | gradual_degradation | 131 | 2m18.257709744s | 2m18.257709744s | 112.50077ms | 112.50077ms | 0.742 | 0.742 | 2.2s | 7 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| aggressive | gradual_degradation | 173 | 2m18.511286942s | 2m18.511286942s | 112.241073ms | 112.241073ms | 0.740 | 0.740 | 1.2s | 3 | 450 | not detected | 5 | 0.005 | 0.00% | neutral |
| aggressive | sudden_outage | 11 | 306.854065ms | 307.297288ms | 29.758413ms | 29.775736ms | 0.669 | 0.669 | 200ms | 2 | 300 | not detected | 3 | 0.004 | -0.01% | neutral |
| aggressive | sudden_outage | 29 | 305.736046ms | 307.035615ms | 29.586311ms | 29.812684ms | 0.667 | 0.667 | 2s | 5 | 300 | not detected | 3 | 0.004 | -0.07% | neutral |
| aggressive | sudden_outage | 47 | 306.757116ms | 307.058184ms | 29.297237ms | 29.386626ms | 0.676 | 0.676 | 2.4s | 7 | 300 | not detected | 1 | 0.002 | -0.02% | neutral |
| aggressive | sudden_outage | 71 | 306.766218ms | 308.348868ms | 29.362123ms | 29.436265ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | -0.04% | neutral |
| aggressive | sudden_outage | 101 | 306.026121ms | 306.150606ms | 29.696734ms | 29.751595ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | -0.01% | neutral |
| aggressive | sudden_outage | 131 | 308.156379ms | 308.545061ms | 29.458433ms | 29.570855ms | 0.676 | 0.676 | 2s | 5 | 300 | not detected | 1 | 0.002 | -0.03% | neutral |
| aggressive | sudden_outage | 173 | 307.890143ms | 308.721609ms | 29.101676ms | 29.690264ms | 0.673 | 0.673 | 800ms | 2 | 300 | not detected | 3 | 0.004 | -0.14% | neutral |
| aggressive | capacity_aggregation | 11 | 1m19.876239452s | 1m6.783958505s | 25ms | 28ms | 0.988 | 0.911 | 0s | 0 | 588 | not detected | 99 | 0.076 | 18.21% | win |
| aggressive | capacity_aggregation | 29 | 1m19.971533361s | 1m5.735170987s | 25ms | 28ms | 0.988 | 0.897 | 0s | 0 | 588 | not detected | 109 | 0.085 | 19.99% | win |
| aggressive | capacity_aggregation | 47 | 1m19.89581372s | 1m9.10582206s | 25ms | 28ms | 0.988 | 0.925 | 0s | 0 | 597 | not detected | 75 | 0.062 | 14.95% | neutral |
| aggressive | capacity_aggregation | 71 | 1m19.767790605s | 1m4.718183251s | 25ms | 28ms | 0.988 | 0.896 | 0s | 0 | 588 | not detected | 114 | 0.087 | 20.87% | win |
| aggressive | capacity_aggregation | 101 | 1m20.040921681s | 1m6.233921723s | 25ms | 28ms | 0.990 | 0.910 | 0s | 0 | 586 | not detected | 111 | 0.081 | 18.78% | win |
| aggressive | capacity_aggregation | 131 | 1m19.734245977s | 1m7.294629197s | 25ms | 28ms | 0.988 | 0.915 | 0s | 0 | 593 | not detected | 95 | 0.072 | 17.25% | win |
| aggressive | capacity_aggregation | 173 | 1m19.865475022s | 1m4.33255732s | 25ms | 28ms | 0.988 | 0.896 | 0s | 0 | 584 | not detected | 115 | 0.090 | 21.43% | win |
| aggressive | transient_spike | 11 | 12.6925426s | 12.6925426s | 90ms | 90ms | 0.396 | 0.396 | 200ms | 2 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| aggressive | transient_spike | 29 | 12.307633594s | 12.307633594s | 90ms | 90ms | 0.384 | 0.384 | 4.4s | 5 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| aggressive | transient_spike | 47 | 12.854235214s | 12.854235214s | 90ms | 90ms | 0.402 | 0.402 | 4.6s | 6 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | transient_spike | 71 | 12.659328791s | 12.659328791s | 90ms | 90ms | 0.400 | 0.400 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | transient_spike | 101 | 12.607221347s | 12.607221347s | 90ms | 90ms | 0.398 | 0.398 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | transient_spike | 131 | 12.512324827s | 12.512324827s | 90ms | 90ms | 0.398 | 0.398 | 4.4s | 5 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | transient_spike | 173 | 12.652228169s | 12.652228169s | 90ms | 90ms | 0.398 | 0.398 | 800ms | 2 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| aggressive | stale_candidate | 11 | 46.540000047s | 27.830000028s | 110ms | 110ms | 0.889 | 0.550 | 9.333333324s | 10 | 92 | 34.333333299s | 8 | 0.023 | 43.97% | win |
| aggressive | stale_candidate | 29 | 51.736666713s | 18.376666682s | 110ms | 110ms | 0.883 | 0.327 | 9.333333324s | 7 | 54 | 21.666666645s | 8 | 0.023 | 68.63% | win |
| aggressive | stale_candidate | 47 | 51.540000046s | 19.04333335s | 110ms | 110ms | 0.883 | 0.374 | 9.333333324s | 9 | 61 | 23.666666643s | 6 | 0.029 | 66.88% | win |
| aggressive | stale_candidate | 71 | 50.403333377s | 3.666666669s | 110ms | 110ms | 0.889 | 0.152 | 8.333333325s | 8 | 22 | 10.999999989s | 8 | 0.023 | 91.39% | win |
| aggressive | stale_candidate | 101 | 48.010000047s | 12.453333343s | 110ms | 110ms | 0.883 | 0.269 | 9.666666657s | 8 | 43 | 17.999999982s | 8 | 0.023 | 76.20% | win |
| aggressive | stale_candidate | 131 | 47.873333381s | 8.92333334s | 110ms | 110ms | 0.883 | 0.211 | 9.333333324s | 8 | 32 | 14.333333319s | 8 | 0.023 | 82.44% | win |
| aggressive | stale_candidate | 173 | 46.87333338s | 2s | 110ms | 110ms | 0.883 | 0.094 | 4.666666662s | 1 | 11 | 7.333333326s | 6 | 0.029 | 93.85% | win |
| aggressive | recovery_no_flap | 11 | 44.799011002s | 44.799011002s | 95ms | 95ms | 0.752 | 0.752 | 200ms | 2 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| aggressive | recovery_no_flap | 29 | 44.937203373s | 44.937203373s | 95ms | 95ms | 0.750 | 0.750 | 4.2s | 5 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| aggressive | recovery_no_flap | 47 | 44.810457119s | 44.810457119s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | recovery_no_flap | 71 | 44.958361864s | 44.958361864s | 95ms | 95ms | 0.755 | 0.755 | 0s | 0 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | recovery_no_flap | 101 | 44.837077225s | 44.837077225s | 95ms | 95ms | 0.755 | 0.755 | 0s | 0 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | recovery_no_flap | 131 | 44.901274352s | 44.901274352s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | recovery_no_flap | 173 | 44.9015634s | 44.9015634s | 95ms | 95ms | 0.755 | 0.755 | 800ms | 2 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| aggressive | all_channels_bad | 11 | 2m10.9s | 2m10.9s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| aggressive | all_channels_bad | 29 | 2m9.8s | 2m9.8s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| aggressive | all_channels_bad | 47 | 2m5.2s | 2m5.2s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | all_channels_bad | 71 | 2m9.4s | 2m9.4s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | all_channels_bad | 101 | 2m5.7s | 2m5.7s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | all_channels_bad | 131 | 2m5.1s | 2m5.1s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| aggressive | all_channels_bad | 173 | 2m9.3s | 2m9.3s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| aggressive | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.500 | 10s | 1 | 6 | not detected | 3 | 0.167 | 11.55% | neutral |
| aggressive | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.417 | 10s | 1 | 5 | 1m10s | 4 | 0.333 | 17.33% | win |
| aggressive | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| aggressive | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.500 | 10s | 1 | 6 | 1m10s | 2 | 0.250 | 11.55% | neutral |
| aggressive | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| aggressive | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| aggressive | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 20s | 1 | 7 | not detected | 3 | 0.167 | 5.78% | neutral |
| fast-low-probe | gradual_degradation | 11 | 2m18.05810847s | 2m18.05810847s | 111.638008ms | 111.638008ms | 0.738 | 0.738 | 400ms | 2 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | gradual_degradation | 29 | 2m17.230127598s | 2m17.230127598s | 111.973508ms | 111.973508ms | 0.737 | 0.737 | 2.4s | 5 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| fast-low-probe | gradual_degradation | 47 | 2m18.543146187s | 2m18.543146187s | 112.017461ms | 112.017461ms | 0.743 | 0.743 | 2.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | gradual_degradation | 71 | 2m18.842157616s | 2m18.842157616s | 112.679273ms | 112.679273ms | 0.742 | 0.742 | 2.4s | 7 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | gradual_degradation | 101 | 2m17.896601353s | 2m17.896601353s | 112.288179ms | 112.288179ms | 0.740 | 0.740 | 0s | 0 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| fast-low-probe | gradual_degradation | 131 | 2m18.257709744s | 2m18.257709744s | 112.50077ms | 112.50077ms | 0.742 | 0.742 | 2.2s | 7 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| fast-low-probe | gradual_degradation | 173 | 2m18.511286942s | 2m18.511286942s | 112.241073ms | 112.241073ms | 0.740 | 0.740 | 1.2s | 3 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| fast-low-probe | sudden_outage | 11 | 306.854065ms | 307.058392ms | 29.758413ms | 29.758413ms | 0.669 | 0.669 | 200ms | 2 | 300 | not detected | 1 | 0.002 | -0.00% | neutral |
| fast-low-probe | sudden_outage | 29 | 305.736046ms | 307.035615ms | 29.586311ms | 29.812684ms | 0.667 | 0.667 | 2s | 5 | 300 | not detected | 3 | 0.004 | -0.07% | neutral |
| fast-low-probe | sudden_outage | 47 | 306.757116ms | 307.058184ms | 29.297237ms | 29.386626ms | 0.676 | 0.676 | 2.4s | 7 | 300 | not detected | 1 | 0.002 | -0.02% | neutral |
| fast-low-probe | sudden_outage | 71 | 306.766218ms | 308.348868ms | 29.362123ms | 29.436265ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | -0.04% | neutral |
| fast-low-probe | sudden_outage | 101 | 306.026121ms | 306.415917ms | 29.696734ms | 29.759224ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 3 | 0.004 | -0.02% | neutral |
| fast-low-probe | sudden_outage | 131 | 308.156379ms | 308.545061ms | 29.458433ms | 29.570855ms | 0.676 | 0.676 | 2s | 5 | 300 | not detected | 1 | 0.002 | -0.03% | neutral |
| fast-low-probe | sudden_outage | 173 | 307.890143ms | 308.721609ms | 29.101676ms | 29.690264ms | 0.673 | 0.673 | 800ms | 2 | 300 | not detected | 3 | 0.004 | -0.14% | neutral |
| fast-low-probe | capacity_aggregation | 11 | 1m19.876239452s | 1m15.158759632s | 25ms | 25ms | 0.988 | 0.960 | 0s | 0 | 621 | not detected | 35 | 0.028 | 6.77% | neutral |
| fast-low-probe | capacity_aggregation | 29 | 1m19.971533361s | 1m14.21172293s | 25ms | 25ms | 0.988 | 0.954 | 0s | 0 | 621 | not detected | 47 | 0.033 | 8.23% | neutral |
| fast-low-probe | capacity_aggregation | 47 | 1m19.89581372s | 1m16.497989955s | 25ms | 25ms | 0.988 | 0.968 | 0s | 0 | 626 | not detected | 27 | 0.019 | 4.71% | neutral |
| fast-low-probe | capacity_aggregation | 71 | 1m19.767790605s | 1m14.99050491s | 25ms | 25ms | 0.988 | 0.960 | 0s | 0 | 624 | not detected | 37 | 0.028 | 6.66% | neutral |
| fast-low-probe | capacity_aggregation | 101 | 1m20.040921681s | 1m13.96030001s | 25ms | 25ms | 0.990 | 0.956 | 0s | 0 | 617 | not detected | 47 | 0.035 | 8.49% | neutral |
| fast-low-probe | capacity_aggregation | 131 | 1m19.734245977s | 1m15.759011792s | 25ms | 25ms | 0.988 | 0.964 | 0s | 0 | 626 | not detected | 33 | 0.024 | 5.59% | neutral |
| fast-low-probe | capacity_aggregation | 173 | 1m19.865475022s | 1m14.283082441s | 25ms | 25ms | 0.988 | 0.956 | 0s | 0 | 620 | not detected | 43 | 0.032 | 7.77% | neutral |
| fast-low-probe | transient_spike | 11 | 12.6925426s | 12.6925426s | 90ms | 90ms | 0.396 | 0.396 | 200ms | 2 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | transient_spike | 29 | 12.307633594s | 12.307633594s | 90ms | 90ms | 0.384 | 0.384 | 4.4s | 5 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| fast-low-probe | transient_spike | 47 | 12.854235214s | 12.854235214s | 90ms | 90ms | 0.402 | 0.402 | 4.6s | 6 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | transient_spike | 71 | 12.659328791s | 12.659328791s | 90ms | 90ms | 0.400 | 0.400 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | transient_spike | 101 | 12.607221347s | 12.607221347s | 90ms | 90ms | 0.398 | 0.398 | 0s | 0 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| fast-low-probe | transient_spike | 131 | 12.512324827s | 12.512324827s | 90ms | 90ms | 0.398 | 0.398 | 4.4s | 5 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | transient_spike | 173 | 12.652228169s | 12.652228169s | 90ms | 90ms | 0.398 | 0.398 | 800ms | 2 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| fast-low-probe | stale_candidate | 11 | 46.540000047s | 33.223333365s | 110ms | 110ms | 0.889 | 0.643 | 9.333333324s | 10 | 109 | 39.666666627s | 8 | 0.035 | 32.22% | win |
| fast-low-probe | stale_candidate | 29 | 51.736666713s | 25.103333353s | 110ms | 110ms | 0.883 | 0.439 | 9.333333324s | 7 | 73 | 27.999999972s | 8 | 0.023 | 56.24% | win |
| fast-low-probe | stale_candidate | 47 | 51.540000046s | 51.600000047s | 110ms | 110ms | 0.883 | 0.877 | 9.333333324s | 9 | 150 | not detected | 1 | 0.006 | 0.53% | neutral |
| fast-low-probe | stale_candidate | 71 | 50.403333377s | 12.650000012s | 110ms | 110ms | 0.889 | 0.292 | 8.666666658s | 9 | 46 | 18.999999981s | 8 | 0.023 | 76.95% | win |
| fast-low-probe | stale_candidate | 101 | 48.010000047s | 27.300000026s | 110ms | 110ms | 0.883 | 0.544 | 9.666666657s | 8 | 89 | 33.3333333s | 6 | 0.018 | 47.24% | win |
| fast-low-probe | stale_candidate | 131 | 47.873333381s | 20.240000018s | 110ms | 110ms | 0.883 | 0.398 | 9.333333324s | 8 | 66 | 25.666666641s | 8 | 0.023 | 61.46% | win |
| fast-low-probe | stale_candidate | 173 | 46.87333338s | 46.070000046s | 110ms | 110ms | 0.883 | 0.871 | 9.666666657s | 9 | 149 | not detected | 7 | 0.023 | 2.44% | neutral |
| fast-low-probe | recovery_no_flap | 11 | 44.799011002s | 44.799011002s | 95ms | 95ms | 0.752 | 0.752 | 200ms | 2 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | recovery_no_flap | 29 | 44.937203373s | 44.937203373s | 95ms | 95ms | 0.750 | 0.750 | 4.2s | 5 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| fast-low-probe | recovery_no_flap | 47 | 44.810457119s | 44.810457119s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | recovery_no_flap | 71 | 44.958361864s | 44.958361864s | 95ms | 95ms | 0.755 | 0.755 | 0s | 0 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | recovery_no_flap | 101 | 44.837077225s | 44.837077225s | 95ms | 95ms | 0.755 | 0.755 | 0s | 0 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| fast-low-probe | recovery_no_flap | 131 | 44.901274352s | 44.901274352s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | recovery_no_flap | 173 | 44.9015634s | 44.9015634s | 95ms | 95ms | 0.755 | 0.755 | 800ms | 2 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| fast-low-probe | all_channels_bad | 11 | 2m10.9s | 2m10.9s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | all_channels_bad | 29 | 2m9.8s | 2m9.8s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| fast-low-probe | all_channels_bad | 47 | 2m5.2s | 2m5.2s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | all_channels_bad | 71 | 2m9.4s | 2m9.4s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | all_channels_bad | 101 | 2m5.7s | 2m5.7s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| fast-low-probe | all_channels_bad | 131 | 2m5.1s | 2m5.1s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-low-probe | all_channels_bad | 173 | 2m9.3s | 2m9.3s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| fast-low-probe | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 1 | 0.083 | 5.78% | neutral |
| fast-low-probe | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.500 | 10s | 1 | 6 | not detected | 3 | 0.167 | 11.55% | neutral |
| fast-low-probe | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| fast-low-probe | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 1 | 0.083 | 0.00% | neutral |
| fast-low-probe | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| fast-low-probe | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| fast-low-probe | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 20s | 1 | 7 | not detected | 3 | 0.167 | 5.78% | neutral |
| short-window | gradual_degradation | 11 | 2m18.05810847s | 2m18.05810847s | 111.638008ms | 111.638008ms | 0.738 | 0.738 | 2.2s | 6 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| short-window | gradual_degradation | 29 | 2m17.230127598s | 2m17.230127598s | 111.973508ms | 111.973508ms | 0.737 | 0.737 | 2.4s | 5 | 450 | not detected | 7 | 0.010 | 0.00% | neutral |
| short-window | gradual_degradation | 47 | 2m18.543146187s | 2m18.543146187s | 112.017461ms | 112.017461ms | 0.743 | 0.743 | 2.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | gradual_degradation | 71 | 2m18.842157616s | 2m18.842157616s | 112.679273ms | 112.679273ms | 0.742 | 0.742 | 2.4s | 7 | 450 | not detected | 3 | 0.005 | 0.00% | neutral |
| short-window | gradual_degradation | 101 | 2m17.896601353s | 2m17.896601353s | 112.288179ms | 112.288179ms | 0.740 | 0.740 | 2.2s | 7 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | gradual_degradation | 131 | 2m18.257709744s | 2m18.257709744s | 112.50077ms | 112.50077ms | 0.742 | 0.742 | 2.2s | 7 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| short-window | gradual_degradation | 173 | 2m18.511286942s | 2m18.511286942s | 112.241073ms | 112.241073ms | 0.740 | 0.740 | 1.4s | 4 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| short-window | sudden_outage | 11 | 306.854065ms | 307.297288ms | 29.758413ms | 29.775736ms | 0.669 | 0.669 | 2s | 5 | 300 | not detected | 3 | 0.004 | -0.01% | neutral |
| short-window | sudden_outage | 29 | 305.736046ms | 308.918614ms | 29.586311ms | 29.943397ms | 0.667 | 0.667 | 2s | 5 | 300 | not detected | 7 | 0.013 | -0.12% | neutral |
| short-window | sudden_outage | 47 | 306.757116ms | 307.058184ms | 29.297237ms | 29.386626ms | 0.676 | 0.676 | 2.4s | 7 | 300 | not detected | 1 | 0.002 | -0.02% | neutral |
| short-window | sudden_outage | 71 | 306.766218ms | 308.694669ms | 29.362123ms | 29.602297ms | 0.673 | 0.673 | 2s | 4 | 300 | not detected | 3 | 0.007 | -0.08% | neutral |
| short-window | sudden_outage | 101 | 306.026121ms | 306.150606ms | 29.696734ms | 29.751595ms | 0.673 | 0.673 | 2s | 4 | 300 | not detected | 1 | 0.002 | -0.01% | neutral |
| short-window | sudden_outage | 131 | 308.156379ms | 309.590947ms | 29.458433ms | 29.597748ms | 0.676 | 0.676 | 2s | 5 | 300 | not detected | 3 | 0.004 | -0.05% | neutral |
| short-window | sudden_outage | 173 | 307.890143ms | 308.721609ms | 29.101676ms | 29.690264ms | 0.673 | 0.673 | 1.2s | 3 | 300 | not detected | 3 | 0.004 | -0.14% | neutral |
| short-window | capacity_aggregation | 11 | 1m19.876239452s | 10.58662876s | 25ms | 28ms | 0.988 | 0.893 | 0s | 0 | 274 | 6s | 104 | 0.006 | 86.02% | win |
| short-window | capacity_aggregation | 29 | 1m19.971533361s | 59.443432988s | 25ms | 28ms | 0.988 | 0.943 | 0s | 0 | 12 | 1.125s | 36 | 0.010 | 26.57% | win |
| short-window | capacity_aggregation | 47 | 1m19.89581372s | 13.460771449s | 25ms | 28ms | 0.988 | 0.929 | 0s | 0 | 280 | 3.5s | 90 | 0.006 | 81.86% | win |
| short-window | capacity_aggregation | 71 | 1m19.767790605s | 11.796013397s | 25ms | 28ms | 0.988 | 0.911 | 0s | 0 | 332 | 1.125s | 134 | 0.006 | 84.59% | win |
| short-window | capacity_aggregation | 101 | 1m20.040921681s | 8.764070848s | 25ms | 28ms | 0.990 | 0.876 | 0s | 0 | 289 | 1.125s | 126 | 0.006 | 88.12% | win |
| short-window | capacity_aggregation | 131 | 1m19.734245977s | 8.731177384s | 25ms | 28ms | 0.988 | 0.871 | 0s | 0 | 271 | 1.625s | 115 | 0.007 | 88.06% | win |
| short-window | capacity_aggregation | 173 | 1m19.865475022s | 16.02342494s | 25ms | 28ms | 0.988 | 0.917 | 0s | 0 | 337 | 1.125s | 78 | 0.006 | 79.64% | win |
| short-window | transient_spike | 11 | 12.6925426s | 12.6925426s | 90ms | 90ms | 0.396 | 0.396 | 4.4s | 5 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| short-window | transient_spike | 29 | 12.307633594s | 12.307633594s | 90ms | 90ms | 0.384 | 0.384 | 4.4s | 5 | 300 | not detected | 7 | 0.013 | 0.00% | neutral |
| short-window | transient_spike | 47 | 12.854235214s | 12.854235214s | 90ms | 90ms | 0.402 | 0.402 | 4.6s | 6 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | transient_spike | 71 | 12.659328791s | 12.659328791s | 90ms | 90ms | 0.400 | 0.400 | 4.4s | 4 | 300 | not detected | 3 | 0.007 | 0.00% | neutral |
| short-window | transient_spike | 101 | 12.607221347s | 12.607221347s | 90ms | 90ms | 0.398 | 0.398 | 4.2s | 4 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | transient_spike | 131 | 12.512324827s | 12.512324827s | 90ms | 90ms | 0.398 | 0.398 | 4.4s | 5 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| short-window | transient_spike | 173 | 12.652228169s | 12.652228169s | 90ms | 90ms | 0.398 | 0.398 | 1.2s | 3 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| short-window | stale_candidate | 11 | 46.540000047s | 30.693333363s | 110ms | 110ms | 0.889 | 0.596 | 9.99999999s | 12 | 101 | 37.666666629s | 10 | 0.029 | 37.94% | win |
| short-window | stale_candidate | 29 | 51.736666713s | 24.103333354s | 110ms | 110ms | 0.883 | 0.474 | 9.666666657s | 8 | 76 | 29.99999997s | 16 | 0.047 | 57.40% | win |
| short-window | stale_candidate | 47 | 51.540000046s | 29.026666694s | 110ms | 110ms | 0.883 | 0.526 | 9.333333324s | 9 | 88 | 33.666666633s | 8 | 0.029 | 48.00% | win |
| short-window | stale_candidate | 71 | 50.403333377s | 8.923333342s | 110ms | 110ms | 0.889 | 0.246 | 9.333333324s | 10 | 35 | 15.666666651s | 10 | 0.029 | 83.10% | win |
| short-window | stale_candidate | 101 | 48.010000047s | 43.873333379s | 110ms | 110ms | 0.883 | 0.842 | 9.666666657s | 8 | 144 | not detected | 15 | 0.047 | 8.98% | neutral |
| short-window | stale_candidate | 131 | 47.873333381s | 46.540000047s | 110ms | 110ms | 0.883 | 0.854 | 9.333333324s | 8 | 146 | not detected | 11 | 0.035 | 3.22% | neutral |
| short-window | stale_candidate | 173 | 46.87333338s | 27.830000032s | 110ms | 110ms | 0.883 | 0.561 | 9.666666657s | 9 | 93 | 35.999999964s | 16 | 0.053 | 44.65% | win |
| short-window | recovery_no_flap | 11 | 44.799011002s | 44.799011002s | 95ms | 95ms | 0.752 | 0.752 | 4.2s | 5 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| short-window | recovery_no_flap | 29 | 44.937203373s | 44.937203373s | 95ms | 95ms | 0.750 | 0.750 | 4.2s | 5 | 450 | not detected | 7 | 0.010 | 0.00% | neutral |
| short-window | recovery_no_flap | 47 | 44.810457119s | 44.810457119s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | recovery_no_flap | 71 | 44.958361864s | 44.958361864s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 3 | 0.005 | 0.00% | neutral |
| short-window | recovery_no_flap | 101 | 44.837077225s | 44.837077225s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | recovery_no_flap | 131 | 44.901274352s | 44.901274352s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| short-window | recovery_no_flap | 173 | 44.9015634s | 44.9015634s | 95ms | 95ms | 0.755 | 0.755 | 1.2s | 3 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| short-window | all_channels_bad | 11 | 2m10.9s | 2m10.9s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| short-window | all_channels_bad | 29 | 2m9.8s | 2m9.8s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 7 | 0.013 | 0.00% | neutral |
| short-window | all_channels_bad | 47 | 2m5.2s | 2m5.2s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | all_channels_bad | 71 | 2m9.4s | 2m9.4s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 3 | 0.007 | 0.00% | neutral |
| short-window | all_channels_bad | 101 | 2m5.7s | 2m5.7s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| short-window | all_channels_bad | 131 | 2m5.1s | 2m5.1s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| short-window | all_channels_bad | 173 | 2m9.3s | 2m9.3s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| short-window | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.500 | not detected | 0 | 6 | not detected | 3 | 0.167 | 11.55% | neutral |
| short-window | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.417 | not detected | 0 | 5 | 1m10s | 4 | 0.333 | 17.33% | win |
| short-window | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | not detected | 0 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| short-window | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.500 | not detected | 0 | 6 | 1m10s | 2 | 0.250 | 11.55% | neutral |
| short-window | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | not detected | 0 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| short-window | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | not detected | 0 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| short-window | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | not detected | 0 | 7 | not detected | 3 | 0.167 | 5.78% | neutral |
| stable-recovery | gradual_degradation | 11 | 2m18.05810847s | 2m18.05810847s | 111.638008ms | 111.638008ms | 0.738 | 0.738 | 2.2s | 6 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| stable-recovery | gradual_degradation | 29 | 2m17.230127598s | 1.77226021s | 111.973508ms | 42.406282ms | 0.737 | 0.095 | 2.6s | 7 | 52 | 11.8s | 14 | 0.010 | 98.47% | win |
| stable-recovery | gradual_degradation | 47 | 2m18.543146187s | 2m18.543146187s | 112.017461ms | 112.017461ms | 0.743 | 0.743 | 2.2s | 5 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| stable-recovery | gradual_degradation | 71 | 2m18.842157616s | 2m18.842157616s | 112.679273ms | 112.679273ms | 0.742 | 0.742 | 2.4s | 7 | 450 | not detected | 5 | 0.005 | 0.00% | neutral |
| stable-recovery | gradual_degradation | 101 | 2m17.896601353s | 2m17.896601353s | 112.288179ms | 112.288179ms | 0.740 | 0.740 | 2.2s | 7 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | gradual_degradation | 131 | 2m18.257709744s | 2m18.257709744s | 112.50077ms | 112.50077ms | 0.742 | 0.742 | 2.2s | 7 | 450 | not detected | 5 | 0.005 | 0.00% | neutral |
| stable-recovery | gradual_degradation | 173 | 2m18.511286942s | 2m18.511286942s | 112.241073ms | 112.241073ms | 0.740 | 0.740 | 1.4s | 4 | 450 | not detected | 5 | 0.005 | 0.00% | neutral |
| stable-recovery | sudden_outage | 11 | 306.854065ms | 307.297288ms | 29.758413ms | 29.775736ms | 0.669 | 0.669 | 2s | 5 | 300 | not detected | 3 | 0.004 | -0.01% | neutral |
| stable-recovery | sudden_outage | 29 | 305.736046ms | 468.508565ms | 29.586311ms | 39.871299ms | 0.667 | 0.040 | 2s | 5 | 10 | 3.6s | 10 | 0.013 | 81.04% | win |
| stable-recovery | sudden_outage | 47 | 306.757116ms | 307.844452ms | 29.297237ms | 29.386626ms | 0.676 | 0.676 | 2.4s | 7 | 300 | not detected | 3 | 0.004 | -0.03% | neutral |
| stable-recovery | sudden_outage | 71 | 306.766218ms | 308.694669ms | 29.362123ms | 29.569943ms | 0.673 | 0.673 | 2s | 4 | 300 | not detected | 5 | 0.007 | -0.07% | neutral |
| stable-recovery | sudden_outage | 101 | 306.026121ms | 306.150606ms | 29.696734ms | 29.751595ms | 0.673 | 0.673 | 2s | 4 | 300 | not detected | 1 | 0.002 | -0.01% | neutral |
| stable-recovery | sudden_outage | 131 | 308.156379ms | 309.774647ms | 29.458433ms | 29.677918ms | 0.676 | 0.676 | 2s | 5 | 300 | not detected | 5 | 0.007 | -0.07% | neutral |
| stable-recovery | sudden_outage | 173 | 307.890143ms | 308.945026ms | 29.101676ms | 29.693464ms | 0.673 | 0.673 | 1.2s | 3 | 300 | not detected | 5 | 0.007 | -0.15% | neutral |
| stable-recovery | capacity_aggregation | 11 | 1m19.876239452s | 13.731430651s | 25ms | 28ms | 0.988 | 0.943 | 0s | 0 | 311 | 6.75s | 95 | 0.007 | 81.44% | win |
| stable-recovery | capacity_aggregation | 29 | 1m19.971533361s | 12.392319646s | 25ms | 28ms | 0.988 | 0.906 | 0s | 0 | 249 | 1.125s | 74 | 0.008 | 83.43% | win |
| stable-recovery | capacity_aggregation | 47 | 1m19.89581372s | 21.413388682s | 25ms | 28ms | 0.988 | 0.954 | 0s | 0 | 324 | 19.125s | 76 | 0.008 | 72.69% | win |
| stable-recovery | capacity_aggregation | 71 | 1m19.767790605s | 12.440845467s | 25ms | 28ms | 0.988 | 0.889 | 0s | 0 | 277 | 1.125s | 60 | 0.007 | 83.83% | win |
| stable-recovery | capacity_aggregation | 101 | 1m20.040921681s | 16.240445033s | 25ms | 28ms | 0.990 | 0.921 | 0s | 0 | 363 | 9.5s | 64 | 0.007 | 79.23% | win |
| stable-recovery | capacity_aggregation | 131 | 1m19.734245977s | 19.649592812s | 25ms | 28ms | 0.988 | 0.947 | 0s | 0 | 306 | 17.5s | 85 | 0.007 | 74.80% | win |
| stable-recovery | capacity_aggregation | 173 | 1m19.865475022s | 10.519421577s | 25ms | 28ms | 0.988 | 0.896 | 0s | 0 | 286 | 1.375s | 69 | 0.008 | 85.63% | win |
| stable-recovery | transient_spike | 11 | 12.6925426s | 12.6925426s | 90ms | 90ms | 0.396 | 0.396 | 4.4s | 5 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| stable-recovery | transient_spike | 29 | 12.307633594s | 10.228134668s | 90ms | 90ms | 0.384 | 0.140 | 4.4s | 5 | 52 | 11.8s | 14 | 0.013 | 25.09% | win |
| stable-recovery | transient_spike | 47 | 12.854235214s | 12.854235214s | 90ms | 90ms | 0.402 | 0.402 | 4.6s | 6 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| stable-recovery | transient_spike | 71 | 12.659328791s | 12.659328791s | 90ms | 90ms | 0.400 | 0.400 | 4.4s | 4 | 300 | not detected | 5 | 0.007 | 0.00% | neutral |
| stable-recovery | transient_spike | 101 | 12.607221347s | 12.607221347s | 90ms | 90ms | 0.398 | 0.398 | 4.2s | 4 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | transient_spike | 131 | 12.512324827s | 12.512324827s | 90ms | 90ms | 0.398 | 0.398 | 4.4s | 5 | 300 | not detected | 5 | 0.007 | 0.00% | neutral |
| stable-recovery | transient_spike | 173 | 12.652228169s | 12.652228169s | 90ms | 90ms | 0.398 | 0.398 | 1.2s | 3 | 300 | not detected | 5 | 0.007 | 0.00% | neutral |
| stable-recovery | stale_candidate | 11 | 46.540000047s | 31.556666698s | 110ms | 110ms | 0.889 | 0.655 | 9.99999999s | 12 | 107 | 39.666666627s | 14 | 0.053 | 35.41% | win |
| stable-recovery | stale_candidate | 29 | 51.736666713s | 44.540000041s | 110ms | 110ms | 0.883 | 0.754 | 9.666666657s | 8 | 128 | 46.999999953s | 18 | 0.058 | 16.20% | win |
| stable-recovery | stale_candidate | 47 | 51.540000046s | 51.600000047s | 110ms | 110ms | 0.883 | 0.871 | 9.99999999s | 10 | 149 | not detected | 3 | 0.012 | 0.44% | neutral |
| stable-recovery | stale_candidate | 71 | 50.403333377s | 46.343333376s | 110ms | 110ms | 0.889 | 0.854 | 10.333333323s | 11 | 144 | not detected | 15 | 0.047 | 9.67% | neutral |
| stable-recovery | stale_candidate | 101 | 48.010000047s | 39.086666707s | 110ms | 110ms | 0.883 | 0.743 | 9.99999999s | 9 | 127 | 46.66666662s | 12 | 0.035 | 21.04% | win |
| stable-recovery | stale_candidate | 131 | 47.873333381s | 47.070000047s | 110ms | 110ms | 0.883 | 0.865 | 9.666666657s | 9 | 148 | not detected | 7 | 0.023 | 2.12% | neutral |
| stable-recovery | stale_candidate | 173 | 46.87333338s | 27.300000028s | 110ms | 110ms | 0.883 | 0.561 | 9.99999999s | 10 | 94 | 35.666666631s | 14 | 0.041 | 45.12% | win |
| stable-recovery | recovery_no_flap | 11 | 44.799011002s | 44.799011002s | 95ms | 95ms | 0.752 | 0.752 | 4.2s | 5 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| stable-recovery | recovery_no_flap | 29 | 44.937203373s | 6.87s | 95ms | 95ms | 0.750 | 0.112 | 4.2s | 5 | 52 | 11.8s | 14 | 0.010 | 86.50% | win |
| stable-recovery | recovery_no_flap | 47 | 44.810457119s | 44.810457119s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| stable-recovery | recovery_no_flap | 71 | 44.958361864s | 44.958361864s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 5 | 0.005 | 0.00% | neutral |
| stable-recovery | recovery_no_flap | 101 | 44.837077225s | 44.837077225s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | recovery_no_flap | 131 | 44.901274352s | 44.901274352s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 5 | 0.005 | 0.00% | neutral |
| stable-recovery | recovery_no_flap | 173 | 44.9015634s | 44.9015634s | 95ms | 95ms | 0.755 | 0.755 | 1.2s | 3 | 450 | not detected | 5 | 0.005 | 0.00% | neutral |
| stable-recovery | all_channels_bad | 11 | 2m10.9s | 2m10.9s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| stable-recovery | all_channels_bad | 29 | 2m9.8s | 1m16.515s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 34 | 0.013 | 41.66% | win |
| stable-recovery | all_channels_bad | 47 | 2m5.2s | 2m5.2s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| stable-recovery | all_channels_bad | 71 | 2m9.4s | 2m9.4s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 5 | 0.007 | 0.00% | neutral |
| stable-recovery | all_channels_bad | 101 | 2m5.7s | 2m5.7s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| stable-recovery | all_channels_bad | 131 | 2m5.1s | 2m5.1s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 5 | 0.007 | 0.00% | neutral |
| stable-recovery | all_channels_bad | 173 | 2m9.3s | 2m9.3s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 5 | 0.007 | 0.00% | neutral |
| stable-recovery | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.500 | 10s | 1 | 6 | not detected | 3 | 0.167 | 11.55% | neutral |
| stable-recovery | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.417 | 10s | 1 | 5 | 1m10s | 4 | 0.333 | 17.33% | win |
| stable-recovery | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| stable-recovery | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 1 | 0.083 | 0.00% | neutral |
| stable-recovery | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| stable-recovery | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| stable-recovery | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 20s | 1 | 7 | not detected | 3 | 0.167 | 5.78% | neutral |
| fast-recovery | gradual_degradation | 11 | 2m18.05810847s | 2m18.05810847s | 111.638008ms | 111.638008ms | 0.738 | 0.738 | 2.2s | 6 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| fast-recovery | gradual_degradation | 29 | 2m17.230127598s | 2m17.230127598s | 111.973508ms | 111.973508ms | 0.737 | 0.737 | 2.4s | 5 | 450 | not detected | 7 | 0.008 | 0.00% | neutral |
| fast-recovery | gradual_degradation | 47 | 2m18.543146187s | 2m18.543146187s | 112.017461ms | 112.017461ms | 0.743 | 0.743 | 2.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | gradual_degradation | 71 | 2m18.842157616s | 2m18.842157616s | 112.679273ms | 112.679273ms | 0.742 | 0.742 | 2.4s | 7 | 450 | not detected | 3 | 0.005 | 0.00% | neutral |
| fast-recovery | gradual_degradation | 101 | 2m17.896601353s | 2m17.896601353s | 112.288179ms | 112.288179ms | 0.740 | 0.740 | 2.2s | 7 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | gradual_degradation | 131 | 2m18.257709744s | 2m18.257709744s | 112.50077ms | 112.50077ms | 0.742 | 0.742 | 2.2s | 7 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| fast-recovery | gradual_degradation | 173 | 2m18.511286942s | 2m18.511286942s | 112.241073ms | 112.241073ms | 0.740 | 0.740 | 1.4s | 4 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| fast-recovery | sudden_outage | 11 | 306.854065ms | 307.297288ms | 29.758413ms | 29.775736ms | 0.669 | 0.669 | 2s | 5 | 300 | not detected | 3 | 0.004 | -0.01% | neutral |
| fast-recovery | sudden_outage | 29 | 305.736046ms | 468.508565ms | 29.586311ms | 39.871299ms | 0.667 | 0.040 | 2s | 5 | 10 | 3.6s | 8 | 0.011 | 81.04% | win |
| fast-recovery | sudden_outage | 47 | 306.757116ms | 307.058184ms | 29.297237ms | 29.386626ms | 0.676 | 0.676 | 2.4s | 7 | 300 | not detected | 1 | 0.002 | -0.02% | neutral |
| fast-recovery | sudden_outage | 71 | 306.766218ms | 308.694669ms | 29.362123ms | 29.602297ms | 0.673 | 0.673 | 2s | 4 | 300 | not detected | 3 | 0.007 | -0.08% | neutral |
| fast-recovery | sudden_outage | 101 | 306.026121ms | 306.150606ms | 29.696734ms | 29.751595ms | 0.673 | 0.673 | 2s | 4 | 300 | not detected | 1 | 0.002 | -0.01% | neutral |
| fast-recovery | sudden_outage | 131 | 308.156379ms | 309.590947ms | 29.458433ms | 29.597748ms | 0.676 | 0.676 | 2s | 5 | 300 | not detected | 3 | 0.004 | -0.05% | neutral |
| fast-recovery | sudden_outage | 173 | 307.890143ms | 308.721609ms | 29.101676ms | 29.690264ms | 0.673 | 0.673 | 1.2s | 3 | 300 | not detected | 3 | 0.004 | -0.14% | neutral |
| fast-recovery | capacity_aggregation | 11 | 1m19.876239452s | 27.789018651s | 25ms | 28ms | 0.988 | 0.938 | 0s | 0 | 317 | 6s | 26 | 0.006 | 67.24% | win |
| fast-recovery | capacity_aggregation | 29 | 1m19.971533361s | 29.29975655s | 25ms | 28ms | 0.988 | 0.935 | 0s | 0 | 293 | 1.125s | 28 | 0.010 | 64.81% | win |
| fast-recovery | capacity_aggregation | 47 | 1m19.89581372s | 31.451140162s | 25ms | 28ms | 0.988 | 0.946 | 0s | 0 | 325 | 3.5s | 20 | 0.006 | 63.55% | win |
| fast-recovery | capacity_aggregation | 71 | 1m19.767790605s | 26.364619745s | 25ms | 28ms | 0.988 | 0.926 | 0s | 0 | 217 | 1.125s | 56 | 0.006 | 68.82% | win |
| fast-recovery | capacity_aggregation | 101 | 1m20.040921681s | 32.037065841s | 25ms | 28ms | 0.990 | 0.949 | 0s | 0 | 313 | 1.125s | 20 | 0.006 | 62.81% | win |
| fast-recovery | capacity_aggregation | 131 | 1m19.734245977s | 23.923662872s | 25ms | 28ms | 0.988 | 0.944 | 0s | 0 | 218 | 1.625s | 26 | 0.007 | 70.25% | win |
| fast-recovery | capacity_aggregation | 173 | 1m19.865475022s | 21.720573315s | 25ms | 28ms | 0.988 | 0.932 | 0s | 0 | 206 | 1.125s | 35 | 0.006 | 72.08% | win |
| fast-recovery | transient_spike | 11 | 12.6925426s | 12.6925426s | 90ms | 90ms | 0.396 | 0.396 | 4.4s | 5 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| fast-recovery | transient_spike | 29 | 12.307633594s | 12.307633594s | 90ms | 90ms | 0.384 | 0.384 | 4.4s | 5 | 300 | not detected | 7 | 0.011 | 0.00% | neutral |
| fast-recovery | transient_spike | 47 | 12.854235214s | 12.854235214s | 90ms | 90ms | 0.402 | 0.402 | 4.6s | 6 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | transient_spike | 71 | 12.659328791s | 12.659328791s | 90ms | 90ms | 0.400 | 0.400 | 4.4s | 4 | 300 | not detected | 3 | 0.007 | 0.00% | neutral |
| fast-recovery | transient_spike | 101 | 12.607221347s | 12.607221347s | 90ms | 90ms | 0.398 | 0.398 | 4.2s | 4 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | transient_spike | 131 | 12.512324827s | 12.512324827s | 90ms | 90ms | 0.398 | 0.398 | 4.4s | 5 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| fast-recovery | transient_spike | 173 | 12.652228169s | 12.652228169s | 90ms | 90ms | 0.398 | 0.398 | 1.2s | 3 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| fast-recovery | stale_candidate | 11 | 46.540000047s | 30.693333363s | 110ms | 110ms | 0.889 | 0.596 | 9.99999999s | 12 | 101 | 37.666666629s | 10 | 0.029 | 37.94% | win |
| fast-recovery | stale_candidate | 29 | 51.736666713s | 20.906666684s | 110ms | 110ms | 0.883 | 0.380 | 9.666666657s | 8 | 63 | 24.999999975s | 14 | 0.035 | 63.80% | win |
| fast-recovery | stale_candidate | 47 | 51.540000046s | 23.436666688s | 110ms | 110ms | 0.883 | 0.439 | 9.333333324s | 9 | 73 | 28.333333305s | 6 | 0.023 | 58.67% | win |
| fast-recovery | stale_candidate | 71 | 50.403333377s | 8.923333342s | 110ms | 110ms | 0.889 | 0.246 | 9.333333324s | 10 | 35 | 15.666666651s | 10 | 0.029 | 83.10% | win |
| fast-recovery | stale_candidate | 101 | 48.010000047s | 26.966666693s | 110ms | 110ms | 0.883 | 0.538 | 9.666666657s | 8 | 88 | 33.3333333s | 10 | 0.029 | 47.72% | win |
| fast-recovery | stale_candidate | 131 | 47.873333381s | 20.240000018s | 110ms | 110ms | 0.883 | 0.398 | 9.333333324s | 8 | 65 | 25.666666641s | 10 | 0.029 | 61.53% | win |
| fast-recovery | stale_candidate | 173 | 46.87333338s | 14.846666681s | 110ms | 110ms | 0.883 | 0.339 | 9.666666657s | 9 | 56 | 22.666666644s | 10 | 0.035 | 70.74% | win |
| fast-recovery | recovery_no_flap | 11 | 44.799011002s | 44.799011002s | 95ms | 95ms | 0.752 | 0.752 | 4.2s | 5 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| fast-recovery | recovery_no_flap | 29 | 44.937203373s | 44.937203373s | 95ms | 95ms | 0.750 | 0.750 | 4.2s | 5 | 450 | not detected | 7 | 0.008 | 0.00% | neutral |
| fast-recovery | recovery_no_flap | 47 | 44.810457119s | 44.810457119s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | recovery_no_flap | 71 | 44.958361864s | 44.958361864s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 3 | 0.005 | 0.00% | neutral |
| fast-recovery | recovery_no_flap | 101 | 44.837077225s | 44.837077225s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | recovery_no_flap | 131 | 44.901274352s | 44.901274352s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| fast-recovery | recovery_no_flap | 173 | 44.9015634s | 44.9015634s | 95ms | 95ms | 0.755 | 0.755 | 1.2s | 3 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| fast-recovery | all_channels_bad | 11 | 2m10.9s | 2m10.9s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| fast-recovery | all_channels_bad | 29 | 2m9.8s | 2m9.8s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 7 | 0.011 | 0.00% | neutral |
| fast-recovery | all_channels_bad | 47 | 2m5.2s | 2m5.2s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | all_channels_bad | 71 | 2m9.4s | 2m9.4s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 3 | 0.007 | 0.00% | neutral |
| fast-recovery | all_channels_bad | 101 | 2m5.7s | 2m5.7s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| fast-recovery | all_channels_bad | 131 | 2m5.1s | 2m5.1s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| fast-recovery | all_channels_bad | 173 | 2m9.3s | 2m9.3s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| fast-recovery | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.500 | 10s | 1 | 6 | not detected | 3 | 0.167 | 11.55% | neutral |
| fast-recovery | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.417 | 10s | 1 | 5 | 1m10s | 4 | 0.333 | 17.33% | win |
| fast-recovery | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| fast-recovery | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.500 | 10s | 1 | 6 | 1m10s | 2 | 0.250 | 11.55% | neutral |
| fast-recovery | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| fast-recovery | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| fast-recovery | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 20s | 1 | 7 | not detected | 3 | 0.167 | 5.78% | neutral |
| hard-failure-confirmed | gradual_degradation | 11 | 2m18.05810847s | 2m18.05810847s | 111.638008ms | 111.638008ms | 0.738 | 0.738 | 2.2s | 6 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| hard-failure-confirmed | gradual_degradation | 29 | 2m17.230127598s | 1.294409667s | 111.973508ms | 40.919076ms | 0.737 | 0.060 | 2.4s | 5 | 31 | 7.8s | 10 | 0.008 | 98.83% | win |
| hard-failure-confirmed | gradual_degradation | 47 | 2m18.543146187s | 2m18.543146187s | 112.017461ms | 112.017461ms | 0.743 | 0.743 | 2.2s | 5 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| hard-failure-confirmed | gradual_degradation | 71 | 2m18.842157616s | 2m18.842157616s | 112.679273ms | 112.679273ms | 0.742 | 0.742 | 2.4s | 7 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | gradual_degradation | 101 | 2m17.896601353s | 2m17.896601353s | 112.288179ms | 112.288179ms | 0.740 | 0.740 | 2.2s | 7 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | gradual_degradation | 131 | 2m18.257709744s | 2m18.257709744s | 112.50077ms | 112.50077ms | 0.742 | 0.742 | 2.2s | 7 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| hard-failure-confirmed | gradual_degradation | 173 | 2m18.511286942s | 2m18.511286942s | 112.241073ms | 112.241073ms | 0.740 | 0.740 | 1.4s | 4 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| hard-failure-confirmed | sudden_outage | 11 | 306.854065ms | 307.297288ms | 29.758413ms | 29.775736ms | 0.669 | 0.669 | 2.6s | 8 | 300 | not detected | 3 | 0.004 | -0.01% | neutral |
| hard-failure-confirmed | sudden_outage | 29 | 305.736046ms | 471.487188ms | 29.586311ms | 40.071145ms | 0.667 | 0.038 | 2.2s | 6 | 11 | 3.8s | 8 | 0.011 | 80.86% | win |
| hard-failure-confirmed | sudden_outage | 47 | 306.757116ms | 307.844452ms | 29.297237ms | 29.386626ms | 0.676 | 0.676 | 2.6s | 8 | 300 | not detected | 3 | 0.004 | -0.03% | neutral |
| hard-failure-confirmed | sudden_outage | 71 | 306.766218ms | 308.348868ms | 29.362123ms | 29.436265ms | 0.673 | 0.673 | 2.2s | 5 | 300 | not detected | 1 | 0.002 | -0.04% | neutral |
| hard-failure-confirmed | sudden_outage | 101 | 306.026121ms | 306.150606ms | 29.696734ms | 29.751595ms | 0.673 | 0.673 | 2.2s | 5 | 300 | not detected | 1 | 0.002 | -0.01% | neutral |
| hard-failure-confirmed | sudden_outage | 131 | 308.156379ms | 309.590947ms | 29.458433ms | 29.597748ms | 0.676 | 0.676 | 2.2s | 6 | 300 | not detected | 3 | 0.004 | -0.05% | neutral |
| hard-failure-confirmed | sudden_outage | 173 | 307.890143ms | 308.721609ms | 29.101676ms | 29.690264ms | 0.673 | 0.673 | 1.2s | 3 | 300 | not detected | 3 | 0.004 | -0.14% | neutral |
| hard-failure-confirmed | capacity_aggregation | 11 | 1m19.876239452s | 37.332630014s | 25ms | 28ms | 0.988 | 0.942 | 0s | 0 | 398 | 6s | 28 | 0.006 | 57.55% | win |
| hard-failure-confirmed | capacity_aggregation | 29 | 1m19.971533361s | 28.096971912s | 25ms | 28ms | 0.988 | 0.929 | 0s | 0 | 283 | 1.125s | 36 | 0.008 | 66.16% | win |
| hard-failure-confirmed | capacity_aggregation | 47 | 1m19.89581372s | 25.389663277s | 25ms | 28ms | 0.988 | 0.957 | 0s | 0 | 400 | 19s | 25 | 0.007 | 67.40% | win |
| hard-failure-confirmed | capacity_aggregation | 71 | 1m19.767790605s | 36.231622305s | 25ms | 28ms | 0.988 | 0.946 | 0s | 0 | 352 | 1.125s | 28 | 0.006 | 57.52% | win |
| hard-failure-confirmed | capacity_aggregation | 101 | 1m20.040921681s | 37.749730773s | 25ms | 28ms | 0.990 | 0.949 | 0s | 0 | 399 | 5.5s | 22 | 0.006 | 57.03% | win |
| hard-failure-confirmed | capacity_aggregation | 131 | 1m19.734245977s | 23.494753725s | 25ms | 28ms | 0.988 | 0.932 | 0s | 0 | 229 | 2.625s | 35 | 0.006 | 71.33% | win |
| hard-failure-confirmed | capacity_aggregation | 173 | 1m19.865475022s | 26.19558416s | 25ms | 28ms | 0.988 | 0.924 | 0s | 0 | 194 | 1.125s | 35 | 0.006 | 68.50% | win |
| hard-failure-confirmed | transient_spike | 11 | 12.6925426s | 12.6925426s | 90ms | 90ms | 0.396 | 0.396 | 4.4s | 5 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| hard-failure-confirmed | transient_spike | 29 | 12.307633594s | 4.412328922s | 90ms | 90ms | 0.384 | 0.087 | 4.4s | 5 | 31 | 7.8s | 10 | 0.011 | 62.49% | win |
| hard-failure-confirmed | transient_spike | 47 | 12.854235214s | 12.854235214s | 90ms | 90ms | 0.402 | 0.402 | 4.6s | 6 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| hard-failure-confirmed | transient_spike | 71 | 12.659328791s | 12.659328791s | 90ms | 90ms | 0.400 | 0.400 | 4.4s | 4 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | transient_spike | 101 | 12.607221347s | 12.607221347s | 90ms | 90ms | 0.398 | 0.398 | 4.2s | 4 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | transient_spike | 131 | 12.512324827s | 12.512324827s | 90ms | 90ms | 0.398 | 0.398 | 4.4s | 5 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| hard-failure-confirmed | transient_spike | 173 | 12.652228169s | 12.652228169s | 90ms | 90ms | 0.398 | 0.398 | 1.2s | 3 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| hard-failure-confirmed | stale_candidate | 11 | 46.540000047s | 31.556666698s | 110ms | 110ms | 0.889 | 0.626 | 9.99999999s | 12 | 106 | 39.333333294s | 10 | 0.035 | 35.61% | win |
| hard-failure-confirmed | stale_candidate | 29 | 51.736666713s | 26.633333356s | 110ms | 110ms | 0.883 | 0.468 | 9.666666657s | 8 | 78 | 29.99999997s | 12 | 0.035 | 53.26% | win |
| hard-failure-confirmed | stale_candidate | 47 | 51.540000046s | 51.600000047s | 110ms | 110ms | 0.883 | 0.871 | 9.333333324s | 9 | 149 | not detected | 3 | 0.012 | 0.44% | neutral |
| hard-failure-confirmed | stale_candidate | 71 | 50.403333377s | 46.343333376s | 110ms | 110ms | 0.889 | 0.854 | 9.333333324s | 10 | 144 | not detected | 15 | 0.047 | 9.67% | neutral |
| hard-failure-confirmed | stale_candidate | 101 | 48.010000047s | 37.28333337s | 110ms | 110ms | 0.883 | 0.719 | 9.99999999s | 9 | 121 | 44.333333289s | 10 | 0.029 | 25.34% | win |
| hard-failure-confirmed | stale_candidate | 131 | 47.873333381s | 47.070000047s | 110ms | 110ms | 0.883 | 0.865 | 9.333333324s | 8 | 148 | not detected | 7 | 0.023 | 2.12% | neutral |
| hard-failure-confirmed | stale_candidate | 173 | 46.87333338s | 17.376666685s | 110ms | 110ms | 0.883 | 0.392 | 9.99999999s | 10 | 63 | 24.999999975s | 12 | 0.035 | 65.80% | win |
| hard-failure-confirmed | recovery_no_flap | 11 | 44.799011002s | 44.799011002s | 95ms | 95ms | 0.752 | 0.752 | 4.2s | 5 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| hard-failure-confirmed | recovery_no_flap | 29 | 44.937203373s | 1.9s | 95ms | 95ms | 0.750 | 0.075 | 4.2s | 5 | 31 | 7.8s | 10 | 0.008 | 94.36% | win |
| hard-failure-confirmed | recovery_no_flap | 47 | 44.810457119s | 44.810457119s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| hard-failure-confirmed | recovery_no_flap | 71 | 44.958361864s | 44.958361864s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | recovery_no_flap | 101 | 44.837077225s | 44.837077225s | 95ms | 95ms | 0.755 | 0.755 | 4.2s | 4 | 450 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | recovery_no_flap | 131 | 44.901274352s | 44.901274352s | 95ms | 95ms | 0.757 | 0.757 | 4.2s | 5 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| hard-failure-confirmed | recovery_no_flap | 173 | 44.9015634s | 44.9015634s | 95ms | 95ms | 0.755 | 0.755 | 1.2s | 3 | 450 | not detected | 3 | 0.003 | 0.00% | neutral |
| hard-failure-confirmed | all_channels_bad | 11 | 2m10.9s | 2m10.9s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| hard-failure-confirmed | all_channels_bad | 29 | 2m9.8s | 1m27.38s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 22 | 0.011 | 32.89% | win |
| hard-failure-confirmed | all_channels_bad | 47 | 2m5.2s | 2m5.2s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| hard-failure-confirmed | all_channels_bad | 71 | 2m9.4s | 2m9.4s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | all_channels_bad | 101 | 2m5.7s | 2m5.7s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 1 | 0.002 | 0.00% | neutral |
| hard-failure-confirmed | all_channels_bad | 131 | 2m5.1s | 2m5.1s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| hard-failure-confirmed | all_channels_bad | 173 | 2m9.3s | 2m9.3s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 3 | 0.004 | 0.00% | neutral |
| hard-failure-confirmed | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.500 | 10s | 1 | 6 | not detected | 3 | 0.167 | 11.55% | neutral |
| hard-failure-confirmed | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.417 | 10s | 1 | 5 | 1m10s | 4 | 0.333 | 17.33% | win |
| hard-failure-confirmed | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| hard-failure-confirmed | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 1 | 0.083 | 0.00% | neutral |
| hard-failure-confirmed | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| hard-failure-confirmed | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| hard-failure-confirmed | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 20s | 1 | 7 | not detected | 3 | 0.167 | 5.78% | neutral |
