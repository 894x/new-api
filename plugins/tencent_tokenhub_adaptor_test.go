package plugins_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenHubPluginAdaptorPreservesAliasRequestAndRebuildsMappedModel(t *testing.T) {
	adaptor := relay.GetTaskAdaptor(constant.TaskPlatform("100"))
	require.NotNil(t, adaptor)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"fx-alias","template":"hug","future_field":false}`))
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		OriginModelName: "fx-alias", TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://tokenhub.example", UpstreamModelName: "fx-alias"},
	}
	adaptor.Init(info)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	info.UpstreamModelName = "yt-video-fx"
	validator, ok := adaptor.(channel.MappedTaskRequestValidator)
	require.True(t, ok)
	require.Nil(t, validator.ValidateMappedRequest(c, info))
	reader, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"yt-video-fx","template":"hug","future_field":false}`, string(body))
	assert.Equal(t, "fx-alias", info.OriginModelName)
}

func TestTokenHubPluginAdaptorRejectsMissingHumanActorDurationAfterMapping(t *testing.T) {
	adaptor := relay.GetTaskAdaptor(constant.TaskPlatform("100"))
	require.NotNil(t, adaptor)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"actor-alias","prompt":"hello"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		OriginModelName: "actor-alias", TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://tokenhub.example", UpstreamModelName: "actor-alias"},
	}
	adaptor.Init(info)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	info.UpstreamModelName = "yt-video-humanactor"
	validator, ok := adaptor.(channel.MappedTaskRequestValidator)
	require.True(t, ok)
	taskErr := validator.ValidateMappedRequest(c, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
	assert.ErrorContains(t, taskErr.Error, "billing_duration_seconds is required")
}
