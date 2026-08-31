package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestModelPriceHelperTieredUsesPreloadedRequestInput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"tiered-test-model":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"tiered-test-model":"param(\"stream\") == true ? tier(\"stream\", p * 3) : tier(\"base\", p * 2)"}`,
	}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/channel/test/1", nil)
	req.Body = nil
	req.ContentLength = 0
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	ctx.Set("group", "default")

	info := &relaycommon.RelayInfo{
		OriginModelName: "tiered-test-model",
		UserGroup:       "default",
		UsingGroup:      "default",
		RequestHeaders:  map[string]string{"Content-Type": "application/json"},
		BillingRequestInput: &billingexpr.RequestInput{
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    []byte(`{"stream":true}`),
		},
	}

	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{
		BillingRatios: map[string]float64{"n": 3},
	})
	require.NoError(t, err)
	require.Equal(t, 1500, priceData.QuotaToPreConsume)
	require.NotNil(t, info.TieredBillingSnapshot)
	require.Equal(t, "stream", info.TieredBillingSnapshot.EstimatedTier)
	require.Equal(t, billing_setting.BillingModeTieredExpr, info.TieredBillingSnapshot.BillingMode)
	require.Equal(t, common.QuotaPerUnit, info.TieredBillingSnapshot.QuotaPerUnit)
}

func TestModelPriceHelperTieredPreConsumeMaxTokensFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":    `{"tiered-fallback-model":"tiered_expr"}`,
		"billing_setting.billing_expr":    `{"tiered-fallback-model":"tier(\"base\", p * 3 + c * 15)"}`,
		"group_ratio_setting.group_ratio": `{"default":1,"free":0}`,
	}))

	const promptTokens = 1000

	cases := []struct {
		name      string
		group     string
		maxTokens int
		expected  int
	}{
		{
			// max_tokens omitted in a paid group -> fall back to 8192 completion tokens.
			// p*3 + c*15 = 1000*3 + 8192*15 = 125880 -> /1e6 * 500000 = 62940
			name:      "non-free group falls back to 8192 completion tokens",
			group:     "default",
			maxTokens: 0,
			expected:  62940,
		},
		{
			// explicit max_tokens is used verbatim, no fallback.
			// 1000*3 + 100*15 = 4500 -> /1e6 * 500000 = 2250
			name:      "explicit max_tokens is used verbatim",
			group:     "default",
			maxTokens: 100,
			expected:  2250,
		},
		{
			// free group (ratio 0) stays zero; fallback is gated on non-zero group ratio.
			name:      "free group stays zero without fallback",
			group:     "free",
			maxTokens: 0,
			expected:  0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			req.Header.Set("Content-Type", "application/json")
			ctx.Request = req
			ctx.Set("group", tc.group)

			info := &relaycommon.RelayInfo{
				OriginModelName: "tiered-fallback-model",
				UserGroup:       tc.group,
				UsingGroup:      tc.group,
				RequestHeaders:  map[string]string{"Content-Type": "application/json"},
				BillingRequestInput: &billingexpr.RequestInput{
					Headers: map[string]string{"Content-Type": "application/json"},
					Body:    []byte(`{}`),
				},
			}

			priceData, err := ModelPriceHelper(ctx, info, promptTokens, &types.TokenCountMeta{MaxTokens: tc.maxTokens})
			require.NoError(t, err)
			require.Equal(t, tc.expected, priceData.QuotaToPreConsume)
		})
	}
}

func TestModelPriceHelperTieredPreConsumeFallbackWhenMonthlyDiscountReplacesZeroGroupRatio(t *testing.T) {
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":    `{"monthly-zero-group-tiered":"tiered_expr"}`,
		"billing_setting.billing_expr":    `{"monthly-zero-group-tiered":"tier(\"base\", c * 15)"}`,
		"group_ratio_setting.group_ratio": `{"monthly-zero":0}`,
		"group_ratio_setting.model_tiered_ratios": `{
			"monthly-zero":{"monthly-zero-group-tiered":{
				"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC",
				"tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]
			}}
		}`,
	}))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("group", "monthly-zero")
	info := &relaycommon.RelayInfo{
		OriginModelName: "monthly-zero-group-tiered",
		UserGroup:       "monthly-zero",
		UsingGroup:      "monthly-zero",
		StartTime:       time.Unix(10, 0),
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{}`),
		},
	}

	priceData, err := ModelPriceHelper(ctx, info, 0, &types.TokenCountMeta{})

	require.NoError(t, err)
	require.NotNil(t, info.GroupModelDiscountSnapshot)
	require.NotNil(t, info.TieredBillingSnapshot)
	require.Equal(t, defaultTieredPreConsumeMaxTokens, info.TieredBillingSnapshot.EstimatedCompletionTokens)
	require.Equal(t, 61_440, priceData.OriginalQuotaToPreConsume)
	require.Equal(t, 61_440, priceData.QuotaToPreConsume)
	require.False(t, priceData.FreeModel)
}

func TestModelPriceHelperTieredKeepsBeforeGroupFallbackForFreeGroupRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":                `{"free-retry-tiered":"tiered_expr"}`,
		"billing_setting.billing_expr":                `{"free-retry-tiered":"tier(\"base\", c * 15)"}`,
		"group_ratio_setting.group_ratio":             `{"free-retry":0}`,
		"group_ratio_setting.model_tiered_ratios":     `{}`,
		"quota_setting.enable_free_model_pre_consume": "false",
	}))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("group", "free-retry")
	info := &relaycommon.RelayInfo{
		OriginModelName: "free-retry-tiered",
		UserGroup:       "free-retry",
		UsingGroup:      "free-retry",
		StartTime:       time.Unix(10, 0),
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{}`),
		},
	}

	priceData, err := ModelPriceHelper(ctx, info, 0, &types.TokenCountMeta{})

	require.NoError(t, err)
	require.Nil(t, info.GroupModelDiscountSnapshot)
	require.NotNil(t, info.TieredBillingSnapshot)
	require.Equal(t, defaultTieredPreConsumeMaxTokens, info.TieredBillingSnapshot.EstimatedCompletionTokens)
	require.Equal(t, 61_440, priceData.OriginalQuotaToPreConsume)
	require.Zero(t, priceData.QuotaToPreConsume)
	require.True(t, priceData.FreeModel)
}

func TestModelPriceHelperTieredPreConsumesImageOutputTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":    `{"tiered-image-model":"tiered_expr"}`,
		"billing_setting.billing_expr":    `{"tiered-image-model":"tier(\"base\", p * 2 + c * 8 + img_o * 40)"}`,
		"group_ratio_setting.group_ratio": `{"default":1}`,
	}))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	ctx.Set("group", "default")
	info := &relaycommon.RelayInfo{
		OriginModelName: "tiered-image-model",
		UserGroup:       "default",
		UsingGroup:      "default",
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{}`),
		},
	}

	priceData, err := ModelPriceHelper(ctx, info, 100, &types.TokenCountMeta{MaxTokens: 200})
	require.NoError(t, err)
	// p*2 + img_o*40 = 100*2 + 200*40 = 8200; /1e6 * 500000 = 4100.
	require.Equal(t, 4100, priceData.QuotaToPreConsume)
}

func TestModelPriceHelperTieredRejectsPreConsumeOverflow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":    `{"tiered-overflow-model":"tiered_expr"}`,
		"billing_setting.billing_expr":    `{"tiered-overflow-model":"tier(\"overflow\", p * 1000000000000000)"}`,
		"group_ratio_setting.group_ratio": `{"default":1}`,
	}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set("group", "default")
	info := &relaycommon.RelayInfo{
		OriginModelName: "tiered-overflow-model",
		UserGroup:       "default",
		UsingGroup:      "default",
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{}`),
		},
	}

	_, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})

	var clamp *common.QuotaClamp
	require.ErrorAs(t, err, &clamp)
	require.Equal(t, "QuotaRound", clamp.Op)
	require.Equal(t, common.QuotaClampOverflow, clamp.Kind)
}

func TestModelPriceHelperRequestBillingRatiosOnlyApplyToFixedPrice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedModelPrices := ratio_setting.ModelPrice2JSONString()
	savedModelRatios := ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedModelPrices))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(savedModelRatios))
	})

	modelPrices, err := common.Marshal(map[string]float64{
		"fixed-image-price":      0.04,
		"fractional-image-price": 0.0000012,
		"overflow-image-price":   float64(common.MaxQuota) / common.QuotaPerUnit / 2,
	})
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(string(modelPrices)))
	modelRatios, err := common.Marshal(map[string]float64{"ratio-image-price": 15})
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(modelRatios)))

	tests := []struct {
		name           string
		model          string
		wantQuota      int
		wantUsePrice   bool
		wantImageCount bool
	}{
		{
			name:           "fixed price applies image count",
			model:          "fixed-image-price",
			wantQuota:      180000,
			wantUsePrice:   true,
			wantImageCount: true,
		},
		{
			name:         "ratio price ignores request billing ratios",
			model:        "ratio-image-price",
			wantQuota:    15000,
			wantUsePrice: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Set("group", "default")
			info := &relaycommon.RelayInfo{
				OriginModelName: tt.model,
				UserGroup:       "default",
				UsingGroup:      "default",
			}
			meta := &types.TokenCountMeta{
				ImagePriceRatio: 3,
				BillingRatios:   map[string]float64{"n": 3},
			}

			priceData, err := ModelPriceHelper(ctx, info, 1000, meta)

			require.NoError(t, err)
			require.Equal(t, tt.wantQuota, priceData.QuotaToPreConsume)
			require.Equal(t, tt.wantUsePrice, priceData.UsePrice)
			require.Equal(t, tt.wantImageCount, priceData.HasOtherRatio("n"))
			require.Equal(t, priceData.OtherRatios(), info.PriceData.OtherRatios())
		})
	}

	newInfo := func(model string) (*gin.Context, *relaycommon.RelayInfo) {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Set("group", "default")
		return ctx, &relaycommon.RelayInfo{
			OriginModelName: model,
			UserGroup:       "default",
			UsingGroup:      "default",
		}
	}
	meta := &types.TokenCountMeta{BillingRatios: map[string]float64{"n": 3}}

	ctx, info := newInfo("fractional-image-price")
	priceData, err := ModelPriceHelper(ctx, info, 0, meta)
	require.NoError(t, err)
	// 0.0000012 * 500000 * 3 = 1.8, then truncate once to 1.
	require.Equal(t, 1, priceData.QuotaToPreConsume)

	ctx, info = newInfo("overflow-image-price")
	_, err = ModelPriceHelper(ctx, info, 0, meta)
	var clamp *common.QuotaClamp
	require.ErrorAs(t, err, &clamp)
	require.Equal(t, "QuotaFromFloat", clamp.Op)
	require.Equal(t, common.QuotaClampOverflow, clamp.Kind)
	require.Nil(t, info.Billing)
}

func TestModelPriceHelperPerCallKeepsOriginalQuotaBeforeGroupDiscount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedModelPrices := ratio_setting.ModelPrice2JSONString()
	savedGroupRatios := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedModelPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroupRatios))
	})

	prices, err := common.Marshal(map[string]float64{"per-call-original-test": 0.04})
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(string(prices)))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"vip":0.5}`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("group", "vip")
	info := &relaycommon.RelayInfo{
		OriginModelName: "per-call-original-test",
		UserGroup:       "vip",
		UsingGroup:      "vip",
	}

	priceData, err := ModelPriceHelperPerCall(ctx, info)

	require.NoError(t, err)
	require.Equal(t, 20000, priceData.OriginalQuota)
	require.Equal(t, 10000, priceData.Quota)

	priceData, err = ModelPriceHelper(ctx, info, 0, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.Equal(t, 20000, priceData.OriginalQuotaToPreConsume)
	require.Equal(t, 10000, priceData.QuotaToPreConsume)
}

func TestModelPriceHelperPreConsumesOriginalWhenMonthlyDiscountIsActive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedModelPrices := ratio_setting.ModelPrice2JSONString()
	savedGroupRatios := ratio_setting.GroupRatio2JSONString()
	savedTieredRatios := ratio_setting.ModelTieredRatios2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedModelPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroupRatios))
		require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(savedTieredRatios))
	})

	prices, err := common.Marshal(map[string]float64{"monthly-discount-preconsume": 0.04})
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(string(prices)))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"vip":0.5}`))
	require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(`{
		"vip":{"monthly-discount-preconsume":{
			"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC",
			"tiers":[{"min_monthly_original_quota":0,"ratio":0.9}]
		}}
	}`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("group", "vip")
	info := &relaycommon.RelayInfo{
		OriginModelName: "monthly-discount-preconsume",
		UserGroup:       "vip",
		UsingGroup:      "vip",
		StartTime:       time.Unix(10, 0),
	}

	priceData, err := ModelPriceHelper(ctx, info, 0, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.NotNil(t, info.GroupModelDiscountSnapshot)
	require.Equal(t, 20000, priceData.OriginalQuotaToPreConsume)
	require.Equal(t, 20000, priceData.QuotaToPreConsume, "pre-consume must conservatively reserve original quota")

	priceData, err = ModelPriceHelperPerCall(ctx, info)
	require.NoError(t, err)
	require.NotNil(t, info.GroupModelDiscountSnapshot)
	require.Equal(t, 20000, priceData.OriginalQuota)
	require.Equal(t, 10000, priceData.Quota, "the legacy fixed-ratio fallback remains available for settlement")
}

func TestModelPriceHelperKeepsPositiveMonthlyOriginalLedgerEligible(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedModelPrices := ratio_setting.ModelPrice2JSONString()
	savedGroupRatios := ratio_setting.GroupRatio2JSONString()
	savedTieredRatios := ratio_setting.ModelTieredRatios2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedModelPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroupRatios))
		require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(savedTieredRatios))
	})

	tinyPrice := 0.4 / common.QuotaPerUnit
	prices, err := common.Marshal(map[string]float64{"monthly-minimum-original": tinyPrice})
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(string(prices)))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"vip":3}`))
	require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(`{
		"vip":{"monthly-minimum-original":{
			"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC",
			"tiers":[{"min_monthly_original_quota":0,"ratio":0.9}]
		}}
	}`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("group", "vip")
	info := &relaycommon.RelayInfo{
		OriginModelName: "monthly-minimum-original",
		UserGroup:       "vip",
		UsingGroup:      "vip",
		StartTime:       time.Unix(10, 0),
	}

	priceData, err := ModelPriceHelper(ctx, info, 0, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.Equal(t, 1, priceData.OriginalQuotaToPreConsume)
	require.Equal(t, 1, priceData.QuotaToPreConsume)

	priceData, err = ModelPriceHelperPerCall(ctx, info)
	require.NoError(t, err)
	require.Equal(t, 1, priceData.OriginalQuota)
	require.Equal(t, 1, priceData.Quota)
}

func TestInactiveMonthlyDiscountDoesNotRejectLegalLegacyQuotaWhenOriginalOverflows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	overflowingPrice := float64(common.MaxQuota) * 2 / common.QuotaPerUnit
	prices, err := common.Marshal(map[string]float64{
		"legacy-fixed-overflow":    overflowingPrice,
		"legacy-per-call-overflow": overflowingPrice,
	})
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(string(prices)))
	ratios, err := common.Marshal(map[string]float64{
		"legacy-ratio-overflow": float64(common.MaxQuota) * 2,
	})
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(ratios)))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"legacy-zero":0,"legacy-tiny":0.000000000001}`))
	require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(`{
		"legacy-zero":{"*":{
			"enabled":false,"effective_from":0,"effective_until":null,"timezone":"UTC",
			"tiers":[{"min_monthly_original_quota":0,"ratio":0.9}]
		}},
		"legacy-tiny":{"*":{
			"enabled":false,"effective_from":0,"effective_until":null,"timezone":"UTC",
			"tiers":[{"min_monthly_original_quota":0,"ratio":0.9}]
		}}
	}`))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"legacy-tiered-overflow":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"legacy-tiered-overflow":"tier(\"overflow\", p * 1000000000000000)"}`,
	}))

	newRequest := func(model, group string) (*gin.Context, *relaycommon.RelayInfo) {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		ctx.Set("group", group)
		return ctx, &relaycommon.RelayInfo{
			OriginModelName: model,
			UserGroup:       group,
			UsingGroup:      group,
			StartTime:       time.Unix(10, 0),
			BillingRequestInput: &billingexpr.RequestInput{
				Body: []byte(`{}`),
			},
		}
	}

	t.Run("ratio billing with a zero group ratio", func(t *testing.T) {
		ctx, info := newRequest("legacy-ratio-overflow", "legacy-zero")
		priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})

		require.NoError(t, err)
		require.Nil(t, info.GroupModelDiscountSnapshot)
		require.Zero(t, priceData.QuotaToPreConsume)
	})

	t.Run("fixed billing with a tiny group ratio", func(t *testing.T) {
		ctx, info := newRequest("legacy-fixed-overflow", "legacy-tiny")
		priceData, err := ModelPriceHelper(ctx, info, 0, &types.TokenCountMeta{})

		require.NoError(t, err)
		require.Nil(t, info.GroupModelDiscountSnapshot)
		require.Zero(t, priceData.QuotaToPreConsume)
	})

	t.Run("per-call billing with a zero group ratio", func(t *testing.T) {
		ctx, info := newRequest("legacy-per-call-overflow", "legacy-zero")
		priceData, err := ModelPriceHelperPerCall(ctx, info)

		require.NoError(t, err)
		require.Nil(t, info.GroupModelDiscountSnapshot)
		require.Zero(t, priceData.Quota)
	})

	t.Run("tiered expression billing with a tiny group ratio", func(t *testing.T) {
		ctx, info := newRequest("legacy-tiered-overflow", "legacy-tiny")
		priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})

		require.NoError(t, err)
		require.Nil(t, info.GroupModelDiscountSnapshot)
		require.Equal(t, 500000, priceData.QuotaToPreConsume)
	})

	require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(`{
		"legacy-zero":{"legacy-fixed-overflow":{
			"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC",
			"tiers":[{"min_monthly_original_quota":0,"ratio":0.9}]
		}}
	}`))
	ctx, info := newRequest("legacy-fixed-overflow", "legacy-zero")
	_, err = ModelPriceHelper(ctx, info, 0, &types.TokenCountMeta{})
	var clamp *common.QuotaClamp
	require.ErrorAs(t, err, &clamp, "an active monthly policy still requires a representable original quota")
	require.Equal(t, common.QuotaClampOverflow, clamp.Kind)
}
