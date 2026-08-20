package sim

import (
	"math/rand"
	"time"
)

type StaticPolicy struct {
	random *rand.Rand
}

func NewStaticPolicy(seed int64) *StaticPolicy {
	return &StaticPolicy{random: rand.New(rand.NewSource(seed))}
}

func (p *StaticPolicy) Select(_ time.Duration, candidates []Candidate) Decision {
	if len(candidates) == 0 {
		return Decision{}
	}
	bestPriority := candidates[0].Priority
	for _, candidate := range candidates[1:] {
		if candidate.Priority > bestPriority {
			bestPriority = candidate.Priority
		}
	}
	totalWeight := 0.0
	dominantID := ""
	dominantWeight := -1.0
	maxUint := ^uint(0)
	for _, candidate := range candidates {
		if candidate.Priority != bestPriority {
			continue
		}
		lotteryWeight := float64(maxUint)
		if candidate.Weight <= maxUint-10 {
			lotteryWeight = float64(candidate.Weight + 10)
		}
		totalWeight += lotteryWeight
		if lotteryWeight > dominantWeight {
			dominantID = candidate.ID
			dominantWeight = lotteryWeight
		}
	}
	pick := p.random.Float64() * totalWeight
	for _, candidate := range candidates {
		if candidate.Priority != bestPriority {
			continue
		}
		lotteryWeight := float64(maxUint)
		if candidate.Weight <= maxUint-10 {
			lotteryWeight = float64(candidate.Weight + 10)
		}
		pick -= lotteryWeight
		if pick < 0 {
			return Decision{CandidateID: candidate.ID, DominantCandidate: dominantID}
		}
	}
	return Decision{}
}

func (p *StaticPolicy) Observe(Sample) {}
