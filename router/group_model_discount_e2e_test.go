package router

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/groupdiscount"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupModelMonthlyDiscountSettlesOriginalPriceAcrossTierBoundaryE2E(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.Channel{},
		&model.ChannelModelOverride{},
		&model.Log{},
		&model.UserSubscription{},
		&model.UserGroupModelMonthlyUsage{},
		&model.GroupModelDiscountSettlement{},
		&model.GroupModelDiscountAdjustment{},
		&model.BillingRefundOperation{},
		&model.BillingAdmissionReserveOperation{},
	))
	ratio_setting.InitRatioSettings()

	originalQuotaPerUnit := common.QuotaPerUnit
	originalLogConsumeEnabled := common.LogConsumeEnabled
	originalBatchUpdateEnabled := common.BatchUpdateEnabled
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalModelPrices := ratio_setting.ModelPrice2JSONString()
	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	originalTieredRatios := ratio_setting.ModelTieredRatios2JSONString()
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
		common.LogConsumeEnabled = originalLogConsumeEnabled
		common.BatchUpdateEnabled = originalBatchUpdateEnabled
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(originalModelPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
		require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(originalTieredRatios))
	})

	common.QuotaPerUnit = 1_000
	common.LogConsumeEnabled = true
	common.BatchUpdateEnabled = false
	common.MemoryCacheEnabled = true
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"gpt-monthly-e2e":0.3}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"vip":0.5}`))
	require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(`{
		"vip":{"gpt-monthly-e2e":{
			"enabled":true,
			"effective_from":0,
			"effective_until":null,
			"timezone":"UTC",
			"tiers":[
				{"min_monthly_original_quota":0,"ratio":0.9},
				{"min_monthly_original_quota":500,"ratio":0.8}
			]
		}}
	}`))

	var upstreamFails atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if upstreamFails.Load() {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = writer.Write([]byte(`{"error":{"message":"upstream failed","type":"upstream_error"}}`))
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"id":"chatcmpl-monthly-discount",
			"object":"chat.completion",
			"created":1,
			"model":"gpt-monthly-e2e",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
		}`))
	}))
	t.Cleanup(upstream.Close)

	user := model.User{
		Username: "monthly-discount-e2e",
		Status:   common.UserStatusEnabled,
		Group:    "vip",
		Quota:    5_000,
	}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{
		UserId:         user.Id,
		Key:            "monthlydiscounte2ekey",
		Name:           "monthly-discount-token",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		RemainQuota:    5_000,
		UnlimitedQuota: false,
	}
	require.NoError(t, model.DB.Create(&token).Error)

	priority := int64(100)
	channel := model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Name:     "monthly-discount-upstream",
		Key:      "test-key",
		Status:   common.ChannelStatusEnabled,
		BaseURL:  common.GetPointer(upstream.URL),
		Models:   "gpt-monthly-e2e",
		Group:    "vip",
		Priority: &priority,
	}
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	model.InitChannelCache()

	engine := gin.New()
	SetRelayRouter(engine)
	for requestNumber := 0; requestNumber < 2; requestNumber++ {
		body := []byte(`{"model":"gpt-monthly-e2e","messages":[{"role":"user","content":"hello"}]}`)
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		request.Header.Set("Authorization", "Bearer monthlydiscounte2ekey")
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()

		engine.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		var payload struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
		require.Len(t, payload.Choices, 1)
		assert.Equal(t, "ok", payload.Choices[0].Message.Content)
	}

	// A failed upstream request is pre-consumed at the original price but must
	// be fully returned. It never advances the authoritative monthly cursor and
	// never creates a consume log/settlement.
	upstreamFails.Store(true)
	failedRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(
		[]byte(`{"model":"gpt-monthly-e2e","messages":[{"role":"user","content":"fail"}]}`),
	))
	failedRequest.Header.Set("Authorization", "Bearer monthlydiscounte2ekey")
	failedRequest.Header.Set("Content-Type", "application/json")
	failedRecorder := httptest.NewRecorder()
	engine.ServeHTTP(failedRecorder, failedRequest)
	require.NotEqual(t, http.StatusOK, failedRecorder.Code, failedRecorder.Body.String())
	require.Eventually(t, func() bool {
		var currentUser model.User
		var currentToken model.Token
		if model.DB.First(&currentUser, user.Id).Error != nil || model.DB.First(&currentToken, token.Id).Error != nil {
			return false
		}
		return currentUser.Quota == 4_470 && currentToken.RemainQuota == 4_470
	}, time.Second, 10*time.Millisecond)

	require.NoError(t, model.DB.First(&user, user.Id).Error)
	assert.Equal(t, 4_470, user.Quota)
	assert.Equal(t, 530, user.UsedQuota)
	assert.Equal(t, 2, user.RequestCount)

	require.NoError(t, model.DB.First(&token, token.Id).Error)
	assert.Equal(t, 4_470, token.RemainQuota)
	assert.Equal(t, 530, token.UsedQuota)

	require.NoError(t, model.DB.First(&channel, channel.Id).Error)
	assert.EqualValues(t, 530, channel.UsedQuota)

	var usage model.UserGroupModelMonthlyUsage
	require.NoError(t, model.DB.Where("user_id = ? AND using_group = ? AND origin_model = ?", user.Id, "vip", "gpt-monthly-e2e").First(&usage).Error)
	assert.EqualValues(t, 600, usage.OriginalQuota)
	assert.EqualValues(t, 530, usage.ChargedQuota)
	assert.Equal(t, "600", usage.ProgressQuota)

	var settlements []model.GroupModelDiscountSettlement
	require.NoError(t, model.DB.Order("id ASC").Find(&settlements).Error)
	require.Len(t, settlements, 2)
	assert.Equal(t, []int64{300, 300}, []int64{settlements[0].OriginalQuota, settlements[1].OriginalQuota})
	assert.Equal(t, []int64{270, 260}, []int64{settlements[0].ChargedQuota, settlements[1].ChargedQuota})
	assert.Equal(t, groupdiscount.ProgressBasisOriginal, settlements[0].ProgressBasis)
	assert.Equal(t, groupdiscount.ProgressBasisOriginal, settlements[1].ProgressBasis)
	assert.Equal(t, model.GroupModelDiscountStatusSettled, settlements[0].Status)
	assert.Equal(t, model.GroupModelDiscountStatusSettled, settlements[1].Status)

	var secondSegments []groupdiscount.Segment
	require.NoError(t, common.UnmarshalJsonStr(settlements[1].Segments, &secondSegments))
	require.Len(t, secondSegments, 2)
	assert.EqualValues(t, 200, secondSegments[0].OriginalQuota)
	assert.Equal(t, 0.9, secondSegments[0].Ratio)
	assert.EqualValues(t, 100, secondSegments[1].OriginalQuota)
	assert.Equal(t, 0.8, secondSegments[1].Ratio)

	var logs []model.Log
	require.NoError(t, model.DB.Where("type = ?", model.LogTypeConsume).Order("id ASC").Find(&logs).Error)
	require.Len(t, logs, 2)
	assert.Equal(t, []int{270, 260}, []int{logs[0].Quota, logs[1].Quota})
	assert.Contains(t, logs[0].Other, `"progress_basis":"original"`)
	assert.Contains(t, logs[0].Other, `"original_quota":300`)
	assert.Contains(t, logs[1].Other, `"monthly_original_after":600`)
}

func TestGroupModelMonthlyDiscountSettledPriceProgressAcrossTierBoundaryE2E(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.Channel{},
		&model.ChannelModelOverride{},
		&model.Log{},
		&model.UserSubscription{},
		&model.UserGroupModelMonthlyUsage{},
		&model.GroupModelDiscountSettlement{},
		&model.GroupModelDiscountAdjustment{},
		&model.BillingRefundOperation{},
		&model.BillingAdmissionReserveOperation{},
	))
	ratio_setting.InitRatioSettings()

	originalQuotaPerUnit := common.QuotaPerUnit
	originalLogConsumeEnabled := common.LogConsumeEnabled
	originalBatchUpdateEnabled := common.BatchUpdateEnabled
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalModelPrices := ratio_setting.ModelPrice2JSONString()
	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	originalTieredRatios := ratio_setting.ModelTieredRatios2JSONString()
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
		common.LogConsumeEnabled = originalLogConsumeEnabled
		common.BatchUpdateEnabled = originalBatchUpdateEnabled
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(originalModelPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
		require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(originalTieredRatios))
	})

	common.QuotaPerUnit = 1_000
	common.LogConsumeEnabled = true
	common.BatchUpdateEnabled = false
	common.MemoryCacheEnabled = true
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"gpt-settled-progress-e2e":0.3}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"settled-progress":0.5}`))
	require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(`{
		"settled-progress":{
			"progress_basis":"charged",
			"models":{"gpt-settled-progress-e2e":{
				"enabled":true,
				"effective_from":0,
				"effective_until":null,
				"timezone":"UTC",
				"tiers":[
					{"min_monthly_original_quota":0,"ratio":0.8},
					{"min_monthly_original_quota":401,"ratio":0.7}
				]
			}}
		}
	}`))

	var upstreamFails atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if upstreamFails.Load() {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = writer.Write([]byte(`{"error":{"message":"upstream failed","type":"upstream_error"}}`))
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"id":"chatcmpl-settled-progress",
			"object":"chat.completion",
			"created":1,
			"model":"gpt-settled-progress-e2e",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
		}`))
	}))
	t.Cleanup(upstream.Close)

	user := model.User{
		Username: "settled-progress-e2e",
		Status:   common.UserStatusEnabled,
		Group:    "settled-progress",
		Quota:    5_000,
	}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{
		UserId:         user.Id,
		Key:            "settledprogresse2ekey",
		Name:           "settled-progress-token",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		RemainQuota:    5_000,
		UnlimitedQuota: false,
	}
	require.NoError(t, model.DB.Create(&token).Error)

	priority := int64(100)
	channel := model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Name:     "settled-progress-upstream",
		Key:      "test-key",
		Status:   common.ChannelStatusEnabled,
		BaseURL:  common.GetPointer(upstream.URL),
		Models:   "gpt-settled-progress-e2e",
		Group:    "settled-progress",
		Priority: &priority,
	}
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	model.InitChannelCache()

	engine := gin.New()
	SetRelayRouter(engine)
	for requestNumber := 0; requestNumber < 2; requestNumber++ {
		body := []byte(`{"model":"gpt-settled-progress-e2e","messages":[{"role":"user","content":"hello"}]}`)
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		request.Header.Set("Authorization", "Bearer settledprogresse2ekey")
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()

		engine.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		var payload struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
		require.Len(t, payload.Choices, 1)
		assert.Equal(t, "ok", payload.Choices[0].Message.Content)
	}

	// A failed request must return the original-price admission reserve without
	// advancing either the exact discounted progress cursor or actual charges.
	upstreamFails.Store(true)
	failedRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(
		[]byte(`{"model":"gpt-settled-progress-e2e","messages":[{"role":"user","content":"fail"}]}`),
	))
	failedRequest.Header.Set("Authorization", "Bearer settledprogresse2ekey")
	failedRequest.Header.Set("Content-Type", "application/json")
	failedRecorder := httptest.NewRecorder()
	engine.ServeHTTP(failedRecorder, failedRequest)
	require.NotEqual(t, http.StatusOK, failedRecorder.Code, failedRecorder.Body.String())
	require.Eventually(t, func() bool {
		var currentUser model.User
		var currentToken model.Token
		if model.DB.First(&currentUser, user.Id).Error != nil || model.DB.First(&currentToken, token.Id).Error != nil {
			return false
		}
		return currentUser.Quota == 4_530 && currentToken.RemainQuota == 4_530
	}, time.Second, 10*time.Millisecond)

	require.NoError(t, model.DB.First(&user, user.Id).Error)
	assert.Equal(t, 4_530, user.Quota)
	assert.Equal(t, 470, user.UsedQuota)
	assert.Equal(t, 2, user.RequestCount)

	require.NoError(t, model.DB.First(&token, token.Id).Error)
	assert.Equal(t, 4_530, token.RemainQuota)
	assert.Equal(t, 470, token.UsedQuota)

	require.NoError(t, model.DB.First(&channel, channel.Id).Error)
	assert.EqualValues(t, 470, channel.UsedQuota)

	var usage model.UserGroupModelMonthlyUsage
	require.NoError(t, model.DB.Where("user_id = ? AND using_group = ? AND origin_model = ?", user.Id, "settled-progress", "gpt-settled-progress-e2e").First(&usage).Error)
	assert.EqualValues(t, 600, usage.OriginalQuota)
	assert.EqualValues(t, 470, usage.ChargedQuota)
	assert.Equal(t, "470.125", usage.ProgressQuota)

	var settlements []model.GroupModelDiscountSettlement
	require.NoError(t, model.DB.Order("id ASC").Find(&settlements).Error)
	require.Len(t, settlements, 2)
	assert.Equal(t, []int64{300, 300}, []int64{settlements[0].OriginalQuota, settlements[1].OriginalQuota})
	assert.Equal(t, []int64{240, 230}, []int64{settlements[0].ChargedQuota, settlements[1].ChargedQuota})
	assert.Equal(t, []string{"240", "230.125"}, []string{settlements[0].ProgressQuota, settlements[1].ProgressQuota})
	assert.Equal(t, "470.125", settlements[1].MonthlyProgressAfter)
	assert.Equal(t, model.GroupModelDiscountStatusSettled, settlements[0].Status)
	assert.Equal(t, model.GroupModelDiscountStatusSettled, settlements[1].Status)

	var secondSegments []groupdiscount.Segment
	require.NoError(t, common.UnmarshalJsonStr(settlements[1].Segments, &secondSegments))
	require.Len(t, secondSegments, 2)
	assert.Equal(t, "201.25", secondSegments[0].OriginalQuotaExact)
	assert.Equal(t, "161", secondSegments[0].ProgressQuota)
	assert.Equal(t, 0.8, secondSegments[0].Ratio)
	assert.Equal(t, "98.75", secondSegments[1].OriginalQuotaExact)
	assert.Equal(t, "69.125", secondSegments[1].ProgressQuota)
	assert.Equal(t, 0.7, secondSegments[1].Ratio)

	var logs []model.Log
	require.NoError(t, model.DB.Where("type = ?", model.LogTypeConsume).Order("id ASC").Find(&logs).Error)
	require.Len(t, logs, 2)
	assert.Equal(t, []int{240, 230}, []int{logs[0].Quota, logs[1].Quota})
	assert.Contains(t, logs[0].Other, `"progress_basis":"charged"`)
	assert.Contains(t, logs[1].Other, `"monthly_progress_after":"470.125"`)
}

func TestGroupModelMonthlyDiscountTieredExprChargesWhenFixedGroupRatioIsZeroE2E(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.Channel{},
		&model.ChannelModelOverride{},
		&model.Log{},
		&model.UserSubscription{},
		&model.UserGroupModelMonthlyUsage{},
		&model.GroupModelDiscountSettlement{},
		&model.GroupModelDiscountAdjustment{},
		&model.BillingRefundOperation{},
		&model.BillingAdmissionReserveOperation{},
	))
	ratio_setting.InitRatioSettings()

	savedConfig := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		savedConfig[key] = value
		return nil
	}))
	originalQuotaPerUnit := common.QuotaPerUnit
	originalLogConsumeEnabled := common.LogConsumeEnabled
	originalBatchUpdateEnabled := common.BatchUpdateEnabled
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
		common.LogConsumeEnabled = originalLogConsumeEnabled
		common.BatchUpdateEnabled = originalBatchUpdateEnabled
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		require.NoError(t, config.GlobalConfig.LoadFromDB(savedConfig))
	})

	common.QuotaPerUnit = 1_000
	common.LogConsumeEnabled = true
	common.BatchUpdateEnabled = false
	common.MemoryCacheEnabled = true
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":    `{"gpt-monthly-zero-tiered-e2e":"tiered_expr"}`,
		"billing_setting.billing_expr":    `{"gpt-monthly-zero-tiered-e2e":"tier(\"base\", c * 15)"}`,
		"group_ratio_setting.group_ratio": `{"monthly-zero-e2e":0}`,
		"group_ratio_setting.model_tiered_ratios": `{
			"monthly-zero-e2e":{"gpt-monthly-zero-tiered-e2e":{
				"enabled":true,
				"effective_from":0,
				"effective_until":null,
				"timezone":"UTC",
				"tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]
			}}
		}`,
	}))

	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"id":"chatcmpl-monthly-zero-tiered",
			"object":"chat.completion",
			"created":1,
			"model":"gpt-monthly-zero-tiered-e2e",
			"choices":[{"index":0,"message":{"role":"assistant","content":"paid"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":1,"completion_tokens":1000,"total_tokens":1001}
		}`))
	}))
	t.Cleanup(upstream.Close)

	user := model.User{
		Username: "monthly-zero-tiered-e2e",
		Status:   common.UserStatusEnabled,
		Group:    "monthly-zero-e2e",
		Quota:    1_000,
	}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{
		UserId:         user.Id,
		Key:            "monthlyzerotierede2ekey",
		Name:           "monthly-zero-tiered-token",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		RemainQuota:    1_000,
		UnlimitedQuota: false,
	}
	require.NoError(t, model.DB.Create(&token).Error)

	priority := int64(100)
	channel := model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Name:     "monthly-zero-tiered-upstream",
		Key:      "test-key",
		Status:   common.ChannelStatusEnabled,
		BaseURL:  common.GetPointer(upstream.URL),
		Models:   "gpt-monthly-zero-tiered-e2e",
		Group:    "monthly-zero-e2e",
		Priority: &priority,
	}
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	model.InitChannelCache()

	engine := gin.New()
	SetRelayRouter(engine)
	body := []byte(`{"model":"gpt-monthly-zero-tiered-e2e","messages":[{"role":"user","content":"hello"}]}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer monthlyzerotierede2ekey")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Eventually(t, func() bool {
		var currentUser model.User
		var currentToken model.Token
		if model.DB.First(&currentUser, user.Id).Error != nil || model.DB.First(&currentToken, token.Id).Error != nil {
			return false
		}
		return currentUser.Quota == 988 && currentToken.RemainQuota == 988
	}, time.Second, 10*time.Millisecond)

	require.NoError(t, model.DB.First(&user, user.Id).Error)
	assert.Equal(t, 988, user.Quota)
	assert.Equal(t, 12, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
	require.NoError(t, model.DB.First(&token, token.Id).Error)
	assert.Equal(t, 988, token.RemainQuota)
	assert.Equal(t, 12, token.UsedQuota)
	require.NoError(t, model.DB.First(&channel, channel.Id).Error)
	assert.EqualValues(t, 12, channel.UsedQuota)

	var usage model.UserGroupModelMonthlyUsage
	require.NoError(t, model.DB.Where(
		"user_id = ? AND using_group = ? AND origin_model = ?",
		user.Id,
		"monthly-zero-e2e",
		"gpt-monthly-zero-tiered-e2e",
	).First(&usage).Error)
	assert.EqualValues(t, 15, usage.OriginalQuota)
	assert.EqualValues(t, 12, usage.ChargedQuota)
	assert.Equal(t, "15", usage.ProgressQuota)

	var settlement model.GroupModelDiscountSettlement
	require.NoError(t, model.DB.First(&settlement).Error)
	assert.EqualValues(t, 15, settlement.OriginalQuota)
	assert.EqualValues(t, 12, settlement.ChargedQuota)
	assert.Equal(t, model.GroupModelDiscountStatusSettled, settlement.Status)
	assert.True(t, settlement.AccountingApplied)
}

func TestGroupModelMonthlyDiscountAutoGroupRetryCreatesBillingSessionForFinalPaidGroupE2E(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.Channel{},
		&model.ChannelModelOverride{},
		&model.Log{},
		&model.UserSubscription{},
		&model.UserGroupModelMonthlyUsage{},
		&model.GroupModelDiscountSettlement{},
		&model.GroupModelDiscountAdjustment{},
		&model.BillingRefundOperation{},
		&model.BillingAdmissionReserveOperation{},
	))
	ratio_setting.InitRatioSettings()

	savedConfig := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		savedConfig[key] = value
		return nil
	}))
	originalQuotaPerUnit := common.QuotaPerUnit
	originalLogConsumeEnabled := common.LogConsumeEnabled
	originalBatchUpdateEnabled := common.BatchUpdateEnabled
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalRetryTimes := common.RetryTimes
	originalUserUsableGroups := setting.UserUsableGroups2JSONString()
	originalMaxTokenAutoGroups := setting.GetMaxTokenAutoGroups()
	originalRetryStatusCodes := operation_setting.AutomaticRetryStatusCodesToString()
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
		common.LogConsumeEnabled = originalLogConsumeEnabled
		common.BatchUpdateEnabled = originalBatchUpdateEnabled
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		common.RetryTimes = originalRetryTimes
		require.NoError(t, config.GlobalConfig.LoadFromDB(savedConfig))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUserUsableGroups))
		require.NoError(t, setting.UpdateMaxTokenAutoGroups(strconv.Itoa(originalMaxTokenAutoGroups)))
		require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString(originalRetryStatusCodes))
	})

	common.QuotaPerUnit = 1_000
	common.LogConsumeEnabled = true
	common.BatchUpdateEnabled = false
	common.MemoryCacheEnabled = true
	common.RetryTimes = 1
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{
		"auto":"Auto",
		"auto-free-e2e":"Auto free",
		"auto-monthly-e2e":"Auto monthly"
	}`))
	require.NoError(t, setting.UpdateMaxTokenAutoGroups("2"))
	require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString("500"))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":                `{"gpt-auto-monthly-e2e":"tiered_expr"}`,
		"billing_setting.billing_expr":                `{"gpt-auto-monthly-e2e":"tier(\"base\", c * 15)"}`,
		"quota_setting.enable_free_model_pre_consume": "false",
		"group_ratio_setting.group_ratio":             `{"auto-free-e2e":0,"auto-monthly-e2e":0.5}`,
		"group_ratio_setting.model_tiered_ratios": `{
			"auto-monthly-e2e":{"gpt-auto-monthly-e2e":{
				"enabled":true,
				"effective_from":0,
				"effective_until":null,
				"timezone":"UTC",
				"tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]
			}}
		}`,
	}))

	var freeGroupAttempts atomic.Int32
	freeGroupUpstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		freeGroupAttempts.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte(`{"error":{"message":"retry on paid group","type":"upstream_error"}}`))
	}))
	t.Cleanup(freeGroupUpstream.Close)

	var monthlyGroupAttempts atomic.Int32
	monthlyGroupUpstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		monthlyGroupAttempts.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"id":"chatcmpl-auto-monthly",
			"object":"chat.completion",
			"created":1,
			"model":"gpt-auto-monthly-e2e",
			"choices":[{"index":0,"message":{"role":"assistant","content":"paid retry"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":1,"completion_tokens":1000,"total_tokens":1001}
		}`))
	}))
	t.Cleanup(monthlyGroupUpstream.Close)

	user := model.User{
		Username: "auto-monthly-discount-e2e",
		Status:   common.UserStatusEnabled,
		Group:    "auto-free-e2e",
		Quota:    1_000,
	}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{
		UserId:          user.Id,
		Key:             "automonthlydiscounte2ekey",
		Name:            "auto-monthly-discount-token",
		Status:          common.TokenStatusEnabled,
		ExpiredTime:     -1,
		RemainQuota:     1_000,
		UnlimitedQuota:  false,
		Group:           "auto",
		CrossGroupRetry: true,
	}
	require.NoError(t, token.SetAutoGroups([]string{"auto-free-e2e", "auto-monthly-e2e"}))
	require.NoError(t, model.DB.Create(&token).Error)

	priority := int64(100)
	freeGroupChannel := model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Name:     "auto-free-monthly-upstream",
		Key:      "free-test-key",
		Status:   common.ChannelStatusEnabled,
		BaseURL:  common.GetPointer(freeGroupUpstream.URL),
		Models:   "gpt-auto-monthly-e2e",
		Group:    "auto-free-e2e",
		Priority: &priority,
	}
	require.NoError(t, model.DB.Create(&freeGroupChannel).Error)
	require.NoError(t, freeGroupChannel.AddAbilities(nil))
	monthlyGroupChannel := model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Name:     "auto-monthly-upstream",
		Key:      "monthly-test-key",
		Status:   common.ChannelStatusEnabled,
		BaseURL:  common.GetPointer(monthlyGroupUpstream.URL),
		Models:   "gpt-auto-monthly-e2e",
		Group:    "auto-monthly-e2e",
		Priority: &priority,
	}
	require.NoError(t, model.DB.Create(&monthlyGroupChannel).Error)
	require.NoError(t, monthlyGroupChannel.AddAbilities(nil))
	model.InitChannelCache()

	engine := gin.New()
	SetRelayRouter(engine)
	body := []byte(`{"model":"gpt-auto-monthly-e2e","messages":[{"role":"user","content":"hello"}]}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer automonthlydiscounte2ekey")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Len(t, payload.Choices, 1)
	assert.Equal(t, "paid retry", payload.Choices[0].Message.Content)
	assert.GreaterOrEqual(t, freeGroupAttempts.Load(), int32(1))
	assert.Equal(t, int32(1), monthlyGroupAttempts.Load())

	require.NoError(t, model.DB.First(&user, user.Id).Error)
	assert.Equal(t, 988, user.Quota)
	assert.Equal(t, 12, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
	require.NoError(t, model.DB.First(&token, token.Id).Error)
	assert.Equal(t, 988, token.RemainQuota)
	assert.Equal(t, 12, token.UsedQuota)
	require.NoError(t, model.DB.First(&freeGroupChannel, freeGroupChannel.Id).Error)
	assert.Zero(t, freeGroupChannel.UsedQuota)
	require.NoError(t, model.DB.First(&monthlyGroupChannel, monthlyGroupChannel.Id).Error)
	assert.EqualValues(t, 12, monthlyGroupChannel.UsedQuota)

	var admissionOperations []model.BillingAdmissionReserveOperation
	require.NoError(t, model.DB.Order("id ASC").Find(&admissionOperations).Error)
	require.Len(t, admissionOperations, 1, "the free first group must not create a billing session")
	admission := admissionOperations[0]
	assert.Equal(t, model.BillingAdmissionReserveModeInitial, admission.Mode)
	assert.Equal(t, model.BillingAdmissionReserveStatusApplied, admission.Status)
	assert.Zero(t, admission.FromQuota)
	assert.Equal(t, 123, admission.TargetQuota)
	assert.Equal(t, 123, admission.FundingReservedQuota)
	assert.Equal(t, 123, admission.TokenReservedQuota)

	var freeGroupUsageCount int64
	require.NoError(t, model.DB.Model(&model.UserGroupModelMonthlyUsage{}).
		Where("user_id = ? AND using_group = ? AND origin_model = ?", user.Id, "auto-free-e2e", "gpt-auto-monthly-e2e").
		Count(&freeGroupUsageCount).Error)
	assert.Zero(t, freeGroupUsageCount, "the free first group must not create a monthly cursor")

	var usage model.UserGroupModelMonthlyUsage
	require.NoError(t, model.DB.Where(
		"user_id = ? AND using_group = ? AND origin_model = ?",
		user.Id,
		"auto-monthly-e2e",
		"gpt-auto-monthly-e2e",
	).First(&usage).Error)
	assert.EqualValues(t, 15, usage.OriginalQuota)
	assert.EqualValues(t, 12, usage.ChargedQuota)
	assert.Equal(t, "15", usage.ProgressQuota)

	var settlements []model.GroupModelDiscountSettlement
	require.NoError(t, model.DB.Order("id ASC").Find(&settlements).Error)
	require.Len(t, settlements, 1, "only the final using group may own the settlement")
	settlement := settlements[0]
	assert.Equal(t, "auto-monthly-e2e", settlement.UsingGroup)
	assert.Equal(t, "gpt-auto-monthly-e2e", settlement.OriginModel)
	assert.EqualValues(t, 15, settlement.OriginalQuota)
	assert.EqualValues(t, 12, settlement.ChargedQuota)
	assert.Equal(t, groupdiscount.ProgressBasisOriginal, settlement.ProgressBasis)
	assert.Equal(t, model.GroupModelDiscountStatusSettled, settlement.Status)
	assert.True(t, settlement.AccountingApplied)
	assert.Equal(t, user.Id, settlement.AccountingUserID)
	assert.Equal(t, monthlyGroupChannel.Id, settlement.AccountingChannelID)
	assert.Equal(t, 12, settlement.AccountingQuotaDelta)
	assert.Equal(t, 1, settlement.AccountingRequestCountDelta)

	var logs []model.Log
	require.NoError(t, model.DB.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, "auto-monthly-e2e", logs[0].Group)
	assert.Equal(t, monthlyGroupChannel.Id, logs[0].ChannelId)
	assert.Equal(t, 12, logs[0].Quota)
	assert.Contains(t, logs[0].Other, `"progress_basis":"original"`)
}
