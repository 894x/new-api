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
	_, ok := adaptor.(channel.AliNativeVideoConverter)
	require.True(t, ok)
}
