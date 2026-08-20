package relay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel/volcengine"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFinalAudioCapacityUsesVolcengineMetadataTransformedBody(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	const modelName = "volcengine-audio-capacity-model"
	common.SetContextKey(c, constant.ContextKeyOriginalModel, modelName)
	info := &relaycommon.RelayInfo{
		OriginModelName: modelName,
		RelayFormat:     relaytypes.RelayFormatOpenAIAudio,
		RelayMode:       relayconstant.RelayModeAudioSpeech,
		ChannelMeta:     &relaycommon.ChannelMeta{ApiKey: "app-id|access-token"},
	}
	request := dto.AudioRequest{
		Model: modelName,
		Input: "a",
		Voice: "alloy",
		Metadata: json.RawMessage(`{
			"request":{"text":"a much longer provider-native text supplied by the Volcengine metadata override","operation":"submit"}
		}`),
	}
	reader, err := (&volcengine.Adaptor{}).ConvertAudioRequest(c, info, request)
	require.NoError(t, err)

	finalBody, closer, err := replayableFinalAudioCapacityBody(info, reader)
	require.NoError(t, err)
	require.NotNil(t, closer)
	t.Cleanup(func() { require.NoError(t, closer.Close()) })
	_, replayable := finalBody.(common.ReplayableBody)
	require.True(t, replayable)

	tokens, err := service.EstimateFinalChannelModelCapacityTokens(c, info, finalBody.(common.ReplayableBody), 1)

	require.NoError(t, err)
	assert.Greater(t, tokens, int64(1))
}
