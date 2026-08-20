package sim

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStaticPolicyUsesHighestPriorityBeforeWeight(t *testing.T) {
	policy := NewStaticPolicy(9)
	candidates := []Candidate{
		{ID: "high", Priority: 100, Weight: 1},
		{ID: "low", Priority: 50, Weight: 1000},
	}

	for i := 0; i < 20; i++ {
		assert.Equal(t, "high", policy.Select(0, candidates).CandidateID)
	}
}

func TestStaticPolicyMatchesChannelLotteryWeightOffset(t *testing.T) {
	policy := NewStaticPolicy(19)
	candidates := []Candidate{
		{ID: "zero-configured-weight", Priority: 100, Weight: 0},
		{ID: "heavy", Priority: 100, Weight: 90},
	}

	zeroSelections := 0
	const requests = 11000
	for i := 0; i < requests; i++ {
		if policy.Select(0, candidates).CandidateID == "zero-configured-weight" {
			zeroSelections++
		}
	}

	// The production lottery uses configured weight + 10, so the expected
	// share is 10 / (10 + 100), not zero.
	assert.InDelta(t, 10.0/110.0, float64(zeroSelections)/requests, 0.015)
}

func TestStaticPolicySaturatesConfiguredWeightBeforeAddingTen(t *testing.T) {
	policy := NewStaticPolicy(17)
	candidates := []Candidate{
		{ID: "ordinary", Priority: 100, Weight: 0},
		{ID: "max-weight", Priority: 100, Weight: ^uint(0)},
	}

	for request := 0; request < 100; request++ {
		assert.Equal(t, "max-weight", policy.Select(0, candidates).CandidateID)
	}
}
