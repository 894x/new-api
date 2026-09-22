package relay

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/stretchr/testify/require"
)

func TestAliTaskAdaptorSupportsNativeDashScopeVideoResponses(t *testing.T) {
	adaptor := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeAli)))
	require.NotNil(t, adaptor)
	native, ok := adaptor.(channel.NativeTaskProtocol)
	require.True(t, ok)
	require.True(t, native.SupportsNativeTaskFormat(constant.TaskResponseFormatAliVideo))
}
