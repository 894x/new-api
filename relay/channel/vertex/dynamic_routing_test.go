package vertex

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVertexInvalidServiceAccountPublishesPreDispatchHealthFailure(t *testing.T) {
	info := &relaycommon.RelayInfo{
		IsStream:        true,
		OriginModelName: "gemini-2.5-pro",
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey:            "{invalid-json",
			UpstreamModelName: "gemini-2.5-pro",
			ChannelOtherSettings: dto.ChannelOtherSettings{
				VertexKeyType: dto.VertexKeyTypeJSON,
			},
		},
	}
	info.BeginDynamicRoutingAttempt(9, info.GetChannelType(), info.OriginModelName, true)
	adaptor := &Adaptor{}
	adaptor.Init(info)

	_, err := adaptor.GetRequestURL(info)
	require.Error(t, err)
	apiErr, ok := err.(*relaytypes.NewAPIError)
	require.True(t, ok)
	sample, observed := info.FinishDynamicRoutingAttempt(apiErr)

	require.True(t, observed)
	assert.True(t, sample.HardFailure)
	assert.True(t, sample.UpstreamStartedAt.IsZero())
	assert.False(t, sample.HasTTFT)
	assert.False(t, sample.HasTPOT)
}

func TestVertexTransientTokenPreflightIsDynamicHardWithoutChannelError(t *testing.T) {
	originalProvider := vertexAccessTokenProvider
	vertexAccessTokenProvider = func(*Adaptor, *relaycommon.RelayInfo) (string, error) {
		return "", errors.New("transient Google token endpoint failure")
	}
	t.Cleanup(func() { vertexAccessTokenProvider = originalProvider })
	originalAutoDisable := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() { common.AutomaticDisableChannelEnabled = originalAutoDisable })

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		IsStream:        true,
		OriginModelName: "gemini-2.5-pro",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-2.5-pro",
			ChannelOtherSettings: dto.ChannelOtherSettings{
				VertexKeyType: dto.VertexKeyTypeJSON,
			},
		},
	}
	info.BeginDynamicRoutingAttempt(9, info.GetChannelType(), info.OriginModelName, true)
	headers := make(http.Header)

	err := (&Adaptor{}).SetupRequestHeader(c, &headers, info)
	require.Error(t, err)
	apiErr, ok := err.(*relaytypes.NewAPIError)
	require.True(t, ok)
	assert.False(t, relaytypes.IsChannelError(apiErr))
	assert.False(t, service.ShouldDisableChannel(apiErr))
	sample, observed := info.FinishDynamicRoutingAttempt(apiErr)

	require.True(t, observed)
	assert.True(t, sample.HardFailure)
	assert.True(t, sample.UpstreamStartedAt.IsZero())
}
