package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
