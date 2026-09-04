package router

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	taskali "github.com/QuantumNous/new-api/relay/channel/task/ali"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWan3ChannelBillingEndToEnd(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.Channel{},
		&model.ChannelModelOverride{},
		&model.Log{},
		&model.Task{},
		&model.UserSubscription{},
	))
	ratio_setting.InitRatioSettings()

	originalQuotaPerUnit := common.QuotaPerUnit
	originalLogConsumeEnabled := common.LogConsumeEnabled
	originalBatchUpdateEnabled := common.BatchUpdateEnabled
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
		common.LogConsumeEnabled = originalLogConsumeEnabled
		common.BatchUpdateEnabled = originalBatchUpdateEnabled
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
	})

	common.QuotaPerUnit = 1_000
	common.LogConsumeEnabled = true
	common.BatchUpdateEnabled = false
	common.MemoryCacheEnabled = true
	previousTaskAdaptorFactory := service.GetTaskAdaptorFunc
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		if platform == constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeAli)) {
			return &taskali.TaskAdaptor{}
		}
		return nil
	}
	t.Cleanup(func() { service.GetTaskAdaptorFunc = previousTaskAdaptorFactory })

	type observedSubmit struct {
		model         string
		prompt        string
		authorization string
		async         string
		body          map[string]any
	}
	observedSubmits := make(chan observedSubmit, 3)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPost && request.URL.Path == "/api/v1/services/aigc/video-generation/video-synthesis" {
			var body map[string]any
			if err := common.DecodeJson(request.Body, &body); err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
			input, _ := body["input"].(map[string]any)
			prompt, _ := input["prompt"].(string)
			modelName, _ := body["model"].(string)
			observedSubmits <- observedSubmit{
				model:         modelName,
				prompt:        prompt,
				authorization: request.Header.Get("Authorization"),
				async:         request.Header.Get("X-DashScope-Async"),
				body:          body,
			}
			taskID := map[string]string{
				"standard-success": "wan3-standard-upstream",
				"prime-success":    "wan3-prime-upstream",
				"prime-failure":    "wan3-failure-upstream",
			}[prompt]
			if taskID == "" {
				http.Error(writer, "unknown test prompt", http.StatusBadRequest)
				return
			}
			_, _ = io.WriteString(writer, `{"output":{"task_id":"`+taskID+`","task_status":"PENDING"},"request_id":"submit-e2e"}`)
			return
		}

		switch request.URL.Path {
		case "/api/v1/tasks/wan3-standard-upstream":
			_, _ = io.WriteString(writer, `{"output":{"task_id":"wan3-standard-upstream","task_status":"SUCCEEDED","video_url":"https://example.com/standard.mp4"},"usage":{"duration":5.0,"input_video_duration":7.5,"output_video_duration":5.0}}`)
		case "/api/v1/tasks/wan3-prime-upstream":
			_, _ = io.WriteString(writer, `{"output":{"task_id":"wan3-prime-upstream","task_status":"SUCCEEDED","video_url":"https://example.com/prime.mp4"},"usage":{"duration":10.0,"input_video_duration":8.0,"output_video_duration":10.0}}`)
		case "/api/v1/tasks/wan3-failure-upstream":
			_, _ = io.WriteString(writer, `{"output":{"task_id":"wan3-failure-upstream","task_status":"FAILED","code":"InvalidInput","message":"test failure"}}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(upstream.Close)

	const initialQuota = 100_000
	user := model.User{
		Username: "wan3-billing-e2e", Status: common.UserStatusEnabled, Group: "default", Quota: initialQuota,
	}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{
		UserId: user.Id, Key: "wan3billinge2ekey", Name: "wan3-billing-e2e-token",
		Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: initialQuota,
	}
	require.NoError(t, model.DB.Create(&token).Error)

	priority := int64(100)
	channel := model.Channel{
		Type: constant.ChannelTypeAli, Name: "wan3-billing-e2e-upstream", Key: "wan3-upstream-key",
		Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(upstream.URL),
		Models: "wan3.0-video,wan3.0-video-prime", Group: "default", Priority: &priority,
	}
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	model.InitChannelCache()

	engine := gin.New()
	SetVideoRouter(engine)

	postTask := func(body string) model.Task {
		request := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/services/aigc/video-generation/video-synthesis",
			bytes.NewReader([]byte(body)),
		)
		request.Header.Set("Authorization", "Bearer wan3billinge2ekey")
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()

		engine.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		var response struct {
			Output struct {
				TaskID string `json:"task_id"`
			} `json:"output"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		require.NotEmpty(t, response.Output.TaskID)
		var task model.Task
		require.NoError(t, model.DB.Where("task_id = ?", response.Output.TaskID).First(&task).Error)
		return task
	}

	pollTask := func(task *model.Task) {
		require.NoError(t, service.UpdateVideoTasks(
			context.Background(),
			constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeAli)),
			map[int][]string{channel.Id: {task.PrivateData.UpstreamTaskID}},
			map[string]*model.Task{task.PrivateData.UpstreamTaskID: task},
		))
		require.NoError(t, model.DB.Where("id = ?", task.ID).First(task).Error)
	}

	standardModelRatio, ok, _ := ratio_setting.GetModelRatio("wan3.0-video")
	require.True(t, ok)
	primeModelRatio, ok, _ := ratio_setting.GetModelRatio("wan3.0-video-prime")
	require.True(t, ok)
	assert.InDelta(t, 2*0.30/ratio_setting.USD2RMB, standardModelRatio, 1e-12)
	assert.InDelta(t, 2*0.45/ratio_setting.USD2RMB, primeModelRatio, 1e-12)
	preConsumedQuota := func(modelRatio float64, seconds, resolutionRatio float64) int {
		fixedGroupBaseQuota := common.QuotaFromFloat(modelRatio / 2 * common.QuotaPerUnit)
		return common.QuotaFromFloat(float64(fixedGroupBaseQuota) * seconds * resolutionRatio)
	}
	settledQuota := func(modelRatio float64, seconds, resolutionRatio float64) int {
		syntheticTokens := common.QuotaRound(seconds * common.QuotaPerUnit / 2)
		return common.QuotaFromFloat(float64(syntheticTokens) * modelRatio * resolutionRatio)
	}

	standardTask := postTask(`{
		"model":"wan3.0-video",
		"input":{"prompt":"standard-success","media":[{"type":"reference_video","url":"https://example.com/reference.mp4"}]},
		"parameters":{"resolution":"720P","duration":5}
	}`)
	standardSubmit := <-observedSubmits
	assert.Equal(t, "wan3.0-video", standardSubmit.model)
	assert.Equal(t, "standard-success", standardSubmit.prompt)
	assert.Equal(t, "Bearer wan3-upstream-key", standardSubmit.authorization)
	assert.Equal(t, "enable", standardSubmit.async)
	standardInput, ok := standardSubmit.body["input"].(map[string]any)
	require.True(t, ok)
	standardMedia, ok := standardInput["media"].([]any)
	require.True(t, ok)
	require.Len(t, standardMedia, 1)
	standardReference, ok := standardMedia[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "reference_video", standardReference["type"])
	standardReserved := preConsumedQuota(standardModelRatio, 30, 2)
	assert.Equal(t, standardReserved, standardTask.Quota)
	require.NotNil(t, standardTask.PrivateData.BillingContext)
	assert.Equal(t, 30.0, standardTask.PrivateData.BillingContext.OtherRatios["seconds"])
	assert.Equal(t, 2.0, standardTask.PrivateData.BillingContext.OtherRatios["resolution-720P"])
	assertWan3E2EBalances(t, user.Id, token.Id, initialQuota-standardReserved, standardReserved, 1)

	pollTask(&standardTask)
	standardActual := settledQuota(standardModelRatio, 12.5, 2)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), standardTask.Status)
	assert.Equal(t, standardActual, standardTask.Quota)
	assert.Equal(t, standardActual, standardTask.PrivateData.BillingContext.NetQuota)
	assert.Equal(t, 1.0, standardTask.PrivateData.BillingContext.OtherRatios["seconds"])
	assertWan3E2EBalances(t, user.Id, token.Id, initialQuota-standardActual, standardActual, 1)

	primeTask := postTask(`{
		"model":"wan3.0-video-prime",
		"input":{"prompt":"prime-success","media":[{"type":"reference_video","url":"https://example.com/prime-reference.mp4"}]},
		"parameters":{"resolution":"1080P","duration":-1}
	}`)
	primeSubmit := <-observedSubmits
	assert.Equal(t, "wan3.0-video-prime", primeSubmit.model)
	assert.Equal(t, "prime-success", primeSubmit.prompt)
	primeReserved := preConsumedQuota(primeModelRatio, 30, 4)
	assert.Equal(t, primeReserved, primeTask.Quota)
	assertWan3E2EBalances(
		t, user.Id, token.Id,
		initialQuota-standardActual-primeReserved,
		standardActual+primeReserved,
		2,
	)

	pollTask(&primeTask)
	primeActual := settledQuota(primeModelRatio, 18, 4)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), primeTask.Status)
	assert.Equal(t, primeActual, primeTask.Quota)
	assertWan3E2EBalances(
		t, user.Id, token.Id,
		initialQuota-standardActual-primeActual,
		standardActual+primeActual,
		2,
	)

	balanceBeforeFailure := initialQuota - standardActual - primeActual
	usedBeforeFailure := standardActual + primeActual
	failedTask := postTask(`{
		"model":"wan3.0-video-prime",
		"input":{"prompt":"prime-failure","media":[{"type":"reference_video","url":"https://example.com/failure-reference.mp4"}]},
		"parameters":{"resolution":"480P","duration":5}
	}`)
	failedSubmit := <-observedSubmits
	assert.Equal(t, "prime-failure", failedSubmit.prompt)
	failedReserved := preConsumedQuota(primeModelRatio, 30, 1)
	assert.Equal(t, failedReserved, failedTask.Quota)
	assertWan3E2EBalances(
		t, user.Id, token.Id,
		balanceBeforeFailure-failedReserved,
		usedBeforeFailure+failedReserved,
		3,
	)

	pollTask(&failedTask)
	assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), failedTask.Status)
	assert.Zero(t, failedTask.Quota)
	require.NotNil(t, failedTask.PrivateData.BillingContext)
	assert.Equal(t, model.TaskRefundStateCommitted, failedTask.PrivateData.BillingContext.RefundState)
	assert.Equal(t, failedReserved, failedTask.PrivateData.BillingContext.RefundedQuota)
	assertWan3E2EBalances(t, user.Id, token.Id, balanceBeforeFailure, usedBeforeFailure, 3)

	require.NoError(t, model.DB.First(&channel, channel.Id).Error)
	assert.EqualValues(t, usedBeforeFailure, channel.UsedQuota)

	var logs []model.Log
	require.NoError(t, model.DB.Where("user_id = ?", user.Id).Order("id ASC").Find(&logs).Error)
	require.Len(t, logs, 6)
	assert.Equal(t, []int{
		standardReserved,
		standardReserved - standardActual,
		primeReserved,
		primeReserved - primeActual,
		failedReserved,
		failedReserved,
	}, []int{logs[0].Quota, logs[1].Quota, logs[2].Quota, logs[3].Quota, logs[4].Quota, logs[5].Quota})
	assert.Equal(t, []int{
		model.LogTypeConsume,
		model.LogTypeRefund,
		model.LogTypeConsume,
		model.LogTypeRefund,
		model.LogTypeConsume,
		model.LogTypeRefund,
	}, []int{logs[0].Type, logs[1].Type, logs[2].Type, logs[3].Type, logs[4].Type, logs[5].Type})
	assert.Contains(t, logs[1].Other, `"actual_quota":`+strconv.Itoa(standardActual))
	assert.Contains(t, logs[3].Other, `"actual_quota":`+strconv.Itoa(primeActual))
	assert.Contains(t, logs[5].Other, `"reason":"task failed, code: InvalidInput , message: test failure"`)
}

func assertWan3E2EBalances(t *testing.T, userID, tokenID, wantBalance, wantUsed, wantRequests int) {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, wantBalance, user.Quota)
	assert.Equal(t, wantUsed, user.UsedQuota)
	assert.Equal(t, wantRequests, user.RequestCount)

	var token model.Token
	require.NoError(t, model.DB.First(&token, tokenID).Error)
	assert.Equal(t, wantBalance, token.RemainQuota)
	assert.Equal(t, wantUsed, token.UsedQuota)
}
