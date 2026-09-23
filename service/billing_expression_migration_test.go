package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These migration examples protect against treating a unit-price
// conversion as proof of settlement equivalence. They use the real legacy and
// expression settlement calculators; no database or provider is contacted.
func TestLegacyPriceExpressionMigrationSettlementBoundaries(t *testing.T) {
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

	for _, tc := range []struct {
		name       string
		price      hosttypes.PriceData
		usage      dto.Usage
		expression string
		group      float64
		legacy     int
		tiered     int
	}{
		{
			name:       "whole quota token prices agree",
			price:      hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 2},
			usage:      dto.Usage{PromptTokens: 1000, CompletionTokens: 200},
			expression: `tier("base", p * 2 + c * 4)`, group: 1,
			legacy: 1400, tiered: 1400,
		},
		{
			name:       "fractional quota rounds in both settlement paths",
			price:      hosttypes.PriceData{ModelRatio: 1.5},
			usage:      dto.Usage{PromptTokens: 1},
			expression: `tier("base", p * 3)`, group: 1,
			legacy: 2, tiered: 2,
		},
		{
			name:       "positive discounted usage loses legacy minimum",
			price:      hosttypes.PriceData{ModelRatio: 0.4},
			usage:      dto.Usage{PromptTokens: 1},
			expression: `tier("base", p * 0.8)`, group: 0.5,
			legacy: 1, tiered: 0,
		},
		{
			name:       "fixed price with usage agrees",
			price:      hosttypes.PriceData{UsePrice: true, ModelPrice: 0.003},
			usage:      dto.Usage{PromptTokens: 1},
			expression: `tier("base", 3000)`, group: 1,
			legacy: 1500, tiered: 1500,
		},
		{
			name:       "constant expression charges explicitly empty usage",
			price:      hosttypes.PriceData{UsePrice: true, ModelPrice: 0.003},
			usage:      dto.Usage{},
			expression: `tier("base", 3000)`, group: 1,
			legacy: 0, tiered: 1500,
		},
		{
			name:       "guarded fixed price preserves empty usage exemption",
			price:      hosttypes.PriceData{UsePrice: true, ModelPrice: 0.003},
			usage:      dto.Usage{},
			expression: `len + c > 0 ? tier("base", 3000) : tier("empty", 0)`, group: 1,
			legacy: 0, tiered: 0,
		},
		{
			name:       "guarded fixed price still charges nonempty usage",
			price:      hosttypes.PriceData{UsePrice: true, ModelPrice: 0.003},
			usage:      dto.Usage{CompletionTokens: 1},
			expression: `len + c > 0 ? tier("base", 3000) : tier("empty", 0)`, group: 1,
			legacy: 1500, tiered: 1500,
		},
		{
			name:       "zero group remains free",
			price:      hosttypes.PriceData{UsePrice: true, ModelPrice: 0.003},
			usage:      dto.Usage{PromptTokens: 1},
			expression: `tier("base", 3000)`, group: 0,
			legacy: 0, tiered: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			tc.price.GroupRatioInfo.GroupRatio = tc.group
			legacyInfo := &relaycommon.RelayInfo{
				OriginModelName: "migration-boundary-fixture", PriceData: tc.price,
			}
			legacy := calculateTextQuotaSummary(ctx, legacyInfo, &tc.usage)

			_, err := billingexpr.CompileFromCache(tc.expression)
			require.NoError(t, err)
			tieredInfo := &relaycommon.RelayInfo{
				OriginModelName: "migration-boundary-fixture",
				PriceData:       hosttypes.PriceData{GroupRatioInfo: tc.price.GroupRatioInfo},
				TieredBillingSnapshot: &billingexpr.BillingSnapshot{
					BillingMode: "tiered_expr", ExprString: tc.expression,
					ExprHash: billingexpr.ExprHashString(tc.expression), ExprVersion: 1,
					QuotaPerUnit: common.QuotaPerUnit, GroupRatio: tc.group,
				},
			}
			tieredSummary := calculateTextQuotaSummary(ctx, tieredInfo, &tc.usage)
			params := BuildTieredTokenParams(&tc.usage, tieredSummary.IsClaudeUsageSemantic, billingexpr.UsedVars(tc.expression))
			applied, quota, result := TryTieredSettle(tieredInfo, params)
			require.True(t, applied)
			require.NotNil(t, result, "a pricing error must not be mistaken for a successful conversion")
			assert.Nil(t, result.Clamp)
			assert.Equal(t, tc.legacy, legacy.Quota)
			assert.Equal(t, tc.tiered, composeTieredTextQuota(tieredInfo, tieredSummary, quota, result))
		})
	}
}
