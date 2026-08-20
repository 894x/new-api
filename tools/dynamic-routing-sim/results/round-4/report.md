# Dynamic routing simulation

Significant improvement gate: 15.00%; maximum regression: 8.00%; minimum scenario win rate: 0.45.

## Ranking

| Rank | Parameters | Average improvement | Worst seed | Win rate | Significant | Regressed runs |
|---:|---|---:|---:|---:|:---:|---:|
| 1 | hard-failure-confirmed | 59.55% | -14.48% | 0.78 | false | 5 |
| 2 | stable-recovery | 58.81% | -14.48% | 0.78 | false | 5 |
| 3 | fast-recovery | 58.15% | -17.97% | 0.78 | false | 7 |
| 4 | balanced | 56.57% | -14.48% | 0.78 | false | 5 |
| 5 | short-window | 55.37% | -17.97% | 0.67 | false | 7 |
| 6 | aggressive | 54.18% | -23.76% | 0.78 | false | 7 |
| 7 | fast-low-probe | 50.83% | -9.47% | 0.67 | false | 1 |
| 8 | conservative | 35.52% | -9.47% | 0.56 | false | 2 |

## Every scenario and seed

| Parameters | Scenario | Seed | Static p95 TTFT | Dynamic p95 TTFT | Static p95 TPOT | Dynamic p95 TPOT | Static SLO violations | Dynamic SLO violations | Detection elapsed | Detection observations | Bad exposure | Mitigation elapsed | Route reversals | Probe cost | Improvement | Result |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| conservative | gradual_degradation | 11 | 2m18.05810847s | 5.944069448s | 111.638008ms | 87.804571ms | 0.738 | 0.220 | 2.4s | 7 | 132 | 33.4s | 0 | 0.013 | 95.30% | win |
| conservative | gradual_degradation | 29 | 2m17.230127598s | 5.851193856s | 111.973508ms | 88.555063ms | 0.737 | 0.223 | 2.6s | 6 | 132 | 33.4s | 0 | 0.013 | 95.32% | win |
| conservative | gradual_degradation | 47 | 2m18.543146187s | 6.249989982s | 112.017461ms | 88.933925ms | 0.743 | 0.243 | 2.8s | 7 | 133 | 33.6s | 0 | 0.013 | 95.08% | win |
| conservative | gradual_degradation | 71 | 2m18.842157616s | 6.098097682s | 112.679273ms | 86.825481ms | 0.742 | 0.232 | 2.6s | 7 | 132 | 33.4s | 0 | 0.013 | 95.22% | win |
| conservative | gradual_degradation | 101 | 2m17.896601353s | 17.910313621s | 112.288179ms | 100.296049ms | 0.740 | 0.290 | 2.8s | 8 | 164 | 40s | 0 | 0.015 | 87.96% | win |
| conservative | gradual_degradation | 131 | 2m18.257709744s | 6.155609811s | 112.50077ms | 89.315548ms | 0.742 | 0.225 | 0s | 0 | 133 | 33.6s | 0 | 0.013 | 95.14% | win |
| conservative | gradual_degradation | 173 | 2m18.511286942s | 2m11.148217617s | 112.241073ms | 112.241073ms | 0.740 | 0.722 | 1.4s | 4 | 436 | not detected | 0 | 0.030 | 5.86% | neutral |
| conservative | sudden_outage | 11 | 15.6s | 479.349032ms | 29.758413ms | 39.873891ms | 0.669 | 0.044 | 2.4s | 6 | 13 | 4s | 0 | 0.011 | 93.48% | win |
| conservative | sudden_outage | 29 | 15.6s | 475.527462ms | 29.586311ms | 40.071145ms | 0.667 | 0.038 | 2.4s | 6 | 11 | 4s | 0 | 0.011 | 93.71% | win |
| conservative | sudden_outage | 47 | 15.6s | 1.332884407s | 29.297237ms | 39.891769ms | 0.676 | 0.060 | 2.4s | 6 | 11 | 4s | 0 | 0.011 | 89.64% | win |
| conservative | sudden_outage | 71 | 15.6s | 1.059917758s | 29.362123ms | 40.221924ms | 0.673 | 0.053 | 2.8s | 7 | 13 | 4.4s | 0 | 0.011 | 90.78% | win |
| conservative | sudden_outage | 101 | 15.6s | 1.329395143s | 29.696734ms | 39.777203ms | 0.673 | 0.062 | 2.6s | 6 | 12 | 4.2s | 0 | 0.011 | 89.56% | win |
| conservative | sudden_outage | 131 | 15.6s | 1.328626655s | 29.458433ms | 40.174049ms | 0.676 | 0.060 | 2.4s | 6 | 11 | 4s | 0 | 0.011 | 89.64% | win |
| conservative | sudden_outage | 173 | 15.6s | 479.736924ms | 29.101676ms | 40.017923ms | 0.673 | 0.049 | 1.2s | 3 | 11 | 4s | 0 | 0.011 | 93.54% | win |
| conservative | capacity_aggregation | 11 | 1m19.876239452s | 26.065736129s | 25ms | 28ms | 0.988 | 0.942 | 0s | 0 | 333 | not detected | 0 | 0.011 | 68.30% | win |
| conservative | capacity_aggregation | 29 | 1m19.971533361s | 26.059618083s | 25ms | 28ms | 0.988 | 0.944 | 0s | 0 | 333 | not detected | 0 | 0.011 | 68.35% | win |
| conservative | capacity_aggregation | 47 | 1m19.89581372s | 26.05044383s | 25ms | 28ms | 0.988 | 0.942 | 0s | 0 | 333 | not detected | 0 | 0.011 | 68.32% | win |
| conservative | capacity_aggregation | 71 | 1m19.767790605s | 26.262468775s | 25ms | 28ms | 0.988 | 0.943 | 0s | 0 | 334 | not detected | 0 | 0.011 | 68.05% | win |
| conservative | capacity_aggregation | 101 | 1m20.040921681s | 26.245578616s | 25ms | 28ms | 0.990 | 0.943 | 0s | 0 | 333 | not detected | 0 | 0.011 | 68.19% | win |
| conservative | capacity_aggregation | 131 | 1m19.734245977s | 26.100518023s | 25ms | 28ms | 0.988 | 0.943 | 0s | 0 | 334 | not detected | 0 | 0.011 | 68.18% | win |
| conservative | capacity_aggregation | 173 | 1m19.865475022s | 26.354953554s | 25ms | 28ms | 0.988 | 0.943 | 0s | 0 | 334 | not detected | 0 | 0.011 | 67.97% | win |
| conservative | transient_spike | 11 | 12.6925426s | 11.260884334s | 90ms | 90ms | 0.396 | 0.260 | 4.4s | 5 | 206 | not detected | 1 | 0.018 | 14.48% | neutral |
| conservative | transient_spike | 29 | 12.307633594s | 11.38156223s | 90ms | 90ms | 0.384 | 0.260 | 4.8s | 6 | 205 | not detected | 1 | 0.018 | 10.91% | neutral |
| conservative | transient_spike | 47 | 12.854235214s | 11.241006615s | 90ms | 90ms | 0.402 | 0.271 | 4.4s | 5 | 209 | not detected | 1 | 0.018 | 15.23% | win |
| conservative | transient_spike | 71 | 12.659328791s | 11.089490677s | 90ms | 90ms | 0.400 | 0.353 | 4.6s | 4 | 291 | not detected | 0 | 0.029 | 12.43% | neutral |
| conservative | transient_spike | 101 | 12.607221347s | 11.148264274s | 90ms | 90ms | 0.398 | 0.360 | 4.6s | 4 | 291 | not detected | 0 | 0.029 | 11.31% | neutral |
| conservative | transient_spike | 131 | 12.512324827s | 11.053289599s | 90ms | 90ms | 0.398 | 0.264 | 4.8s | 6 | 205 | not detected | 1 | 0.018 | 14.35% | neutral |
| conservative | transient_spike | 173 | 12.652228169s | 11.063425406s | 90ms | 90ms | 0.398 | 0.353 | 1.2s | 3 | 291 | not detected | 0 | 0.029 | 12.41% | neutral |
| conservative | stale_candidate | 11 | 46.403333381s | 42.146666713s | 110ms | 110ms | 0.889 | 0.860 | 10.333333323s | 13 | 146 | not detected | 0 | 0.035 | 9.00% | neutral |
| conservative | stale_candidate | 29 | 52.933333379s | 49.266666712s | 110ms | 110ms | 0.883 | 0.854 | 9.99999999s | 11 | 146 | not detected | 0 | 0.035 | 8.01% | regression |
| conservative | stale_candidate | 47 | 51.540000046s | 48.736666712s | 110ms | 110ms | 0.883 | 0.854 | 10.999999989s | 12 | 146 | not detected | 0 | 0.035 | 6.42% | neutral |
| conservative | stale_candidate | 71 | 52.600000048s | 48.736666714s | 110ms | 110ms | 0.889 | 0.860 | 11.333333322s | 13 | 146 | not detected | 0 | 0.035 | 8.00% | neutral |
| conservative | stale_candidate | 101 | 49.070000047s | 46.403333379s | 110ms | 110ms | 0.883 | 0.860 | 10.666666656s | 11 | 146 | not detected | 0 | 0.035 | 6.39% | neutral |
| conservative | stale_candidate | 131 | 47.40333338s | 44.87333338s | 110ms | 110ms | 0.883 | 0.854 | 10.333333323s | 12 | 146 | not detected | 0 | 0.035 | 6.07% | neutral |
| conservative | stale_candidate | 173 | 46.87333338s | 45.070000047s | 110ms | 110ms | 0.883 | 0.854 | 10.666666656s | 11 | 146 | not detected | 0 | 0.035 | 4.75% | neutral |
| conservative | recovery_no_flap | 11 | 44.799011002s | 28.835s | 95ms | 95ms | 0.752 | 0.232 | 4.4s | 5 | 131 | 33.2s | 0 | 0.013 | 50.11% | win |
| conservative | recovery_no_flap | 29 | 44.937203373s | 28.784219768s | 95ms | 95ms | 0.750 | 0.243 | 4.4s | 5 | 134 | 34.8s | 0 | 0.013 | 50.06% | win |
| conservative | recovery_no_flap | 47 | 44.810457119s | 28.830009826s | 95ms | 95ms | 0.757 | 0.245 | 4.4s | 5 | 131 | 33.2s | 0 | 0.013 | 50.07% | win |
| conservative | recovery_no_flap | 71 | 44.958361864s | 28.480308746s | 95ms | 95ms | 0.755 | 0.235 | 4.4s | 4 | 130 | 33s | 0 | 0.013 | 50.83% | win |
| conservative | recovery_no_flap | 101 | 44.837077225s | 36.09s | 95ms | 95ms | 0.755 | 0.293 | 4.4s | 4 | 163 | 39.8s | 0 | 0.015 | 36.45% | win |
| conservative | recovery_no_flap | 131 | 44.901274352s | 28.835s | 95ms | 95ms | 0.757 | 0.238 | 4.4s | 5 | 131 | 33.2s | 0 | 0.013 | 50.21% | win |
| conservative | recovery_no_flap | 173 | 44.9015634s | 42.56534688s | 95ms | 95ms | 0.755 | 0.453 | 1.2s | 3 | 260 | 59.8s | 0 | 0.020 | 18.07% | win |
| conservative | all_channels_bad | 11 | 2m10.9s | 1m46.5s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 0 | 0.027 | 20.86% | win |
| conservative | all_channels_bad | 29 | 2m11.7s | 1m48.9s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 0 | 0.027 | 19.30% | win |
| conservative | all_channels_bad | 47 | 2m7.1s | 1m46.8s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.027 | 17.56% | win |
| conservative | all_channels_bad | 71 | 2m9.6s | 1m51.7s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.027 | 15.61% | win |
| conservative | all_channels_bad | 101 | 2m6.5s | 1m45.7s | 100ms | 100ms | 0.673 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.027 | 18.34% | win |
| conservative | all_channels_bad | 131 | 2m5.1s | 1m45.9s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.027 | 17.35% | win |
| conservative | all_channels_bad | 173 | 2m9.753516526s | 1m50.5s | 100ms | 100ms | 0.673 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.027 | 16.82% | win |
| conservative | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| conservative | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| conservative | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| conservative | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| conservative | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| conservative | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| conservative | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| conservative | healthy_steady_state | 11 | 306.901147ms | 309.864745ms | 29.560991ms | 29.873254ms | 0.015 | 0.015 | not detected | 0 | 0 | not detected | 0 | 0.030 | -0.97% | neutral |
| conservative | healthy_steady_state | 29 | 306.449512ms | 309.587038ms | 29.524996ms | 29.812279ms | 0.015 | 0.015 | not detected | 0 | 0 | not detected | 0 | 0.030 | -0.94% | neutral |
| conservative | healthy_steady_state | 47 | 306.757116ms | 384.784114ms | 29.42026ms | 29.761595ms | 0.028 | 0.028 | not detected | 0 | 0 | not detected | 0 | 0.030 | -9.47% | regression |
| conservative | healthy_steady_state | 71 | 304.971872ms | 309.913902ms | 29.463197ms | 29.779027ms | 0.018 | 0.018 | not detected | 0 | 0 | not detected | 0 | 0.030 | -1.20% | neutral |
| conservative | healthy_steady_state | 101 | 306.026121ms | 320.996206ms | 29.656679ms | 29.816417ms | 0.023 | 0.025 | not detected | 0 | 0 | not detected | 0 | 0.030 | -2.73% | neutral |
| conservative | healthy_steady_state | 131 | 307.461992ms | 335.663555ms | 29.32466ms | 29.677918ms | 0.022 | 0.022 | not detected | 0 | 0 | not detected | 0 | 0.030 | -3.95% | neutral |
| conservative | healthy_steady_state | 173 | 306.762211ms | 309.43963ms | 29.635213ms | 29.854132ms | 0.022 | 0.025 | not detected | 0 | 0 | not detected | 0 | 0.030 | -2.10% | neutral |
| balanced | gradual_degradation | 11 | 2m18.05810847s | 654.958152ms | 111.638008ms | 40.491731ms | 0.738 | 0.027 | 2.4s | 7 | 15 | 4.6s | 0 | 0.013 | 99.25% | win |
| balanced | gradual_degradation | 29 | 2m17.230127598s | 759.731479ms | 111.973508ms | 40.315928ms | 0.737 | 0.032 | 2.6s | 6 | 14 | 4.4s | 0 | 0.013 | 99.18% | win |
| balanced | gradual_degradation | 47 | 2m18.543146187s | 883.925008ms | 112.017461ms | 40.306719ms | 0.743 | 0.047 | 2.4s | 5 | 14 | 4.4s | 0 | 0.013 | 99.09% | win |
| balanced | gradual_degradation | 71 | 2m18.842157616s | 681.380327ms | 112.679273ms | 40.566479ms | 0.742 | 0.035 | 2.4s | 6 | 13 | 4.2s | 0 | 0.013 | 99.22% | win |
| balanced | gradual_degradation | 101 | 2m17.896601353s | 894.737117ms | 112.288179ms | 40.336063ms | 0.740 | 0.043 | 2.2s | 6 | 14 | 4.4s | 0 | 0.013 | 99.09% | win |
| balanced | gradual_degradation | 131 | 2m18.257709744s | 681.552502ms | 112.50077ms | 40.537649ms | 0.742 | 0.033 | 2.2s | 6 | 13 | 4.2s | 0 | 0.013 | 99.22% | win |
| balanced | gradual_degradation | 173 | 2m18.511286942s | 890.975405ms | 112.241073ms | 40.555345ms | 0.740 | 0.042 | 1.4s | 4 | 13 | 4.2s | 0 | 0.013 | 99.10% | win |
| balanced | sudden_outage | 11 | 15.6s | 477.877788ms | 29.758413ms | 40.036541ms | 0.669 | 0.038 | 2.2s | 5 | 10 | 3.8s | 0 | 0.018 | 93.77% | win |
| balanced | sudden_outage | 29 | 15.6s | 477.005412ms | 29.586311ms | 39.871299ms | 0.667 | 0.040 | 2.2s | 5 | 10 | 3.8s | 0 | 0.018 | 93.74% | win |
| balanced | sudden_outage | 47 | 15.6s | 1.062780158s | 29.297237ms | 40.199194ms | 0.676 | 0.056 | 2.2s | 5 | 10 | 3.8s | 0 | 0.018 | 90.93% | win |
| balanced | sudden_outage | 71 | 15.6s | 478.860123ms | 29.362123ms | 39.983305ms | 0.673 | 0.047 | 2.2s | 4 | 10 | 3.8s | 0 | 0.018 | 93.64% | win |
| balanced | sudden_outage | 101 | 15.6s | 479.387647ms | 29.696734ms | 40.248819ms | 0.673 | 0.049 | 2.4s | 5 | 11 | 4s | 0 | 0.018 | 93.52% | win |
| balanced | sudden_outage | 131 | 15.6s | 478.127474ms | 29.458433ms | 40.293239ms | 0.676 | 0.040 | 2.2s | 5 | 10 | 3.8s | 0 | 0.018 | 93.73% | win |
| balanced | sudden_outage | 173 | 15.6s | 1.332870167s | 29.101676ms | 40.048681ms | 0.673 | 0.060 | 1.2s | 3 | 10 | 3.8s | 0 | 0.018 | 89.69% | win |
| balanced | capacity_aggregation | 11 | 1m19.876239452s | 36.421045246s | 25ms | 28ms | 0.988 | 0.938 | 0s | 0 | 383 | 3.375s | 2 | 0.007 | 58.36% | win |
| balanced | capacity_aggregation | 29 | 1m19.971533361s | 36.627172725s | 25ms | 28ms | 0.988 | 0.942 | 0s | 0 | 376 | 3.25s | 2 | 0.007 | 58.17% | win |
| balanced | capacity_aggregation | 47 | 1m19.89581372s | 36.770404785s | 25ms | 28ms | 0.988 | 0.939 | 0s | 0 | 383 | 3.375s | 2 | 0.007 | 58.05% | win |
| balanced | capacity_aggregation | 71 | 1m19.767790605s | 36.204382036s | 25ms | 28ms | 0.988 | 0.939 | 0s | 0 | 382 | 3.375s | 2 | 0.007 | 58.53% | win |
| balanced | capacity_aggregation | 101 | 1m20.040921681s | 36.446674146s | 25ms | 28ms | 0.990 | 0.943 | 0s | 0 | 382 | 3.375s | 2 | 0.007 | 58.43% | win |
| balanced | capacity_aggregation | 131 | 1m19.734245977s | 36.592243941s | 25ms | 28ms | 0.988 | 0.942 | 0s | 0 | 383 | 3.375s | 2 | 0.007 | 58.09% | win |
| balanced | capacity_aggregation | 173 | 1m19.865475022s | 36.250471072s | 25ms | 28ms | 0.988 | 0.940 | 0s | 0 | 381 | 3.25s | 2 | 0.007 | 58.50% | win |
| balanced | transient_spike | 11 | 12.6925426s | 1.474575717s | 90ms | 40.91354ms | 0.396 | 0.062 | 4.4s | 5 | 22 | 6.2s | 0 | 0.020 | 85.95% | win |
| balanced | transient_spike | 29 | 12.307633594s | 1.920996335s | 90ms | 90ms | 0.384 | 0.069 | 4.8s | 6 | 24 | 6.6s | 0 | 0.020 | 77.99% | win |
| balanced | transient_spike | 47 | 12.854235214s | 1.455645454s | 90ms | 40.846125ms | 0.402 | 0.082 | 4.4s | 5 | 22 | 6.2s | 0 | 0.020 | 85.85% | win |
| balanced | transient_spike | 71 | 12.659328791s | 1.689962938s | 90ms | 40.987696ms | 0.400 | 0.076 | 4.6s | 4 | 23 | 6.4s | 0 | 0.020 | 84.37% | win |
| balanced | transient_spike | 101 | 12.607221347s | 1.830747626s | 90ms | 40.995495ms | 0.398 | 0.078 | 4.6s | 4 | 23 | 6.4s | 0 | 0.020 | 83.47% | win |
| balanced | transient_spike | 131 | 12.512324827s | 1.959281014s | 90ms | 90ms | 0.398 | 0.084 | 4.8s | 6 | 24 | 6.6s | 0 | 0.020 | 77.81% | win |
| balanced | transient_spike | 173 | 12.652228169s | 1.857941039s | 90ms | 40.951473ms | 0.398 | 0.089 | 1.2s | 3 | 23 | 6.4s | 0 | 0.020 | 83.16% | win |
| balanced | stale_candidate | 11 | 46.403333381s | 27.103333361s | 110ms | 110ms | 0.889 | 0.550 | 9.99999999s | 12 | 91 | 34.666666632s | 0 | 0.035 | 45.67% | win |
| balanced | stale_candidate | 29 | 52.933333379s | 30.026666695s | 110ms | 110ms | 0.883 | 0.538 | 9.666666657s | 9 | 90 | 34.333333299s | 0 | 0.035 | 47.59% | win |
| balanced | stale_candidate | 47 | 51.540000046s | 28.496666694s | 110ms | 110ms | 0.883 | 0.526 | 9.333333324s | 9 | 90 | 34.333333299s | 0 | 0.035 | 49.26% | win |
| balanced | stale_candidate | 71 | 52.600000048s | 35.223333367s | 110ms | 110ms | 0.889 | 0.667 | 10.333333323s | 11 | 109 | 40.999999959s | 0 | 0.041 | 36.23% | win |
| balanced | stale_candidate | 101 | 49.070000047s | 27.830000028s | 110ms | 110ms | 0.883 | 0.550 | 9.99999999s | 9 | 90 | 34.333333299s | 0 | 0.035 | 47.24% | win |
| balanced | stale_candidate | 131 | 47.40333338s | 25.966666694s | 110ms | 110ms | 0.883 | 0.538 | 9.666666657s | 10 | 90 | 34.333333299s | 0 | 0.035 | 49.05% | win |
| balanced | stale_candidate | 173 | 46.87333338s | 26.300000027s | 110ms | 110ms | 0.883 | 0.544 | 9.99999999s | 10 | 90 | 34.333333299s | 0 | 0.035 | 47.80% | win |
| balanced | recovery_no_flap | 11 | 44.799011002s | 1.358971675s | 95ms | 40.750888ms | 0.752 | 0.053 | 4.4s | 5 | 22 | 6.2s | 0 | 0.015 | 96.68% | win |
| balanced | recovery_no_flap | 29 | 44.937203373s | 1.3901921s | 95ms | 40.635981ms | 0.750 | 0.060 | 4.4s | 5 | 22 | 6.2s | 0 | 0.015 | 96.61% | win |
| balanced | recovery_no_flap | 47 | 44.810457119s | 1.438387173s | 95ms | 40.825291ms | 0.757 | 0.072 | 4.4s | 5 | 23 | 6.2s | 0 | 0.015 | 96.48% | win |
| balanced | recovery_no_flap | 71 | 44.958361864s | 1.3590909s | 95ms | 40.858579ms | 0.755 | 0.065 | 4.4s | 4 | 22 | 6.2s | 0 | 0.015 | 96.63% | win |
| balanced | recovery_no_flap | 101 | 44.837077225s | 1.397312725s | 95ms | 40.589333ms | 0.755 | 0.070 | 4.4s | 4 | 22 | 6.2s | 0 | 0.015 | 96.56% | win |
| balanced | recovery_no_flap | 131 | 44.901274352s | 1.377752802s | 95ms | 40.846107ms | 0.757 | 0.065 | 4.4s | 5 | 22 | 6.2s | 0 | 0.015 | 96.61% | win |
| balanced | recovery_no_flap | 173 | 44.9015634s | 1.398788593s | 95ms | 40.741428ms | 0.755 | 0.067 | 1.2s | 3 | 22 | 6.2s | 0 | 0.015 | 96.57% | win |
| balanced | all_channels_bad | 11 | 2m10.9s | 1m22.2s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 0 | 0.040 | 39.27% | win |
| balanced | all_channels_bad | 29 | 2m11.7s | 1m23s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 0 | 0.040 | 38.68% | win |
| balanced | all_channels_bad | 47 | 2m7.1s | 1m19.9s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.040 | 38.45% | win |
| balanced | all_channels_bad | 71 | 2m9.6s | 1m23.4s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.040 | 37.66% | win |
| balanced | all_channels_bad | 101 | 2m6.5s | 1m20.2s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.040 | 38.45% | win |
| balanced | all_channels_bad | 131 | 2m5.1s | 1m20.7s | 100ms | 100ms | 0.676 | 0.678 | 0s | 0 | 300 | not detected | 0 | 0.040 | 37.35% | win |
| balanced | all_channels_bad | 173 | 2m9.753516526s | 1m23.2s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.040 | 37.61% | win |
| balanced | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| balanced | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| balanced | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| balanced | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| balanced | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| balanced | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| balanced | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| balanced | healthy_steady_state | 11 | 306.901147ms | 363.70768ms | 29.560991ms | 29.991747ms | 0.015 | 0.015 | not detected | 0 | 0 | not detected | 0 | 0.050 | -7.57% | neutral |
| balanced | healthy_steady_state | 29 | 306.449512ms | 359.462254ms | 29.524996ms | 29.953468ms | 0.015 | 0.017 | not detected | 0 | 0 | not detected | 0 | 0.050 | -7.88% | neutral |
| balanced | healthy_steady_state | 47 | 306.757116ms | 425.449182ms | 29.42026ms | 29.977762ms | 0.028 | 0.028 | not detected | 0 | 0 | not detected | 0 | 0.050 | -14.48% | regression |
| balanced | healthy_steady_state | 71 | 304.971872ms | 370.046734ms | 29.463197ms | 29.915952ms | 0.018 | 0.018 | not detected | 0 | 0 | not detected | 0 | 0.050 | -8.64% | regression |
| balanced | healthy_steady_state | 101 | 306.026121ms | 406.359393ms | 29.656679ms | 29.974328ms | 0.023 | 0.025 | not detected | 0 | 0 | not detected | 0 | 0.050 | -12.77% | regression |
| balanced | healthy_steady_state | 131 | 307.461992ms | 406.434429ms | 29.32466ms | 29.9471ms | 0.022 | 0.023 | not detected | 0 | 0 | not detected | 0 | 0.050 | -13.38% | regression |
| balanced | healthy_steady_state | 173 | 306.762211ms | 371.181352ms | 29.635213ms | 29.993773ms | 0.022 | 0.022 | not detected | 0 | 0 | not detected | 0 | 0.050 | -8.18% | regression |
| aggressive | gradual_degradation | 11 | 2m18.05810847s | 740.933169ms | 111.638008ms | 40.645944ms | 0.738 | 0.040 | 0s | 0 | 24 | 6.8s | 0 | 0.023 | 99.18% | win |
| aggressive | gradual_degradation | 29 | 2m17.230127598s | 759.457226ms | 111.973508ms | 40.302296ms | 0.737 | 0.030 | 2.4s | 5 | 12 | 4.2s | 0 | 0.022 | 99.18% | win |
| aggressive | gradual_degradation | 47 | 2m18.543146187s | 1.062780158s | 112.017461ms | 40.548329ms | 0.743 | 0.060 | 2.2s | 5 | 24 | 6.8s | 0 | 0.023 | 98.97% | win |
| aggressive | gradual_degradation | 71 | 2m18.842157616s | 676.163776ms | 112.679273ms | 40.551492ms | 0.742 | 0.037 | 600ms | 3 | 13 | 4.4s | 0 | 0.022 | 99.22% | win |
| aggressive | gradual_degradation | 101 | 2m17.896601353s | 814.84125ms | 112.288179ms | 40.314085ms | 0.740 | 0.037 | 400ms | 3 | 10 | 3.8s | 0 | 0.022 | 99.15% | win |
| aggressive | gradual_degradation | 131 | 2m18.257709744s | 663.83331ms | 112.50077ms | 40.489754ms | 0.742 | 0.028 | 0s | 0 | 10 | 3.8s | 0 | 0.022 | 99.24% | win |
| aggressive | gradual_degradation | 173 | 2m18.511286942s | 873.722023ms | 112.241073ms | 40.593847ms | 0.740 | 0.037 | 1.2s | 3 | 9 | 3.6s | 0 | 0.022 | 99.11% | win |
| aggressive | sudden_outage | 11 | 15.6s | 477.538086ms | 29.758413ms | 39.981094ms | 0.669 | 0.038 | 0s | 0 | 9 | 3.6s | 0 | 0.029 | 93.84% | win |
| aggressive | sudden_outage | 29 | 15.6s | 476.763595ms | 29.586311ms | 39.786424ms | 0.667 | 0.038 | 2s | 5 | 9 | 3.6s | 0 | 0.029 | 93.85% | win |
| aggressive | sudden_outage | 47 | 15.6s | 1.332884407s | 29.297237ms | 39.983315ms | 0.676 | 0.060 | 2.4s | 7 | 11 | 4s | 0 | 0.029 | 89.64% | win |
| aggressive | sudden_outage | 71 | 15.6s | 479.737793ms | 29.362123ms | 40.16844ms | 0.673 | 0.044 | 400ms | 1 | 9 | 3.6s | 0 | 0.029 | 93.72% | win |
| aggressive | sudden_outage | 101 | 15.6s | 477.070664ms | 29.696734ms | 40.302567ms | 0.673 | 0.036 | 200ms | 1 | 9 | 3.6s | 0 | 0.029 | 93.86% | win |
| aggressive | sudden_outage | 131 | 15.6s | 1.002554073s | 29.458433ms | 39.973344ms | 0.676 | 0.051 | 0s | 0 | 9 | 3.6s | 0 | 0.029 | 91.34% | win |
| aggressive | sudden_outage | 173 | 15.6s | 478.373374ms | 29.101676ms | 40.107881ms | 0.673 | 0.047 | 800ms | 2 | 9 | 3.6s | 0 | 0.029 | 93.70% | win |
| aggressive | capacity_aggregation | 11 | 1m19.876239452s | 1m6.379054481s | 25ms | 28ms | 0.988 | 0.907 | 0s | 0 | 588 | not detected | 0 | 0.081 | 18.83% | win |
| aggressive | capacity_aggregation | 29 | 1m19.971533361s | 1m6.404914573s | 25ms | 28ms | 0.988 | 0.907 | 0s | 0 | 588 | not detected | 0 | 0.081 | 18.88% | win |
| aggressive | capacity_aggregation | 47 | 1m19.89581372s | 1m6.386169879s | 25ms | 28ms | 0.988 | 0.907 | 0s | 0 | 588 | not detected | 0 | 0.081 | 18.85% | win |
| aggressive | capacity_aggregation | 71 | 1m19.767790605s | 1m6.111167294s | 25ms | 28ms | 0.988 | 0.907 | 0s | 0 | 588 | not detected | 0 | 0.081 | 18.99% | win |
| aggressive | capacity_aggregation | 101 | 1m20.040921681s | 1m6.357418244s | 25ms | 28ms | 0.990 | 0.907 | 0s | 0 | 588 | not detected | 0 | 0.081 | 18.98% | win |
| aggressive | capacity_aggregation | 131 | 1m19.734245977s | 1m6.141091172s | 25ms | 28ms | 0.988 | 0.907 | 0s | 0 | 588 | not detected | 0 | 0.081 | 18.95% | win |
| aggressive | capacity_aggregation | 173 | 1m19.865475022s | 1m6.225737267s | 25ms | 28ms | 0.988 | 0.907 | 0s | 0 | 588 | not detected | 0 | 0.081 | 18.94% | win |
| aggressive | transient_spike | 11 | 12.6925426s | 1.459856285s | 90ms | 40.920944ms | 0.396 | 0.067 | 0s | 0 | 22 | 6.4s | 0 | 0.031 | 85.87% | win |
| aggressive | transient_spike | 29 | 12.307633594s | 1.442787767s | 90ms | 40.680677ms | 0.384 | 0.069 | 4.4s | 5 | 20 | 6s | 0 | 0.031 | 85.70% | win |
| aggressive | transient_spike | 47 | 12.854235214s | 2.125018021s | 90ms | 90ms | 0.402 | 0.069 | 4.6s | 6 | 23 | 6.6s | 0 | 0.031 | 77.77% | win |
| aggressive | transient_spike | 71 | 12.659328791s | 1.3590909s | 90ms | 40.955626ms | 0.400 | 0.062 | 400ms | 1 | 20 | 6s | 0 | 0.031 | 86.67% | win |
| aggressive | transient_spike | 101 | 12.607221347s | 1.103546528s | 90ms | 40.938649ms | 0.398 | 0.056 | 200ms | 1 | 19 | 5.8s | 0 | 0.031 | 88.22% | win |
| aggressive | transient_spike | 131 | 12.512324827s | 1.461701521s | 90ms | 40.907068ms | 0.398 | 0.071 | 0s | 0 | 20 | 6s | 0 | 0.031 | 85.75% | win |
| aggressive | transient_spike | 173 | 12.652228169s | 1.424262714s | 90ms | 40.874622ms | 0.398 | 0.076 | 800ms | 2 | 20 | 6s | 0 | 0.031 | 86.04% | win |
| aggressive | stale_candidate | 11 | 46.403333381s | 7.590000008s | 110ms | 110ms | 0.889 | 0.211 | 9.333333324s | 9 | 32 | 14.333333319s | 0 | 0.029 | 83.86% | win |
| aggressive | stale_candidate | 29 | 52.933333379s | 8.92333334s | 110ms | 110ms | 0.883 | 0.240 | 9.333333324s | 8 | 32 | 14.333333319s | 0 | 0.029 | 83.92% | win |
| aggressive | stale_candidate | 47 | 51.540000046s | 8.590000007s | 110ms | 110ms | 0.883 | 0.205 | 9.333333324s | 8 | 32 | 14.333333319s | 0 | 0.029 | 84.37% | win |
| aggressive | stale_candidate | 71 | 52.600000048s | 7.590000008s | 110ms | 110ms | 0.889 | 0.234 | 9.333333324s | 10 | 32 | 14.333333319s | 0 | 0.029 | 85.67% | win |
| aggressive | stale_candidate | 101 | 49.070000047s | 27.496666695s | 110ms | 110ms | 0.883 | 0.550 | 9.333333324s | 8 | 89 | 34.999999965s | 0 | 0.058 | 48.21% | win |
| aggressive | stale_candidate | 131 | 47.40333338s | 8.590000007s | 110ms | 110ms | 0.883 | 0.211 | 9.333333324s | 8 | 32 | 14.333333319s | 0 | 0.029 | 82.81% | win |
| aggressive | stale_candidate | 173 | 46.87333338s | 32.420000037s | 110ms | 110ms | 0.883 | 0.673 | 9.333333324s | 9 | 112 | 43.33333329s | 0 | 0.070 | 34.33% | win |
| aggressive | recovery_no_flap | 11 | 44.799011002s | 8.955s | 95ms | 95ms | 0.752 | 0.113 | 0s | 0 | 58 | 14.2s | 0 | 0.028 | 83.31% | win |
| aggressive | recovery_no_flap | 29 | 44.937203373s | 1.364257656s | 95ms | 40.463509ms | 0.750 | 0.055 | 4.2s | 5 | 19 | 5.8s | 0 | 0.023 | 96.72% | win |
| aggressive | recovery_no_flap | 47 | 44.810457119s | 1.440621128s | 95ms | 40.825291ms | 0.757 | 0.073 | 4.2s | 5 | 23 | 6.6s | 0 | 0.023 | 96.45% | win |
| aggressive | recovery_no_flap | 71 | 44.958361864s | 1.066075324s | 95ms | 40.809664ms | 0.755 | 0.055 | 400ms | 1 | 19 | 5.8s | 0 | 0.023 | 97.14% | win |
| aggressive | recovery_no_flap | 101 | 44.837077225s | 1.327419454s | 95ms | 40.785436ms | 0.755 | 0.058 | 200ms | 1 | 19 | 5.8s | 0 | 0.023 | 96.75% | win |
| aggressive | recovery_no_flap | 131 | 44.901274352s | 479.033485ms | 95ms | 40.652801ms | 0.757 | 0.050 | 0s | 0 | 19 | 5.8s | 0 | 0.023 | 98.00% | win |
| aggressive | recovery_no_flap | 173 | 44.9015634s | 1.375425574s | 95ms | 40.704799ms | 0.755 | 0.070 | 800ms | 2 | 19 | 5.8s | 0 | 0.023 | 96.63% | win |
| aggressive | all_channels_bad | 11 | 2m10.9s | 54.4s | 100ms | 100ms | 0.669 | 0.671 | 0s | 0 | 300 | not detected | 0 | 0.053 | 57.27% | win |
| aggressive | all_channels_bad | 29 | 2m11.7s | 1m34.845s | 100ms | 85ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 0 | 0.031 | 27.85% | win |
| aggressive | all_channels_bad | 47 | 2m7.1s | 53.5s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.053 | 56.25% | win |
| aggressive | all_channels_bad | 71 | 2m9.6s | 53.61s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.053 | 57.09% | win |
| aggressive | all_channels_bad | 101 | 2m6.5s | 1m30.18s | 100ms | 85ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.031 | 28.72% | win |
| aggressive | all_channels_bad | 131 | 2m5.1s | 1m33.2s | 100ms | 85ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.031 | 24.83% | win |
| aggressive | all_channels_bad | 173 | 2m9.753516526s | 1m34.045s | 100ms | 85ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.031 | 27.32% | win |
| aggressive | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| aggressive | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| aggressive | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| aggressive | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| aggressive | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| aggressive | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| aggressive | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| aggressive | healthy_steady_state | 11 | 306.901147ms | 407.648716ms | 29.560991ms | 32.750776ms | 0.015 | 0.017 | not detected | 0 | 0 | not detected | 0 | 0.080 | -19.00% | regression |
| aggressive | healthy_steady_state | 29 | 306.449512ms | 404.875564ms | 29.524996ms | 33.100322ms | 0.015 | 0.015 | not detected | 0 | 0 | not detected | 0 | 0.080 | -18.72% | regression |
| aggressive | healthy_steady_state | 47 | 306.757116ms | 437.268377ms | 29.42026ms | 32.945425ms | 0.028 | 0.028 | not detected | 0 | 0 | not detected | 0 | 0.080 | -21.48% | regression |
| aggressive | healthy_steady_state | 71 | 304.971872ms | 396.689806ms | 29.463197ms | 33.187525ms | 0.018 | 0.018 | not detected | 0 | 0 | not detected | 0 | 0.080 | -18.24% | regression |
| aggressive | healthy_steady_state | 101 | 306.026121ms | 421.115833ms | 29.656679ms | 34.34952ms | 0.023 | 0.023 | not detected | 0 | 0 | not detected | 0 | 0.080 | -22.18% | regression |
| aggressive | healthy_steady_state | 131 | 307.461992ms | 417.259343ms | 29.32466ms | 34.941927ms | 0.022 | 0.022 | not detected | 0 | 0 | not detected | 0 | 0.080 | -23.76% | regression |
| aggressive | healthy_steady_state | 173 | 306.762211ms | 432.732635ms | 29.635213ms | 34.392347ms | 0.022 | 0.022 | not detected | 0 | 0 | not detected | 0 | 0.080 | -23.74% | regression |
| fast-low-probe | gradual_degradation | 11 | 2m18.05810847s | 635.816516ms | 111.638008ms | 40.389584ms | 0.738 | 0.023 | 400ms | 2 | 11 | 4s | 0 | 0.008 | 99.26% | win |
| fast-low-probe | gradual_degradation | 29 | 2m17.230127598s | 759.457226ms | 111.973508ms | 40.302296ms | 0.737 | 0.030 | 2.6s | 6 | 12 | 4.2s | 0 | 0.008 | 99.18% | win |
| fast-low-probe | gradual_degradation | 47 | 2m18.543146187s | 883.925008ms | 112.017461ms | 40.304224ms | 0.743 | 0.047 | 2.4s | 5 | 13 | 4.4s | 0 | 0.008 | 99.09% | win |
| fast-low-probe | gradual_degradation | 71 | 2m18.842157616s | 2.227372024s | 112.679273ms | 45.946326ms | 0.742 | 0.105 | 0s | 0 | 55 | 13s | 0 | 0.010 | 98.19% | win |
| fast-low-probe | gradual_degradation | 101 | 2m17.896601353s | 935.748055ms | 112.288179ms | 40.326863ms | 0.740 | 0.045 | 0s | 0 | 13 | 4.4s | 0 | 0.008 | 99.07% | win |
| fast-low-probe | gradual_degradation | 131 | 2m18.257709744s | 665.696629ms | 112.50077ms | 40.492487ms | 0.742 | 0.032 | 2.2s | 6 | 12 | 4.2s | 0 | 0.008 | 99.23% | win |
| fast-low-probe | gradual_degradation | 173 | 2m18.511286942s | 22.730946467s | 112.241073ms | 105.18757ms | 0.740 | 0.293 | 1.2s | 3 | 168 | 36.4s | 0 | 0.017 | 85.14% | win |
| fast-low-probe | sudden_outage | 11 | 15.6s | 477.877788ms | 29.758413ms | 40.036541ms | 0.669 | 0.038 | 200ms | 2 | 10 | 3.8s | 0 | 0.011 | 93.77% | win |
| fast-low-probe | sudden_outage | 29 | 15.6s | 477.005412ms | 29.586311ms | 39.871299ms | 0.667 | 0.040 | 2.2s | 5 | 10 | 3.8s | 0 | 0.011 | 93.74% | win |
| fast-low-probe | sudden_outage | 47 | 15.6s | 1.062780158s | 29.297237ms | 40.163736ms | 0.676 | 0.056 | 2.2s | 5 | 10 | 3.8s | 0 | 0.011 | 90.94% | win |
| fast-low-probe | sudden_outage | 71 | 15.6s | 478.860123ms | 29.362123ms | 39.983305ms | 0.673 | 0.047 | 0s | 0 | 10 | 3.8s | 0 | 0.011 | 93.64% | win |
| fast-low-probe | sudden_outage | 101 | 15.6s | 1.039136832s | 29.696734ms | 40.248819ms | 0.673 | 0.051 | 0s | 0 | 11 | 4s | 0 | 0.011 | 91.03% | win |
| fast-low-probe | sudden_outage | 131 | 15.6s | 477.864046ms | 29.458433ms | 40.270961ms | 0.676 | 0.038 | 2.2s | 5 | 10 | 3.8s | 0 | 0.011 | 93.76% | win |
| fast-low-probe | sudden_outage | 173 | 15.6s | 1.341113429s | 29.101676ms | 39.998469ms | 0.673 | 0.062 | 800ms | 2 | 10 | 3.8s | 0 | 0.011 | 89.62% | win |
| fast-low-probe | capacity_aggregation | 11 | 1m19.876239452s | 1m14.778295051s | 25ms | 25ms | 0.988 | 0.957 | 0s | 0 | 620 | not detected | 0 | 0.031 | 7.23% | neutral |
| fast-low-probe | capacity_aggregation | 29 | 1m19.971533361s | 1m14.795383989s | 25ms | 25ms | 0.988 | 0.957 | 0s | 0 | 620 | not detected | 0 | 0.031 | 7.28% | neutral |
| fast-low-probe | capacity_aggregation | 47 | 1m19.89581372s | 1m14.823676803s | 25ms | 25ms | 0.988 | 0.957 | 0s | 0 | 620 | not detected | 0 | 0.031 | 7.21% | neutral |
| fast-low-probe | capacity_aggregation | 71 | 1m19.767790605s | 1m14.658041714s | 25ms | 25ms | 0.988 | 0.957 | 0s | 0 | 620 | not detected | 0 | 0.031 | 7.26% | neutral |
| fast-low-probe | capacity_aggregation | 101 | 1m20.040921681s | 1m14.811831075s | 25ms | 25ms | 0.990 | 0.960 | 0s | 0 | 620 | not detected | 0 | 0.031 | 7.33% | neutral |
| fast-low-probe | capacity_aggregation | 131 | 1m19.734245977s | 1m14.690853956s | 25ms | 25ms | 0.988 | 0.957 | 0s | 0 | 620 | not detected | 0 | 0.031 | 7.19% | neutral |
| fast-low-probe | capacity_aggregation | 173 | 1m19.865475022s | 1m14.710734424s | 25ms | 25ms | 0.988 | 0.957 | 0s | 0 | 620 | not detected | 0 | 0.031 | 7.28% | neutral |
| fast-low-probe | transient_spike | 11 | 12.6925426s | 1.466972016s | 90ms | 40.91354ms | 0.396 | 0.060 | 200ms | 2 | 21 | 6s | 0 | 0.011 | 86.02% | win |
| fast-low-probe | transient_spike | 29 | 12.307633594s | 1.920996335s | 90ms | 90ms | 0.384 | 0.069 | 4.8s | 6 | 23 | 6.4s | 0 | 0.011 | 77.97% | win |
| fast-low-probe | transient_spike | 47 | 12.854235214s | 1.417704584s | 90ms | 40.846125ms | 0.402 | 0.080 | 4.4s | 5 | 21 | 6s | 0 | 0.011 | 86.10% | win |
| fast-low-probe | transient_spike | 71 | 12.659328791s | 10.369451158s | 90ms | 90ms | 0.400 | 0.133 | 0s | 0 | 174 | 12.8s | 1 | 0.013 | 26.56% | win |
| fast-low-probe | transient_spike | 101 | 12.607221347s | 1.419126917s | 90ms | 40.995495ms | 0.398 | 0.078 | 0s | 0 | 22 | 6.2s | 0 | 0.011 | 85.76% | win |
| fast-low-probe | transient_spike | 131 | 12.512324827s | 1.959281014s | 90ms | 90ms | 0.398 | 0.080 | 4.8s | 6 | 23 | 6.4s | 0 | 0.011 | 77.89% | win |
| fast-low-probe | transient_spike | 173 | 12.652228169s | 11.063425406s | 90ms | 90ms | 0.398 | 0.191 | 800ms | 2 | 133 | 16.2s | 1 | 0.016 | 19.48% | win |
| fast-low-probe | stale_candidate | 11 | 46.403333381s | 25.770000025s | 110ms | 110ms | 0.889 | 0.520 | 9.333333324s | 10 | 85 | 31.666666635s | 0 | 0.023 | 48.97% | win |
| fast-low-probe | stale_candidate | 29 | 52.933333379s | 28.163333359s | 110ms | 110ms | 0.883 | 0.503 | 9.333333324s | 8 | 84 | 31.666666635s | 0 | 0.023 | 51.29% | win |
| fast-low-probe | stale_candidate | 47 | 51.540000046s | 26.966666691s | 110ms | 110ms | 0.883 | 0.509 | 9.333333324s | 9 | 85 | 31.999999968s | 0 | 0.023 | 52.07% | win |
| fast-low-probe | stale_candidate | 71 | 52.600000048s | 26.633333358s | 110ms | 110ms | 0.889 | 0.509 | 9.666666657s | 10 | 84 | 31.666666635s | 0 | 0.023 | 53.05% | win |
| fast-low-probe | stale_candidate | 101 | 49.070000047s | 25.300000026s | 110ms | 110ms | 0.883 | 0.520 | 9.666666657s | 8 | 85 | 31.999999968s | 0 | 0.023 | 51.63% | win |
| fast-low-probe | stale_candidate | 131 | 47.40333338s | 25.496666693s | 110ms | 110ms | 0.883 | 0.509 | 9.333333324s | 9 | 84 | 31.666666635s | 0 | 0.023 | 50.83% | win |
| fast-low-probe | stale_candidate | 173 | 46.87333338s | 25.693333366s | 110ms | 110ms | 0.883 | 0.515 | 9.666666657s | 8 | 85 | 31.666666635s | 0 | 0.023 | 48.93% | win |
| fast-low-probe | recovery_no_flap | 11 | 44.799011002s | 478.960332ms | 95ms | 40.696441ms | 0.752 | 0.050 | 200ms | 2 | 21 | 6s | 0 | 0.008 | 97.95% | win |
| fast-low-probe | recovery_no_flap | 29 | 44.937203373s | 1.377540368s | 95ms | 40.476027ms | 0.750 | 0.058 | 4.4s | 5 | 21 | 6s | 0 | 0.008 | 96.65% | win |
| fast-low-probe | recovery_no_flap | 47 | 44.810457119s | 1.411640135s | 95ms | 40.824045ms | 0.757 | 0.070 | 4.4s | 5 | 21 | 6s | 0 | 0.008 | 96.53% | win |
| fast-low-probe | recovery_no_flap | 71 | 44.958361864s | 6.87s | 95ms | 95ms | 0.755 | 0.117 | 0s | 0 | 54 | 12.8s | 0 | 0.010 | 86.42% | win |
| fast-low-probe | recovery_no_flap | 101 | 44.837077225s | 1.354234951s | 95ms | 40.839377ms | 0.755 | 0.063 | 0s | 0 | 21 | 6s | 0 | 0.008 | 96.65% | win |
| fast-low-probe | recovery_no_flap | 131 | 44.901274352s | 1.014703488s | 95ms | 40.710898ms | 0.757 | 0.053 | 4.4s | 5 | 21 | 6s | 0 | 0.008 | 97.19% | win |
| fast-low-probe | recovery_no_flap | 173 | 44.9015634s | 11.64s | 95ms | 95ms | 0.755 | 0.147 | 800ms | 2 | 70 | 16.2s | 0 | 0.012 | 78.81% | win |
| fast-low-probe | all_channels_bad | 11 | 2m10.9s | 58.32s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 0 | 0.018 | 54.47% | win |
| fast-low-probe | all_channels_bad | 29 | 2m11.7s | 57.365s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 0 | 0.018 | 55.03% | win |
| fast-low-probe | all_channels_bad | 47 | 2m7.1s | 56.41s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.018 | 53.94% | win |
| fast-low-probe | all_channels_bad | 71 | 2m9.6s | 56.365s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.018 | 54.92% | win |
| fast-low-probe | all_channels_bad | 101 | 2m6.5s | 55.055s | 100ms | 100ms | 0.673 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.018 | 55.20% | win |
| fast-low-probe | all_channels_bad | 131 | 2m5.1s | 58.165s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.018 | 51.93% | win |
| fast-low-probe | all_channels_bad | 173 | 2m9.753516526s | 56.165s | 100ms | 100ms | 0.673 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.018 | 55.17% | win |
| fast-low-probe | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| fast-low-probe | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| fast-low-probe | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| fast-low-probe | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| fast-low-probe | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| fast-low-probe | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| fast-low-probe | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.667 | 10s | 1 | 8 | not detected | 0 | 0.000 | 0.00% | neutral |
| fast-low-probe | healthy_steady_state | 11 | 306.901147ms | 309.864745ms | 29.560991ms | 29.873254ms | 0.015 | 0.015 | not detected | 0 | 0 | not detected | 0 | 0.030 | -0.97% | neutral |
| fast-low-probe | healthy_steady_state | 29 | 306.449512ms | 309.587038ms | 29.524996ms | 29.812279ms | 0.015 | 0.015 | not detected | 0 | 0 | not detected | 0 | 0.030 | -0.94% | neutral |
| fast-low-probe | healthy_steady_state | 47 | 306.757116ms | 384.784114ms | 29.42026ms | 29.761595ms | 0.028 | 0.028 | not detected | 0 | 0 | not detected | 0 | 0.030 | -9.47% | regression |
| fast-low-probe | healthy_steady_state | 71 | 304.971872ms | 309.913902ms | 29.463197ms | 29.779027ms | 0.018 | 0.018 | not detected | 0 | 0 | not detected | 0 | 0.030 | -1.20% | neutral |
| fast-low-probe | healthy_steady_state | 101 | 306.026121ms | 320.996206ms | 29.656679ms | 29.816417ms | 0.023 | 0.025 | not detected | 0 | 0 | not detected | 0 | 0.030 | -2.73% | neutral |
| fast-low-probe | healthy_steady_state | 131 | 307.461992ms | 335.663555ms | 29.32466ms | 29.677918ms | 0.022 | 0.022 | not detected | 0 | 0 | not detected | 0 | 0.030 | -3.95% | neutral |
| fast-low-probe | healthy_steady_state | 173 | 306.762211ms | 309.43963ms | 29.635213ms | 29.854132ms | 0.022 | 0.025 | not detected | 0 | 0 | not detected | 0 | 0.030 | -2.10% | neutral |
| short-window | gradual_degradation | 11 | 2m18.05810847s | 722.582878ms | 111.638008ms | 40.730137ms | 0.738 | 0.035 | 2.2s | 6 | 22 | 6.2s | 0 | 0.017 | 99.19% | win |
| short-window | gradual_degradation | 29 | 2m17.230127598s | 759.457226ms | 111.973508ms | 40.322263ms | 0.737 | 0.030 | 2.4s | 5 | 13 | 4.2s | 0 | 0.017 | 99.18% | win |
| short-window | gradual_degradation | 47 | 2m18.543146187s | 822.599885ms | 112.017461ms | 40.247034ms | 0.743 | 0.042 | 2.2s | 5 | 12 | 4s | 0 | 0.017 | 99.14% | win |
| short-window | gradual_degradation | 71 | 2m18.842157616s | 673.700086ms | 112.679273ms | 40.666681ms | 0.742 | 0.033 | 2.4s | 7 | 12 | 4s | 0 | 0.017 | 99.23% | win |
| short-window | gradual_degradation | 101 | 2m17.896601353s | 814.84125ms | 112.288179ms | 40.326863ms | 0.740 | 0.037 | 2.2s | 7 | 11 | 4s | 0 | 0.017 | 99.14% | win |
| short-window | gradual_degradation | 131 | 2m18.257709744s | 665.66754ms | 112.50077ms | 40.537649ms | 0.742 | 0.028 | 2.2s | 7 | 11 | 4s | 0 | 0.017 | 99.24% | win |
| short-window | gradual_degradation | 173 | 2m18.511286942s | 2.014025397s | 112.241073ms | 44.473ms | 0.740 | 0.113 | 1.4s | 4 | 63 | 14.8s | 0 | 0.022 | 98.30% | win |
| short-window | sudden_outage | 11 | 15.6s | 477.968524ms | 29.758413ms | 39.982664ms | 0.669 | 0.036 | 2s | 5 | 9 | 3.4s | 0 | 0.022 | 93.87% | win |
| short-window | sudden_outage | 29 | 15.6s | 476.763595ms | 29.586311ms | 39.808762ms | 0.667 | 0.038 | 2s | 5 | 9 | 3.4s | 0 | 0.022 | 93.85% | win |
| short-window | sudden_outage | 47 | 15.6s | 1.332884407s | 29.297237ms | 39.891769ms | 0.676 | 0.060 | 2.4s | 7 | 11 | 4s | 0 | 0.022 | 89.64% | win |
| short-window | sudden_outage | 71 | 15.6s | 479.737793ms | 29.362123ms | 40.255287ms | 0.673 | 0.044 | 2s | 4 | 9 | 3.4s | 0 | 0.022 | 93.72% | win |
| short-window | sudden_outage | 101 | 15.6s | 477.265495ms | 29.696734ms | 40.202318ms | 0.673 | 0.036 | 2s | 4 | 9 | 3.4s | 0 | 0.022 | 93.86% | win |
| short-window | sudden_outage | 131 | 15.6s | 1.386585722s | 29.458433ms | 40.354446ms | 0.676 | 0.060 | 2s | 5 | 18 | 3.4s | 2 | 0.022 | 88.92% | win |
| short-window | sudden_outage | 173 | 15.6s | 478.373374ms | 29.101676ms | 40.07886ms | 0.673 | 0.047 | 1.2s | 3 | 9 | 3.4s | 0 | 0.022 | 93.70% | win |
| short-window | capacity_aggregation | 11 | 1m19.876239452s | 7.330567273s | 25ms | 28ms | 0.988 | 0.863 | 0s | 0 | 284 | 1.125s | 8 | 0.006 | 89.52% | win |
| short-window | capacity_aggregation | 29 | 1m19.971533361s | 6.294367405s | 25ms | 28ms | 0.988 | 0.847 | 0s | 0 | 277 | 1.125s | 9 | 0.006 | 90.70% | win |
| short-window | capacity_aggregation | 47 | 1m19.89581372s | 6.315259432s | 25ms | 28ms | 0.988 | 0.839 | 0s | 0 | 280 | 1.125s | 9 | 0.006 | 90.71% | win |
| short-window | capacity_aggregation | 71 | 1m19.767790605s | 6.211246974s | 25ms | 28ms | 0.988 | 0.844 | 0s | 0 | 287 | 1.125s | 9 | 0.006 | 90.81% | win |
| short-window | capacity_aggregation | 101 | 1m20.040921681s | 7.429708966s | 25ms | 28ms | 0.990 | 0.878 | 0s | 0 | 285 | 1.125s | 7 | 0.006 | 89.31% | win |
| short-window | capacity_aggregation | 131 | 1m19.734245977s | 6.40461911s | 25ms | 28ms | 0.988 | 0.840 | 0s | 0 | 276 | 1.125s | 9 | 0.006 | 90.58% | win |
| short-window | capacity_aggregation | 173 | 1m19.865475022s | 6.35882459s | 25ms | 28ms | 0.988 | 0.840 | 0s | 0 | 283 | 1.125s | 9 | 0.006 | 90.66% | win |
| short-window | transient_spike | 11 | 12.6925426s | 1.466972016s | 90ms | 40.91354ms | 0.396 | 0.060 | 4.4s | 5 | 21 | 6s | 0 | 0.022 | 86.04% | win |
| short-window | transient_spike | 29 | 12.307633594s | 1.357105892s | 90ms | 40.772027ms | 0.384 | 0.058 | 4.4s | 5 | 21 | 6s | 0 | 0.022 | 86.31% | win |
| short-window | transient_spike | 47 | 12.854235214s | 1.474251698s | 90ms | 40.978182ms | 0.402 | 0.082 | 4.6s | 6 | 22 | 6.2s | 0 | 0.022 | 85.68% | win |
| short-window | transient_spike | 71 | 12.659328791s | 1.463118736s | 90ms | 40.958353ms | 0.400 | 0.076 | 4.4s | 4 | 21 | 6s | 0 | 0.022 | 85.78% | win |
| short-window | transient_spike | 101 | 12.607221347s | 1.440466991s | 90ms | 40.937172ms | 0.398 | 0.071 | 4.2s | 4 | 21 | 6s | 0 | 0.022 | 85.89% | win |
| short-window | transient_spike | 131 | 12.512324827s | 1.466285777s | 90ms | 40.948585ms | 0.398 | 0.082 | 4.4s | 5 | 21 | 6s | 0 | 0.022 | 85.43% | win |
| short-window | transient_spike | 173 | 12.652228169s | 10.555543752s | 90ms | 90ms | 0.398 | 0.198 | 1.2s | 3 | 191 | 18s | 1 | 0.031 | 21.90% | win |
| short-window | stale_candidate | 11 | 46.403333381s | 42.736666714s | 110ms | 110ms | 0.889 | 0.836 | 9.666666657s | 9 | 142 | not detected | 0 | 0.058 | 8.24% | neutral |
| short-window | stale_candidate | 29 | 52.933333379s | 45.873333379s | 110ms | 110ms | 0.883 | 0.830 | 9.666666657s | 9 | 142 | not detected | 0 | 0.058 | 14.22% | neutral |
| short-window | stale_candidate | 47 | 51.540000046s | 44.736666712s | 110ms | 110ms | 0.883 | 0.830 | 9.333333324s | 9 | 142 | not detected | 0 | 0.058 | 14.19% | neutral |
| short-window | stale_candidate | 71 | 52.600000048s | 45.480000047s | 110ms | 110ms | 0.889 | 0.836 | 9.666666657s | 10 | 142 | not detected | 0 | 0.058 | 14.39% | neutral |
| short-window | stale_candidate | 101 | 49.070000047s | 43.343333379s | 110ms | 110ms | 0.883 | 0.830 | 9.666666657s | 8 | 142 | not detected | 0 | 0.058 | 12.32% | neutral |
| short-window | stale_candidate | 131 | 47.40333338s | 42.540000047s | 110ms | 110ms | 0.883 | 0.830 | 9.666666657s | 10 | 142 | not detected | 0 | 0.058 | 11.12% | neutral |
| short-window | stale_candidate | 173 | 46.87333338s | 42.146666713s | 110ms | 110ms | 0.883 | 0.830 | 9.666666657s | 9 | 142 | not detected | 0 | 0.058 | 10.99% | neutral |
| short-window | recovery_no_flap | 11 | 44.799011002s | 479.252397ms | 95ms | 40.696441ms | 0.752 | 0.050 | 4.2s | 5 | 21 | 6s | 0 | 0.017 | 97.96% | win |
| short-window | recovery_no_flap | 29 | 44.937203373s | 1.382915563s | 95ms | 40.530234ms | 0.750 | 0.057 | 4.2s | 5 | 20 | 5.8s | 0 | 0.017 | 96.67% | win |
| short-window | recovery_no_flap | 47 | 44.810457119s | 1.405185701s | 95ms | 40.584314ms | 0.757 | 0.068 | 4.2s | 5 | 20 | 5.8s | 0 | 0.017 | 96.57% | win |
| short-window | recovery_no_flap | 71 | 44.958361864s | 1.352093515s | 95ms | 40.851936ms | 0.755 | 0.062 | 4.2s | 4 | 20 | 5.8s | 0 | 0.017 | 96.69% | win |
| short-window | recovery_no_flap | 101 | 44.837077225s | 1.354234951s | 95ms | 40.839377ms | 0.755 | 0.062 | 4.2s | 4 | 21 | 6s | 0 | 0.017 | 96.66% | win |
| short-window | recovery_no_flap | 131 | 44.901274352s | 1.340493203s | 95ms | 40.842365ms | 0.757 | 0.062 | 4.2s | 5 | 20 | 5.8s | 0 | 0.017 | 96.70% | win |
| short-window | recovery_no_flap | 173 | 44.9015634s | 1.378399892s | 95ms | 40.73532ms | 0.755 | 0.073 | 1.2s | 3 | 21 | 6s | 0 | 0.017 | 96.58% | win |
| short-window | all_channels_bad | 11 | 2m10.9s | 1m8.74s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 0 | 0.033 | 47.58% | win |
| short-window | all_channels_bad | 29 | 2m11.7s | 1m14.005s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 0 | 0.031 | 44.10% | win |
| short-window | all_channels_bad | 47 | 2m7.1s | 1m29.335s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.022 | 29.01% | win |
| short-window | all_channels_bad | 71 | 2m9.6s | 1m30.18s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.022 | 30.19% | win |
| short-window | all_channels_bad | 101 | 2m6.5s | 1m12.095s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.031 | 43.46% | win |
| short-window | all_channels_bad | 131 | 2m5.1s | 1m29.89s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.022 | 27.53% | win |
| short-window | all_channels_bad | 173 | 2m9.753516526s | 1m14.805s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.031 | 42.82% | win |
| short-window | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | not detected | 0 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| short-window | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | not detected | 0 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| short-window | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | not detected | 0 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| short-window | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | not detected | 0 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| short-window | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | not detected | 0 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| short-window | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | not detected | 0 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| short-window | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | not detected | 0 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| short-window | healthy_steady_state | 11 | 306.901147ms | 381.578745ms | 29.560991ms | 30.748562ms | 0.015 | 0.015 | not detected | 0 | 0 | not detected | 0 | 0.060 | -11.17% | regression |
| short-window | healthy_steady_state | 29 | 306.449512ms | 381.469428ms | 29.524996ms | 31.157905ms | 0.015 | 0.017 | not detected | 0 | 0 | not detected | 0 | 0.060 | -12.78% | regression |
| short-window | healthy_steady_state | 47 | 306.757116ms | 444.552055ms | 29.42026ms | 30.273464ms | 0.028 | 0.030 | not detected | 0 | 0 | not detected | 0 | 0.060 | -17.97% | regression |
| short-window | healthy_steady_state | 71 | 304.971872ms | 376.322213ms | 29.463197ms | 31.229065ms | 0.018 | 0.018 | not detected | 0 | 0 | not detected | 0 | 0.060 | -11.83% | regression |
| short-window | healthy_steady_state | 101 | 306.026121ms | 378.530084ms | 29.656679ms | 30.52698ms | 0.023 | 0.023 | not detected | 0 | 0 | not detected | 0 | 0.060 | -10.05% | regression |
| short-window | healthy_steady_state | 131 | 307.461992ms | 384.485697ms | 29.32466ms | 31.116071ms | 0.022 | 0.022 | not detected | 0 | 0 | not detected | 0 | 0.060 | -12.40% | regression |
| short-window | healthy_steady_state | 173 | 306.762211ms | 398.355915ms | 29.635213ms | 31.324661ms | 0.022 | 0.023 | not detected | 0 | 0 | not detected | 0 | 0.060 | -14.47% | regression |
| stable-recovery | gradual_degradation | 11 | 2m18.05810847s | 654.958152ms | 111.638008ms | 40.491731ms | 0.738 | 0.027 | 2.4s | 7 | 16 | 4.6s | 0 | 0.013 | 99.25% | win |
| stable-recovery | gradual_degradation | 29 | 2m17.230127598s | 776.144845ms | 111.973508ms | 40.322263ms | 0.737 | 0.032 | 2.6s | 6 | 14 | 4.4s | 0 | 0.013 | 99.17% | win |
| stable-recovery | gradual_degradation | 47 | 2m18.543146187s | 883.925008ms | 112.017461ms | 40.306719ms | 0.743 | 0.047 | 2.4s | 5 | 14 | 4.4s | 0 | 0.013 | 99.09% | win |
| stable-recovery | gradual_degradation | 71 | 2m18.842157616s | 676.163776ms | 112.679273ms | 40.666681ms | 0.742 | 0.035 | 2.4s | 6 | 15 | 4.2s | 0 | 0.013 | 99.22% | win |
| stable-recovery | gradual_degradation | 101 | 2m17.896601353s | 894.737117ms | 112.288179ms | 40.336063ms | 0.740 | 0.043 | 2.2s | 6 | 14 | 4.4s | 0 | 0.013 | 99.09% | win |
| stable-recovery | gradual_degradation | 131 | 2m18.257709744s | 678.23531ms | 112.50077ms | 40.564176ms | 0.742 | 0.033 | 2.2s | 6 | 14 | 4.2s | 0 | 0.013 | 99.22% | win |
| stable-recovery | gradual_degradation | 173 | 2m18.511286942s | 890.975405ms | 112.241073ms | 40.555345ms | 0.740 | 0.042 | 1.4s | 4 | 13 | 4.2s | 0 | 0.013 | 99.10% | win |
| stable-recovery | sudden_outage | 11 | 15.6s | 477.877788ms | 29.758413ms | 40.036541ms | 0.669 | 0.038 | 2.2s | 5 | 10 | 3.8s | 0 | 0.018 | 93.77% | win |
| stable-recovery | sudden_outage | 29 | 15.6s | 477.005412ms | 29.586311ms | 39.871299ms | 0.667 | 0.040 | 2.2s | 5 | 10 | 3.8s | 0 | 0.018 | 93.74% | win |
| stable-recovery | sudden_outage | 47 | 15.6s | 1.062780158s | 29.297237ms | 40.199194ms | 0.676 | 0.056 | 2.2s | 5 | 10 | 3.8s | 0 | 0.018 | 90.93% | win |
| stable-recovery | sudden_outage | 71 | 15.6s | 478.860123ms | 29.362123ms | 39.983305ms | 0.673 | 0.047 | 2.2s | 4 | 10 | 3.8s | 0 | 0.018 | 93.64% | win |
| stable-recovery | sudden_outage | 101 | 15.6s | 479.387647ms | 29.696734ms | 40.248819ms | 0.673 | 0.049 | 2.4s | 5 | 11 | 4s | 0 | 0.018 | 93.52% | win |
| stable-recovery | sudden_outage | 131 | 15.6s | 478.127474ms | 29.458433ms | 40.293239ms | 0.676 | 0.040 | 2.2s | 5 | 10 | 3.8s | 0 | 0.018 | 93.73% | win |
| stable-recovery | sudden_outage | 173 | 15.6s | 1.332870167s | 29.101676ms | 40.048681ms | 0.673 | 0.060 | 1.2s | 3 | 10 | 3.8s | 0 | 0.018 | 89.69% | win |
| stable-recovery | capacity_aggregation | 11 | 1m19.876239452s | 11.563571524s | 25ms | 28ms | 0.988 | 0.908 | 0s | 0 | 266 | 3.25s | 4 | 0.007 | 84.18% | win |
| stable-recovery | capacity_aggregation | 29 | 1m19.971533361s | 11.618700883s | 25ms | 28ms | 0.988 | 0.906 | 0s | 0 | 266 | 3.125s | 4 | 0.007 | 84.18% | win |
| stable-recovery | capacity_aggregation | 47 | 1m19.89581372s | 11.58629394s | 25ms | 28ms | 0.988 | 0.907 | 0s | 0 | 264 | 3.25s | 4 | 0.007 | 84.20% | win |
| stable-recovery | capacity_aggregation | 71 | 1m19.767790605s | 11.551655032s | 25ms | 28ms | 0.988 | 0.906 | 0s | 0 | 267 | 3.25s | 4 | 0.007 | 84.19% | win |
| stable-recovery | capacity_aggregation | 101 | 1m20.040921681s | 11.490471015s | 25ms | 28ms | 0.990 | 0.914 | 0s | 0 | 265 | 3.25s | 4 | 0.007 | 84.27% | win |
| stable-recovery | capacity_aggregation | 131 | 1m19.734245977s | 11.610125239s | 25ms | 28ms | 0.988 | 0.908 | 0s | 0 | 268 | 3.25s | 4 | 0.007 | 84.10% | win |
| stable-recovery | capacity_aggregation | 173 | 1m19.865475022s | 11.605976302s | 25ms | 28ms | 0.988 | 0.908 | 0s | 0 | 266 | 3.125s | 4 | 0.007 | 84.15% | win |
| stable-recovery | transient_spike | 11 | 12.6925426s | 1.466972016s | 90ms | 40.91354ms | 0.396 | 0.060 | 4.4s | 5 | 22 | 6.2s | 0 | 0.020 | 86.04% | win |
| stable-recovery | transient_spike | 29 | 12.307633594s | 1.920996335s | 90ms | 90ms | 0.384 | 0.069 | 4.8s | 6 | 24 | 6.6s | 0 | 0.020 | 77.99% | win |
| stable-recovery | transient_spike | 47 | 12.854235214s | 1.417704584s | 90ms | 40.846125ms | 0.402 | 0.080 | 4.4s | 5 | 22 | 6.2s | 0 | 0.020 | 86.11% | win |
| stable-recovery | transient_spike | 71 | 12.659328791s | 1.46716421s | 90ms | 40.987696ms | 0.400 | 0.073 | 4.6s | 4 | 23 | 6.4s | 0 | 0.020 | 85.65% | win |
| stable-recovery | transient_spike | 101 | 12.607221347s | 1.419126917s | 90ms | 40.995495ms | 0.398 | 0.076 | 4.6s | 4 | 23 | 6.4s | 0 | 0.020 | 85.82% | win |
| stable-recovery | transient_spike | 131 | 12.512324827s | 1.959281014s | 90ms | 90ms | 0.398 | 0.082 | 4.8s | 6 | 24 | 6.6s | 0 | 0.020 | 77.87% | win |
| stable-recovery | transient_spike | 173 | 12.652228169s | 1.450459768s | 90ms | 40.951473ms | 0.398 | 0.087 | 1.2s | 3 | 23 | 6.4s | 0 | 0.020 | 85.49% | win |
| stable-recovery | stale_candidate | 11 | 46.403333381s | 27.103333361s | 110ms | 110ms | 0.889 | 0.550 | 9.99999999s | 12 | 91 | 34.666666632s | 0 | 0.035 | 45.67% | win |
| stable-recovery | stale_candidate | 29 | 52.933333379s | 30.026666695s | 110ms | 110ms | 0.883 | 0.544 | 9.666666657s | 9 | 90 | 34.333333299s | 0 | 0.035 | 47.40% | win |
| stable-recovery | stale_candidate | 47 | 51.540000046s | 28.496666694s | 110ms | 110ms | 0.883 | 0.526 | 9.99999999s | 10 | 90 | 34.333333299s | 0 | 0.035 | 49.26% | win |
| stable-recovery | stale_candidate | 71 | 52.600000048s | 35.223333367s | 110ms | 110ms | 0.889 | 0.667 | 10.333333323s | 11 | 109 | 40.999999959s | 0 | 0.041 | 36.24% | win |
| stable-recovery | stale_candidate | 101 | 49.070000047s | 27.830000028s | 110ms | 110ms | 0.883 | 0.532 | 9.99999999s | 9 | 90 | 34.333333299s | 0 | 0.035 | 47.15% | win |
| stable-recovery | stale_candidate | 131 | 47.40333338s | 25.966666694s | 110ms | 110ms | 0.883 | 0.538 | 9.666666657s | 10 | 90 | 34.333333299s | 0 | 0.035 | 49.05% | win |
| stable-recovery | stale_candidate | 173 | 46.87333338s | 26.300000027s | 110ms | 110ms | 0.883 | 0.544 | 9.99999999s | 10 | 90 | 34.333333299s | 0 | 0.035 | 47.81% | win |
| stable-recovery | recovery_no_flap | 11 | 44.799011002s | 1.053901177s | 95ms | 40.762979ms | 0.752 | 0.052 | 4.4s | 5 | 22 | 6.2s | 0 | 0.015 | 97.13% | win |
| stable-recovery | recovery_no_flap | 29 | 44.937203373s | 1.3901921s | 95ms | 40.635981ms | 0.750 | 0.062 | 4.4s | 5 | 22 | 6.2s | 0 | 0.015 | 96.61% | win |
| stable-recovery | recovery_no_flap | 47 | 44.810457119s | 1.426974106s | 95ms | 40.648906ms | 0.757 | 0.075 | 4.4s | 5 | 22 | 6.2s | 0 | 0.015 | 96.49% | win |
| stable-recovery | recovery_no_flap | 71 | 44.958361864s | 1.3590909s | 95ms | 40.858579ms | 0.755 | 0.065 | 4.4s | 4 | 22 | 6.2s | 0 | 0.015 | 96.64% | win |
| stable-recovery | recovery_no_flap | 101 | 44.837077225s | 1.397312725s | 95ms | 40.643002ms | 0.755 | 0.070 | 4.4s | 4 | 22 | 6.2s | 0 | 0.015 | 96.56% | win |
| stable-recovery | recovery_no_flap | 131 | 44.901274352s | 1.377752802s | 95ms | 40.846107ms | 0.757 | 0.065 | 4.4s | 5 | 22 | 6.2s | 0 | 0.015 | 96.61% | win |
| stable-recovery | recovery_no_flap | 173 | 44.9015634s | 1.398788593s | 95ms | 40.741428ms | 0.755 | 0.067 | 1.2s | 3 | 22 | 6.2s | 0 | 0.015 | 96.58% | win |
| stable-recovery | all_channels_bad | 11 | 2m10.9s | 1m25.5s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 0 | 0.040 | 36.79% | win |
| stable-recovery | all_channels_bad | 29 | 2m11.7s | 1m27.1s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 0 | 0.040 | 35.72% | win |
| stable-recovery | all_channels_bad | 47 | 2m7.1s | 1m32s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.042 | 29.64% | win |
| stable-recovery | all_channels_bad | 71 | 2m9.6s | 1m34s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.042 | 29.60% | win |
| stable-recovery | all_channels_bad | 101 | 2m6.5s | 1m30.6s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.042 | 30.55% | win |
| stable-recovery | all_channels_bad | 131 | 2m5.1s | 1m30.9s | 100ms | 100ms | 0.676 | 0.678 | 0s | 0 | 300 | not detected | 0 | 0.042 | 29.44% | win |
| stable-recovery | all_channels_bad | 173 | 2m9.753516526s | 1m34.9s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.042 | 29.13% | win |
| stable-recovery | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| stable-recovery | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| stable-recovery | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| stable-recovery | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| stable-recovery | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| stable-recovery | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| stable-recovery | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| stable-recovery | healthy_steady_state | 11 | 306.901147ms | 363.70768ms | 29.560991ms | 29.991747ms | 0.015 | 0.015 | not detected | 0 | 0 | not detected | 0 | 0.050 | -7.57% | neutral |
| stable-recovery | healthy_steady_state | 29 | 306.449512ms | 359.462254ms | 29.524996ms | 29.953468ms | 0.015 | 0.017 | not detected | 0 | 0 | not detected | 0 | 0.050 | -7.88% | neutral |
| stable-recovery | healthy_steady_state | 47 | 306.757116ms | 425.449182ms | 29.42026ms | 29.977762ms | 0.028 | 0.028 | not detected | 0 | 0 | not detected | 0 | 0.050 | -14.48% | regression |
| stable-recovery | healthy_steady_state | 71 | 304.971872ms | 370.046734ms | 29.463197ms | 29.915952ms | 0.018 | 0.018 | not detected | 0 | 0 | not detected | 0 | 0.050 | -8.64% | regression |
| stable-recovery | healthy_steady_state | 101 | 306.026121ms | 406.359393ms | 29.656679ms | 29.974328ms | 0.023 | 0.025 | not detected | 0 | 0 | not detected | 0 | 0.050 | -12.77% | regression |
| stable-recovery | healthy_steady_state | 131 | 307.461992ms | 406.434429ms | 29.32466ms | 29.9471ms | 0.022 | 0.023 | not detected | 0 | 0 | not detected | 0 | 0.050 | -13.38% | regression |
| stable-recovery | healthy_steady_state | 173 | 306.762211ms | 371.181352ms | 29.635213ms | 29.993773ms | 0.022 | 0.022 | not detected | 0 | 0 | not detected | 0 | 0.050 | -8.18% | regression |
| fast-recovery | gradual_degradation | 11 | 2m18.05810847s | 648.349873ms | 111.638008ms | 40.45184ms | 0.738 | 0.022 | 2.2s | 6 | 12 | 4s | 0 | 0.017 | 99.26% | win |
| fast-recovery | gradual_degradation | 29 | 2m17.230127598s | 759.457226ms | 111.973508ms | 40.322263ms | 0.737 | 0.030 | 2.4s | 5 | 13 | 4.2s | 0 | 0.017 | 99.18% | win |
| fast-recovery | gradual_degradation | 47 | 2m18.543146187s | 822.599885ms | 112.017461ms | 40.247034ms | 0.743 | 0.042 | 2.2s | 5 | 12 | 4s | 0 | 0.017 | 99.14% | win |
| fast-recovery | gradual_degradation | 71 | 2m18.842157616s | 1.18842005s | 112.679273ms | 40.955626ms | 0.742 | 0.067 | 2.4s | 7 | 31 | 8.2s | 0 | 0.018 | 98.89% | win |
| fast-recovery | gradual_degradation | 101 | 2m17.896601353s | 814.84125ms | 112.288179ms | 40.326863ms | 0.740 | 0.037 | 2.2s | 7 | 11 | 4s | 0 | 0.017 | 99.14% | win |
| fast-recovery | gradual_degradation | 131 | 2m18.257709744s | 665.66754ms | 112.50077ms | 40.537649ms | 0.742 | 0.028 | 2.2s | 7 | 11 | 4s | 0 | 0.017 | 99.24% | win |
| fast-recovery | gradual_degradation | 173 | 2m18.511286942s | 2.014025397s | 112.241073ms | 44.473ms | 0.740 | 0.113 | 1.4s | 4 | 63 | 14.8s | 0 | 0.022 | 98.30% | win |
| fast-recovery | sudden_outage | 11 | 15.6s | 477.968524ms | 29.758413ms | 39.982664ms | 0.669 | 0.036 | 2s | 5 | 9 | 3.4s | 0 | 0.022 | 93.87% | win |
| fast-recovery | sudden_outage | 29 | 15.6s | 476.763595ms | 29.586311ms | 39.808762ms | 0.667 | 0.038 | 2s | 5 | 9 | 3.4s | 0 | 0.022 | 93.85% | win |
| fast-recovery | sudden_outage | 47 | 15.6s | 1.332884407s | 29.297237ms | 39.891769ms | 0.676 | 0.060 | 2.4s | 7 | 11 | 4s | 0 | 0.022 | 89.64% | win |
| fast-recovery | sudden_outage | 71 | 15.6s | 479.737793ms | 29.362123ms | 40.255287ms | 0.673 | 0.044 | 2s | 4 | 9 | 3.4s | 0 | 0.022 | 93.72% | win |
| fast-recovery | sudden_outage | 101 | 15.6s | 477.265495ms | 29.696734ms | 40.202318ms | 0.673 | 0.036 | 2s | 4 | 9 | 3.4s | 0 | 0.022 | 93.86% | win |
| fast-recovery | sudden_outage | 131 | 15.6s | 1.386585722s | 29.458433ms | 40.324396ms | 0.676 | 0.062 | 2s | 5 | 18 | 3.4s | 2 | 0.022 | 88.89% | win |
| fast-recovery | sudden_outage | 173 | 15.6s | 478.373374ms | 29.101676ms | 40.07886ms | 0.673 | 0.047 | 1.2s | 3 | 9 | 3.4s | 0 | 0.022 | 93.70% | win |
| fast-recovery | capacity_aggregation | 11 | 1m19.876239452s | 29.146310126s | 25ms | 28ms | 0.988 | 0.942 | 0s | 0 | 291 | 1.125s | 2 | 0.006 | 65.16% | win |
| fast-recovery | capacity_aggregation | 29 | 1m19.971533361s | 29.931097787s | 25ms | 28ms | 0.988 | 0.940 | 0s | 0 | 314 | 1.125s | 2 | 0.006 | 64.88% | win |
| fast-recovery | capacity_aggregation | 47 | 1m19.89581372s | 29.244526898s | 25ms | 28ms | 0.988 | 0.940 | 0s | 0 | 310 | 1.125s | 2 | 0.006 | 65.47% | win |
| fast-recovery | capacity_aggregation | 71 | 1m19.767790605s | 29.096476904s | 25ms | 28ms | 0.988 | 0.939 | 0s | 0 | 291 | 1.125s | 2 | 0.006 | 65.21% | win |
| fast-recovery | capacity_aggregation | 101 | 1m20.040921681s | 29.812563477s | 25ms | 28ms | 0.990 | 0.944 | 0s | 0 | 314 | 1.125s | 2 | 0.006 | 65.04% | win |
| fast-recovery | capacity_aggregation | 131 | 1m19.734245977s | 29.698943803s | 25ms | 28ms | 0.988 | 0.943 | 0s | 0 | 314 | 1.125s | 2 | 0.006 | 64.96% | win |
| fast-recovery | capacity_aggregation | 173 | 1m19.865475022s | 29.731350675s | 25ms | 28ms | 0.988 | 0.940 | 0s | 0 | 316 | 1.125s | 2 | 0.006 | 65.02% | win |
| fast-recovery | transient_spike | 11 | 12.6925426s | 2.463070483s | 90ms | 90ms | 0.396 | 0.080 | 4.4s | 5 | 30 | 7.8s | 0 | 0.024 | 74.77% | win |
| fast-recovery | transient_spike | 29 | 12.307633594s | 1.357105892s | 90ms | 40.772027ms | 0.384 | 0.058 | 4.4s | 5 | 21 | 6s | 0 | 0.022 | 86.31% | win |
| fast-recovery | transient_spike | 47 | 12.854235214s | 1.474251698s | 90ms | 40.978182ms | 0.402 | 0.082 | 4.6s | 6 | 22 | 6.2s | 0 | 0.022 | 85.68% | win |
| fast-recovery | transient_spike | 71 | 12.659328791s | 2.462867219s | 90ms | 90ms | 0.400 | 0.093 | 4.4s | 4 | 29 | 7.8s | 0 | 0.024 | 74.57% | win |
| fast-recovery | transient_spike | 101 | 12.607221347s | 1.472372969s | 90ms | 40.697543ms | 0.398 | 0.076 | 4.2s | 4 | 20 | 5.8s | 0 | 0.022 | 85.73% | win |
| fast-recovery | transient_spike | 131 | 12.512324827s | 1.466285777s | 90ms | 40.948585ms | 0.398 | 0.082 | 4.4s | 5 | 21 | 6s | 0 | 0.022 | 85.43% | win |
| fast-recovery | transient_spike | 173 | 12.652228169s | 10.555543752s | 90ms | 90ms | 0.398 | 0.191 | 1.2s | 3 | 223 | 18s | 1 | 0.031 | 22.06% | win |
| fast-recovery | stale_candidate | 11 | 46.403333381s | 17.04333335s | 110ms | 110ms | 0.889 | 0.363 | 9.666666657s | 9 | 59 | 23.666666643s | 0 | 0.029 | 66.46% | win |
| fast-recovery | stale_candidate | 29 | 52.933333379s | 19.04333335s | 110ms | 110ms | 0.883 | 0.351 | 9.666666657s | 9 | 58 | 23.33333331s | 0 | 0.029 | 67.91% | win |
| fast-recovery | stale_candidate | 47 | 51.540000046s | 18.043333351s | 110ms | 110ms | 0.883 | 0.374 | 9.333333324s | 9 | 60 | 23.666666643s | 0 | 0.029 | 68.57% | win |
| fast-recovery | stale_candidate | 71 | 52.600000048s | 17.04333335s | 110ms | 110ms | 0.889 | 0.351 | 9.666666657s | 10 | 58 | 23.33333331s | 0 | 0.029 | 70.44% | win |
| fast-recovery | stale_candidate | 101 | 49.070000047s | 16.710000017s | 110ms | 110ms | 0.883 | 0.368 | 9.666666657s | 8 | 58 | 23.33333331s | 0 | 0.029 | 68.89% | win |
| fast-recovery | stale_candidate | 131 | 47.40333338s | 16.710000017s | 110ms | 110ms | 0.883 | 0.351 | 9.666666657s | 10 | 58 | 23.33333331s | 0 | 0.029 | 68.15% | win |
| fast-recovery | stale_candidate | 173 | 46.87333338s | 15.650000017s | 110ms | 110ms | 0.883 | 0.351 | 9.666666657s | 9 | 58 | 23.33333331s | 0 | 0.029 | 69.20% | win |
| fast-recovery | recovery_no_flap | 11 | 44.799011002s | 1.459856285s | 95ms | 40.891058ms | 0.752 | 0.063 | 4.2s | 5 | 29 | 7.8s | 0 | 0.018 | 96.36% | win |
| fast-recovery | recovery_no_flap | 29 | 44.937203373s | 1.382915563s | 95ms | 40.530234ms | 0.750 | 0.057 | 4.2s | 5 | 20 | 5.8s | 0 | 0.017 | 96.67% | win |
| fast-recovery | recovery_no_flap | 47 | 44.810457119s | 1.405185701s | 95ms | 40.584314ms | 0.757 | 0.068 | 4.2s | 5 | 20 | 5.8s | 0 | 0.017 | 96.57% | win |
| fast-recovery | recovery_no_flap | 71 | 44.958361864s | 1.431452957s | 95ms | 40.973313ms | 0.755 | 0.072 | 4.2s | 4 | 29 | 7.8s | 0 | 0.018 | 96.37% | win |
| fast-recovery | recovery_no_flap | 101 | 44.837077225s | 1.388342245s | 95ms | 40.499706ms | 0.755 | 0.067 | 4.2s | 4 | 20 | 5.8s | 0 | 0.017 | 96.61% | win |
| fast-recovery | recovery_no_flap | 131 | 44.901274352s | 1.340493203s | 95ms | 40.842365ms | 0.757 | 0.062 | 4.2s | 5 | 20 | 5.8s | 0 | 0.017 | 96.70% | win |
| fast-recovery | recovery_no_flap | 173 | 44.9015634s | 8.955s | 95ms | 95ms | 0.755 | 0.128 | 1.2s | 3 | 60 | 14.4s | 0 | 0.022 | 83.20% | win |
| fast-recovery | all_channels_bad | 11 | 2m10.9s | 1m30.845s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 0 | 0.022 | 30.61% | win |
| fast-recovery | all_channels_bad | 29 | 2m11.7s | 1m32.445s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 0 | 0.022 | 29.76% | win |
| fast-recovery | all_channels_bad | 47 | 2m7.1s | 1m29.335s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.022 | 29.01% | win |
| fast-recovery | all_channels_bad | 71 | 2m9.6s | 54.455s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.040 | 56.36% | win |
| fast-recovery | all_channels_bad | 101 | 2m6.5s | 1m27.18s | 100ms | 85ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.022 | 31.40% | win |
| fast-recovery | all_channels_bad | 131 | 2m5.1s | 1m29.89s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.022 | 27.53% | win |
| fast-recovery | all_channels_bad | 173 | 2m9.753516526s | 55.055s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.040 | 56.16% | win |
| fast-recovery | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| fast-recovery | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| fast-recovery | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| fast-recovery | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| fast-recovery | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| fast-recovery | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| fast-recovery | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| fast-recovery | healthy_steady_state | 11 | 306.901147ms | 381.578745ms | 29.560991ms | 30.748562ms | 0.015 | 0.015 | not detected | 0 | 0 | not detected | 0 | 0.060 | -11.17% | regression |
| fast-recovery | healthy_steady_state | 29 | 306.449512ms | 381.469428ms | 29.524996ms | 31.157905ms | 0.015 | 0.017 | not detected | 0 | 0 | not detected | 0 | 0.060 | -12.78% | regression |
| fast-recovery | healthy_steady_state | 47 | 306.757116ms | 444.552055ms | 29.42026ms | 30.273464ms | 0.028 | 0.030 | not detected | 0 | 0 | not detected | 0 | 0.060 | -17.97% | regression |
| fast-recovery | healthy_steady_state | 71 | 304.971872ms | 376.322213ms | 29.463197ms | 31.229065ms | 0.018 | 0.018 | not detected | 0 | 0 | not detected | 0 | 0.060 | -11.83% | regression |
| fast-recovery | healthy_steady_state | 101 | 306.026121ms | 378.530084ms | 29.656679ms | 30.52698ms | 0.023 | 0.023 | not detected | 0 | 0 | not detected | 0 | 0.060 | -10.05% | regression |
| fast-recovery | healthy_steady_state | 131 | 307.461992ms | 384.485697ms | 29.32466ms | 31.116071ms | 0.022 | 0.022 | not detected | 0 | 0 | not detected | 0 | 0.060 | -12.40% | regression |
| fast-recovery | healthy_steady_state | 173 | 306.762211ms | 398.355915ms | 29.635213ms | 31.324661ms | 0.022 | 0.023 | not detected | 0 | 0 | not detected | 0 | 0.060 | -14.47% | regression |
| hard-failure-confirmed | gradual_degradation | 11 | 2m18.05810847s | 635.816516ms | 111.638008ms | 40.389584ms | 0.738 | 0.023 | 2.4s | 7 | 11 | 4s | 0 | 0.013 | 99.26% | win |
| hard-failure-confirmed | gradual_degradation | 29 | 2m17.230127598s | 759.731479ms | 111.973508ms | 40.315928ms | 0.737 | 0.032 | 2.6s | 6 | 13 | 4.4s | 0 | 0.013 | 99.18% | win |
| hard-failure-confirmed | gradual_degradation | 47 | 2m18.543146187s | 883.925008ms | 112.017461ms | 40.306719ms | 0.743 | 0.047 | 2.4s | 5 | 13 | 4.4s | 0 | 0.013 | 99.09% | win |
| hard-failure-confirmed | gradual_degradation | 71 | 2m18.842157616s | 676.163776ms | 112.679273ms | 40.551492ms | 0.742 | 0.035 | 2.4s | 6 | 12 | 4.2s | 0 | 0.013 | 99.22% | win |
| hard-failure-confirmed | gradual_degradation | 101 | 2m17.896601353s | 894.737117ms | 112.288179ms | 40.326863ms | 0.740 | 0.043 | 2.2s | 6 | 13 | 4.4s | 0 | 0.013 | 99.09% | win |
| hard-failure-confirmed | gradual_degradation | 131 | 2m18.257709744s | 678.23531ms | 112.50077ms | 40.508129ms | 0.742 | 0.033 | 2.2s | 6 | 12 | 4.2s | 0 | 0.013 | 99.22% | win |
| hard-failure-confirmed | gradual_degradation | 173 | 2m18.511286942s | 890.975405ms | 112.241073ms | 40.554002ms | 0.740 | 0.042 | 1.4s | 4 | 12 | 4.2s | 0 | 0.013 | 99.10% | win |
| hard-failure-confirmed | sudden_outage | 11 | 15.6s | 1.399761034s | 29.758413ms | 40.030253ms | 0.669 | 0.064 | 2.4s | 6 | 20 | 4s | 2 | 0.018 | 88.68% | win |
| hard-failure-confirmed | sudden_outage | 29 | 15.6s | 475.527462ms | 29.586311ms | 40.071145ms | 0.667 | 0.038 | 2.4s | 6 | 11 | 4s | 0 | 0.018 | 93.71% | win |
| hard-failure-confirmed | sudden_outage | 47 | 15.6s | 1.332884407s | 29.297237ms | 39.911486ms | 0.676 | 0.060 | 2.4s | 6 | 11 | 4s | 0 | 0.018 | 89.64% | win |
| hard-failure-confirmed | sudden_outage | 71 | 15.6s | 1.059917758s | 29.362123ms | 40.221924ms | 0.673 | 0.053 | 2.8s | 7 | 13 | 4.4s | 0 | 0.018 | 90.78% | win |
| hard-failure-confirmed | sudden_outage | 101 | 15.6s | 1.328659254s | 29.696734ms | 39.777203ms | 0.673 | 0.060 | 2.6s | 6 | 12 | 4.2s | 0 | 0.018 | 89.60% | win |
| hard-failure-confirmed | sudden_outage | 131 | 15.6s | 1.344122193s | 29.458433ms | 40.210329ms | 0.676 | 0.062 | 2.4s | 6 | 11 | 4s | 0 | 0.018 | 89.54% | win |
| hard-failure-confirmed | sudden_outage | 173 | 15.6s | 479.497374ms | 29.101676ms | 40.039804ms | 0.673 | 0.047 | 1.2s | 3 | 11 | 4s | 0 | 0.018 | 93.57% | win |
| hard-failure-confirmed | capacity_aggregation | 11 | 1m19.876239452s | 36.900507488s | 25ms | 28ms | 0.988 | 0.943 | 0s | 0 | 360 | 1.125s | 2 | 0.006 | 57.72% | win |
| hard-failure-confirmed | capacity_aggregation | 29 | 1m19.971533361s | 25.222714186s | 25ms | 28ms | 0.988 | 0.943 | 0s | 0 | 238 | 1.125s | 2 | 0.006 | 68.90% | win |
| hard-failure-confirmed | capacity_aggregation | 47 | 1m19.89581372s | 37.042298517s | 25ms | 28ms | 0.988 | 0.944 | 0s | 0 | 360 | 1.125s | 2 | 0.006 | 57.60% | win |
| hard-failure-confirmed | capacity_aggregation | 71 | 1m19.767790605s | 22.210940613s | 25ms | 28ms | 0.988 | 0.926 | 0s | 0 | 250 | 1.125s | 3 | 0.006 | 73.29% | win |
| hard-failure-confirmed | capacity_aggregation | 101 | 1m20.040921681s | 35.166703648s | 25ms | 28ms | 0.990 | 0.946 | 0s | 0 | 342 | 1.125s | 2 | 0.006 | 59.57% | win |
| hard-failure-confirmed | capacity_aggregation | 131 | 1m19.734245977s | 25.071434603s | 25ms | 28ms | 0.988 | 0.940 | 0s | 0 | 223 | 1.125s | 2 | 0.006 | 68.95% | win |
| hard-failure-confirmed | capacity_aggregation | 173 | 1m19.865475022s | 28.502344591s | 25ms | 28ms | 0.988 | 0.943 | 0s | 0 | 286 | 1.125s | 2 | 0.006 | 65.91% | win |
| hard-failure-confirmed | transient_spike | 11 | 12.6925426s | 1.466972016s | 90ms | 40.91354ms | 0.396 | 0.060 | 4.4s | 5 | 21 | 6.2s | 0 | 0.020 | 86.05% | win |
| hard-failure-confirmed | transient_spike | 29 | 12.307633594s | 1.920996335s | 90ms | 90ms | 0.384 | 0.069 | 4.8s | 6 | 23 | 6.4s | 0 | 0.020 | 78.01% | win |
| hard-failure-confirmed | transient_spike | 47 | 12.854235214s | 1.417704584s | 90ms | 40.846125ms | 0.402 | 0.080 | 4.4s | 5 | 21 | 6.2s | 0 | 0.020 | 86.12% | win |
| hard-failure-confirmed | transient_spike | 71 | 12.659328791s | 1.46716421s | 90ms | 40.987696ms | 0.400 | 0.073 | 4.6s | 4 | 22 | 6.4s | 0 | 0.020 | 85.66% | win |
| hard-failure-confirmed | transient_spike | 101 | 12.607221347s | 1.440466991s | 90ms | 40.995495ms | 0.398 | 0.078 | 4.6s | 4 | 22 | 6.4s | 0 | 0.020 | 85.67% | win |
| hard-failure-confirmed | transient_spike | 131 | 12.512324827s | 1.959281014s | 90ms | 90ms | 0.398 | 0.082 | 4.8s | 6 | 23 | 6.4s | 0 | 0.020 | 77.88% | win |
| hard-failure-confirmed | transient_spike | 173 | 12.652228169s | 1.450459768s | 90ms | 40.951473ms | 0.398 | 0.089 | 1.2s | 3 | 22 | 6.4s | 0 | 0.020 | 85.45% | win |
| hard-failure-confirmed | stale_candidate | 11 | 46.403333381s | 20.10333336s | 110ms | 110ms | 0.889 | 0.433 | 9.99999999s | 12 | 71 | 27.333333306s | 0 | 0.029 | 60.06% | win |
| hard-failure-confirmed | stale_candidate | 29 | 52.933333379s | 22.63333336s | 110ms | 110ms | 0.883 | 0.444 | 9.666666657s | 9 | 71 | 27.333333306s | 0 | 0.029 | 60.88% | win |
| hard-failure-confirmed | stale_candidate | 47 | 51.540000046s | 21.710000022s | 110ms | 110ms | 0.883 | 0.427 | 9.333333324s | 9 | 71 | 27.333333306s | 0 | 0.029 | 62.00% | win |
| hard-failure-confirmed | stale_candidate | 71 | 52.600000048s | 28.163333367s | 110ms | 110ms | 0.889 | 0.573 | 9.666666657s | 10 | 90 | 33.999999966s | 0 | 0.035 | 50.01% | win |
| hard-failure-confirmed | stale_candidate | 101 | 49.070000047s | 20.710000021s | 110ms | 110ms | 0.883 | 0.433 | 9.99999999s | 9 | 71 | 27.333333306s | 0 | 0.029 | 61.32% | win |
| hard-failure-confirmed | stale_candidate | 131 | 47.40333338s | 20.180000021s | 110ms | 110ms | 0.883 | 0.433 | 9.666666657s | 10 | 71 | 27.333333306s | 0 | 0.029 | 61.02% | win |
| hard-failure-confirmed | stale_candidate | 173 | 46.87333338s | 20.103333362s | 110ms | 110ms | 0.883 | 0.439 | 9.99999999s | 10 | 72 | 27.666666639s | 0 | 0.029 | 60.36% | win |
| hard-failure-confirmed | recovery_no_flap | 11 | 44.799011002s | 1.053901177s | 95ms | 40.696441ms | 0.752 | 0.052 | 4.4s | 5 | 21 | 6.2s | 0 | 0.015 | 97.13% | win |
| hard-failure-confirmed | recovery_no_flap | 29 | 44.937203373s | 1.377540368s | 95ms | 40.463509ms | 0.750 | 0.058 | 4.4s | 5 | 21 | 6.2s | 0 | 0.015 | 96.65% | win |
| hard-failure-confirmed | recovery_no_flap | 47 | 44.810457119s | 1.411640135s | 95ms | 40.824045ms | 0.757 | 0.070 | 4.4s | 5 | 21 | 6.2s | 0 | 0.015 | 96.53% | win |
| hard-failure-confirmed | recovery_no_flap | 71 | 44.958361864s | 1.327112504s | 95ms | 40.829322ms | 0.755 | 0.058 | 4.4s | 4 | 21 | 6.2s | 0 | 0.015 | 96.72% | win |
| hard-failure-confirmed | recovery_no_flap | 101 | 44.837077225s | 1.347242403s | 95ms | 40.839377ms | 0.755 | 0.062 | 4.4s | 4 | 21 | 6.2s | 0 | 0.015 | 96.67% | win |
| hard-failure-confirmed | recovery_no_flap | 131 | 44.901274352s | 1.057842609s | 95ms | 40.74255ms | 0.757 | 0.055 | 4.4s | 5 | 21 | 6.2s | 0 | 0.015 | 97.12% | win |
| hard-failure-confirmed | recovery_no_flap | 173 | 44.9015634s | 1.378399892s | 95ms | 40.73532ms | 0.755 | 0.072 | 1.2s | 3 | 21 | 6.2s | 0 | 0.015 | 96.59% | win |
| hard-failure-confirmed | all_channels_bad | 11 | 2m10.9s | 1m8.7s | 100ms | 100ms | 0.669 | 0.669 | 0s | 0 | 300 | not detected | 0 | 0.038 | 48.16% | win |
| hard-failure-confirmed | all_channels_bad | 29 | 2m11.7s | 1m12.5s | 100ms | 100ms | 0.667 | 0.667 | 0s | 0 | 300 | not detected | 0 | 0.038 | 45.84% | win |
| hard-failure-confirmed | all_channels_bad | 47 | 2m7.1s | 1m8.8s | 100ms | 100ms | 0.676 | 0.676 | 0s | 0 | 300 | not detected | 0 | 0.038 | 46.41% | win |
| hard-failure-confirmed | all_channels_bad | 71 | 2m9.6s | 1m11.3s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.038 | 45.78% | win |
| hard-failure-confirmed | all_channels_bad | 101 | 2m6.5s | 1m8.2s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.038 | 46.74% | win |
| hard-failure-confirmed | all_channels_bad | 131 | 2m5.1s | 1m8.5s | 100ms | 100ms | 0.676 | 0.678 | 0s | 0 | 300 | not detected | 0 | 0.038 | 45.70% | win |
| hard-failure-confirmed | all_channels_bad | 173 | 2m9.753516526s | 1m11s | 100ms | 100ms | 0.673 | 0.673 | 0s | 0 | 300 | not detected | 0 | 0.038 | 45.88% | win |
| hard-failure-confirmed | low_traffic | 11 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| hard-failure-confirmed | low_traffic | 29 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| hard-failure-confirmed | low_traffic | 47 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| hard-failure-confirmed | low_traffic | 71 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| hard-failure-confirmed | low_traffic | 101 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| hard-failure-confirmed | low_traffic | 131 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| hard-failure-confirmed | low_traffic | 173 | 1.8s | 1.8s | 100ms | 100ms | 0.667 | 0.583 | 10s | 1 | 7 | not detected | 0 | 0.083 | 5.78% | neutral |
| hard-failure-confirmed | healthy_steady_state | 11 | 306.901147ms | 363.70768ms | 29.560991ms | 29.991747ms | 0.015 | 0.015 | not detected | 0 | 0 | not detected | 0 | 0.050 | -7.57% | neutral |
| hard-failure-confirmed | healthy_steady_state | 29 | 306.449512ms | 359.462254ms | 29.524996ms | 29.953468ms | 0.015 | 0.017 | not detected | 0 | 0 | not detected | 0 | 0.050 | -7.88% | neutral |
| hard-failure-confirmed | healthy_steady_state | 47 | 306.757116ms | 425.449182ms | 29.42026ms | 29.977762ms | 0.028 | 0.028 | not detected | 0 | 0 | not detected | 0 | 0.050 | -14.48% | regression |
| hard-failure-confirmed | healthy_steady_state | 71 | 304.971872ms | 370.046734ms | 29.463197ms | 29.915952ms | 0.018 | 0.018 | not detected | 0 | 0 | not detected | 0 | 0.050 | -8.64% | regression |
| hard-failure-confirmed | healthy_steady_state | 101 | 306.026121ms | 406.359393ms | 29.656679ms | 29.974328ms | 0.023 | 0.025 | not detected | 0 | 0 | not detected | 0 | 0.050 | -12.77% | regression |
| hard-failure-confirmed | healthy_steady_state | 131 | 307.461992ms | 406.434429ms | 29.32466ms | 29.9471ms | 0.022 | 0.023 | not detected | 0 | 0 | not detected | 0 | 0.050 | -13.38% | regression |
| hard-failure-confirmed | healthy_steady_state | 173 | 306.762211ms | 371.181352ms | 29.635213ms | 29.993773ms | 0.022 | 0.022 | not detected | 0 | 0 | not detected | 0 | 0.050 | -8.18% | regression |
