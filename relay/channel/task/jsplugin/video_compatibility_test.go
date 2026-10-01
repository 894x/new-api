package jsplugin

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWanVideoCompatibilitySupportsMoyu(t *testing.T) {
	source, err := plugins.Source("moyu-wan3")
	require.NoError(t, err)
	registry := pluginruntime.NewRegistry()
	plugin, err := registry.RegisterFactory(source, pluginruntime.Options{Key: "moyu-wan3"})
	require.NoError(t, err)
	for _, route := range []struct{ method, path string }{
		{http.MethodPost, "/moyu-wan3/api/v1/services/aigc/video-generation/video-synthesis"},
		{http.MethodGet, "/moyu-wan3/api/v1/tasks/:task_id"},
	} {
		_, registered := registry.Generation().LookupDeclaredRoute(route.method, route.path)
		assert.False(t, registered, "plugin-specific route must not be exposed: %s %s", route.method, route.path)
	}
	adaptor := New(plugin)
	require.True(t, adaptor.SupportsNativeTaskFormat(constant.TaskResponseFormatAliVideo))
	bindings := registry.Generation().LookupEndpointCandidates(http.MethodPost, "/api/v1/services/aigc/video-generation/video-synthesis", "wan3.0-video")
	require.Len(t, bindings, 1)
	assert.Equal(t, pluginruntime.ProtocolWanVideo, bindings[0].Protocol)
	info := &relaycommon.RelayInfo{
		OriginModelName: "wan3.0-video",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://www.moyu.info", ApiKey: "test-key",
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	adaptor.Init(info)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/services/aigc/video-generation/video-synthesis", strings.NewReader(`{
		"model":"wan3.0-video","prompt":"a tracking shot",
		"metadata":{"model":"wan3.0-video","input":{"prompt":"a tracking shot"},
		"parameters":{"resolution":"720P","ratio":"16:9","duration":5,"seed":0,"watermark":false}}
	}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(ctx, constant.ContextKeyTaskResponseFormat, constant.TaskResponseFormatAliVideo)
	ctx.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Generation: registry.Generation(), Plugin: plugin})
	require.Nil(t, adaptor.ValidateRequestAndSetAction(ctx, info))
	requestURL, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://www.moyu.info/v1/video/generations", requestURL)
	reader, err := adaptor.BuildRequestBody(ctx, info)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"wan3.0-video","prompt":"a tracking shot","resolution":"720P","ratio":"16:9","duration":5,"seed":0,"watermark":false}`, string(body))
	pinnedValue, exists := ctx.Get(pluginruntime.ContextKeyPinnedRoute)
	require.True(t, exists)
	pinned := pinnedValue.(pluginruntime.PinnedRoute)
	submitted, err := pinned.Route.CallHook(ctx.Request.Context(), plugin.Engine, pinned.Route.Render, map[string]any{}, map[string]any{"task_id": "public-task"})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"request_id": "", "output": map[string]any{"task_id": "public-task", "task_status": "PENDING"}}, submitted)

	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/tasks/public-task", nil)
	rendered, err := adaptor.RenderNativeTask(ctx, constant.TaskResponseFormatAliVideo, &model.Task{
		TaskID: "public-task", Platform: "moyu-wan3", Status: model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "provider-task"},
		Data:        []byte(`{"code":"success","data":{"task_id":"provider-task","status":"SUCCESS","result_url":"https://cdn.example/video.mp4"}}`),
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"request_id":"","output":{"task_id":"public-task","task_status":"SUCCEEDED","video_url":"https://cdn.example/video.mp4"}}`, string(rendered))

	ctx, _ = gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/services/aigc/video-generation/video-synthesis", strings.NewReader(`{
		"model":"wan3.0-video","metadata":{"model":"wan3.0-video","input":{"prompt":"x"},"parameters":{"duration":31}}
	}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(ctx, constant.ContextKeyTaskResponseFormat, constant.TaskResponseFormatAliVideo)
	ctx.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Generation: registry.Generation(), Plugin: plugin})
	taskErr := New(plugin).ValidateRequestAndSetAction(ctx, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
}

func TestWanVideoProtocolClaimsAndRequiredHooks(t *testing.T) {
	source, err := plugins.Source("moyu-wan3")
	require.NoError(t, err)
	source = strings.ReplaceAll(source, "\r\n", "\n")
	renamed := strings.Replace(source, `key: "moyu-wan3"`, `key: "custom-wan"`, 1)
	plugin, err := pluginruntime.CompilePlugin(renamed, pluginruntime.Options{})
	require.NoError(t, err)
	assert.True(t, New(plugin).SupportsNativeTaskFormat(constant.TaskResponseFormatAliVideo), "a declared protocol must work regardless of the plugin key")
	assert.False(t, New(plugin).SupportsNativeTaskFormat(constant.TaskResponseFormatDoubaoVideo))

	for _, member := range []string{"decodeRequest: createVideoTask", "renderSubmitted: native.taskCreated", "render: native.taskStatus"} {
		t.Run(member, func(t *testing.T) {
			name, _, _ := strings.Cut(member, ":")
			_, err := pluginruntime.CompilePlugin(strings.Replace(source, member, name+": null", 1), pluginruntime.Options{})
			require.ErrorContains(t, err, "is missing hook")
		})
	}
	unclaimed := strings.Replace(source, `  protocols: ["wan_video"],`+"\n", "", 1)
	unclaimed = strings.Replace(unclaimed, `export const protocols = {
  wan_video: {
    decodeRequest: createVideoTask,
    renderSubmitted: native.taskCreated,
    render: native.taskStatus,
  },
};`, "", 1)
	plugin, err = pluginruntime.CompilePlugin(unclaimed, pluginruntime.Options{})
	require.NoError(t, err)
	assert.False(t, New(plugin).SupportsNativeTaskFormat(constant.TaskResponseFormatAliVideo), "native hooks alone must not claim a shared protocol")

	alibabaSource, err := plugins.Source("alibaba")
	require.NoError(t, err)
	plugin, err = pluginruntime.CompilePlugin(alibabaSource, pluginruntime.Options{})
	require.NoError(t, err)
	route, supported := New(plugin).nativeCompatibilityRoute(constant.TaskResponseFormatAliVideo, http.MethodPost)
	require.True(t, supported)
	assert.Equal(t, pluginruntime.ProtocolWanVideo, route.Protocol)
}

func TestWanVideoProtocolModelScopeAndAliases(t *testing.T) {
	source, err := plugins.Source("moyu-wan3")
	require.NoError(t, err)
	source = strings.Replace(source, `protocols: ["wan_video"]`, `protocols: [{name: "wan_video", models: ["wan3.0-video"]}]`, 1)
	registry := pluginruntime.NewRegistry()
	plugin, err := registry.RegisterFactory(source, pluginruntime.Options{})
	require.NoError(t, err)
	for _, test := range []struct {
		name, model, upstreamModel string
		allowed                    bool
	}{
		{"declared", "wan3.0-video", "wan3.0-video", true},
		{"excluded", "wan3.0-video-prime", "wan3.0-video-prime", false},
		{"mapped alias", "wan-video-public", "wan3.0-video", true},
		{"alias to excluded model", "wan-video-public", "wan3.0-video-prime", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			body, err := common.Marshal(map[string]any{"metadata": map[string]any{"model": test.model, "input": map[string]any{"prompt": "x"}}})
			require.NoError(t, err)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/services/aigc/video-generation/video-synthesis", strings.NewReader(string(body)))
			ctx.Request.Header.Set("Content-Type", "application/json")
			ctx.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Generation: registry.Generation(), Plugin: plugin})
			info := &relaycommon.RelayInfo{OriginModelName: test.model, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: test.upstreamModel}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
			taskErr := New(plugin).prepareNativeCompatibilityRequest(ctx, info, constant.TaskResponseFormatAliVideo)
			if test.allowed {
				require.Nil(t, taskErr)
				request, _ := ctx.Get("task_request")
				assert.Equal(t, test.model, request.(map[string]any)["model"])
			} else {
				require.NotNil(t, taskErr)
				assert.Equal(t, "invalid_api_platform", taskErr.Code)
				assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
			}
		})
	}
}

func TestOpenAIVideoCompatibilityUsesHostArtifactAccess(t *testing.T) {
	previousSecret, previousAddress := common.CryptoSecret, system_setting.TaskPublicAddress
	common.CryptoSecret, system_setting.TaskPublicAddress = "video-compatibility-test-secret", "https://gateway.example/prefix"
	t.Cleanup(func() { common.CryptoSecret, system_setting.TaskPublicAddress = previousSecret, previousAddress })
	source := strings.Replace(mockPlugin, `export function listArtifacts() { return []; }`, `export function listArtifacts(task) {
  if (task.status !== "SUCCESS") throw new Error("unfinished artifact queried");
  return [{key:"video",type:"video"}];
}`, 1)
	plugin, err := pluginruntime.NewRegistry().Register(source, pluginruntime.Options{})
	require.NoError(t, err)
	adaptor := New(plugin)
	for _, status := range []model.TaskStatus{model.TaskStatusInProgress, model.TaskStatusFailure, model.TaskStatusSuccess} {
		t.Run(string(status), func(t *testing.T) {
			task := &model.Task{TaskID: "public-task", Status: status, PrivateData: model.TaskPrivateData{
				UpstreamTaskID: "private-task", ResultURL: "https://provider.example/private-task.mp4?secret=private",
			}}
			encoded, err := adaptor.ConvertToOpenAIVideo(task)
			require.NoError(t, err)
			var video dto.OpenAIVideo
			require.NoError(t, common.Unmarshal(encoded, &video))
			assert.Equal(t, task.TaskID, video.ID)
			assert.Empty(t, video.TaskID)
			assert.NotContains(t, string(encoded), "provider.example")
			assert.NotContains(t, string(encoded), "private-task")
			if status != model.TaskStatusSuccess {
				assert.Empty(t, video.Metadata)
				return
			}
			contentURL, ok := video.Metadata["url"].(string)
			require.True(t, ok)
			parsed, err := url.Parse(contentURL)
			require.NoError(t, err)
			assert.Equal(t, "gateway.example", parsed.Host)
			assert.Equal(t, "/prefix/v1/tasks/public-task/artifacts/video/content", parsed.Path)
			access := parsed.Query().Get(service.TaskArtifactAccessQueryParameter)
			assert.True(t, service.VerifyTaskArtifactAccess(access, task.TaskID, "video"))
			assert.False(t, service.VerifyTaskArtifactAccess(access, "other-task", "video"))
			assert.False(t, service.VerifyTaskArtifactAccess(access, task.TaskID, "last_frame"))
		})
	}
	system_setting.TaskPublicAddress = "https://gateway.example?invalid=true"
	_, err = adaptor.ConvertToOpenAIVideo(&model.Task{TaskID: "public-task", Status: model.TaskStatusSuccess})
	require.ErrorContains(t, err, "build video content URL")
}

func TestArtifactHooksReceiveLegacyResultURLOnlyOnSuccess(t *testing.T) {
	source := strings.Replace(mockPlugin, `export function listArtifacts() { return []; }`, `export function listArtifacts(task) {
  const expected = task.status === "SUCCESS" ? "https://legacy.example/video.mp4" : "";
  if (task.resultUrl !== expected) throw new Error("unexpected legacy result URL");
  return task.resultUrl ? [{key:"video",type:"video"}] : [];
}`, 1)
	plugin, err := pluginruntime.NewRegistry().Register(source, pluginruntime.Options{})
	require.NoError(t, err)
	for _, status := range []model.TaskStatus{model.TaskStatusInProgress, model.TaskStatusFailure, model.TaskStatusSuccess} {
		t.Run(string(status), func(t *testing.T) {
			artifacts, err := New(plugin).ListArtifacts(&model.Task{
				TaskID: "public-task", Status: status, FailReason: "https://legacy.example/video.mp4",
			})
			require.NoError(t, err)
			if status == model.TaskStatusSuccess {
				require.Len(t, artifacts, 1)
				assert.Equal(t, "video", artifacts[0].Key)
			} else {
				assert.Empty(t, artifacts)
			}
		})
	}
}
