package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDynamicRoutingRequestEligibleOnlyForStreamingTextGeneration(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		stream bool
		want   bool
	}{
		{name: "chat stream", path: "/v1/chat/completions", stream: true, want: true},
		{name: "legacy completion stream", path: "/v1/completions", stream: true, want: true},
		{name: "claude stream", path: "/v1/messages", stream: true, want: true},
		{name: "responses stream", path: "/v1/responses", stream: true, want: true},
		{name: "playground stream", path: "/pg/chat/completions", stream: true, want: true},
		{name: "gemini stream action", path: "/v1beta/models/gemini-2.5-flash:streamGenerateContent", want: true},
		{name: "non-stream chat", path: "/v1/chat/completions", want: false},
		{name: "responses compaction", path: "/v1/responses/compact", stream: true, want: false},
		{name: "gemini non-stream action", path: "/v1beta/models/gemini-2.5-flash:generateContent", want: false},
		{name: "embedding", path: "/v1/embeddings", stream: true, want: false},
		{name: "image", path: "/v1/images/generations", stream: true, want: false},
		{name: "video task", path: "/v1/video/generations", stream: true, want: false},
		{name: "realtime", path: "/v1/realtime", stream: true, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, isDynamicRoutingRequestEligible(test.path, test.stream))
		})
	}
}

func TestGetModelRequestPreservesStreamFlagForEligibility(t *testing.T) {
	ctx := videoResolutionTestContext(t, "/v1/chat/completions", `{"model":"chat-model","stream":true}`)

	request, shouldSelect, err := getModelRequest(ctx)

	require.NoError(t, err)
	assert.True(t, shouldSelect)
	assert.Equal(t, "chat-model", request.Model)
	assert.True(t, request.Stream)
}
