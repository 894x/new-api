package sim

import "time"

func BuiltinScenarios() []Scenario {
	slo := SLO{TTFT: time.Second, TPOT: 60 * time.Millisecond}
	primaryGood := Phase{
		TTFT: 250 * time.Millisecond, TPOT: 25 * time.Millisecond,
		TTFTJitter: 60 * time.Millisecond, TPOTJitter: 5 * time.Millisecond,
		LongTailRate: 0.02, LongTailDelay: 800 * time.Millisecond,
	}
	fallbackGood := Phase{
		TTFT: 400 * time.Millisecond, TPOT: 35 * time.Millisecond,
		TTFTJitter: 80 * time.Millisecond, TPOTJitter: 6 * time.Millisecond,
		LongTailRate: 0.03, LongTailDelay: time.Second,
	}
	channel := func(id string, channelID int, priority int64, concurrency int, phases ...Phase) Channel {
		return Channel{ID: id, ChannelID: channelID, Priority: priority, Weight: 100, Concurrency: concurrency, Timeline: phases}
	}
	fault := func(at time.Duration, bad ...string) FaultSpec {
		return FaultSpec{At: at, BadChannels: bad, MitigationWindow: 10, MitigatedBadShare: 0.2}
	}

	staleArrivals := constantArrivals(2, 10*time.Second)
	for _, arrival := range constantArrivals(3, 50*time.Second) {
		staleArrivals = append(staleArrivals, 90*time.Second+arrival)
	}

	return []Scenario{
		{
			Name: "gradual_degradation", Arrivals: constantArrivals(5, 120*time.Second), OutputTokens: 32, SLO: slo,
			Channels: []Channel{
				channel("primary", 1, 100, 8, primaryGood,
					Phase{Start: 30 * time.Second, TTFT: 700 * time.Millisecond, TPOT: 45 * time.Millisecond, TTFTJitter: 100 * time.Millisecond, TPOTJitter: 8 * time.Millisecond},
					Phase{Start: 50 * time.Second, TTFT: 1800 * time.Millisecond, TPOT: 100 * time.Millisecond, TTFTJitter: 250 * time.Millisecond, TPOTJitter: 15 * time.Millisecond}),
				channel("fallback", 2, 50, 8, fallbackGood),
			},
			Fault: fault(30*time.Second, "primary"),
		},
		{
			Name: "sudden_outage", Arrivals: constantArrivals(5, 90*time.Second), OutputTokens: 24, SLO: slo,
			Channels: []Channel{
				channel("primary", 1, 100, 8, primaryGood,
					Phase{Start: 30 * time.Second, TTFT: 2 * time.Second, TPOT: 100 * time.Millisecond, HardFailureRate: 0.8, HTTP503Rate: 0.2}),
				channel("fallback", 2, 50, 8, fallbackGood),
			},
			Fault: fault(30*time.Second, "primary"),
		},
		{
			Name: "capacity_aggregation", Arrivals: constantArrivals(8, 90*time.Second), OutputTokens: 20, SLO: slo,
			Channels: []Channel{
				channel("primary", 1, 100, 3, Phase{TTFT: 250 * time.Millisecond, TPOT: 25 * time.Millisecond, TTFTJitter: 30 * time.Millisecond}),
				channel("fallback", 2, 50, 4, Phase{TTFT: 350 * time.Millisecond, TPOT: 28 * time.Millisecond, TTFTJitter: 40 * time.Millisecond}),
			},
			Fault: fault(10*time.Second, "primary"),
		},
		{
			Name: "transient_spike", Arrivals: constantArrivals(5, 90*time.Second), OutputTokens: 24, SLO: slo,
			Channels: []Channel{
				channel("primary", 1, 100, 8, primaryGood,
					Phase{Start: 30 * time.Second, TTFT: 2200 * time.Millisecond, TPOT: 90 * time.Millisecond, TTFTJitter: 300 * time.Millisecond},
					Phase{Start: 38 * time.Second, TTFT: primaryGood.TTFT, TPOT: primaryGood.TPOT, TTFTJitter: primaryGood.TTFTJitter, TPOTJitter: primaryGood.TPOTJitter}),
				channel("fallback", 2, 50, 8, fallbackGood),
			},
			Fault: fault(30*time.Second, "primary"),
		},
		{
			Name: "stale_candidate", Arrivals: staleArrivals, OutputTokens: 24, SLO: slo,
			Channels: []Channel{
				channel("primary", 1, 100, 6, primaryGood,
					Phase{Start: 90 * time.Second, TTFT: 2 * time.Second, TPOT: 110 * time.Millisecond, HTTP503Rate: 0.2}),
				channel("fallback", 2, 50, 6, fallbackGood),
			},
			Fault: fault(90*time.Second, "primary"),
		},
		{
			Name: "recovery_no_flap", Arrivals: constantArrivals(5, 120*time.Second), OutputTokens: 24, SLO: slo,
			Channels: []Channel{
				channel("primary", 1, 100, 8, primaryGood,
					Phase{Start: 30 * time.Second, TTFT: 1900 * time.Millisecond, TPOT: 95 * time.Millisecond},
					Phase{Start: 60 * time.Second, TTFT: primaryGood.TTFT, TPOT: primaryGood.TPOT, TTFTJitter: primaryGood.TTFTJitter, TPOTJitter: primaryGood.TPOTJitter}),
				channel("fallback", 2, 50, 8, fallbackGood),
			},
			Fault: fault(30*time.Second, "primary"),
		},
		{
			Name: "all_channels_bad", Arrivals: constantArrivals(5, 90*time.Second), OutputTokens: 24, SLO: slo,
			Channels: []Channel{
				channel("primary", 1, 100, 6, primaryGood,
					Phase{Start: 30 * time.Second, TTFT: 2 * time.Second, TPOT: 100 * time.Millisecond, HTTP503Rate: 0.15}),
				channel("fallback", 2, 50, 6, fallbackGood,
					Phase{Start: 30 * time.Second, TTFT: 1600 * time.Millisecond, TPOT: 85 * time.Millisecond, HTTP503Rate: 0.1}),
			},
			Fault: fault(30*time.Second, "primary", "fallback"),
		},
		{
			Name: "low_traffic", Arrivals: constantArrivals(0.1, 120*time.Second), OutputTokens: 24, SLO: slo,
			Channels: []Channel{
				channel("primary", 1, 100, 4, primaryGood,
					Phase{Start: 40 * time.Second, TTFT: 1800 * time.Millisecond, TPOT: 100 * time.Millisecond}),
				channel("fallback", 2, 50, 4, fallbackGood),
			},
			Fault: FaultSpec{At: 40 * time.Second, BadChannels: []string{"primary"}, MitigationWindow: 3, MitigatedBadShare: 0.34},
		},
		{
			Name: "healthy_steady_state", Arrivals: constantArrivals(5, 120*time.Second), OutputTokens: 24, SLO: slo,
			Channels: []Channel{
				channel("primary", 1, 100, 8, primaryGood),
				channel("fallback", 2, 50, 8, fallbackGood),
			},
		},
	}
}

func constantArrivals(ratePerSecond float64, duration time.Duration) []time.Duration {
	if ratePerSecond <= 0 || duration <= 0 {
		return nil
	}
	step := time.Duration(float64(time.Second) / ratePerSecond)
	arrivals := make([]time.Duration, 0, int(duration/step))
	for at := time.Duration(0); at < duration; at += step {
		arrivals = append(arrivals, at)
	}
	return arrivals
}
