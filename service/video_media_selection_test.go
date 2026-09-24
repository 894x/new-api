package service

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestVideoMediaSelectionPreservesPolicyAndAssetIntersection(t *testing.T) {
	for _, cache := range []bool{true, false} {
		for _, dynamic := range []bool{true, false} {
			t.Run(fmt.Sprintf("cache=%t/dynamic=%t", cache, dynamic), func(t *testing.T) {
				db := setupGroupModelChannelPolicyTest(t)
				common.MemoryCacheEnabled = cache
				configureDynamicRoutingForTest(t, dynamic)
				// The only authorized channel requires conversion. The unauthorized direct
				// channel must never win merely because its delivery tier is cheaper.
				for _, id := range []int{4101, 4102} {
					var channel model.Channel
					require.NoError(t, db.First(&channel, "id = ?", id).Error)
					channel.SetOtherSettings(dto.ChannelOtherSettings{ParameterCapabilities: &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{"messages.*.content.*.video_url": {ParticipateInSelection: common.GetPointer(true), Media: &dto.MediaCapability{Kind: "video", Formats: map[string]dto.MediaFormatCapability{"url": {Supported: common.GetPointer(true)}, "base64": {Supported: common.GetPointer(id == 4102)}}, Conversions: dto.MediaConversions{Base64ToURL: common.GetPointer(true)}}}}}})
					require.NoError(t, db.Model(&channel).Update("settings", channel.OtherSettings).Error)
				}
				model.InitChannelCache()
				ctx := newChannelSelectContext()
				param := &RetryParam{Ctx: ctx, TokenGroup: "vip", ModelName: "model-a", RequestPath: "/v1/chat/completions", RequestBody: []byte(`{"messages":[{"content":[{"video_url":{"url":"data:video/mp4;base64,AAAA"}}]}]}`), AllowedChannelIds: map[int]struct{}{4101: {}, 4102: {}}, DynamicRoutingEligible: dynamic}
				channel, _, err := CacheGetRandomSatisfiedChannel(param)
				require.NoError(t, err)
				require.NotNil(t, channel)
				assert.Equal(t, 4101, channel.Id)
				assert.Len(t, param.AllowedChannelIds, 2)
				param.AllowedChannelIds = map[int]struct{}{4102: {}}
				channel, _, err = CacheGetRandomSatisfiedChannel(param)
				require.NoError(t, err)
				assert.Nil(t, channel, "deny-all intersection must stay deny-all")
			})
		}
	}
}
