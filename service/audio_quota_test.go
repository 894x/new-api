package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/groupdiscount"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	hosttypes "github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateAudioQuotaKeepsOriginalBeforeGroupDiscount(t *testing.T) {
	result := calculateAudioQuota(QuotaInfo{
		InputDetails:  TokenDetails{TextTokens: 100, AudioTokens: 20},
		OutputDetails: TokenDetails{TextTokens: 50, AudioTokens: 10},
		ModelName:     "audio-original-test",
		ModelRatio:    2,
		GroupRatio:    0.5,
	})

	require.Nil(t, result.OriginalClamp)
	require.Nil(t, result.ChargedClamp)
	// Default completion/audio ratios are exercised through the production
	// setting lookup; the relationship must always be exactly pre-group vs.
	// post-group for a 0.5 group ratio.
	assert.Greater(t, result.OriginalQuota, 0)
	assert.Equal(t, result.OriginalQuota/2, result.ChargedQuota)
}

func TestCalculateAudioQuotaFixedPriceKeepsOriginalBeforeGroupDiscount(t *testing.T) {
	result := calculateAudioQuota(QuotaInfo{
		UsePrice:   true,
		ModelPrice: 0.04,
		GroupRatio: 0.5,
	})

	require.Nil(t, result.OriginalClamp)
	require.Nil(t, result.ChargedClamp)
	assert.Equal(t, common.QuotaFromFloat(0.04*common.QuotaPerUnit), result.OriginalQuota)
	assert.Equal(t, common.QuotaFromFloat(0.04*common.QuotaPerUnit*0.5), result.ChargedQuota)
}

func TestCalculateAudioQuotaKeepsPositiveOriginalLedgerEligible(t *testing.T) {
	result := calculateAudioQuota(QuotaInfo{
		UsePrice:   true,
		ModelPrice: 0.4 / common.QuotaPerUnit,
		GroupRatio: 2,
	})

	require.Nil(t, result.OriginalClamp)
	assert.Equal(t, 1, result.OriginalQuota)
	assert.Equal(t, 1, result.ChargedQuota)
}

type wssRecordingBillingSettler struct {
	preConsumed int
	reserves    []int
	settles     []int
}

func (s *wssRecordingBillingSettler) Settle(actualQuota int) error {
	s.settles = append(s.settles, actualQuota)
	return nil
}

func (s *wssRecordingBillingSettler) Refund(*gin.Context) {}

func (s *wssRecordingBillingSettler) NeedsRefund() bool { return s.preConsumed > 0 }

func (s *wssRecordingBillingSettler) GetPreConsumedQuota() int { return s.preConsumed }

func (s *wssRecordingBillingSettler) Reserve(targetQuota int) error {
	s.reserves = append(s.reserves, targetQuota)
	if targetQuota > s.preConsumed {
		s.preConsumed = targetQuota
	}
	return nil
}

func (s *wssRecordingBillingSettler) ReserveForAdmission(targetQuota int) error {
	return s.Reserve(targetQuota)
}

func TestPreWssConsumeQuotaOnlyReservesCumulativeTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	billing := &wssRecordingBillingSettler{}
	relayInfo := &relaycommon.RelayInfo{
		OriginModelName: "wss-original-test",
		Billing:         billing,
		PriceData: hosttypes.PriceData{
			ModelRatio: 1,
			GroupRatioInfo: hosttypes.GroupRatioInfo{
				GroupRatio: 0.5,
			},
		},
	}

	firstTotal := &dto.RealtimeUsage{
		TotalTokens:       100,
		InputTokens:       100,
		InputTokenDetails: dto.InputTokenDetails{TextTokens: 100},
	}
	require.NoError(t, PreWssConsumeQuota(ctx, relayInfo, firstTotal))

	secondTotal := &dto.RealtimeUsage{
		TotalTokens:       200,
		InputTokens:       200,
		InputTokenDetails: dto.InputTokenDetails{TextTokens: 200},
	}
	require.NoError(t, PreWssConsumeQuota(ctx, relayInfo, secondTotal))

	require.Len(t, billing.reserves, 2)
	assert.Greater(t, billing.reserves[0], 0)
	assert.Equal(t, billing.reserves[0]*2, billing.reserves[1], "the second target must be cumulative, not current reserve plus cumulative usage")
	assert.Empty(t, billing.settles, "WSS chunks must not settle or directly charge funding")
	assert.Equal(t, billing.GetPreConsumedQuota(), relayInfo.FinalPreConsumedQuota)
}

func TestPreWssConsumeQuotaOnlyPropagatesOriginalClampForActiveMonthlyDiscount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	overflowingPrice := float64(common.MaxQuota) * 2 / common.QuotaPerUnit

	tests := []struct {
		name              string
		monthlySnapshot   *groupdiscount.Snapshot
		wantReserved      int
		wantOriginalClamp bool
	}{
		{
			name:         "inactive policy preserves legal legacy charge",
			wantReserved: 0,
		},
		{
			name: "active policy propagates original overflow",
			monthlySnapshot: &groupdiscount.Snapshot{
				PolicyHash:  "active-overflow-policy",
				UsingGroup:  "legacy-tiny",
				OriginModel: "wss-original-overflow",
				PeriodStart: 1,
				PeriodEnd:   2,
				Tiers:       []groupdiscount.Tier{{MinMonthlyOriginalQuota: 0, Ratio: 0.9}},
			},
			wantReserved:      common.MaxQuota,
			wantOriginalClamp: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			billing := &wssRecordingBillingSettler{}
			relayInfo := &relaycommon.RelayInfo{
				OriginModelName:            "wss-original-overflow",
				Billing:                    billing,
				GroupModelDiscountSnapshot: tt.monthlySnapshot,
				PriceData: hosttypes.PriceData{
					UsePrice:   true,
					ModelPrice: overflowingPrice,
					GroupRatioInfo: hosttypes.GroupRatioInfo{
						GroupRatio: 0.000000000001,
					},
				},
			}

			require.NoError(t, PreWssConsumeQuota(ctx, relayInfo, &dto.RealtimeUsage{
				TotalTokens:       1,
				InputTokens:       1,
				InputTokenDetails: dto.InputTokenDetails{TextTokens: 1},
			}))

			require.Equal(t, []int{tt.wantReserved}, billing.reserves)
			if tt.wantOriginalClamp {
				require.NotNil(t, relayInfo.QuotaClamp)
				assert.Equal(t, common.QuotaClampOverflow, relayInfo.QuotaClamp.Kind)
			} else {
				require.Nil(t, relayInfo.QuotaClamp)
			}
		})
	}
}

func TestBuildRealtimeTieredTokenParamsSeparatesReferencedSubcategories(t *testing.T) {
	usage := &dto.RealtimeUsage{
		InputTokens:  100,
		OutputTokens: 40,
		InputTokenDetails: dto.InputTokenDetails{
			CachedTokens: 20,
			AudioTokens:  30,
		},
		OutputTokenDetails: dto.OutputTokenDetails{AudioTokens: 10},
	}

	params := buildRealtimeTieredTokenParams(usage, map[string]bool{
		"cr": true,
		"ai": true,
		"ao": true,
	})

	assert.Equal(t, float64(50), params.P)
	assert.Equal(t, float64(30), params.C)
	assert.Equal(t, float64(100), params.Len)
	assert.Equal(t, float64(20), params.CR)
	assert.Equal(t, float64(30), params.AI)
	assert.Equal(t, float64(10), params.AO)
}

func TestPreWssConsumeQuotaUsesTieredExpressionBeforeGroupForMonthlyReservation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	billing := &wssRecordingBillingSettler{}
	const expr = `tier("realtime", p * 2)`
	monthlySnapshot := groupdiscount.Snapshot{
		PolicyHash:  "wss-policy",
		UsingGroup:  "vip",
		OriginModel: "gpt-realtime",
		PeriodStart: 1,
		PeriodEnd:   2,
		Tiers:       []groupdiscount.Tier{{MinMonthlyOriginalQuota: 0, Ratio: 0.8}},
	}
	relayInfo := &relaycommon.RelayInfo{
		OriginModelName:            "gpt-realtime",
		UsingGroup:                 "vip",
		Billing:                    billing,
		GroupModelDiscountSnapshot: &monthlySnapshot,
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode:  "tiered_expr",
			ExprString:   expr,
			ExprHash:     billingexpr.ExprHashString(expr),
			GroupRatio:   0.5,
			QuotaPerUnit: 1_000_000,
		},
	}

	require.NoError(t, PreWssConsumeQuota(ctx, relayInfo, &dto.RealtimeUsage{
		TotalTokens:       100,
		InputTokens:       100,
		InputTokenDetails: dto.InputTokenDetails{TextTokens: 100},
	}))

	assert.Equal(t, []int{200}, billing.reserves, "monthly reservation must use the expression result before GroupRatio")
}

func TestPostWssConsumeQuotaKeepsOriginModelAcrossMappingForLedgerAndLog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prepareGroupModelDiscountServiceTest(t)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", 7801).Delete(&model.User{}).Error)
	require.NoError(t, model.DB.Where("id = ?", 9801).Delete(&model.Channel{}).Error)
	require.NoError(t, model.DB.Create(&model.User{
		Id: 7801, Username: "wss-origin-model-user", AffCode: "wss-origin-model-user", Status: common.UserStatusEnabled,
	}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{
		Id: 9801, Name: "wss-origin-model-channel", Key: "sk-wss-origin-model", Status: common.ChannelStatusEnabled,
	}).Error)
	require.NoError(t, model.LOG_DB.Exec("DELETE FROM logs").Error)
	t.Cleanup(func() {
		model.LOG_DB.Exec("DELETE FROM logs")
		model.DB.Unscoped().Where("id = ?", 7801).Delete(&model.User{})
		model.DB.Where("id = ?", 9801).Delete(&model.Channel{})
	})

	const (
		requestID     = "wss-origin-model-mapping"
		originModel   = "client-realtime-model"
		upstreamModel = "mapped-realtime-model"
	)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
	ctx.Set(common.RequestIdKey, requestID)

	now := time.Now()
	snapshot := groupdiscount.Snapshot{
		PolicyHash:   "wss-origin-model-policy",
		UsingGroup:   "vip",
		OriginModel:  originModel,
		MatchedModel: originModel,
		Timezone:     "UTC",
		PeriodStart:  now.Add(-time.Hour).Unix(),
		PeriodEnd:    now.Add(time.Hour).Unix(),
		Tiers: []groupdiscount.Tier{
			{MinMonthlyOriginalQuota: 0, Ratio: 0.8},
		},
	}
	billing := &wssRecordingBillingSettler{preConsumed: 1_000}
	relayInfo := &relaycommon.RelayInfo{
		RequestId:                  requestID,
		UserId:                     7801,
		UserQuota:                  1_000_000_000,
		TokenId:                    8801,
		UsingGroup:                 "vip",
		OriginModelName:            originModel,
		StartTime:                  now.Add(-time.Second),
		FirstResponseTime:          now,
		Billing:                    billing,
		GroupModelDiscountSnapshot: &snapshot,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:         9801,
			UpstreamModelName: upstreamModel,
			IsModelMapped:     true,
		},
		PriceData: hosttypes.PriceData{
			UsePrice:   true,
			ModelPrice: 0.001,
			GroupRatioInfo: hosttypes.GroupRatioInfo{
				GroupRatio: 0.5,
			},
		},
	}
	usage := &dto.RealtimeUsage{
		TotalTokens:       10,
		InputTokens:       10,
		InputTokenDetails: dto.InputTokenDetails{TextTokens: 10},
	}

	PostWssConsumeQuota(ctx, relayInfo, upstreamModel, usage, "")

	settlement, err := model.GetGroupModelDiscountSettlement(requestID)
	require.NoError(t, err)
	assert.Equal(t, originModel, settlement.OriginModel)

	var consumeLog model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", requestID).First(&consumeLog).Error)
	assert.Equal(t, originModel, consumeLog.ModelName)
}

func TestBillingSessionReserveForAdmissionRejectsInsufficientWalletQuotaAtomically(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const (
		userID   = 97901
		tokenID  = 97902
		tokenKey = "task-admission-strict-reserve"
	)
	require.NoError(t, model.DB.Where("id = ?", tokenID).Delete(&model.Token{}).Error)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", userID).Delete(&model.User{}).Error)
	t.Cleanup(func() {
		model.DB.Where("id = ?", tokenID).Delete(&model.Token{})
		model.DB.Unscoped().Where("id = ?", userID).Delete(&model.User{})
	})
	require.NoError(t, model.DB.Create(&model.User{
		Id:       userID,
		Username: "task-admission-strict-reserve",
		Status:   common.UserStatusEnabled,
		Quota:    150,
		AffCode:  "task-admission-strict-reserve-aff",
	}).Error)
	seedToken(t, tokenID, userID, tokenKey, 150)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	relayInfo := &relaycommon.RelayInfo{
		RequestId:       "task-admission-strict-reserve",
		UserId:          userID,
		UserQuota:       150,
		TokenId:         tokenID,
		TokenKey:        tokenKey,
		ForcePreConsume: true,
		UserSetting: dto.UserSetting{
			BillingPreference: "wallet_only",
		},
	}
	require.Nil(t, PreConsumeBilling(ctx, 100, relayInfo))
	session, ok := relayInfo.Billing.(*BillingSession)
	require.True(t, ok)

	err := session.ReserveForAdmission(200)
	require.Error(t, err)
	assert.Equal(t, 100, session.GetPreConsumedQuota())

	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 50, user.Quota, "failed strict reserve must not make the wallet negative")
	var token model.Token
	require.NoError(t, model.DB.First(&token, tokenID).Error)
	assert.Equal(t, 50, token.RemainQuota, "token reserve must not run after wallet admission fails")
}

func TestPreWssConsumeQuotaRejectsCumulativeUsageThatExceedsWalletQuota(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const (
		userID   = 97911
		tokenID  = 97912
		tokenKey = "wss-admission-strict-reserve"
	)
	require.NoError(t, model.DB.Where("id = ?", tokenID).Delete(&model.Token{}).Error)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", userID).Delete(&model.User{}).Error)
	t.Cleanup(func() {
		model.DB.Where("id = ?", tokenID).Delete(&model.Token{})
		model.DB.Unscoped().Where("id = ?", userID).Delete(&model.User{})
	})
	require.NoError(t, model.DB.Create(&model.User{
		Id:       userID,
		Username: "wss-admission-strict-reserve",
		Status:   common.UserStatusEnabled,
		Quota:    150,
		AffCode:  "wss-admission-strict-reserve-aff",
	}).Error)
	seedToken(t, tokenID, userID, tokenKey, 150)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	relayInfo := &relaycommon.RelayInfo{
		RequestId:       "wss-admission-strict-reserve",
		OriginModelName: "wss-admission-model",
		UserId:          userID,
		UserQuota:       150,
		TokenId:         tokenID,
		TokenKey:        tokenKey,
		ForcePreConsume: true,
		UserSetting: dto.UserSetting{
			BillingPreference: "wallet_only",
		},
		PriceData: hosttypes.PriceData{
			UsePrice:   true,
			ModelPrice: 200 / common.QuotaPerUnit,
			GroupRatioInfo: hosttypes.GroupRatioInfo{
				GroupRatio: 1,
			},
		},
	}
	require.Nil(t, PreConsumeBilling(ctx, 100, relayInfo))

	err := PreWssConsumeQuota(ctx, relayInfo, &dto.RealtimeUsage{TotalTokens: 1})
	require.Error(t, err)
	assert.Equal(t, 100, relayInfo.Billing.GetPreConsumedQuota())

	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 50, user.Quota, "failed WSS top-up must not make the wallet negative")
	var token model.Token
	require.NoError(t, model.DB.First(&token, tokenID).Error)
	assert.Equal(t, 50, token.RemainQuota, "token reserve must not run after wallet admission fails")
}
