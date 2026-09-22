package controller

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	tencenttokenhub "github.com/QuantumNous/new-api/relay/channel/tencent_tokenhub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenHubChannelTestRejectsVideoWithoutUpstreamSubmission(t *testing.T) {
	for _, modelName := range []string{
		"hy-video-1.5", "yt-video-2.0", "yt-video-fx", "yt-video-humanactor",
		"kl-video-v3", "kl-video-v2-6", "kl-video-v2-5-turbo", "kl-video-v2-1-master", "kl-video-v2-1",
		"vd-video-q3-pro", "vd-video-q3-turbo",
	} {
		t.Run(modelName, func(t *testing.T) {
			channel := &model.Channel{Type: constant.ChannelTypeTokenHub, Models: modelName}
			result := testChannel(context.Background(), channel, 0, " "+modelName+" ", "", false)
			require.ErrorContains(t, result.localErr, "video channel test is not supported")
			assert.Nil(t, result.context)
			assert.Nil(t, result.newAPIError)
		})
	}
	for _, channel := range []*model.Channel{
		{Type: constant.ChannelTypeTokenHub, Models: "yt-video-fx,hy-image-lite"},
		{Type: constant.ChannelTypeTokenHub, Models: "hy-image-lite", TestModel: common.GetPointer(" yt-video-fx ")},
	} {
		result := testChannel(context.Background(), channel, 0, "", "", false)
		require.ErrorContains(t, result.localErr, "video channel test is not supported")
	}
	for _, modelName := range []string{"hy-image-lite", "hy-image-v3.0", "unknown-video-model", ""} {
		assert.False(t, tencenttokenhub.IsVideoModel(modelName), "must not classify %q as a video model", modelName)
	}
}
