package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoubaoPluginNativeHistoricalTaskQueries(t *testing.T) {
	f := setupAssetStorageE2E(t)
	previousHide := operation_setting.GetErrorSetting().HideErrorDetails
	operation_setting.UpdateHideErrorDetails(true)
	t.Cleanup(func() { operation_setting.UpdateHideErrorDetails(previousHide) })
	var customer model.User
	require.NoError(t, model.DB.Where("username = ?", "asset-e2e-other").First(&customer).Error)
	for _, platform := range []constant.TaskPlatform{"doubao", "54", "45"} {
		for _, tc := range []struct {
			state model.TaskStatus
			want  string
		}{
			{model.TaskStatusSubmitted, "queued"},
			{model.TaskStatusInProgress, "running"},
			{model.TaskStatusSuccess, "succeeded"},
			{model.TaskStatusFailure, "failed"},
		} {
			t.Run(string(platform)+"/"+tc.want, func(t *testing.T) {
				task := model.Task{TaskID: model.GenerateTaskID(), Platform: platform, UserId: customer.Id,
					Status: tc.state, CreatedAt: 100, UpdatedAt: 200,
					Properties:  model.Properties{OriginModelName: "public-video-alias", UpstreamModelName: "private-deployment"},
					PrivateData: model.TaskPrivateData{UpstreamTaskID: "private-upstream-id", ResultURL: "https://cdn.example.com/final.mp4"},
				}
				task.SetData(map[string]any{"id": "private-upstream-id", "model": "private-deployment", "status": "queued",
					"seed": 0, "generate_audio": false, "content": map[string]any{"last_frame_url": "https://cdn.example.com/last.png"},
					"resolution": "480p", "ratio": "16:9", "duration": 4, "framespersecond": 24,
					"tools": []any{map[string]any{"type": "web_search"}}, "safety_identifier": "user-123",
					"priority": 0, "draft": false, "draft_task_id": "task_draft", "service_tier": "default", "execution_expires_after": 172800,
					"usage": map[string]any{"completion_tokens": 321, "total_tokens": 321, "tool_usage": map[string]any{"web_search": 1}},
				})
				if tc.state == model.TaskStatusFailure {
					task.FailReason = "provider-private-diagnostic"
					task.SetData(map[string]any{"error": map[string]any{"code": "ProviderPrivateCode", "message": task.FailReason}, "debug": "private-opaque-field"})
				}
				require.NoError(t, model.DB.Create(&task).Error)
				for _, prefix := range []string{"/api/v3/contents/generations/tasks/", "/doubao/api/v3/contents/generations/tasks/"} {
					status, _, body := f.request(t, http.MethodGet, prefix+task.TaskID, "assete2eotherkey", nil)
					require.Equal(t, http.StatusOK, status, string(body))
					var response map[string]any
					require.NoError(t, common.Unmarshal(body, &response))
					assert.Equal(t, task.TaskID, response["id"])
					assert.Equal(t, "public-video-alias", response["model"])
					assert.Equal(t, tc.want, response["status"])
					assert.Equal(t, float64(100), response["created_at"])
					assert.Equal(t, float64(200), response["updated_at"])
					assert.NotContains(t, string(body), "private-upstream-id")
					assert.NotContains(t, string(body), "private-deployment")
					if tc.state == model.TaskStatusFailure {
						assert.NotContains(t, string(body), "provider-private-diagnostic")
						assert.NotContains(t, string(body), "private-opaque-field")
						assert.Contains(t, response, "error")
					} else {
						assert.Equal(t, float64(0), response["seed"])
						assert.Equal(t, false, response["generate_audio"])
						assert.Equal(t, map[string]any{"completion_tokens": float64(321), "total_tokens": float64(321), "tool_usage": map[string]any{"web_search": float64(1)}}, response["usage"])
						for field, want := range map[string]any{
							"resolution": "480p", "ratio": "16:9", "duration": float64(4), "framespersecond": float64(24),
							"tools": []any{map[string]any{"type": "web_search"}}, "safety_identifier": "user-123",
							"priority": float64(0), "draft": false, "draft_task_id": "task_draft", "service_tier": "default", "execution_expires_after": float64(172800),
						} {
							assert.Equal(t, want, response[field], field)
						}
						if tc.state == model.TaskStatusSuccess {
							assert.Equal(t, map[string]any{"video_url": task.PrivateData.ResultURL, "last_frame_url": "https://cdn.example.com/last.png"}, response["content"])
						}
					}
					// An administrator is still not allowed to query another account's task via a user token.
					status, _, body = f.request(t, http.MethodGet, prefix+task.TaskID, "assete2euserkey", nil)
					assert.Contains(t, []int{http.StatusBadRequest, http.StatusNotFound}, status, string(body))
					assert.NotContains(t, string(body), "final.mp4")
				}
			})
		}
	}
}

func TestDoubaoPluginDraftReferencesPinOwnedHistoricalChannel(t *testing.T) {
	f := setupAssetStorageE2E(t)
	var original model.Channel
	require.NoError(t, model.DB.Where("name = ?", "asset-e2e-video").First(&original).Error)
	var submitted atomic.Int32
	var received chan map[string]any = make(chan map[string]any, 2)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := common.DecodeJson(r.Body, &body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		received <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"draft-result-%d"}`, submitted.Add(1))
	}))
	t.Cleanup(provider.Close)
	require.NoError(t, model.DB.Model(&original).Update("base_url", provider.URL).Error)
	otherChannel := model.Channel{Type: original.Type, Name: "higher-priority-non-origin", Key: "unused", Status: common.ChannelStatusEnabled,
		BaseURL: common.GetPointer(f.upstream.URL), Models: original.Models, Group: "default", Priority: common.GetPointer(int64(100))}
	require.NoError(t, model.DB.Create(&otherChannel).Error)
	require.NoError(t, otherChannel.AddAbilities(nil))
	model.InitChannelCache()
	origin := model.Task{TaskID: "task_owned_draft", UserId: f.user.Id, ChannelId: original.Id,
		Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeDoubaoVideo)), Status: model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "provider-owned-draft"}}
	require.NoError(t, model.DB.Create(&origin).Error)
	wrongPlatform := model.Task{TaskID: "task_wrong_platform", UserId: f.user.Id, ChannelId: original.Id, Platform: "sora"}
	require.NoError(t, model.DB.Create(&wrongPlatform).Error)
	otherOrigin := model.Task{TaskID: "task_other_channel", UserId: f.user.Id, ChannelId: otherChannel.Id, Platform: "doubao"}
	require.NoError(t, model.DB.Create(&otherOrigin).Error)
	for _, path := range []string{"/api/v3/contents/generations/tasks", "/doubao/api/v3/contents/generations/tasks"} {
		t.Run(path, func(t *testing.T) {
			for _, tc := range []struct {
				name, token string
				ids         []string
				want        int
			}{
				{"owned historical", "assete2euserkey", []string{origin.TaskID}, http.StatusOK},
				{"foreign account", "assete2eotherkey", []string{origin.TaskID}, http.StatusBadRequest},
				{"unknown draft", "assete2euserkey", []string{"task_unknown"}, http.StatusBadRequest},
				{"wrong platform", "assete2euserkey", []string{wrongPlatform.TaskID}, http.StatusBadRequest},
				{"mixed channels", "assete2euserkey", []string{origin.TaskID, otherOrigin.TaskID}, http.StatusBadRequest},
			} {
				t.Run(tc.name, func(t *testing.T) {
					content := []any{map[string]any{"type": "text", "text": "Finish this draft"}}
					for _, id := range tc.ids {
						content = append(content, map[string]any{"type": "draft_task", "draft_task": map[string]any{"id": id}})
					}
					before := submitted.Load()
					status, _, body := f.request(t, http.MethodPost, path, tc.token, map[string]any{"model": original.Models, "content": content, "duration": 4})
					require.Equal(t, tc.want, status, string(body))
					if tc.want != http.StatusOK {
						assert.Equal(t, before, submitted.Load())
						return
					}
					require.Equal(t, before+1, submitted.Load())
					payload := <-received
					require.Equal(t, []any{content[0], map[string]any{"type": "draft_task", "draft_task": map[string]any{"id": "provider-owned-draft"}}}, payload["content"])
					var result struct{ ID string }
					require.NoError(t, common.Unmarshal(body, &result))
					var task model.Task
					require.NoError(t, model.DB.Where("task_id = ?", result.ID).First(&task).Error)
					assert.Equal(t, original.Id, task.ChannelId)
					assert.Equal(t, constant.TaskPlatform("doubao"), task.Platform)
				})
			}
		})
	}
	f.state.mu.Lock()
	assert.Empty(t, f.state.requests, "no request may reach the non-origin channel")
	f.state.mu.Unlock()
}
