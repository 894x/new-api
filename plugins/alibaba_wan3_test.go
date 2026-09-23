package plugins_test

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func alibabaPlugin(t *testing.T) *pluginruntime.LoadedPlugin {
	t.Helper()
	source, err := builtinplugins.Source("alibaba")
	require.NoError(t, err)
	plugin, err := pluginruntime.NewRegistry().RegisterFactory(source, pluginruntime.Options{Key: "alibaba"})
	require.NoError(t, err)
	return plugin
}

func TestWan3CompatibilityProtocolsUseAutomaticDurationAndSize(t *testing.T) {
	plugin := alibabaPlugin(t)
	for _, tc := range []struct{ protocol, request string }{
		{"openai_responses", `{"input":"animate","duration":-1,"size":"1280*720"}`},
		{"openai_video", `{"prompt":"animate","seconds":-1,"size":"1280*720"}`},
		{"openai_video", `{"prompt":"animate","auto_duration":true,"size":"1280*720"}`},
		{"openai_responses", `{"input":"animate","metadata":{"parameters":{"duration":-1}},"size":"1280*720"}`},
	} {
		t.Run(tc.protocol+tc.request, func(t *testing.T) {
			var request map[string]any
			require.NoError(t, common.UnmarshalJsonStr(tc.request, &request))
			intent, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{tc.protocol, "decodeRequest"}, map[string]any{"model": "my-wan", "upstreamModel": "wan3.0-video", "body": map[string]any{"kind": "json", "value": request}})
			require.NoError(t, err)
			ctx := map[string]any{"model": "my-wan", "upstreamModel": "wan3.0-video", "modelMappingResolved": true, "requestBody": intent.(map[string]any)["requestBody"]}
			value, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
			require.NoError(t, err)
			encoded, err := common.Marshal(value.(map[string]any)["body"])
			require.NoError(t, err)
			assert.JSONEq(t, `{"model":"wan3.0-video","input":{"prompt":"animate"},"parameters":{"duration":-1,"resolution":"720P","ratio":"16:9","prompt_extend":true}}`, string(encoded))
			usage, err := plugin.Engine.Call(t.Context(), "extractUsage", ctx)
			require.NoError(t, err)
			encoded, err = common.Marshal(usage)
			require.NoError(t, err)
			assert.JSONEq(t, `{"seconds":30,"resolution":"720P"}`, string(encoded))
			ctx["upstreamModel"] = "wan2.7-t2v"
			_, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
			require.ErrorContains(t, err, "smart duration")
		})
	}
	_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{"upstreamModel": "wan3.0-video", "requestBody": map[string]any{"prompt": "animate", "size": "1000*1000"}})
	require.ErrorContains(t, err, "invalid size")
}

func TestWan3ResponsesImageOnlyUsesMappedModel(t *testing.T) {
	plugin := alibabaPlugin(t)
	for _, model := range []string{"wan3.0-video", "wan3.0-video-prime", "wan2.7-t2v"} {
		t.Run(model, func(t *testing.T) {
			value, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_responses", "decodeRequest"}, map[string]any{
				"model": "my-wan", "upstreamModel": model, "body": map[string]any{"kind": "json", "value": map[string]any{"input": []any{map[string]any{"type": "input_image", "image_url": "https://cdn.example/frame.png"}}}},
			})
			if model == "wan2.7-t2v" {
				require.ErrorContains(t, err, "input is required")
				return
			}
			require.NoError(t, err)
			intent := value.(map[string]any)
			assert.Equal(t, "image_to_video", intent["action"])
			value, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{"model": "my-wan", "upstreamModel": model, "modelMappingResolved": true, "requestBody": intent["requestBody"]})
			require.NoError(t, err)
			encoded, err := common.Marshal(value.(map[string]any)["body"].(map[string]any)["input"])
			require.NoError(t, err)
			assert.JSONEq(t, `{"prompt":"","media":[{"type":"first_frame","url":"https://cdn.example/frame.png"}]}`, string(encoded))
		})
	}
}

func TestWanCompletionUsesProviderResolutionAndCombinedDuration(t *testing.T) {
	plugin := alibabaPlugin(t)
	for _, tc := range []struct{ body, want string }{
		{`{"usage":{"duration":7.5,"output_video_duration":7.5,"SR":720}}`, `{"seconds":7.5,"resolution":"720P"}`},
		{`{"usage":{"input_video_duration":8,"output_video_duration":7.5,"SR":1080}}`, `{"seconds":15.5,"resolution":"1080P"}`},
		{`{"usage":{"input_video_duration":25,"output_video_duration":15,"SR":480}}`, `{"seconds":40,"resolution":"480P"}`},
		{`{"output":{"duration":5,"resolution":"1080p"}}`, `{"seconds":5,"resolution":"1080P"}`},
	} {
		t.Run(tc.body, func(t *testing.T) {
			var body map[string]any
			require.NoError(t, common.UnmarshalJsonStr(tc.body, &body))
			value, err := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", map[string]any{}, map[string]any{}, body)
			require.NoError(t, err)
			encoded, err := common.Marshal(value)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(encoded))
		})
	}
}

func TestWan3NativePluginPreservesPayloadAndMappedModels(t *testing.T) {
	plugin := alibabaPlugin(t)
	const body = `{"model":"customer-wan","input":{"media":[{"type":"reference_video","url":"https://cdn.example/input.mp4"}],"future_input":false},"parameters":{"duration":-1,"resolution":"1080P","ratio":"adaptive","seed":0,"audio":false,"watermark":false,"prompt_extend":false},"future_option":{"enabled":false,"count":0}}`
	var request map[string]any
	require.NoError(t, common.UnmarshalJsonStr(body, &request))
	intent, err := plugin.Engine.CallMember(context.Background(), "native", "createVideoTask", map[string]any{"body": map[string]any{"kind": "json", "value": request}})
	require.NoError(t, err)
	canonical := intent.(map[string]any)["requestBody"]
	ctx := map[string]any{"model": "customer-wan", "upstreamModel": "wan3.0-video-prime", "requestBody": canonical, "baseUrl": "https://provider.example", "apiKey": "submit-key", "modelMappingResolved": true}
	value, err := plugin.Engine.Call(context.Background(), "buildSubmitRequest", ctx)
	require.NoError(t, err)
	descriptor := value.(map[string]any)
	expected := map[string]any{}
	require.NoError(t, common.UnmarshalJsonStr(body, &expected))
	expected["model"] = "wan3.0-video-prime"
	expectedJSON, err := common.Marshal(expected)
	require.NoError(t, err)
	bodyJSON, err := common.Marshal(descriptor["body"])
	require.NoError(t, err)
	assert.JSONEq(t, string(expectedJSON), string(bodyJSON))
	assert.Equal(t, map[string]any{"Authorization": "Bearer submit-key", "Content-Type": "application/json", "X-DashScope-Async": "enable"}, descriptor["headers"])
	assert.Equal(t, "https://provider.example/api/v1/services/aigc/video-generation/video-synthesis", descriptor["url"])
	assert.Equal(t, "image_to_video", descriptor["action"])
	ctx["usagePurpose"] = "billing_ratios"
	ratios, err := plugin.Engine.Call(context.Background(), "extractUsage", ctx)
	require.NoError(t, err)
	encoded, err := common.Marshal(ratios)
	require.NoError(t, err)
	assert.JSONEq(t, `{"seconds":30,"resolution-1080P":4}`, string(encoded))
	ctx["upstreamModel"] = "wan2.7-t2v"
	_, err = plugin.Engine.Call(context.Background(), "buildSubmitRequest", ctx)
	require.Error(t, err)
}

func TestAlibabaNativeCreationPreservesProviderFieldsAndPublicIdentity(t *testing.T) {
	plugin := alibabaPlugin(t)
	response := map[string]any{"request_id": "request-1", "future_field": false, "output": map[string]any{"task_id": "provider-private", "task_status": "RUNNING", "queued_count": 0}}
	parsed, err := plugin.Engine.Call(context.Background(), "parseSubmitResponse", map[string]any{"publicTaskId": "task_public"}, map[string]any{"body": response})
	require.NoError(t, err)
	result := parsed.(map[string]any)
	assert.Equal(t, "provider-private", result["taskId"])
	view, err := plugin.Engine.CallMember(context.Background(), "native", "taskCreated", map[string]any{}, map[string]any{"task_id": "task_public", "data": result["taskData"]})
	require.NoError(t, err)
	body, err := common.Marshal(view)
	require.NoError(t, err)
	assert.JSONEq(t, `{"request_id":"request-1","future_field":false,"output":{"task_id":"task_public","task_status":"RUNNING","queued_count":0}}`, string(body))
	assert.Equal(t, "provider-private", response["output"].(map[string]any)["task_id"])
	failed, err := plugin.Engine.Call(context.Background(), "parseSubmitResponse", map[string]any{}, map[string]any{"statusCode": 200, "body": map[string]any{"code": "InvalidInput", "message": "provider rejected"}})
	require.NoError(t, err)
	failure, err := common.Marshal(failed)
	require.NoError(t, err)
	assert.JSONEq(t, `{"error":{"code":"ali_api_error","message":"InvalidInput: provider rejected","httpStatus":502}}`, string(failure))
}

func TestWan3NativeNullDurationUsesOfficialDefaultReservationAndPayload(t *testing.T) {
	plugin := alibabaPlugin(t)
	request := map[string]any{"model": "wan3.0-video", "input": map[string]any{"prompt": "animate"}, "parameters": map[string]any{"duration": nil, "resolution": nil}}
	intent, err := plugin.Engine.CallMember(context.Background(), "native", "createVideoTask", map[string]any{"body": map[string]any{"kind": "json", "value": request}})
	require.NoError(t, err)
	ctx := map[string]any{"model": "wan3.0-video", "upstreamModel": "wan3.0-video", "modelMappingResolved": true, "requestBody": intent.(map[string]any)["requestBody"], "usagePurpose": "billing_ratios"}
	value, err := plugin.Engine.Call(context.Background(), "extractUsage", ctx)
	require.NoError(t, err)
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	assert.JSONEq(t, `{"seconds":5,"resolution-1080P":4}`, string(encoded))
	descriptor, err := plugin.Engine.Call(context.Background(), "buildSubmitRequest", ctx)
	require.NoError(t, err)
	payload, err := common.Marshal(descriptor.(map[string]any)["body"])
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"wan3.0-video","input":{"prompt":"animate"},"parameters":{"duration":5,"resolution":"1080P","ratio":"adaptive","prompt_extend":true}}`, string(payload))
}

func TestWan3NativePluginRejectsInvalidQuantitiesBeforeSubmit(t *testing.T) {
	plugin := alibabaPlugin(t)
	for _, tc := range []struct {
		name  string
		patch map[string]any
	}{
		{"negative", map[string]any{"duration": -2}}, {"zero", map[string]any{"duration": 0}},
		{"too short", map[string]any{"duration": 1}}, {"too long", map[string]any{"duration": 31}},
		{"huge", map[string]any{"duration": 1e30}}, {"fraction", map[string]any{"duration": 2.5}},
		{"boolean", map[string]any{"duration": true}}, {"array", map[string]any{"duration": []any{5}}},
		{"resolution", map[string]any{"resolution": "4K"}},
		{"seed", map[string]any{"seed": 2147483648.0}}, {"ratio", map[string]any{"ratio": "2:1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := map[string]any{"model": "wan3.0-video", "input": map[string]any{"prompt": "animate"}, "parameters": tc.patch}
			intent, err := plugin.Engine.CallMember(context.Background(), "native", "createVideoTask", map[string]any{"body": map[string]any{"kind": "json", "value": request}})
			require.NoError(t, err)
			_, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{"model": "wan3.0-video", "upstreamModel": "wan3.0-video", "requestBody": intent.(map[string]any)["requestBody"]})
			require.Error(t, err)
		})
	}
}

func TestWanPluginSharedMediaDefaultsAndOptionalZeros(t *testing.T) {
	plugin := alibabaPlugin(t)
	for _, tc := range []struct{ name, model, input, expected string }{
		{"wan3 frames", "wan3.0-video", `{"prompt":"animate","images":["first.png","last.png"],"metadata":{"parameters":{"seed":0,"audio":false,"watermark":false,"prompt_extend":false}}}`, `{"model":"wan3.0-video","input":{"prompt":"animate","media":[{"type":"first_frame","url":"first.png"},{"type":"last_frame","url":"last.png"}]},"parameters":{"duration":5,"resolution":"1080P","ratio":"adaptive","seed":0,"audio":false,"watermark":false,"prompt_extend":false}}`},
		{"wan27 frames", "wan2.7-i2v", `{"prompt":"animate","images":["first.png","last.png"],"metadata":{"input":{"audio_url":"voice.mp3"},"parameters":{"seed":0,"watermark":false,"prompt_extend":false}}}`, `{"model":"wan2.7-i2v","input":{"prompt":"animate","media":[{"type":"first_frame","url":"first.png"},{"type":"last_frame","url":"last.png"},{"type":"driving_audio","url":"voice.mp3"}]},"parameters":{"duration":5,"resolution":"1080P","seed":0,"watermark":false,"prompt_extend":false}}`},
		{"wan25 legacy image", "wan2.5-i2v-preview", `{"prompt":"animate","image":"preferred.png","images":["other.png"]}`, `{"model":"wan2.5-i2v-preview","input":{"prompt":"animate","img_url":"preferred.png"},"parameters":{"duration":5,"resolution":"1080P","prompt_extend":true}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var request any
			require.NoError(t, common.UnmarshalJsonStr(tc.input, &request))
			value, err := plugin.Engine.Call(context.Background(), "buildSubmitRequest", map[string]any{"model": tc.model, "upstreamModel": tc.model, "baseUrl": "https://provider.example", "requestBody": request})
			require.NoError(t, err)
			body, err := common.Marshal(value.(map[string]any)["body"])
			require.NoError(t, err)
			assert.JSONEq(t, tc.expected, string(body))
		})
	}
}

func TestWan27PluginPreservesMediaSelection(t *testing.T) {
	plugin := alibabaPlugin(t)
	for _, tc := range []struct{ name, request, media string }{
		{"direct image wins", `{"image":" direct.png ","images":["other.png"," last.png "],"input_reference":"ref.png"}`, `[{"type":"first_frame","url":"direct.png"},{"type":"last_frame","url":"last.png"}]`},
		{"skip empty images", `{"image":" ","images":[" "," first.png "," last.png "],"input_reference":"ref.png"}`, `[{"type":"first_frame","url":"first.png"},{"type":"last_frame","url":"last.png"}]`},
		{"input reference fallback", `{"image":" ","images":[" "],"input_reference":" ref.png "}`, `[{"type":"first_frame","url":"ref.png"}]`},
		{"explicit media wins", `{"image":"direct.png","images":["first.png","last.png"],"metadata":{"input":{"media":[{"type":"first_clip","url":"clip.mp4"}]}}}`, `[{"type":"first_clip","url":"clip.mp4"}]`},
		{"explicit frame fields", `{"image":"direct.png","metadata":{"input":{"first_frame_url":"first.png","last_frame_url":"last.png","audio_url":"voice.mp3"}}}`, `[{"type":"first_frame","url":"first.png"},{"type":"last_frame","url":"last.png"},{"type":"driving_audio","url":"voice.mp3"}]`},
		{"image required", `{}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var req map[string]any
			require.NoError(t, common.UnmarshalJsonStr(tc.request, &req))
			req["prompt"] = "animate"
			req["size"], req["duration"] = "720p", 10
			value, err := plugin.Engine.Call(context.Background(), "buildSubmitRequest", map[string]any{"model": "wan2.7-i2v", "upstreamModel": "wan2.7-i2v", "requestBody": req})
			if tc.media == "" {
				require.ErrorContains(t, err, "requires first_frame")
				return
			}
			require.NoError(t, err)
			body, err := common.Marshal(value.(map[string]any)["body"])
			require.NoError(t, err)
			assert.JSONEq(t, `{"model":"wan2.7-i2v","input":{"prompt":"animate","media":`+tc.media+`},"parameters":{"duration":10,"resolution":"720P","prompt_extend":true}}`, string(body))
		})
	}
}

func TestAlibabaPluginPreservesPollingStatusAndFailureDetails(t *testing.T) {
	plugin := alibabaPlugin(t)
	for _, tc := range []struct{ status, body, expected string }{
		{"PENDING", `{}`, `{"status":"QUEUED"}`},
		{"RUNNING", `{}`, `{"status":"IN_PROGRESS"}`},
		{"unrecognized", `{}`, `{"status":"UNKNOWN","reason":"unrecognized status: unrecognized"}`},
		{"FAILED", `{"message":"top level failure","output":{"code":"InvalidInput","message":"nested failure"}}`, `{"status":"FAILURE","reason":"top level failure","usage":null}`},
		{"CANCELED", `{"output":{"code":"Canceled","message":"cancelled by provider"}}`, `{"status":"FAILURE","reason":"task failed, code: Canceled , message: cancelled by provider","usage":null}`},
		{"UNKNOWN", `{}`, `{"status":"FAILURE","reason":"task failed","usage":null}`},
	} {
		t.Run(tc.status, func(t *testing.T) {
			var body map[string]any
			require.NoError(t, common.UnmarshalJsonStr(tc.body, &body))
			output, ok := body["output"].(map[string]any)
			if !ok {
				output = map[string]any{}
				body["output"] = output
			}
			output["task_status"] = tc.status
			value, err := plugin.Engine.Call(context.Background(), "parseTaskResult", map[string]any{}, body)
			require.NoError(t, err)
			encoded, err := common.Marshal(value)
			require.NoError(t, err)
			assert.JSONEq(t, tc.expected, string(encoded))
		})
	}
}

func TestWanPluginRetainsResolutionPricing(t *testing.T) {
	plugin := alibabaPlugin(t)
	for _, tc := range []struct {
		model, resolution string
		ratio             float64
	}{
		{"wan3.0-video", "480P", 1}, {"wan3.0-video", "720P", 2}, {"wan3.0-video", "1080P", 4},
		{"wan3.0-video-prime", "480P", 1}, {"wan3.0-video-prime", "720P", 2}, {"wan3.0-video-prime", "1080P", 4},
		{"wan2.5-i2v-preview", "480P", 1}, {"wan2.5-i2v-preview", "720P", 2}, {"wan2.5-i2v-preview", "1080P", 1 / 0.3},
		{"wan2.2-i2v-plus", "480P", 1}, {"wan2.2-i2v-plus", "1080P", 5},
		{"wan2.2-i2v-flash", "480P", 1}, {"wan2.2-i2v-flash", "720P", 2},
	} {
		t.Run(tc.model+"/"+tc.resolution, func(t *testing.T) {
			value, err := plugin.Engine.Call(context.Background(), "extractUsage", map[string]any{
				"model": tc.model, "upstreamModel": tc.model, "usagePurpose": "billing_ratios",
				"requestBody": map[string]any{"prompt": "animate", "image": "https://cdn.example/first.png", "duration": 5, "size": tc.resolution},
			})
			require.NoError(t, err)
			encoded, err := common.Marshal(value)
			require.NoError(t, err)
			var ratios map[string]float64
			require.NoError(t, common.Unmarshal(encoded, &ratios))
			assert.Equal(t, map[string]float64{"seconds": 5, "resolution-" + tc.resolution: tc.ratio}, ratios)
		})
	}
}

func TestAlibabaWan3(t *testing.T) {
	plugin := alibabaPlugin(t)

	roundTrip := func(t *testing.T, value any) map[string]any {
		encoded, marshalErr := common.Marshal(value)
		require.NoError(t, marshalErr)
		var decoded map[string]any
		require.NoError(t, common.Unmarshal(encoded, &decoded))
		return decoded
	}
	submitCtx := func(model, upstream string, body map[string]any) map[string]any {
		return map[string]any{"model": model, "upstreamModel": upstream, "baseUrl": "https://dashscope.aliyuncs.com", "apiKey": "k", "requestBody": body}
	}
	usageCtx := func(purpose string, body map[string]any) map[string]any {
		return map[string]any{"model": "wan3.0-video", "upstreamModel": "wan3.0-video", "usagePurpose": purpose, "requestBody": body}
	}
	decodeResponses := func(model string, body map[string]any) (map[string]any, error) {
		value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_responses", "decodeRequest"}, map[string]any{"model": model, "body": map[string]any{"kind": "json", "value": body}, "stream": false})
		if callErr != nil {
			return nil, callErr
		}
		return roundTrip(t, value), nil
	}

	t.Run("duration -1 becomes the auto_duration marker so the host accepts the body", func(t *testing.T) {
		resolved, callErr := decodeResponses("wan3.0-video", map[string]any{"model": "wan3.0-video", "input": "a cat", "duration": -1})
		require.NoError(t, callErr)
		requestBody := resolved["requestBody"].(map[string]any)
		assert.Equal(t, true, requestBody["auto_duration"])
		assert.NotContains(t, requestBody, "duration")

		value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{"model": "wan3.0-video", "body": map[string]any{"kind": "json", "value": map[string]any{"model": "wan3.0-video", "prompt": "a cat", "seconds": -1}}})
		require.NoError(t, callErr)
		requestBody = roundTrip(t, value)["requestBody"].(map[string]any)
		assert.Equal(t, true, requestBody["auto_duration"])
		assert.NotContains(t, requestBody, "seconds")
		assert.NotContains(t, requestBody, "duration")

		value, callErr = plugin.Engine.CallPath(t.Context(), "native", []string{"createVideoTask"}, map[string]any{"body": map[string]any{"kind": "json", "value": map[string]any{
			"model": "wan3.0-video", "input": map[string]any{"prompt": "a cat"}, "parameters": map[string]any{"duration": -1, "ratio": "16:9"},
		}}})
		require.NoError(t, callErr)
		requestBody = roundTrip(t, value)["requestBody"].(map[string]any)
		assert.Equal(t, true, requestBody["auto_duration"])
		assert.NotContains(t, requestBody, "duration")
		parameters := requestBody["metadata"].(map[string]any)["parameters"].(map[string]any)
		assert.Equal(t, "16:9", parameters["ratio"])
		assert.NotContains(t, parameters, "duration")
	})

	t.Run("auto_duration submits -1 upstream and bills 30 seconds up front", func(t *testing.T) {
		body := map[string]any{"model": "wan3.0-video", "prompt": "a cat", "auto_duration": true}
		value, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", submitCtx("wan3.0-video", "wan3.0-video", body))
		require.NoError(t, callErr)
		parameters := roundTrip(t, value)["body"].(map[string]any)["parameters"].(map[string]any)
		assert.Equal(t, float64(-1), parameters["duration"])

		value, callErr = plugin.Engine.Call(t.Context(), "extractUsage", usageCtx("facts", body))
		require.NoError(t, callErr)
		assert.Equal(t, map[string]any{"seconds": float64(30), "resolution": "1080P"}, roundTrip(t, value))

		value, callErr = plugin.Engine.Call(t.Context(), "extractUsage", usageCtx("billing_ratios", body))
		require.NoError(t, callErr)
		assert.Equal(t, float64(30), roundTrip(t, value)["seconds"])

		_, callErr = plugin.Engine.Call(t.Context(), "buildSubmitRequest", submitCtx("wan2.7-t2v", "wan2.7-t2v", map[string]any{"model": "wan2.7-t2v", "prompt": "a cat", "auto_duration": true}))
		require.ErrorContains(t, callErr, "only supported by wan3.0")
	})

	t.Run("channel-mapped alias resolves defaults from the upstream model", func(t *testing.T) {
		direct, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", submitCtx("wan3.0-video", "wan3.0-video", map[string]any{"model": "wan3.0-video", "prompt": "a cat"}))
		require.NoError(t, callErr)
		alias, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", submitCtx("my-wan3", "wan3.0-video", map[string]any{"model": "my-wan3", "prompt": "a cat"}))
		require.NoError(t, callErr)
		assert.Equal(t, roundTrip(t, direct)["body"], roundTrip(t, alias)["body"])
		assert.Equal(t, "1080P", roundTrip(t, alias)["body"].(map[string]any)["parameters"].(map[string]any)["resolution"])
	})

	t.Run("size maps to a resolution tier and unknown sizes are rejected", func(t *testing.T) {
		value, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", submitCtx("wan3.0-video", "wan3.0-video", map[string]any{"model": "wan3.0-video", "prompt": "a cat", "size": "1280*720"}))
		require.NoError(t, callErr)
		parameters := roundTrip(t, value)["body"].(map[string]any)["parameters"].(map[string]any)
		assert.Equal(t, "720P", parameters["resolution"])
		assert.NotContains(t, parameters, "size")
		assert.Equal(t, "16:9", parameters["ratio"])

		_, callErr = plugin.Engine.Call(t.Context(), "buildSubmitRequest", submitCtx("wan3.0-video", "wan3.0-video", map[string]any{"model": "wan3.0-video", "prompt": "a cat", "size": "1000*1000"}))
		require.ErrorContains(t, callErr, "invalid size")
		_, callErr = plugin.Engine.Call(t.Context(), "buildSubmitRequest", submitCtx("wan3.0-video", "wan3.0-video", map[string]any{"model": "wan3.0-video", "prompt": "a cat", "duration": 31}))
		require.ErrorContains(t, callErr, "between 2 and 30")
	})

	t.Run("image-only input stays rejected for t2v models and accepted for wan3.0", func(t *testing.T) {
		imageOnly := []any{map[string]any{"type": "input_image", "image_url": "https://cdn.example/first.png"}}
		_, callErr := decodeResponses("wan2.7-t2v", map[string]any{"model": "wan2.7-t2v", "input": imageOnly})
		require.ErrorContains(t, callErr, "input is required")

		resolved, callErr := decodeResponses("wan3.0-video", map[string]any{"model": "wan3.0-video", "input": imageOnly})
		require.NoError(t, callErr)
		assert.Equal(t, "image_to_video", resolved["action"])

		value, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", submitCtx("wan3.0-video", "wan3.0-video", map[string]any{"model": "wan3.0-video", "prompt": "", "images": []any{"https://cdn.example/first.png"}}))
		require.NoError(t, callErr)
		input := roundTrip(t, value)["body"].(map[string]any)["input"].(map[string]any)
		assert.Equal(t, []any{map[string]any{"type": "first_frame", "url": "https://cdn.example/first.png"}}, input["media"])
		assert.NotContains(t, input, "img_url")
	})

	t.Run("completion facts read the wan3.0 usage block", func(t *testing.T) {
		value, callErr := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", map[string]any{}, map[string]any{}, map[string]any{
			"output": map[string]any{"task_status": "SUCCEEDED", "video_url": "https://upstream.example/v.mp4"},
			"usage":  map[string]any{"video_count": 1, "duration": 7.5, "output_video_duration": 7.5, "SR": 720, "ratio": "16:9"},
		})
		require.NoError(t, callErr)
		assert.Equal(t, map[string]any{"seconds": 7.5, "resolution": "720P"}, roundTrip(t, value))

		value, callErr = plugin.Engine.Call(t.Context(), "extractUsageOnComplete", map[string]any{}, map[string]any{}, map[string]any{
			"output": map[string]any{"task_status": "SUCCEEDED", "duration": 5, "resolution": "1080p"},
		})
		require.NoError(t, callErr)
		assert.Equal(t, map[string]any{"seconds": float64(5), "resolution": "1080P"}, roundTrip(t, value))
	})
}
