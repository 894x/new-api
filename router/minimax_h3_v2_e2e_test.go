package router

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	projecti18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiniMaxH3OfficialV2LifecycleEndToEnd(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, projecti18n.Init())
	require.NoError(t, model.DB.AutoMigrate(
		&model.Channel{},
		&model.ChannelModelOverride{},
		&model.Log{},
		&model.Task{},
		&model.UserSubscription{},
	))
	ratio_setting.InitRatioSettings()
	previousHideErrorDetails := operation_setting.ShouldHideErrorDetails()
	operation_setting.UpdateHideErrorDetails(false)
	t.Cleanup(func() { operation_setting.UpdateHideErrorDetails(previousHideErrorDetails) })
	previousModelPrices := ratio_setting.ModelPrice2JSONString()
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"MiniMax-H3":0.01,"customer-h3":0.01}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousModelPrices))
	})

	type observedRequest struct {
		method        string
		path          string
		authorization string
		body          map[string]any
	}
	upstreamRequests := make(chan observedRequest, 4)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		observation := observedRequest{
			method:        request.Method,
			path:          request.URL.EscapedPath(),
			authorization: request.Header.Get("Authorization"),
		}
		if request.Body != nil {
			_ = common.DecodeJson(request.Body, &observation.body)
		}
		upstreamRequests <- observation
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/v2/video_generation":
			content, _ := observation.body["content"].([]any)
			if len(content) > 0 {
				textItem, _ := content[0].(map[string]any)
				if textItem["text"] == "provider rejects" {
					writer.WriteHeader(http.StatusUnprocessableEntity)
					_, _ = io.WriteString(writer, `{"type":"error","error":{"type":"unprocessable_entity_error","message":"video description contains sensitive content (1026)","http_code":"422"},"request_id":"upstream-request-id"}`)
					return
				}
			}
			_, _ = io.WriteString(writer, `{"task_id":"h3-upstream-task"}`)
		case request.Method == http.MethodGet && request.URL.Path == "/v2/query/video_generation/h3-upstream-task":
			_, _ = io.WriteString(writer, `{
				"task":{
					"id":"h3-upstream-task",
					"model":"vendor-h3",
					"status":"succeeded",
					"created_at":1788512400,
					"updated_at":1788512410,
					"content":{"url":"https://cdn.example/h3.mp4"},
					"resolution":"2K",
					"duration":7,
					"ratio":"16:9",
					"usage":{"output_seconds":7,"input_seconds":0,"input_image_count":1}
				}
			}`)
		default:
			http.Error(writer, "unexpected upstream request", http.StatusNotFound)
		}
	}))
	t.Cleanup(upstream.Close)

	user := model.User{
		Username: "minimax-h3-v2-e2e", Status: common.UserStatusEnabled, Group: "default", Quota: 1_000_000,
	}
	require.NoError(t, model.DB.Create(&user).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		UserId: user.Id, Key: "minimaxh3v2e2ekey", Status: common.TokenStatusEnabled,
		ExpiredTime: -1, UnlimitedQuota: true,
	}).Error)

	priority := int64(100)
	modelMapping := `{"MiniMax-H3":"vendor-h3"}`
	channel := model.Channel{
		Type: constant.ChannelTypeMiniMax, Name: "minimax-h3-v2-upstream", Key: "minimax-upstream-key",
		Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(upstream.URL),
		Models: "MiniMax-H3", Group: "default", Priority: &priority, ModelMapping: &modelMapping,
	}
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	aliasPriority := int64(90)
	aliasMapping := `{"customer-h3":"MiniMax-H3"}`
	aliasChannel := model.Channel{
		Type: constant.ChannelTypeMiniMax, Name: "minimax-h3-v2-alias-upstream", Key: "minimax-upstream-key",
		Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(upstream.URL),
		Models: "customer-h3", Group: "default", Priority: &aliasPriority, ModelMapping: &aliasMapping,
	}
	require.NoError(t, model.DB.Create(&aliasChannel).Error)
	require.NoError(t, aliasChannel.AddAbilities(nil))
	model.InitChannelCache()

	engine := gin.New()
	SetVideoRouter(engine)
	gateway := httptest.NewServer(engine)
	t.Cleanup(gateway.Close)

	submitBody := []byte(`{
		"model":"MiniMax-H3",
		"content":[
			{"type":"text","text":"A paper boat crossing a rain puddle"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://cdn.example/reference.png"}}
		],
		"resolution":"2K",
		"duration":7,
		"ratio":"16:9",
		"aigc_watermark":false
	}`)
	submitRequest, err := http.NewRequest(http.MethodPost, gateway.URL+"/v2/video_generation", bytes.NewReader(submitBody))
	require.NoError(t, err)
	submitRequest.Header.Set("Authorization", "Bearer minimaxh3v2e2ekey")
	submitRequest.Header.Set("Content-Type", "application/json")
	submitResponse, err := http.DefaultClient.Do(submitRequest)
	require.NoError(t, err)
	submitResponseBody, err := io.ReadAll(submitResponse.Body)
	require.NoError(t, err)
	require.NoError(t, submitResponse.Body.Close())
	require.Equal(t, http.StatusOK, submitResponse.StatusCode, string(submitResponseBody))

	var submitPayload struct {
		TaskID string `json:"task_id"`
	}
	require.NoError(t, common.Unmarshal(submitResponseBody, &submitPayload))
	assert.NotEmpty(t, submitPayload.TaskID)
	assert.NotEqual(t, "h3-upstream-task", submitPayload.TaskID)
	assert.NotContains(t, string(submitResponseBody), "h3-upstream-task")

	observedSubmit := <-upstreamRequests
	assert.Equal(t, http.MethodPost, observedSubmit.method)
	assert.Equal(t, "/v2/video_generation", observedSubmit.path)
	assert.Equal(t, "Bearer minimax-upstream-key", observedSubmit.authorization)
	assert.Equal(t, "vendor-h3", observedSubmit.body["model"])
	assert.Equal(t, "2K", observedSubmit.body["resolution"])
	assert.Equal(t, float64(7), observedSubmit.body["duration"])
	assert.Equal(t, "16:9", observedSubmit.body["ratio"])
	watermark, exists := observedSubmit.body["aigc_watermark"]
	assert.True(t, exists)
	assert.Equal(t, false, watermark)
	content, ok := observedSubmit.body["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 2)

	var persistedTask model.Task
	require.NoError(t, model.DB.Where("task_id = ?", submitPayload.TaskID).First(&persistedTask).Error)
	assert.Equal(t, "h3-upstream-task", persistedTask.PrivateData.UpstreamTaskID)
	assert.Equal(t, "MiniMax-H3", persistedTask.Properties.OriginModelName)
	assert.Equal(t, "vendor-h3", persistedTask.Properties.UpstreamModelName)
	assert.NotContains(t, string(persistedTask.Data), "h3-upstream-task")

	previousTaskAdaptorFactory := service.GetTaskAdaptorFunc
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		return relay.GetTaskAdaptor(platform)
	}
	t.Cleanup(func() { service.GetTaskAdaptorFunc = previousTaskAdaptorFactory })
	require.NoError(t, service.UpdateVideoTasks(
		context.Background(),
		persistedTask.Platform,
		map[int][]string{channel.Id: {persistedTask.GetUpstreamTaskID()}},
		map[string]*model.Task{persistedTask.GetUpstreamTaskID(): &persistedTask},
	))

	observedQuery := <-upstreamRequests
	assert.Equal(t, http.MethodGet, observedQuery.method)
	assert.Equal(t, "/v2/query/video_generation/h3-upstream-task", observedQuery.path)
	assert.Equal(t, "Bearer minimax-upstream-key", observedQuery.authorization)

	queryRequest, err := http.NewRequest(
		http.MethodGet,
		gateway.URL+"/v2/query/video_generation/"+submitPayload.TaskID,
		nil,
	)
	require.NoError(t, err)
	queryRequest.Header.Set("Authorization", "Bearer minimaxh3v2e2ekey")
	queryResponse, err := http.DefaultClient.Do(queryRequest)
	require.NoError(t, err)
	queryResponseBody, err := io.ReadAll(queryResponse.Body)
	require.NoError(t, err)
	require.NoError(t, queryResponse.Body.Close())
	require.Equal(t, http.StatusOK, queryResponse.StatusCode, string(queryResponseBody))
	assert.NotContains(t, string(queryResponseBody), "h3-upstream-task")

	var queryPayload struct {
		Task struct {
			ID         string `json:"id"`
			Model      string `json:"model"`
			Status     string `json:"status"`
			Resolution string `json:"resolution"`
			Duration   int    `json:"duration"`
			Ratio      string `json:"ratio"`
			Content    struct {
				URL string `json:"url"`
			} `json:"content"`
		} `json:"task"`
	}
	require.NoError(t, common.Unmarshal(queryResponseBody, &queryPayload))
	assert.Equal(t, submitPayload.TaskID, queryPayload.Task.ID)
	assert.Equal(t, "MiniMax-H3", queryPayload.Task.Model)
	assert.Equal(t, "succeeded", queryPayload.Task.Status)
	assert.Equal(t, "2K", queryPayload.Task.Resolution)
	assert.Equal(t, 7, queryPayload.Task.Duration)
	assert.Equal(t, "16:9", queryPayload.Task.Ratio)
	assert.Equal(t, "https://cdn.example/h3.mp4", queryPayload.Task.Content.URL)

	require.NoError(t, model.DB.Where("task_id = ?", submitPayload.TaskID).First(&persistedTask).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), persistedTask.Status)
	assert.Equal(t, "https://cdn.example/h3.mp4", persistedTask.PrivateData.ResultURL)
	assert.NotContains(t, string(persistedTask.Data), "h3-upstream-task")

	legacyTask := model.Task{
		TaskID:    "task_legacy_hailuo_v1",
		Platform:  persistedTask.Platform,
		UserId:    user.Id,
		ChannelId: channel.Id,
		Status:    model.TaskStatusSuccess,
		Properties: model.Properties{
			OriginModelName:   "MiniMax-Hailuo-2.3",
			UpstreamModelName: "MiniMax-Hailuo-2.3",
		},
	}
	legacyTask.SetData(map[string]any{"task_id": "legacy-upstream-task", "status": "Success"})
	require.NoError(t, model.DB.Create(&legacyTask).Error)
	legacyQueryRequest, err := http.NewRequest(
		http.MethodGet,
		gateway.URL+"/v2/query/video_generation/"+legacyTask.TaskID,
		nil,
	)
	require.NoError(t, err)
	legacyQueryRequest.Header.Set("Authorization", "Bearer minimaxh3v2e2ekey")
	legacyQueryResponse, err := http.DefaultClient.Do(legacyQueryRequest)
	require.NoError(t, err)
	legacyQueryBody, err := io.ReadAll(legacyQueryResponse.Body)
	require.NoError(t, err)
	require.NoError(t, legacyQueryResponse.Body.Close())
	assert.Equal(t, http.StatusBadRequest, legacyQueryResponse.StatusCode, string(legacyQueryBody))
	var legacyQueryPayload struct {
		Type  string `json:"type"`
		Error struct {
			Type     string `json:"type"`
			HTTPCode string `json:"http_code"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(legacyQueryBody, &legacyQueryPayload))
	assert.Equal(t, "error", legacyQueryPayload.Type)
	assert.Equal(t, "bad_request_error", legacyQueryPayload.Error.Type)
	assert.Equal(t, "400", legacyQueryPayload.Error.HTTPCode)
	var userAfterValidRequest model.User
	require.NoError(t, model.DB.First(&userAfterValidRequest, user.Id).Error)
	assert.Equal(t, 995_000, userAfterValidRequest.Quota)
	assert.Equal(t, 5_000, persistedTask.Quota)
	var tasksAfterValidRequest int64
	require.NoError(t, model.DB.Model(&model.Task{}).Count(&tasksAfterValidRequest).Error)
	var logsAfterValidRequest int64
	require.NoError(t, model.DB.Model(&model.Log{}).Count(&logsAfterValidRequest).Error)

	providerRejectedRequest, err := http.NewRequest(
		http.MethodPost,
		gateway.URL+"/v2/video_generation",
		bytes.NewBufferString(`{
			"model":"MiniMax-H3",
			"content":[{"type":"text","text":"provider rejects"}],
			"resolution":"768P",
			"duration":5,
			"ratio":"16:9"
		}`),
	)
	require.NoError(t, err)
	providerRejectedRequest.Header.Set("Authorization", "Bearer minimaxh3v2e2ekey")
	providerRejectedRequest.Header.Set("Content-Type", "application/json")
	providerRejectedResponse, err := http.DefaultClient.Do(providerRejectedRequest)
	require.NoError(t, err)
	providerRejectedBody, err := io.ReadAll(providerRejectedResponse.Body)
	require.NoError(t, err)
	require.NoError(t, providerRejectedResponse.Body.Close())
	require.Equal(t, http.StatusUnprocessableEntity, providerRejectedResponse.StatusCode, string(providerRejectedBody))
	var providerRejectedPayload struct {
		Type  string `json:"type"`
		Error struct {
			Type     string `json:"type"`
			Message  string `json:"message"`
			HTTPCode string `json:"http_code"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(providerRejectedBody, &providerRejectedPayload))
	assert.Equal(t, "error", providerRejectedPayload.Type)
	assert.Equal(t, "unprocessable_entity_error", providerRejectedPayload.Error.Type)
	assert.Equal(t, "video description contains sensitive content (1026)", providerRejectedPayload.Error.Message)
	assert.Equal(t, "422", providerRejectedPayload.Error.HTTPCode)
	providerRejectedUpstream := <-upstreamRequests
	assert.Equal(t, http.MethodPost, providerRejectedUpstream.method)
	assert.Equal(t, "/v2/video_generation", providerRejectedUpstream.path)

	invalidRequest, err := http.NewRequest(
		http.MethodPost,
		gateway.URL+"/v2/video_generation",
		bytes.NewBufferString(`{
			"model":"MiniMax-H3",
			"content":[{"type":"text","text":"invalid duration"}],
			"resolution":"768P",
			"duration":16,
			"ratio":"16:9"
		}`),
	)
	require.NoError(t, err)
	invalidRequest.Header.Set("Authorization", "Bearer minimaxh3v2e2ekey")
	invalidRequest.Header.Set("Content-Type", "application/json")
	invalidResponse, err := http.DefaultClient.Do(invalidRequest)
	require.NoError(t, err)
	invalidResponseBody, err := io.ReadAll(invalidResponse.Body)
	require.NoError(t, err)
	require.NoError(t, invalidResponse.Body.Close())
	require.Equal(t, http.StatusBadRequest, invalidResponse.StatusCode, string(invalidResponseBody))
	var invalidPayload struct {
		Type  string `json:"type"`
		Error struct {
			Type     string `json:"type"`
			Message  string `json:"message"`
			HTTPCode string `json:"http_code"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(invalidResponseBody, &invalidPayload))
	assert.Equal(t, "error", invalidPayload.Type)
	assert.Equal(t, "bad_request_error", invalidPayload.Error.Type)
	assert.Contains(t, invalidPayload.Error.Message, "between 4 and 15")
	assert.Equal(t, "400", invalidPayload.Error.HTTPCode)
	select {
	case unexpectedRequest := <-upstreamRequests:
		t.Fatalf("invalid request reached upstream: %s %s", unexpectedRequest.method, unexpectedRequest.path)
	default:
	}

	missingModelRequest, err := http.NewRequest(
		http.MethodPost,
		gateway.URL+"/v2/video_generation",
		bytes.NewBufferString(`{
			"content":[{"type":"text","text":"missing model"}],
			"resolution":"768P",
			"duration":5,
			"ratio":"16:9"
		}`),
	)
	require.NoError(t, err)
	missingModelRequest.Header.Set("Authorization", "Bearer minimaxh3v2e2ekey")
	missingModelRequest.Header.Set("Content-Type", "application/json")
	missingModelResponse, err := http.DefaultClient.Do(missingModelRequest)
	require.NoError(t, err)
	missingModelResponseBody, err := io.ReadAll(missingModelResponse.Body)
	require.NoError(t, err)
	require.NoError(t, missingModelResponse.Body.Close())
	require.Equal(t, http.StatusBadRequest, missingModelResponse.StatusCode, string(missingModelResponseBody))
	var missingModelPayload struct {
		Type  string `json:"type"`
		Error struct {
			Type     string `json:"type"`
			Message  string `json:"message"`
			HTTPCode string `json:"http_code"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(missingModelResponseBody, &missingModelPayload))
	assert.Equal(t, "error", missingModelPayload.Type)
	assert.Equal(t, "bad_request_error", missingModelPayload.Error.Type)
	assert.NotEmpty(t, missingModelPayload.Error.Message)
	assert.Equal(t, "400", missingModelPayload.Error.HTTPCode)
	select {
	case unexpectedRequest := <-upstreamRequests:
		t.Fatalf("missing-model request reached upstream: %s %s", unexpectedRequest.method, unexpectedRequest.path)
	default:
	}

	strictlyInvalidBodies := []string{
		`{"model":"MiniMax-H3-Max","content":[{"type":"text","text":"unsupported model"}],"resolution":"768P","duration":5,"ratio":"16:9"}`,
		`{"model":"MiniMax-H3","content":[{"type":"text","text":"missing duration"}],"resolution":"768P","ratio":"16:9"}`,
		`{"model":"MiniMax-H3","content":[{"type":"text","text":"missing resolution"}],"duration":5,"ratio":"16:9"}`,
		`{"model":"MiniMax-H3","content":[{"type":"text","text":"missing text ratio"}],"resolution":"768P","duration":5}`,
		`{"model":"MiniMax-H3","content":[{"type":"text","text":"string duration"}],"resolution":"768P","duration":"5","ratio":"16:9"}`,
		`{"model":"MiniMax-H3","content":[{"type":"text","text":"bad resolution"}],"resolution":"12K","duration":5,"ratio":"16:9"}`,
		`{"model":"MiniMax-H3","content":[{"type":"text","text":"bad item"},42],"resolution":"768P","duration":5,"ratio":"16:9"}`,
		`{"model":"MiniMax-H3","content":[{"type":"text","text":"unknown type"},{"type":"binary","binary":{"url":"https://cdn.example/input.bin"}}],"resolution":"768P","duration":5,"ratio":"16:9"}`,
		`{"model":"MiniMax-H3","content":[{"type":"text","text":"bad role"},{"type":"image_url","role":"reference_audio","image_url":{"url":"https://cdn.example/image.png"}}],"resolution":"768P","duration":5}`,
		`{"model":"MiniMax-H3","content":[{"type":"text","text":"missing url"},{"type":"video_url","role":"reference_video","video_url":{}}],"resolution":"768P","duration":5}`,
	}
	longPromptBody, err := common.Marshal(map[string]any{
		"model":      "MiniMax-H3",
		"content":    []any{map[string]any{"type": "text", "text": strings.Repeat("界", 7001)}},
		"resolution": "768P",
		"duration":   5,
		"ratio":      "16:9",
	})
	require.NoError(t, err)
	strictlyInvalidBodies = append(strictlyInvalidBodies, string(longPromptBody))
	for _, body := range strictlyInvalidBodies {
		request, requestErr := http.NewRequest(http.MethodPost, gateway.URL+"/v2/video_generation", bytes.NewBufferString(body))
		require.NoError(t, requestErr)
		request.Header.Set("Authorization", "Bearer minimaxh3v2e2ekey")
		request.Header.Set("Content-Type", "application/json")
		response, responseErr := http.DefaultClient.Do(request)
		require.NoError(t, responseErr)
		responseBody, readErr := io.ReadAll(response.Body)
		require.NoError(t, readErr)
		require.NoError(t, response.Body.Close())
		assert.Equal(t, http.StatusBadRequest, response.StatusCode, string(responseBody))
		var payload struct {
			Type  string `json:"type"`
			Error struct {
				Type     string `json:"type"`
				HTTPCode string `json:"http_code"`
			} `json:"error"`
		}
		require.NoError(t, common.Unmarshal(responseBody, &payload))
		assert.Equal(t, "error", payload.Type)
		assert.Equal(t, "bad_request_error", payload.Error.Type)
		assert.Equal(t, "400", payload.Error.HTTPCode)
	}
	select {
	case unexpectedRequest := <-upstreamRequests:
		t.Fatalf("strictly invalid request reached upstream: %s %s", unexpectedRequest.method, unexpectedRequest.path)
	default:
	}

	reverseMappedInvalidRequest, err := http.NewRequest(
		http.MethodPost,
		gateway.URL+"/v1/videos",
		bytes.NewBufferString(`{"model":"customer-h3","prompt":"invalid mapped duration","duration":16}`),
	)
	require.NoError(t, err)
	reverseMappedInvalidRequest.Header.Set("Authorization", "Bearer minimaxh3v2e2ekey")
	reverseMappedInvalidRequest.Header.Set("Content-Type", "application/json")
	reverseMappedInvalidResponse, err := http.DefaultClient.Do(reverseMappedInvalidRequest)
	require.NoError(t, err)
	reverseMappedInvalidBody, err := io.ReadAll(reverseMappedInvalidResponse.Body)
	require.NoError(t, err)
	require.NoError(t, reverseMappedInvalidResponse.Body.Close())
	assert.Equal(t, http.StatusBadRequest, reverseMappedInvalidResponse.StatusCode, string(reverseMappedInvalidBody))
	assert.Contains(t, string(reverseMappedInvalidBody), "between 4 and 15")
	select {
	case unexpectedRequest := <-upstreamRequests:
		t.Fatalf("reverse-mapped invalid request reached upstream: %s %s", unexpectedRequest.method, unexpectedRequest.path)
	default:
	}

	var userAfterRejectedRequests model.User
	require.NoError(t, model.DB.First(&userAfterRejectedRequests, user.Id).Error)
	assert.Equal(t, userAfterValidRequest.Quota, userAfterRejectedRequests.Quota)
	var tasksAfterRejectedRequests int64
	require.NoError(t, model.DB.Model(&model.Task{}).Count(&tasksAfterRejectedRequests).Error)
	assert.Equal(t, tasksAfterValidRequest, tasksAfterRejectedRequests)
	var logsAfterRejectedRequests int64
	require.NoError(t, model.DB.Model(&model.Log{}).Count(&logsAfterRejectedRequests).Error)
	assert.Equal(t, logsAfterValidRequest, logsAfterRejectedRequests)
}
