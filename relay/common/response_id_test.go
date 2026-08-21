package common

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenBaseRelayInfoCreatesUUIDOnlyForV1ChatCompletions(t *testing.T) {
	chatContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	chatContext.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	chatInfo := genBaseRelayInfo(chatContext, nil)

	parsedID, err := uuid.Parse(chatInfo.ResponseId)
	require.NoError(t, err)
	assert.Equal(t, uuid.Version(4), parsedID.Version())
	assert.Equal(t, chatInfo.ResponseId, common.GetContextKeyString(chatContext, constant.ContextKeyResponseId))

	responsesContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	responsesContext.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	responsesInfo := genBaseRelayInfo(responsesContext, nil)
	assert.Empty(t, responsesInfo.ResponseId)
	assert.Empty(t, common.GetContextKeyString(responsesContext, constant.ContextKeyResponseId))

	playgroundContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	playgroundContext.Request = httptest.NewRequest(http.MethodPost, "/pg/chat/completions", nil)
	playgroundInfo := genBaseRelayInfo(playgroundContext, nil)
	assert.Empty(t, playgroundInfo.ResponseId)
	assert.Empty(t, common.GetContextKeyString(playgroundContext, constant.ContextKeyResponseId))
}
