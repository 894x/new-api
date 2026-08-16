package doubao

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRequestURLUsesDoubaoVideoAPIMode(t *testing.T) {
	tests := []struct {
		name         string
		settingsJSON string
		want         string
	}{
		{
			name:         "default remains doubao v3",
			settingsJSON: `{}`,
			want:         "https://video.example/api/v3/contents/generations/tasks",
		},
		{
			name:         "video generations preset",
			settingsJSON: `{"doubao_video_api_mode":"video_generations"}`,
			want:         "https://video.example/v1/video/generations",
		},
		{
			name: "custom paths",
			settingsJSON: `{
				"doubao_video_api_mode":"custom",
				"doubao_video_submit_path":"/custom/video/tasks",
				"doubao_video_fetch_path":"/custom/video/tasks/{id}"
			}`,
			want: "https://video.example/custom/video/tasks",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var settings dto.ChannelOtherSettings
			require.NoError(t, common.Unmarshal([]byte(tt.settingsJSON), &settings))

			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
				ChannelBaseUrl:       "https://video.example/",
				ChannelOtherSettings: settings,
			}}
			adaptor := &TaskAdaptor{}
			adaptor.Init(info)

			got, err := adaptor.BuildRequestURL(info)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildRequestURLRejectsCustomFetchPathWithoutTaskID(t *testing.T) {
	var settings dto.ChannelOtherSettings
	require.NoError(t, common.Unmarshal([]byte(`{
		"doubao_video_api_mode":"custom",
		"doubao_video_submit_path":"/custom/video/tasks",
		"doubao_video_fetch_path":"/custom/video/tasks/result"
	}`), &settings))

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelBaseUrl:       "https://video.example",
		ChannelOtherSettings: settings,
	}}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	_, err := adaptor.BuildRequestURL(info)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "{id}")
}

func TestFetchTaskUsesConfiguredQueryPath(t *testing.T) {
	tests := []struct {
		name         string
		settingsJSON string
		wantPath     string
	}{
		{
			name:         "video generations preset",
			settingsJSON: `{"doubao_video_api_mode":"video_generations"}`,
			wantPath:     "/v1/video/generations/task_123",
		},
		{
			name: "custom path",
			settingsJSON: `{
				"doubao_video_api_mode":"custom",
				"doubao_video_submit_path":"/custom/video/tasks",
				"doubao_video_fetch_path":"/custom/video/results/{id}"
			}`,
			wantPath: "/custom/video/results/task_123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestedPath := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestedPath <- r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{}`)
			}))
			t.Cleanup(server.Close)

			var settings dto.ChannelOtherSettings
			require.NoError(t, common.Unmarshal([]byte(tt.settingsJSON), &settings))
			adaptor := &TaskAdaptor{}
			adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
				ChannelBaseUrl:       server.URL,
				ChannelOtherSettings: settings,
			}})

			resp, err := adaptor.FetchTask(server.URL, "sk-test", map[string]any{"task_id": "task_123"}, "")
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })
			assert.Equal(t, tt.wantPath, <-requestedPath)
		})
	}
}
