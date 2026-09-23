package relay

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/groupdiscount"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayhelper "github.com/QuantumNous/new-api/relay/helper"
	relaykitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
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

func TestTaskModel2DtoReplacesUpstreamRequestIDs(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("role", common.RoleAdminUser)
	c.Set(common.RequestIdKey, "request-local")
	task := &model.Task{
		TaskID:     "task_public",
		FailReason: "ratio is invalid. Request id: upstream-failure-id (code=InvalidParameter.TaskTypeConstraint)",
		Data:       []byte(`{"data":{"result_url":"ratio is invalid. Request id: upstream-data-id (code=InvalidParameter.TaskTypeConstraint)"}}`),
	}

	result := TaskModel2Dto(c, task)

	assert.Equal(t, "ratio is invalid. request id: request-local (code=InvalidParameter.TaskTypeConstraint)", result.FailReason)
	assert.JSONEq(t, `{"data":{"result_url":"ratio is invalid. request id: request-local (code=InvalidParameter.TaskTypeConstraint)"}}`, string(result.Data))
	assert.NotContains(t, string(result.Data), "upstream-data-id")
}

func TestTaskModel2DtoPreservesMeasuredUsageForCustomers(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("role", common.RoleCommonUser)
	usage := &types.TaskUsage{Kind: types.TaskUsageKindVideoDuration, Unit: types.TaskUsageUnitSecond, Input: 2, Output: 4, Total: 6, InputImages: common.GetPointer(0)}
	result := TaskModel2Dto(c, &model.Task{TaskID: "task_public", Usage: usage})
	encoded, err := common.Marshal(result)
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, common.Unmarshal(encoded, &body))
	assert.Equal(t, map[string]any{"kind": "video_duration", "unit": "second", "input": 2.0, "output": 4.0, "total": 6.0, "input_images": 0.0}, body["usage"])
}

func TestRelayTaskFetchReplacesUpstreamRequestIDInResponse(t *testing.T) {
	const testRelayMode = 987654
	originalBuilder, existed := fetchRespBuilders[testRelayMode]
	fetchRespBuilders[testRelayMode] = func(*gin.Context) ([]byte, *dto.TaskError) {
		return []byte(`{"status":"failed","error":{"message":"ratio is invalid. Request id: upstream-request-id (code=InvalidParameter.TaskTypeConstraint)"}}`), nil
	}
	t.Cleanup(func() {
		if existed {
			fetchRespBuilders[testRelayMode] = originalBuilder
			return
		}
		delete(fetchRespBuilders, testRelayMode)
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/video/generations/task_public", nil)
	c.Set(common.RequestIdKey, "request-local")

	taskErr := RelayTaskFetch(c, testRelayMode)

	require.Nil(t, taskErr)
	assert.JSONEq(t, `{"status":"failed","error":{"message":"ratio is invalid. request id: request-local (code=InvalidParameter.TaskTypeConstraint)"}}`, recorder.Body.String())
	assert.NotContains(t, recorder.Body.String(), "upstream-request-id")
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

func setupRelayChannelDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	previousType := common.MainDatabaseType()
	previousCache := common.MemoryCacheEnabled
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.Channel{}))
	model.DB = database
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
		common.MemoryCacheEnabled = previousCache
		require.NoError(t, sqlDB.Close())
	})
	return database
}

func TestApplyChannelPinPreservesOriginTasksAndRetryMode(t *testing.T) {
	database := setupRelayChannelDB(t)
	channel := &model.Channel{Name: "origin-channel", Key: "sk-test", Status: common.ChannelStatusEnabled, Type: constant.ChannelTypeDoubaoVideo}
	require.NoError(t, database.Create(channel).Error)
	originTask := &model.Task{
		TaskID: "task-lock", ChannelId: channel.Id, Action: "text_to_video", Status: model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "upstream-task-lock"},
		Data:        []byte(`{"id":"upstream-task-lock"}`),
	}

	for _, tc := range []struct {
		name     string
		tokenPin bool
		apply    func(*gin.Context, *relaycommon.RelayInfo) *dto.TaskError
	}{
		{name: "origin affinity", apply: ApplyOriginTaskAffinity},
		{name: "same channel retry", apply: ApplyChannelPin},
		{name: "token pin suppresses channel lock", tokenPin: true, apply: ApplyChannelPin},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			common.SetContextKey(c, constant.ContextKeyOriginTasks, []*model.Task{originTask})
			constraints := service.GetChannelConstraints(c)
			constraints.AddPin(dto.ChannelPin{ChannelId: channel.Id, Source: dto.PinSourceOriginTask, Rank: dto.PinRankOriginTask, RetryMode: dto.PinRetrySameChannel})
			if tc.tokenPin {
				constraints.AddPin(dto.ChannelPin{ChannelId: channel.Id, Source: dto.PinSourceToken, Rank: dto.PinRankToken, RetryMode: dto.PinRetrySingleAttempt})
			}
			info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
			require.Nil(t, tc.apply(c, info))
			if tc.tokenPin {
				assert.Nil(t, info.LockedChannel)
			} else {
				locked, ok := info.LockedChannel.(*model.Channel)
				require.True(t, ok)
				require.NotNil(t, locked)
				assert.Equal(t, channel.Id, locked.Id)
			}
			require.Len(t, info.OriginTasks, 1)
			assert.Equal(t, "task-lock", info.OriginTasks[0].TaskID)
			assert.Equal(t, "upstream-task-lock", info.OriginTasks[0].UpstreamTaskID)
			assert.Equal(t, "text_to_video", info.OriginTasks[0].Action)
			assert.Equal(t, string(model.TaskStatusSuccess), info.OriginTasks[0].Status)
			assert.Equal(t, []byte(originTask.Data), info.OriginTasks[0].Data)
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
func TestTaskModel2DtoNormalizesLegacyAction(t *testing.T) {
	task := &model.Task{Action: "firstTailGenerate"}

	dtoTask := TaskModel2Dto(nil, task)

	assert.Equal(t, constant.TaskActionFirstTailToVideo, dtoTask.Action)
	assert.Equal(t, "firstTailGenerate", task.Action)
}

const mappingOrderSubmitPlugin = `
export const meta = {apiVersion:1,key:"maporder",name:"Map Order",version:"1.0.0",author:{name:"Test"},models:["declared-model"],fetchMode:"per_task"};
export function buildSubmitRequest(ctx) {
  return {url: ctx.baseUrl+"/submit", method:"POST", body:{upstreamModel: ctx.upstreamModel, model: ctx.model}, action:"text_to_video"};
}
export function parseSubmitResponse(){return {taskId:"1"};}
export function buildQueryRequest(){return {url:"https://provider.example"};}
export function parseTaskResult(){return {status:"SUCCESS"};}
`

const mappingOrderRewritePlugin = `
export const meta = {apiVersion:1,key:"maporder-rw",name:"Map Order RW",version:"1.0.0",author:{name:"Test"},models:["declared-model"],fetchMode:"per_task"};
export function buildSubmitRequest(ctx) {
  return {url: ctx.baseUrl+"/submit", method:"POST", body:{upstreamModel: ctx.upstreamModel}, rewriteModel:"rewritten"};
}
export function parseSubmitResponse(){return {taskId:"1"};}
export function buildQueryRequest(){return {url:"https://provider.example"};}
export function parseTaskResult(){return {status:"SUCCESS"};}
`

func pinMappingOrderPlugin(t *testing.T, c *gin.Context, source string) {
	t.Helper()
	plugin, err := pluginruntime.NewRegistry().Register(source, pluginruntime.Options{})
	require.NoError(t, err)
	c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Plugin: plugin})
}

func newTaskSubmitContext(t *testing.T, originalModel, mapping string) (*gin.Context, *relaycommon.RelayInfo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, originalModel)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, "https://provider.example")
	if mapping != "" {
		c.Set("model_mapping", mapping)
	}
	c.Set("task_request", map[string]any{"prompt": "p"})
	return c, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
}

func TestRelayTaskSubmitMapsBeforeValidateWhenOriginSet(t *testing.T) {
	const mapping = `{"alias-model":"mid-model","mid-model":"declared-model"}`

	c, info := newTaskSubmitContext(t, "alias-model", mapping)
	pinMappingOrderPlugin(t, c, mappingOrderSubmitPlugin)
	info.OriginModelName = "alias-model"

	_, taskErr := RelayTaskSubmit(c, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, "model_price_error", taskErr.Code)
	assert.Equal(t, "alias-model", info.OriginModelName)
	assert.Equal(t, "declared-model", info.UpstreamModelName)
	assert.True(t, info.IsModelMapped)
}

func TestRelayTaskSubmitDeclaredNameWithoutMappingIsUnchanged(t *testing.T) {
	c, info := newTaskSubmitContext(t, "declared-model", "")
	pinMappingOrderPlugin(t, c, mappingOrderSubmitPlugin)
	info.OriginModelName = "declared-model"

	_, taskErr := RelayTaskSubmit(c, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, "model_price_error", taskErr.Code)
	assert.Equal(t, "declared-model", info.OriginModelName)
	assert.Equal(t, "declared-model", info.UpstreamModelName)
	assert.False(t, info.IsModelMapped)
}

func TestRelayTaskSubmitDoesNotApplyMappingTwice(t *testing.T) {
	c, info := newTaskSubmitContext(t, "alias-model", `{"alias-model":"declared-model"}`)
	pinMappingOrderPlugin(t, c, mappingOrderRewritePlugin)
	info.OriginModelName = "alias-model"

	_, taskErr := RelayTaskSubmit(c, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, "model_price_error", taskErr.Code)
	assert.Equal(t, "rewritten", info.UpstreamModelName, "late mapping would overwrite rewriteModel with the chain tail")
	assert.Equal(t, "alias-model", info.OriginModelName)
}

func TestRelayTaskSubmitEmptyOriginKeepsLateMapping(t *testing.T) {
	plugin, err := pluginruntime.NewRegistry().Register(mappingOrderSubmitPlugin, pluginruntime.Options{})
	require.NoError(t, err)
	synthesized := service.CoverTaskActionToModelName(constant.TaskPlatform(plugin.Meta.Key), "text_to_video")
	c, info := newTaskSubmitContext(t, "pre-validate-upstream",
		`{"pre-validate-upstream":"should-not-apply-early","`+synthesized+`":"legacy-tail"}`)
	c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Plugin: plugin})
	info.OriginModelName = ""

	_, taskErr := RelayTaskSubmit(c, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, "model_price_error", taskErr.Code)
	assert.Equal(t, synthesized, info.OriginModelName)
	assert.Equal(t, "legacy-tail", info.UpstreamModelName)
	assert.True(t, info.IsModelMapped)
}

const billingFallbackPlugin = `
export const meta = {apiVersion:1,key:"bill-fallback",name:"Bill Fallback",version:"1.0.0",author:{name:"Test"},models:["declared-model"],fetchMode:"per_task"};
export function buildSubmitRequest(ctx) {
  return {url: ctx.baseUrl+"/submit", method:"POST", body:{upstreamModel: ctx.upstreamModel, model: ctx.model}, action:"text_to_video"};
}
export function parseSubmitResponse(){return {taskId:"1"};}
export function buildQueryRequest(){return {url:"https://provider.example"};}
export function parseTaskResult(){return {status:"SUCCESS"};}
`

func saveBillingConfig(t *testing.T) {
	t.Helper()
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})
}

func TestRelayTaskSubmitAliasBillingIdentityAndExprFallback(t *testing.T) {
	const mapping = `{"alias-model":"declared-model"}`
	const aliasExpr = `tier("alias", 2)`
	const tailExpr = `tier("tail", 3)`
	const legacyExpr = `tier("legacy", u("old_units") * 2)`

	tests := []struct {
		name       string
		modes      map[string]string
		exprs      map[string]string
		wantTiered bool
		wantExpr   string
		source     string
	}{
		{
			name:       "alias own tiered wins",
			modes:      map[string]string{"alias-model": "tiered_expr", "declared-model": "tiered_expr"},
			exprs:      map[string]string{"alias-model": aliasExpr, "declared-model": tailExpr},
			wantTiered: true,
			wantExpr:   aliasExpr,
		},
		{
			name:       "fallback uses tail expr",
			modes:      map[string]string{"declared-model": "tiered_expr"},
			exprs:      map[string]string{"declared-model": tailExpr},
			wantTiered: true,
			wantExpr:   tailExpr,
		},
		{
			name:       "explicit alias ratio mode still inherits tail expr",
			modes:      map[string]string{"alias-model": "ratio", "declared-model": "tiered_expr"},
			exprs:      map[string]string{"declared-model": tailExpr},
			wantTiered: true,
			wantExpr:   tailExpr,
		},
		{
			name:       "stored expression keeps running after schema narrows",
			modes:      map[string]string{"declared-model": "tiered_expr"},
			exprs:      map[string]string{"declared-model": legacyExpr},
			wantTiered: true,
			wantExpr:   legacyExpr,
			source: strings.Replace(billingFallbackPlugin, `fetchMode:"per_task"`, `fetchMode:"per_task", usageSchema:{old_units:{type:"number",unit:"count"}}, usageProfiles:[{models:["declared-model"],schema:{seconds:{type:"number",unit:"second"}}}]`, 1) + `
export function extractUsage(){return {old_units:2};}
`,
		},
		{
			name:       "neither tiered uses ordinary pricing",
			wantTiered: false,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			saveBillingConfig(t)
			if len(testCase.modes) > 0 {
				modeJSON, marshalErr := common.Marshal(testCase.modes)
				require.NoError(t, marshalErr)
				exprJSON, marshalErr := common.Marshal(testCase.exprs)
				require.NoError(t, marshalErr)
				require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
					"billing_setting.billing_mode": string(modeJSON),
					"billing_setting.billing_expr": string(exprJSON),
				}))
				if testCase.wantExpr == aliasExpr {
					require.Equal(t, billing_setting.BillingModeTieredExpr, billing_setting.GetBillingMode("alias-model"))
				} else {
					require.Equal(t, billing_setting.BillingModeRatio, billing_setting.GetBillingMode("alias-model"))
					require.Equal(t, billing_setting.BillingModeTieredExpr, billing_setting.GetBillingMode("declared-model"))
				}
			}

			c, info := newTaskSubmitContext(t, "alias-model", mapping)
			c.Set("group", "default")
			info.UserGroup = "default"
			info.UsingGroup = "default"
			source := testCase.source
			if source == "" {
				source = billingFallbackPlugin
			}
			pinMappingOrderPlugin(t, c, source)
			info.OriginModelName = "alias-model"

			_, taskErr := RelayTaskSubmit(c, info)
			require.NotNil(t, taskErr)
			assert.Equal(t, "alias-model", info.OriginModelName)
			assert.Equal(t, "declared-model", info.UpstreamModelName)
			assert.True(t, info.IsModelMapped)

			task := model.InitTask(constant.TaskPlatform("bill-fallback"), info)
			assert.Equal(t, "alias-model", task.Properties.OriginModelName)
			assert.Equal(t, "declared-model", task.Properties.UpstreamModelName)

			if testCase.wantTiered {
				require.NotNil(t, info.TieredBillingSnapshot, "submission error: %+v", taskErr)
				assert.Equal(t, "alias-model", info.TieredBillingSnapshot.ModelName)
				assert.Equal(t, testCase.wantExpr, info.TieredBillingSnapshot.ExprString)
				assert.Equal(t, billingexpr.ExprHashString(testCase.wantExpr), info.TieredBillingSnapshot.ExprHash)
				assert.NotEqual(t, "model_price_error", taskErr.Code)
				if testCase.wantExpr == legacyExpr {
					assert.Equal(t, 4*common.QuotaPerUnit, info.TieredBillingSnapshot.EstimatedQuotaBeforeGroup)
				}
			} else {
				assert.Nil(t, info.TieredBillingSnapshot)
				assert.Equal(t, "model_price_error", taskErr.Code)
			}
		})
	}
}

func TestSharedTaskBillingExpressionSelectionAndFrozenSettlement(t *testing.T) {
	const baseExpr = `tier("base", u("seconds") * 2)`
	const alphaExpr = `tier("alpha", u("seconds") * 3)`
	const betaExpr = `tier("beta", u("credits") * 5)`
	const aliasExpr = `tier("alias", u("credits") * 7)`
	for _, tc := range []struct {
		name, plugin, model, mapping, modelExpr, mode, wantExpr string
		frozenPlugin, frozenExpr                                string
		variants                                                map[string]string
		wantPriceError, profiled                                bool
	}{
		{name: "executing plugin override", plugin: "billing-beta", model: "declared-model", modelExpr: baseExpr, mode: "tiered_expr", variants: map[string]string{"billing-alpha::declared-model": alphaExpr, "billing-beta::declared-model": betaExpr}, wantExpr: betaExpr},
		{name: "override ignores model mode", plugin: "billing-beta", model: "declared-model", modelExpr: baseExpr, mode: "ratio", variants: map[string]string{"billing-beta::declared-model": betaExpr}, wantExpr: betaExpr},
		{name: "model expression fallback", plugin: "billing-alpha", model: "declared-model", modelExpr: baseExpr, mode: "tiered_expr", wantExpr: baseExpr},
		{name: "alias override precedes mapped override", plugin: "billing-beta", model: "alias-model", mapping: `{"alias-model":"declared-model"}`, modelExpr: baseExpr, mode: "tiered_expr", variants: map[string]string{"billing-beta::declared-model": betaExpr, "billing-beta::alias-model": aliasExpr}, wantExpr: aliasExpr},
		{name: "mapped override precedes model fallback", plugin: "billing-beta", model: "alias-model", mapping: `{"alias-model":"declared-model"}`, modelExpr: baseExpr, mode: "tiered_expr", variants: map[string]string{"billing-beta::declared-model": betaExpr}, wantExpr: betaExpr},
		{name: "unconfigured plugin cannot use another schema", plugin: "billing-beta", model: "declared-model", modelExpr: baseExpr, mode: "tiered_expr", wantPriceError: true},
		{name: "missing usage in skipped branch remains incompatible", plugin: "billing-beta", model: "declared-model", modelExpr: `true ? tier("free", 0) : tier("missing", u("seconds"))`, mode: "tiered_expr", wantPriceError: true},
		{name: "fixed pricing is still rejected", plugin: "billing-beta", model: "declared-model", variants: map[string]string{"billing-beta::declared-model": `tier("fixed", fixed(1))`}, wantPriceError: true},
		{name: "same plugin retry keeps frozen expression", plugin: "billing-alpha", model: "declared-model", modelExpr: baseExpr, mode: "tiered_expr", variants: map[string]string{"billing-alpha::declared-model": alphaExpr}, frozenPlugin: "billing-alpha", frozenExpr: baseExpr, wantExpr: baseExpr},
		{name: "different plugin retry selects its own schema and price", plugin: "billing-beta", model: "declared-model", modelExpr: baseExpr, mode: "tiered_expr", variants: map[string]string{"billing-beta::declared-model": betaExpr}, frozenPlugin: "billing-alpha", frozenExpr: alphaExpr, wantExpr: betaExpr},
		{name: "endpoint mapping keeps the declared profile", plugin: "billing-beta", model: "declared-model", mapping: `{"declared-model":"ep-endpoint"}`, modelExpr: baseExpr, mode: "tiered_expr", variants: map[string]string{"billing-beta::declared-model": betaExpr}, wantExpr: betaExpr, profiled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			saveBillingConfig(t)
			registry := pluginruntime.NewRegistry()
			for _, spec := range []struct{ key, field, unit string }{{"billing-alpha", "seconds", "second"}, {"billing-beta", "credits", "credit"}} {
				source := strings.ReplaceAll(billingFallbackPlugin, "bill-fallback", spec.key)
				schema := `usageSchema:{` + spec.field + `:{type:"number",unit:"` + spec.unit + `"}}`
				if tc.profiled && spec.key == "billing-beta" {
					// The endpoint ID is undeclared, so only the declared model's profile carries this schema.
					schema = `usageSchema:{seconds:{type:"number",unit:"second"}},usageProfiles:[{models:["declared-model"],schema:{` + spec.field + `:{type:"number",unit:"` + spec.unit + `"}}}]`
				}
				source = strings.Replace(source, `fetchMode:"per_task"`, `fetchMode:"per_task",`+schema, 1)
				source += `export function extractUsage(){return {` + spec.field + `:2};}`
				_, err := registry.Register(source, pluginruntime.Options{})
				require.NoError(t, err)
			}
			variants := tc.variants
			if variants == nil {
				variants = map[string]string{}
			}
			rawVariants, err := common.Marshal(variants)
			require.NoError(t, err)
			modes, err := common.Marshal(map[string]string{"declared-model": tc.mode})
			require.NoError(t, err)
			expressions, err := common.Marshal(map[string]string{"declared-model": tc.modelExpr})
			require.NoError(t, err)
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
				billing_setting.PluginBillingExprOption: string(rawVariants), "billing_setting.billing_mode": string(modes), "billing_setting.billing_expr": string(expressions),
			}))
			c, info := newTaskSubmitContext(t, tc.model, tc.mapping)
			c.Set("group", "default")
			c.Set("task_plugin_key", tc.plugin)
			info.UserGroup = "default"
			info.UsingGroup = "default"
			info.OriginModelName = tc.model
			plugin, ok := registry.Generation().Get(tc.plugin)
			require.True(t, ok)
			c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Generation: registry.Generation(), Plugin: plugin})
			if tc.frozenPlugin != "" {
				info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{
					BillingMode: "tiered_expr", TaskUsageBilling: true, ModelName: tc.model,
					TaskPluginKey: tc.frozenPlugin, ExprString: tc.frozenExpr, QuotaPerUnit: common.QuotaPerUnit,
				}
			}
			_, taskErr := RelayTaskSubmit(c, info)
			require.NotNil(t, taskErr) // This fixture stops at reservation, before upstream submission.
			if tc.wantPriceError {
				assert.Equal(t, "model_price_error", taskErr.Code)
				assert.Nil(t, info.TieredBillingSnapshot)
				return
			}
			require.NotNil(t, info.TieredBillingSnapshot, "submission error: %+v", taskErr)
			assert.Equal(t, tc.wantExpr, info.TieredBillingSnapshot.ExprString)
			assert.NotEqual(t, "model_price_error", taskErr.Code)
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{billing_setting.PluginBillingExprOption: `{}`, "billing_setting.billing_expr": `{}`}))
			field := "seconds"
			if tc.plugin == "billing-beta" {
				field = "credits"
			}
			result, usage, err := service.EvaluateTaskCompletionUsage(info.TieredBillingSnapshot, map[string]any{field: float64(4)})
			require.NoError(t, err)
			assert.Equal(t, float64(4), usage[field])
			assert.Equal(t, float64(2), info.TieredBillingSnapshot.UsageFacts[field])
			assert.Equal(t, 2*info.TieredBillingSnapshot.EstimatedQuotaAfterGroup, result.ActualQuotaAfterGroup)
			assert.Equal(t, tc.wantExpr, info.TieredBillingSnapshot.ExprString)
		})
	}
}
