package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelCapacityDefaultsOverridesAndReset(t *testing.T) {
	clearChannelModelRoutingTables(t)
	channel := createChannelModelRoutingTestChannel(t, 5901, "capacity-a,capacity-b", 10, 20, common.ChannelStatusEnabled)
	channel.RPM, channel.TPM = common.GetPointer(int64(60)), common.GetPointer(int64(6000))
	require.NoError(t, channel.Update())
	require.NoError(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{{
		ChannelId: channel.Id, Model: "capacity-a", RPM: common.GetPointer(int64(0)), TPM: common.GetPointer(int64(900)),
	}}))
	routes, err := ListChannelModelRoutings(channel.Id)
	require.NoError(t, err)
	require.Len(t, routes, 2)
	assert.Equal(t, int64(0), routes[0].EffectiveRPM)
	assert.Equal(t, int64(900), routes[0].EffectiveTPM)
	assert.Equal(t, int64(60), routes[1].EffectiveRPM)
	assert.Equal(t, int64(6000), routes[1].EffectiveTPM)
	require.NoError(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{{ChannelId: channel.Id, Model: "capacity-a"}}))
	routes, err = ListChannelModelRoutings(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(60), routes[0].EffectiveRPM)
	channel.RPM, channel.TPM = common.GetPointer(int64(0)), common.GetPointer(int64(0))
	require.NoError(t, channel.Update())
	routes, err = ListChannelModelRoutings(channel.Id)
	require.NoError(t, err)
	assert.Zero(t, routes[0].EffectiveRPM)
	assert.Zero(t, routes[0].EffectiveTPM)
}

func TestChannelCapacityLegacyPatchPreservesOverridesAndNullClearsThem(t *testing.T) {
	clearChannelModelRoutingTables(t)
	channel := createChannelModelRoutingTestChannel(t, 5902, "capacity-model", 0, 0, common.ChannelStatusEnabled)
	require.NoError(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{{ChannelId: channel.Id, Model: "capacity-model", RPM: common.GetPointer(int64(20)), TPM: common.GetPointer(int64(200))}}))
	var legacy ChannelModelOverridePatch
	require.NoError(t, common.Unmarshal([]byte(`{"channel_id":5902,"model":"capacity-model","priority_override":10,"weight_override":null}`), &legacy))
	require.NoError(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{legacy}))
	routes, err := ListChannelModelRoutings(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(20), routes[0].EffectiveRPM)
	assert.Equal(t, int64(200), routes[0].EffectiveTPM)
	var clear ChannelModelOverridePatch
	require.NoError(t, common.Unmarshal([]byte(`{"channel_id":5902,"model":"capacity-model","rpm_override":null,"tpm_override":0}`), &clear))
	require.NoError(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{clear}))
	routes, err = ListChannelModelRoutings(channel.Id)
	require.NoError(t, err)
	assert.Nil(t, routes[0].RPMOverride)
	require.NotNil(t, routes[0].TPMOverride)
	assert.Zero(t, *routes[0].TPMOverride)
}

func TestChannelCapacityRejectsInvalidLimitsAtomically(t *testing.T) {
	clearChannelModelRoutingTables(t)
	channel := createChannelModelRoutingTestChannel(t, 5903, "capacity-a,capacity-b", 0, 0, common.ChannelStatusEnabled)
	for _, invalid := range []int64{-1, MaxChannelModelRateLimit + 1} {
		channel.RPM = common.GetPointer(invalid)
		require.Error(t, channel.Update())
		channel.RPM = nil
		require.Error(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{
			{ChannelId: channel.Id, Model: "capacity-a", RPM: common.GetPointer(int64(1))},
			{ChannelId: channel.Id, Model: "capacity-b", TPM: common.GetPointer(invalid)},
		}))
		routes, err := ListChannelModelRoutings(channel.Id)
		require.NoError(t, err)
		assert.Zero(t, routes[0].EffectiveRPM)
	}
}
