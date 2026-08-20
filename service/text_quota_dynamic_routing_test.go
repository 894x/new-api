package service

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordAttemptVisibleCompletionTokensIgnoresBillingReasoningTokens(t *testing.T) {
	info := &relaycommon.RelayInfo{OriginModelName: "gpt-4o"}
	info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), info.OriginModelName, true)
	info.MarkAttemptUpstreamStarted()
	info.RecordAttemptVisibleText("short visible answer")
	billingUsage := &dto.Usage{CompletionTokens: 100_000, CompletionTokenDetails: dto.OutputTokenDetails{ReasoningTokens: 99_990}}

	recordAttemptVisibleCompletionTokens(info)
	sample, ok := info.FinishDynamicRoutingAttempt(nil)

	require.True(t, ok)
	assert.Equal(t, CountTextToken("short visible answer", info.OriginModelName), sample.CompletionTokens)
	assert.NotEqual(t, billingUsage.CompletionTokens, sample.CompletionTokens)
}

func TestRecordAttemptVisibleCompletionTokensRequiresVisibleText(t *testing.T) {
	info := &relaycommon.RelayInfo{OriginModelName: "gpt-4o"}
	info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), info.OriginModelName, true)
	info.MarkAttemptUpstreamStarted()

	recordAttemptVisibleCompletionTokens(info)
	sample, ok := info.FinishDynamicRoutingAttempt(nil)

	require.True(t, ok)
	assert.Zero(t, sample.CompletionTokens)
}

func TestRecordAttemptVisibleCompletionTokensUsesImmutableAttemptModel(t *testing.T) {
	info := &relaycommon.RelayInfo{OriginModelName: "gemini-2.5-pro"}
	info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), info.OriginModelName, true)
	info.MarkAttemptUpstreamStarted()
	info.RecordAttemptVisibleText("visible answer")
	info.OriginModelName = "gemini-2.5-pro-nothinking"

	originalCounter := countAttemptVisibleTextTokens
	t.Cleanup(func() { countAttemptVisibleTextTokens = originalCounter })
	var countedModel string
	countAttemptVisibleTextTokens = func(_ string, model string) int {
		countedModel = model
		return 3
	}

	recordAttemptVisibleCompletionTokens(info)
	sample, ok := info.FinishDynamicRoutingAttempt(nil)

	require.True(t, ok)
	assert.Equal(t, "gemini-2.5-pro", countedModel)
	assert.Equal(t, 3, sample.CompletionTokens)
}
