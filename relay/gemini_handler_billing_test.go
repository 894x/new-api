package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type geminiThinkingAdapterBillingRecorder struct {
	preConsumed  int
	reserveCalls []int
	settleCalls  []int
}

func (b *geminiThinkingAdapterBillingRecorder) Settle(actualQuota int) error {
	b.settleCalls = append(b.settleCalls, actualQuota)
	b.preConsumed = actualQuota
	return nil
}

func (*geminiThinkingAdapterBillingRecorder) Refund(*gin.Context) {}

func (b *geminiThinkingAdapterBillingRecorder) NeedsRefund() bool {
	return b.preConsumed > 0
}

func (b *geminiThinkingAdapterBillingRecorder) GetPreConsumedQuota() int {
	return b.preConsumed
}

func (b *geminiThinkingAdapterBillingRecorder) Reserve(targetQuota int) error {
	return b.ReserveForAdmission(targetQuota)
}

func (b *geminiThinkingAdapterBillingRecorder) ReserveForAdmission(targetQuota int) error {
	b.reserveCalls = append(b.reserveCalls, targetQuota)
	if targetQuota > b.preConsumed {
		b.preConsumed = targetQuota
	}
	return nil
}

func TestGeminiThinkingAdapterRepricesAndSettlesFrozenNoThinkingMonthlyPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainDBType, previousLogDBType := common.MainDatabaseType(), common.LogDatabaseType()
	previousLogConsume, previousBatchUpdate := common.LogConsumeEnabled, common.BatchUpdateEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.LogConsumeEnabled = false
	common.BatchUpdateEnabled = false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainDBType, previousLogDBType)
		common.LogConsumeEnabled = previousLogConsume
		common.BatchUpdateEnabled = previousBatchUpdate
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Channel{},
		&model.UserGroupModelMonthlyUsage{},
		&model.GroupModelDiscountSettlement{},
		&model.GroupModelDiscountAdjustment{},
	))

	previousModelPrices := ratio_setting.ModelPrice2JSONString()
	previousGroupRatios := ratio_setting.GroupRatio2JSONString()
	previousTieredRatios := ratio_setting.ModelTieredRatios2JSONString()
	previousContracts := ratio_setting.GroupGroupRatio2JSONString()
	geminiSettings := model_setting.GetGeminiSettings()
	previousThinkingAdapter := geminiSettings.ThinkingAdapterEnabled
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousModelPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroupRatios))
		require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(previousTieredRatios))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(previousContracts))
		geminiSettings.ThinkingAdapterEnabled = previousThinkingAdapter
	})

	const (
		baseModel       = "gemini-priced-base"
		noThinkingModel = baseModel + "-nothinking"
		usingGroup      = "gemini-vip"
		requestID       = "gemini-thinking-adapter-monthly"
		userID          = 701
		channelID       = 702
	)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{
		"gemini-priced-base":0.0002,
		"gemini-priced-base-nothinking":0.0006
	}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"gemini-vip":0.5}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(`{
		"gemini-vip":{
			"gemini-priced-base":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.95}]},
			"gemini-priced-base-nothinking":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]}
		}
	}`))
	geminiSettings.ThinkingAdapterEnabled = true

	require.NoError(t, db.Create(&model.User{Id: userID, Username: "gemini-thinking-user"}).Error)
	require.NoError(t, db.Create(&model.Channel{Id: channelID, Name: "gemini-thinking-channel"}).Error)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1beta/models/"+baseModel+":generateContent", r.URL.Path)
		_, readErr := io.ReadAll(r.Body)
		assert.NoError(t, readErr)
		w.Header().Set("Content-Type", "application/json")
		_, writeErr := w.Write([]byte(`{
			"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]}}],
			"usageMetadata":{"promptTokenCount":1,"totalTokenCount":1}
		}`))
		assert.NoError(t, writeErr)
	}))
	t.Cleanup(upstream.Close)

	zeroThinkingBudget := 0
	request := &dto.GeminiChatRequest{
		GenerationConfig: dto.GeminiChatGenerationConfig{
			ThinkingConfig: &dto.GeminiThinkingConfig{ThinkingBudget: &zeroThinkingBudget},
		},
		Contents: []dto.GeminiChatContent{{
			Role:  "user",
			Parts: []dto.GeminiPart{{Text: "hello"}},
		}},
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/"+baseModel+":generateContent", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeGemini)
	common.SetContextKey(c, constant.ContextKeyChannelId, channelID)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "test-gemini-key")
	common.SetContextKey(c, constant.ContextKeyOriginalModel, baseModel)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, usingGroup)
	common.SetContextKey(c, constant.ContextKeyUserGroup, usingGroup)

	requestAt := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatGemini,
		RelayMode:       relayconstant.RelayModeGemini,
		RequestURLPath:  c.Request.URL.Path,
		Request:         request,
		OriginModelName: baseModel,
		UserId:          userID,
		UserQuota:       1_000_000,
		UserGroup:       usingGroup,
		UsingGroup:      usingGroup,
		RequestId:       requestID,
		StartTime:       requestAt,
	}
	info.SetEstimatePromptTokens(0)
	initialPrice, err := helper.ModelPriceHelper(c, info, 0, request.GetTokenCountMeta())
	require.NoError(t, err)
	require.Equal(t, 100, initialPrice.OriginalQuotaToPreConsume)
	require.NotNil(t, info.GroupModelDiscountSnapshot)
	require.Equal(t, baseModel, info.GroupModelDiscountSnapshot.OriginModel)
	billing := &geminiThinkingAdapterBillingRecorder{preConsumed: initialPrice.QuotaToPreConsume}
	info.Billing = billing
	info.FinalPreConsumedQuota = initialPrice.QuotaToPreConsume

	// The request-frozen resolver must retain the admission-time 0.8 policy for
	// the no-thinking alias even if the administrator edits the live option
	// before the adapter discovers that effective billing model.
	require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(`{
		"gemini-vip":{
			"gemini-priced-base":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.4}]},
			"gemini-priced-base-nothinking":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.2}]}
		}
	}`))
	require.True(t, isNoThinkingRequest(request))
	copiedRequest, err := common.DeepCopy(request)
	require.NoError(t, err)
	require.True(t, isNoThinkingRequest(copiedRequest))
	require.True(t, helper.HasModelBillingConfig(noThinkingModel))
	require.True(t, model_setting.GetGeminiSettings().ThinkingAdapterEnabled)

	newAPIError := GeminiHelper(c, info)

	require.Nil(t, newAPIError)
	require.Equal(t, noThinkingModel, info.OriginModelName)
	require.NotNil(t, info.GroupModelDiscountSnapshot)
	assert.Equal(t, noThinkingModel, info.GroupModelDiscountSnapshot.OriginModel)
	assert.Equal(t, 0.8, info.GroupModelDiscountSnapshot.Tiers[0].Ratio)
	assert.Equal(t, 0.0006, info.PriceData.ModelPrice)
	assert.Equal(t, 300, info.PriceData.OriginalQuotaToPreConsume)
	assert.Equal(t, []int{300}, billing.reserveCalls)
	assert.Equal(t, []int{240}, billing.settleCalls)

	settlement, err := model.GetGroupModelDiscountSettlement(requestID)
	require.NoError(t, err)
	assert.Equal(t, userID, settlement.UserID)
	assert.Equal(t, usingGroup, settlement.UsingGroup)
	assert.Equal(t, noThinkingModel, settlement.OriginModel)
	assert.EqualValues(t, 300, settlement.OriginalQuota)
	assert.EqualValues(t, 240, settlement.ChargedQuota)
	assert.Equal(t, model.GroupModelDiscountStatusSettled, settlement.Status)
}
