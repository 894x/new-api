package middleware

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupContextForSelectedChannelClearsPreviousChannelOptionalMetadata(t *testing.T) {
	ctx, _ := gin.CreateTestContext(nil)
	organization := "organization-from-full-channel"
	full := &model.Channel{
		Id:                 7101,
		Type:               constant.ChannelTypeAzure,
		Name:               "full",
		Key:                "key-full",
		OpenAIOrganization: &organization,
		Other:              "2026-08-01-preview",
	}
	replacement := &model.Channel{
		Id:   7102,
		Type: constant.ChannelTypeOpenAI,
		Name: "replacement",
		Key:  "key-replacement",
	}

	require.Nil(t, SetupContextForSelectedChannel(ctx, full, "model-a"))
	assert.Equal(t, organization, common.GetContextKeyString(ctx, constant.ContextKeyChannelOrganization))
	assert.Equal(t, "2026-08-01-preview", ctx.GetString("api_version"))
	ctx.Set("region", "stale-region")
	ctx.Set("plugin", "stale-plugin")
	ctx.Set("bot_id", "stale-bot")

	require.Nil(t, SetupContextForSelectedChannel(ctx, replacement, "model-a"))
	assert.Empty(t, common.GetContextKeyString(ctx, constant.ContextKeyChannelOrganization))
	assert.Empty(t, ctx.GetString("api_version"))
	assert.Empty(t, ctx.GetString("region"))
	assert.Empty(t, ctx.GetString("plugin"))
	assert.Empty(t, ctx.GetString("bot_id"))
}
