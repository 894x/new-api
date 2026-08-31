package relay

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/groupdiscount"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayhelper "github.com/QuantumNous/new-api/relay/helper"
	relaykitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type taskReserveRecordingBilling struct {
	preConsumed       int
	reserveErr        error
	reserves          []int
	admissionReserves []int
}

func (*taskReserveRecordingBilling) Settle(int) error { return nil }

func (*taskReserveRecordingBilling) Refund(*gin.Context) {}

func (s *taskReserveRecordingBilling) NeedsRefund() bool { return s.preConsumed > 0 }

func (s *taskReserveRecordingBilling) GetPreConsumedQuota() int { return s.preConsumed }

func (s *taskReserveRecordingBilling) Reserve(targetQuota int) error {
	s.reserves = append(s.reserves, targetQuota)
	if targetQuota > s.preConsumed {
		s.preConsumed = targetQuota
	}
	return nil
}

func (s *taskReserveRecordingBilling) ReserveForAdmission(targetQuota int) error {
	s.admissionReserves = append(s.admissionReserves, targetQuota)
	if s.reserveErr != nil {
		return s.reserveErr
	}
	if targetQuota > s.preConsumed {
		s.preConsumed = targetQuota
	}
	return nil
}

func TestTaskSubmitResponseStaysBufferedUntilDurableCommit(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	original := c.Writer
	buffered := newTaskBufferedResponseWriter(original)
	c.Writer = buffered
	c.Header("X-Task-Billing", "prepared")
	c.JSON(http.StatusAccepted, gin.H{"id": "task_public"})
	c.Writer = original

	assert.Empty(t, recorder.Body.String())
	assert.Empty(t, recorder.Header().Get("X-Task-Billing"))

	result := &TaskSubmitResult{
		responseStatus: buffered.Status(),
		responseHeader: buffered.Header().Clone(),
		responseBody:   append([]byte(nil), buffered.body.Bytes()...),
	}
	require.NoError(t, result.WriteResponse(c))
	assert.Equal(t, http.StatusAccepted, recorder.Code)
	assert.Equal(t, "prepared", recorder.Header().Get("X-Task-Billing"))
	assert.JSONEq(t, `{"id":"task_public"}`, recorder.Body.String())
}

func TestTaskSubmitBufferedSuccessCanBeDiscardedForPersistenceError(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	original := c.Writer
	buffered := newTaskBufferedResponseWriter(original)
	c.Writer = buffered
	c.JSON(http.StatusOK, gin.H{"id": "task_public"})
	c.Writer = original

	c.JSON(http.StatusInternalServerError, gin.H{"error": "insert_task_failed"})

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "task_public")
	assert.Contains(t, recorder.Body.String(), "insert_task_failed")
}

func TestRecalculateTaskSubmitRatiosRebuildsOriginalAndNetFromFrozenPriceInputs(t *testing.T) {
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

	priceData := types.PriceData{
		ModelPrice:    2,
		UsePrice:      true,
		Quota:         99,
		OriginalQuota: 777,
		GroupRatioInfo: types.GroupRatioInfo{
			GroupRatio: 0.25,
		},
	}
	priceData.AddOtherRatio("seconds", 3)
	info := &relaycommon.RelayInfo{PriceData: priceData}

	netQuota, ok := recalcQuotaFromRatios(info, map[string]float64{"seconds": 2})

	require.True(t, ok)
	// The adjusted amount is rebuilt from the frozen model price, never by
	// dividing the already rounded/discounted 99-quota value.
	assert.Equal(t, 100, netQuota)
	assert.Equal(t, 400, info.PriceData.OriginalQuota)
	assert.Equal(t, 100, info.PriceData.Quota)
}

func TestTaskQuotaToPreConsumeUsesOriginalOnlyForCapturedMonthlyPolicy(t *testing.T) {
	info := &relaycommon.RelayInfo{PriceData: types.PriceData{Quota: 250, OriginalQuota: 1000}}

	assert.Equal(t, 250, taskQuotaToPreConsume(info))

	info.GroupModelDiscountSnapshot = &groupdiscount.Snapshot{}
	assert.Equal(t, 1000, taskQuotaToPreConsume(info))
}

func TestRecalculateTaskSubmitRatiosKeepsPositiveMonthlyOriginalBillable(t *testing.T) {
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

	info := &relaycommon.RelayInfo{
		PriceData: types.PriceData{
			ModelPrice: 0.005,
			UsePrice:   true,
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio: 0.5,
			},
		},
		GroupModelDiscountSnapshot: &groupdiscount.Snapshot{},
	}

	_, ok := recalcQuotaFromRatios(info, map[string]float64{"identity": 1})

	require.True(t, ok)
	assert.Equal(t, 1, info.PriceData.OriginalQuota)
	assert.Zero(t, info.PriceData.Quota)
}

func TestRecalculateTaskSubmitRatiosPreservesLegacyNetRoundingOrder(t *testing.T) {
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

	info := &relaycommon.RelayInfo{PriceData: types.PriceData{
		ModelPrice: 0.019,
		UsePrice:   true,
		GroupRatioInfo: types.GroupRatioInfo{
			GroupRatio: 0.5,
		},
	}}

	netQuota, ok := recalcQuotaFromRatios(info, map[string]float64{"seconds": 2})

	require.True(t, ok)
	// Legacy fixed-group path truncates 1.9*0.5 to zero before applying
	// request ratios. The independently computed original remains 1.9*2=3.
	assert.Zero(t, netQuota)
	assert.Equal(t, 3, info.PriceData.OriginalQuota)
}

func TestRecalculateTaskSubmitRatiosAuditsOnlyActiveChargeClamp(t *testing.T) {
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 1
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

	newInfo := func(groupRatio float64) *relaycommon.RelayInfo {
		return &relaycommon.RelayInfo{PriceData: types.PriceData{
			ModelPrice: float64(common.MaxQuota) * 4,
			UsePrice:   true,
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio: groupRatio,
			},
		}}
	}

	t.Run("inactive policy ignores overflowing informational original", func(t *testing.T) {
		info := newInfo(0.125)
		netQuota, ok := recalcQuotaFromRatios(info, map[string]float64{"identity": 1})

		require.True(t, ok)
		assert.Equal(t, common.MaxQuota/2, netQuota)
		assert.Nil(t, info.QuotaClamp)
	})

	t.Run("inactive policy still audits overflowing legacy net", func(t *testing.T) {
		info := newInfo(0.75)
		_, ok := recalcQuotaFromRatios(info, map[string]float64{"identity": 1})

		require.True(t, ok)
		require.NotNil(t, info.QuotaClamp)
		assert.Equal(t, common.QuotaClampOverflow, info.QuotaClamp.Kind)
		assert.Equal(t, float64(common.MaxQuota)*3, info.QuotaClamp.Original)
	})

	t.Run("active policy audits overflowing original", func(t *testing.T) {
		info := newInfo(0.125)
		info.GroupModelDiscountSnapshot = &groupdiscount.Snapshot{}
		_, ok := recalcQuotaFromRatios(info, map[string]float64{"identity": 1})

		require.True(t, ok)
		require.NotNil(t, info.QuotaClamp)
		assert.Equal(t, float64(common.MaxQuota)*4, info.QuotaClamp.Original)
	})
}

func TestResolveOriginTaskBindsMonthlyPolicyToRestoredOriginModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(
		&model.Task{},
		&model.Channel{},
		&model.UserGroupModelMonthlyUsage{},
		&model.GroupModelDiscountSettlement{},
		&model.GroupModelDiscountAdjustment{},
	))

	previousTiered := ratio_setting.ModelTieredRatios2JSONString()
	previousContracts := ratio_setting.GroupGroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(previousTiered))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(previousContracts))
	})
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))
	admissionPolicies := `{
		"vip":{
			"resolved-exact":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]},
			"*":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.7}]}
		}
	}`
	replacementPolicies := `{
		"vip":{
			"resolved-exact":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.2}]},
			"*":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.1}]}
		}
	}`

	channel := &model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-origin-task",
		Status: common.ChannelStatusEnabled,
		Name:   "origin-task-channel",
		Models: "resolved-exact,resolved-wildcard",
		Group:  "vip",
	}
	require.NoError(t, db.Create(channel).Error)

	requestAt := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name         string
		userID       int
		taskID       string
		originModel  string
		matchedModel string
		wantRatio    float64
	}{
		{
			name:         "exact model policy",
			userID:       7101,
			taskID:       "origin-exact-task",
			originModel:  "resolved-exact",
			matchedModel: "resolved-exact",
			wantRatio:    0.8,
		},
		{
			name:         "wildcard policy keeps real origin model",
			userID:       7102,
			taskID:       "origin-wildcard-task",
			originModel:  "resolved-wildcard",
			matchedModel: "*",
			wantRatio:    0.7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(admissionPolicies))
			originTask := &model.Task{
				TaskID:    tt.taskID,
				UserId:    tt.userID,
				ChannelId: channel.Id,
				Properties: model.Properties{
					OriginModelName: tt.originModel,
				},
			}
			require.NoError(t, db.Create(originTask).Error)

			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/"+tt.taskID+"/remix", nil)
			ctx.Params = gin.Params{{Key: "video_id", Value: tt.taskID}}
			info := &relaycommon.RelayInfo{
				UserId:        tt.userID,
				UserGroup:     "ordinary-user",
				UsingGroup:    "vip",
				StartTime:     requestAt,
				ChannelMeta:   &relaycommon.ChannelMeta{ChannelId: channel.Id},
				TaskRelayInfo: &relaycommon.TaskRelayInfo{},
				// This is the resolver produced at admission for a continuation
				// request whose model is only known after loading the origin task.
				GroupModelDiscountResolver: ratio_setting.CaptureModelTieredDiscountResolver(
					"ordinary-user", "", requestAt,
				),
			}
			require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(replacementPolicies))

			require.Nil(t, ResolveOriginTask(ctx, info))
			assert.Equal(t, tt.originModel, info.OriginModelName)

			snapshot, active, err := info.ResolveGroupModelDiscount()
			require.NoError(t, err)
			require.True(t, active)
			assert.Equal(t, tt.originModel, snapshot.OriginModel)
			assert.Equal(t, tt.matchedModel, snapshot.MatchedModel)
			require.Len(t, snapshot.Tiers, 1)
			assert.Equal(t, tt.wantRatio, snapshot.Tiers[0].Ratio)

			reservation, err := model.ReserveGroupModelDiscount(model.GroupModelDiscountReserveInput{
				RequestID:     "settle-" + tt.taskID,
				UserID:        tt.userID,
				UsingGroup:    info.UsingGroup,
				OriginModel:   info.OriginModelName,
				Snapshot:      snapshot,
				OriginalQuota: 100,
			})
			require.NoError(t, err, "the restored model and frozen snapshot must form one ledger scope")
			assert.Equal(t, tt.originModel, reservation.Settlement.OriginModel)
		})
	}
}

func TestReserveTaskSubmitQuotaUsesNewGroupMonthlyOriginalOnRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousQuotaPerUnit := common.QuotaPerUnit
	previousPrices := ratio_setting.ModelPrice2JSONString()
	previousGroups := ratio_setting.GroupRatio2JSONString()
	previousTiered := ratio_setting.ModelTieredRatios2JSONString()
	previousContracts := ratio_setting.GroupGroupRatio2JSONString()
	t.Cleanup(func() {
		common.QuotaPerUnit = previousQuotaPerUnit
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroups))
		require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(previousTiered))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(previousContracts))
	})

	common.QuotaPerUnit = 1_000
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"task-retry-model":1}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"legacy-low":0.1,"monthly-protected":0.1}`))
	require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(`{
		"monthly-protected":{
			"task-retry-model":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]}
		}
	}`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	requestAt := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	info := &relaycommon.RelayInfo{
		UserGroup:       "ordinary-user",
		UsingGroup:      "legacy-low",
		OriginModelName: "task-retry-model",
		StartTime:       requestAt,
		GroupModelDiscountResolver: ratio_setting.CaptureModelTieredDiscountResolver(
			"ordinary-user", "task-retry-model", requestAt,
		),
	}
	legacyPrice, err := relayhelper.ModelPriceHelperPerCall(ctx, info)
	require.NoError(t, err)
	require.Nil(t, info.GroupModelDiscountSnapshot)
	assert.Equal(t, 100, legacyPrice.Quota)

	reserveErr := errors.New("insufficient quota while raising retry reservation")
	billing := &taskReserveRecordingBilling{preConsumed: legacyPrice.Quota, reserveErr: reserveErr}
	info.Billing = billing
	info.PriceData = legacyPrice

	// Auto routing retries the same request in a group protected by a monthly
	// policy. Repricing still has the same legacy fallback net, but the safe
	// reservation target becomes the true pre-group original quota.
	info.UsingGroup = "monthly-protected"
	monthlyPrice, err := relayhelper.ModelPriceHelperPerCall(ctx, info)
	require.NoError(t, err)
	info.PriceData = monthlyPrice
	require.NotNil(t, info.GroupModelDiscountSnapshot)
	assert.Equal(t, 100, monthlyPrice.Quota)
	assert.Equal(t, 1_000, monthlyPrice.OriginalQuota)

	taskErr := reserveTaskSubmitQuota(ctx, info)
	require.NotNil(t, taskErr)
	assert.ErrorIs(t, taskErr.Error, reserveErr)
	assert.Empty(t, billing.reserves, "task admission must not use the debt-permitting post-upstream reserve path")
	assert.Equal(t, []int{1_000}, billing.admissionReserves)
}

func TestRelayTaskSubmitRetryRejectsMonthlyOriginalBeforeUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB := model.DB
	previousRedisEnabled := common.RedisEnabled
	previousBatchUpdateEnabled := common.BatchUpdateEnabled
	previousQuotaPerUnit := common.QuotaPerUnit
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB = db
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.QuotaPerUnit = 1_000
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedisEnabled
		common.BatchUpdateEnabled = previousBatchUpdateEnabled
		common.QuotaPerUnit = previousQuotaPerUnit
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Token{},
		&model.UserSubscription{},
		&model.BillingAdmissionReserveOperation{},
	))

	previousPrices := ratio_setting.ModelPrice2JSONString()
	previousGroups := ratio_setting.GroupRatio2JSONString()
	previousTiered := ratio_setting.ModelTieredRatios2JSONString()
	previousContracts := ratio_setting.GroupGroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroups))
		require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(previousTiered))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(previousContracts))
	})
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"task-strict-retry":1}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"legacy-low":0.1,"monthly-protected":0.1}`))
	require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(`{
		"monthly-protected":{
			"task-strict-retry":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]}
		}
	}`))

	const (
		userID   = 78901
		tokenID  = 78902
		tokenKey = "task-strict-retry-token"
	)
	require.NoError(t, db.Create(&model.User{
		Id:       userID,
		Username: "task-strict-retry-user",
		Status:   common.UserStatusEnabled,
		Group:    "ordinary-user",
		Quota:    150,
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		Id:             tokenID,
		UserId:         userID,
		Key:            tokenKey,
		Name:           "task-strict-retry-token",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		RemainQuota:    150,
		UnlimitedQuota: false,
	}).Error)

	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"base_resp":{"status_code":0},"task_id":"must-not-exist"}`))
	}))
	t.Cleanup(upstream.Close)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewBufferString(`{"prompt":"hello"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeMiniMax)
	common.SetContextKey(ctx, constant.ContextKeyChannelId, 991)
	common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, upstream.URL)
	common.SetContextKey(ctx, constant.ContextKeyChannelKey, "sk-upstream")
	common.SetContextKey(ctx, constant.ContextKeyOriginalModel, "task-strict-retry")

	requestAt := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	info := &relaycommon.RelayInfo{
		RequestId:       "task-strict-retry-request",
		UserId:          userID,
		UserQuota:       150,
		UserGroup:       "ordinary-user",
		UsingGroup:      "legacy-low",
		OriginModelName: "task-strict-retry",
		TokenId:         tokenID,
		TokenKey:        tokenKey,
		StartTime:       requestAt,
		ForcePreConsume: true,
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		UserSetting: relaykitdto.UserSetting{
			BillingPreference: "wallet_only",
		},
		GroupModelDiscountResolver: ratio_setting.CaptureModelTieredDiscountResolver(
			"ordinary-user", "task-strict-retry", requestAt,
		),
		GroupModelDiscountResolverOriginModel: "task-strict-retry",
	}
	legacyPrice, err := relayhelper.ModelPriceHelperPerCall(ctx, info)
	require.NoError(t, err)
	info.PriceData = legacyPrice
	require.Equal(t, 100, legacyPrice.Quota)
	require.Nil(t, service.PreConsumeBilling(ctx, legacyPrice.Quota, info))

	// Auto-group selection has moved this retry to the monthly-protected group.
	// RelayTaskSubmit must reprice and atomically raise the existing reservation
	// to the 1,000 original quota before it can reach BuildRequest/DoRequest.
	info.UsingGroup = "monthly-protected"
	result, taskErr := RelayTaskSubmit(ctx, info)
	require.Nil(t, result)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusForbidden, taskErr.StatusCode)
	assert.Equal(t, int32(0), upstreamCalls.Load())

	var user model.User
	require.NoError(t, db.First(&user, userID).Error)
	assert.Equal(t, 50, user.Quota)
	var token model.Token
	require.NoError(t, db.First(&token, tokenID).Error)
	assert.Equal(t, 50, token.RemainQuota)
}
