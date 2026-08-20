package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEstimateRequestTokenForCapacityBypassesDisabledGlobalAccounting(t *testing.T) {
	originalCountToken := constant.CountToken
	constant.CountToken = false
	t.Cleanup(func() { constant.CountToken = originalCountToken })

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "capacity-model")
	meta := &types.TokenCountMeta{
		TokenType:   types.TokenTypeTextNumber,
		CombineText: "四个字符",
	}
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatEmbedding}

	ordinary, err := EstimateRequestToken(c, meta, info)
	require.NoError(t, err)
	assert.Zero(t, ordinary)

	capacity, err := EstimateRequestTokenForCapacity(c, meta, info)
	require.NoError(t, err)
	assert.Equal(t, 4, capacity)
}
