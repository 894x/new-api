package xunfei

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestXunfeiDefersCapacityAdmissionUntilFinalWebsocketBody(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		IsStream:        true,
		OriginModelName: "xunfei-v4.0",
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey: "app-id|api-secret|api-key",
		},
	}
	info.BeginDynamicRoutingAttempt(7, info.GetChannelType(), info.OriginModelName, true)

	var admitted XunfeiChatRequest
	admissionErr := &service.ChannelModelCapacityAdmissionError{
		ChannelID:  7,
		Model:      info.OriginModelName,
		RetryAfter: time.Second,
	}
	adaptor := &Adaptor{
		capacityAdmitter: func(_ *gin.Context, _ *relaycommon.RelayInfo, body io.Reader) error {
			data, err := io.ReadAll(body)
			require.NoError(t, err)
			require.NoError(t, common.Unmarshal(data, &admitted))
			return admissionErr
		},
	}
	require.True(t, relaychannel.DefersChannelModelCapacityAdmission(adaptor))

	converted, err := adaptor.ConvertOpenAIRequest(c, info, &dto.GeneralOpenAIRequest{
		Model:     info.OriginModelName,
		MaxTokens: common.GetPointer(uint(321)),
		Messages: []dto.Message{
			{Role: "system", Content: "channel system prompt"},
			{Role: "user", Content: "hello"},
		},
	})
	require.NoError(t, err)
	finalRequest, err := adaptor.PrepareFinalOutboundRequest(c, info, converted)
	require.NoError(t, err)
	finalJSON, err := common.Marshal(finalRequest)
	require.NoError(t, err)

	_, err = adaptor.DoRequest(c, info, bytes.NewReader(finalJSON))
	require.NoError(t, err)
	usage, handlerErr := adaptor.DoResponse(c, &http.Response{StatusCode: http.StatusOK}, info)

	assert.Nil(t, usage)
	require.Error(t, handlerErr)
	require.ErrorIs(t, handlerErr, admissionErr)
	assert.Equal(t, uint(321), admitted.Parameter.Chat.MaxTokens)
	require.Len(t, admitted.Payload.Message.Text, 3)
	assert.Equal(t, XunfeiMessage{Role: "user", Content: "channel system prompt"}, admitted.Payload.Message.Text[0])
	assert.Equal(t, XunfeiMessage{Role: "assistant", Content: "Okay"}, admitted.Payload.Message.Text[1])
	assert.Equal(t, XunfeiMessage{Role: "user", Content: "hello"}, admitted.Payload.Message.Text[2])

	_, observed := info.FinishDynamicRoutingAttempt(handlerErr)
	assert.False(t, observed, "local capacity denial must not affect dynamic channel health")
	assert.True(t, errors.Is(handlerErr, admissionErr))
}
