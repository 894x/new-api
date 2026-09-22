package relay

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTaskAdaptorMapsSeedanceSLSChannel(t *testing.T) {
	platform := constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedanceSLS))

	adaptor := GetTaskAdaptor(platform)

	require.NotNil(t, adaptor)
	assert.Equal(t, "Seedance SLS", adaptor.GetChannelName())
}

func TestSeedanceSLSChannelCapabilities(t *testing.T) {
	adaptor := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedanceSLS)))
	native, supportsDoubaoV3 := adaptor.(channel.NativeTaskProtocol)
	require.True(t, supportsDoubaoV3)
	assert.True(t, native.SupportsNativeTaskFormat(constant.TaskResponseFormatDoubaoVideo))

	_, hasSynchronousAPI := common.ChannelType2APIType(constant.ChannelTypeSeedanceSLS)
	assert.False(t, hasSynchronousAPI)
	assert.Equal(t,
		[]constant.EndpointType{constant.EndpointTypeOpenAIVideo},
		common.GetEndpointTypesByChannelType(constant.ChannelTypeSeedanceSLS, "doubao-seedance-2-0"),
	)
	assert.Equal(t, 104, constant.ChannelTypeSeedanceSLS)
	assert.Equal(t, "Seedance SLS", constant.GetChannelTypeName(constant.ChannelTypeSeedanceSLS))
	require.Greater(t, len(constant.ChannelBaseURLs), constant.ChannelTypeSeedanceSLS)
	assert.Equal(t, "https://lm.sls.cn", constant.ChannelBaseURLs[constant.ChannelTypeSeedanceSLS])
}
