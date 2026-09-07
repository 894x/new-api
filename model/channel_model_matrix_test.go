package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelModelMatrixIncludesInheritedPairsAndPublicAliases(t *testing.T) {
	clearChannelModelRoutingTables(t)
	channel := createChannelModelRoutingTestChannel(t, 5701, "alias-b, alias-a,alias-a", 10, 20, common.ChannelStatusEnabled)
	mapping := `{"alias-a":"upstream","alias-b":"upstream"}`
	require.NoError(t, DB.Model(channel).Updates(map[string]any{"rpm": 500, "tpm": 10000, "model_mapping": mapping, "group": "default, premium"}).Error)
	zero := int64(0)
	require.NoError(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{{ChannelId: channel.Id, Model: "alias-a", RPM: &zero}}))
	createChannelModelRoutingTestChannel(t, 5702, "disabled-only", 1, 2, common.ChannelStatusManuallyDisabled)

	matrix, err := ListChannelModelMatrix(ChannelModelMatrixFilter{ModelPageSize: 25, ChannelPageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, []string{"alias-a", "alias-b"}, matrix.Models)
	assert.Equal(t, []string{"default", "premium"}, matrix.Groups)
	require.Len(t, matrix.Channels, 1)
	require.Len(t, matrix.Cells, 2)
	assert.Equal(t, int64(0), matrix.Cells[0].EffectiveRPM)
	require.NotNil(t, matrix.Cells[0].RPMOverride)
	assert.Nil(t, matrix.Cells[1].RPMOverride)
	assert.Equal(t, int64(500), matrix.Cells[1].EffectiveRPM)
	assert.Equal(t, int64(10000), matrix.Cells[1].EffectiveTPM)
	assert.Equal(t, int64(10), matrix.Cells[1].EffectivePriority)
	assert.Equal(t, uint(20), matrix.Cells[1].EffectiveWeight)
	assert.Equal(t, "upstream", matrix.Cells[0].UpstreamModel)
	assert.Equal(t, "upstream", matrix.Cells[1].UpstreamModel)
}

func TestChannelModelMatrixFiltersAndPaginatesBothAxes(t *testing.T) {
	clearChannelModelRoutingTables(t)
	a := createChannelModelRoutingTestChannel(t, 5711, "model-a,model-b", 0, 0, common.ChannelStatusEnabled)
	b := createChannelModelRoutingTestChannel(t, 5712, "model-b,model-c", 0, 0, common.ChannelStatusManuallyDisabled)
	require.NoError(t, DB.Model(a).Updates(map[string]any{"name": "Alpha", "group": "vip"}).Error)
	require.NoError(t, DB.Model(b).Updates(map[string]any{"name": "Beta", "group": "vip-plus"}).Error)
	matrix, err := ListChannelModelMatrix(ChannelModelMatrixFilter{Status: "all", ModelPage: 2, ModelPageSize: 1, ChannelPage: 2, ChannelPageSize: 1})
	require.NoError(t, err)
	assert.Equal(t, 3, matrix.ModelTotal)
	assert.Equal(t, 2, matrix.ChannelTotal)
	assert.Equal(t, []string{"model-b"}, matrix.Models)
	require.Len(t, matrix.Channels, 1)
	assert.Equal(t, b.Id, matrix.Channels[0].ID)
	require.Len(t, matrix.Cells, 1)
	assert.Equal(t, "model-b", matrix.Cells[0].Model)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, matrix.Cells[0].ChannelStatus)

	filtered, err := ListChannelModelMatrix(ChannelModelMatrixFilter{Status: "all", Group: "vip", Model: "MODEL-B", Channel: "alpha", ModelPage: 999, ChannelPage: 999, ModelPageSize: 25, ChannelPageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, filtered.ChannelTotal)
	assert.Equal(t, []string{"model-b"}, filtered.Models)
	assert.Equal(t, 1, filtered.ModelPage)
	assert.Equal(t, 1, filtered.ChannelPage)

	empty, err := ListChannelModelMatrix(ChannelModelMatrixFilter{Model: "%_", ModelPageSize: 25, ChannelPageSize: 10})
	require.NoError(t, err)
	assert.Empty(t, empty.Models)
	assert.Empty(t, empty.Cells)
	assert.NotNil(t, empty.Models)
	assert.NotNil(t, empty.Cells)
}

func TestChannelModelMatrixDoesNotExposeSecretsOrInventUnconfiguredCells(t *testing.T) {
	clearChannelModelRoutingTables(t)
	a := createChannelModelRoutingTestChannel(t, 5721, "model-a", 0, 0, common.ChannelStatusEnabled)
	createChannelModelRoutingTestChannel(t, 5722, "model-b", 0, 0, common.ChannelStatusEnabled)
	require.NoError(t, DB.Model(a).Updates(map[string]any{"key": "secret-key", "base_url": "https://secret-host", "param_override": `{"secret":"secret-param"}`, "model_mapping": "invalid-json-secret"}).Error)
	matrix, err := ListChannelModelMatrix(ChannelModelMatrixFilter{ModelPageSize: 25, ChannelPageSize: 10})
	require.NoError(t, err)
	require.Len(t, matrix.Cells, 2)
	assert.True(t, matrix.Cells[0].MappingError)
	encoded, err := common.Marshal(matrix)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "secret")
	assert.NotContains(t, string(encoded), "base_url")
	assert.NotContains(t, string(encoded), "param_override")
}
