package setting

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupRateLimitsPreserveModelRulesWithLargeCounts(t *testing.T) {
	previous := ModelRequestRateLimitGroup2JSONString()
	t.Cleanup(func() { require.NoError(t, UpdateModelRequestRateLimitGroupByJSONString(previous)) })
	config := `{"vip":{"limits":[2147483648,2147483648,60000],"models":{"large":{"rpm":2147483648},"free":{"rpm":0,"tpm":0}}}}`
	require.NoError(t, UpdateModelRequestRateLimitGroupByJSONString(config))
	assert.JSONEq(t, config, ModelRequestRateLimitGroup2JSONString())
	total, success, tpm, duration := ResolveGroupModelRateLimit("vip", "large")
	assert.Equal(t, 2147483648, total)
	assert.Zero(t, success)
	assert.Equal(t, 60000, tpm)
	assert.EqualValues(t, 60, duration)
	total, success, tpm, _ = ResolveGroupModelRateLimit("vip", "free")
	assert.Zero(t, total)
	assert.Zero(t, success)
	assert.Zero(t, tpm)
	assert.NoError(t, CheckModelRequestRateLimitGroup(`{"vip":[106751991167300,1]}`))
	for _, invalid := range []string{
		`{"vip":[106751991167301,1]}`,
		`{"vip":[1,1,2147483648]}`,
		`{"vip":{"limits":[1,1],"models":{"large":{"rpm":106751991167301}}}}`,
	} {
		assert.Error(t, CheckModelRequestRateLimitGroup(invalid))
	}
}

func TestGroupRateLimitDurationDoesNotWrap(t *testing.T) {
	previous := ModelRequestRateLimitDurationMinutes
	t.Cleanup(func() { ModelRequestRateLimitDurationMinutes = previous })
	for _, tc := range []struct {
		minutes int
		want    int64
	}{{-1, 0}, {0, 0}, {1, 60}, {math.MaxInt, math.MaxInt64}} {
		t.Run(fmt.Sprint(tc.minutes), func(t *testing.T) {
			ModelRequestRateLimitDurationMinutes = tc.minutes
			_, _, _, duration := ResolveGroupModelRateLimit("", "")
			assert.Equal(t, tc.want, duration)
		})
	}
}

func TestCheckModelRequestRateLimitGroupAcceptsOptionalTPM(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{
			name:  "legacy two-value limit",
			value: `{"default":[200,100]}`,
		},
		{
			name:  "limit with TPM",
			value: `{"vip":[0,1000,60000]}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.NoError(t, CheckModelRequestRateLimitGroup(test.value))
		})
	}
}

func TestCheckModelRequestRateLimitGroupRejectsInvalidTPM(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{
			name:  "negative TPM",
			value: `{"default":[200,100,-1]}`,
		},
		{
			name:  "TPM above int32",
			value: `{"default":[200,100,2147483648]}`,
		},
		{
			name:  "missing success limit",
			value: `{"default":[200]}`,
		},
		{
			name:  "extra value",
			value: `{"default":[200,100,60000,1]}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Error(t, CheckModelRequestRateLimitGroup(test.value))
		})
	}
}

func TestGetGroupRateLimitDefaultsLegacyTPMToUnlimited(t *testing.T) {
	previous := ModelRequestRateLimitGroup2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateModelRequestRateLimitGroupByJSONString(previous))
	})

	require.NoError(t, UpdateModelRequestRateLimitGroupByJSONString(`{"default":[200,100]}`))
	totalCount, successCount, tpm, found := GetGroupRateLimit("default")

	assert.True(t, found)
	assert.Equal(t, 200, totalCount)
	assert.Equal(t, 100, successCount)
	assert.Zero(t, tpm)
}

func TestGetGroupRateLimitReturnsConfiguredTPM(t *testing.T) {
	previous := ModelRequestRateLimitGroup2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateModelRequestRateLimitGroupByJSONString(previous))
	})

	require.NoError(t, UpdateModelRequestRateLimitGroupByJSONString(`{"vip":[0,1000,60000]}`))
	totalCount, successCount, tpm, found := GetGroupRateLimit("vip")

	assert.True(t, found)
	assert.Zero(t, totalCount)
	assert.Equal(t, 1000, successCount)
	assert.Equal(t, 60000, tpm)
}

func TestGroupModelRateLimitsRoundTripAndRejectInvalidOverrides(t *testing.T) {
	previous := ModelRequestRateLimitGroup2JSONString()
	t.Cleanup(func() { require.NoError(t, UpdateModelRequestRateLimitGroupByJSONString(previous)) })
	config := `{"vip":{"limits":[200,100,60000],"models":{"gpt-5":{"rpm":30},"claude-sonnet":{"tpm":0}}},"legacy":[200,100]}`
	require.NoError(t, UpdateModelRequestRateLimitGroupByJSONString(config))
	saved := ModelRequestRateLimitGroup2JSONString()
	assert.JSONEq(t, `{"vip":{"limits":[200,100,60000],"models":{"gpt-5":{"rpm":30},"claude-sonnet":{"tpm":0}}},"legacy":[200,100,0]}`, saved)
	for _, invalid := range []string{
		`{"vip":{"models":{"gpt-5":{"rpm":30}}}}`,
		`{"vip":{"limits":[200,100,60000],"models":{"gpt-5":{"rpm":-1}}}}`,
		`{"vip":{"limits":[200,100,60000],"models":{"gpt-5":{"tpm":2147483648}}}}`,
		`{"vip":{"limits":[200,100,60000],"models":{"gpt-5":{"rpm":1.5}}}}`,
		`{"vip":{"limits":[200,100,60000],"models":{"":{"rpm":1}}}}`,
		`{"vip":{"limits":[200,100,60000],"models":{"gpt-5":{"rmp":1}}}}`,
		`{"vip":{"limits":[200,100,60000],"models":{"gpt-5":null}}}`,
	} {
		t.Run(invalid, func(t *testing.T) {
			assert.Error(t, UpdateModelRequestRateLimitGroupByJSONString(invalid))
			assert.JSONEq(t, saved, ModelRequestRateLimitGroup2JSONString(), "invalid updates must preserve active limits")
		})
	}
}
