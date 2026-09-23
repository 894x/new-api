package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/groupdiscount"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageQuantityReserveKeepsMonthlyOriginalQuota(t *testing.T) {
	for _, expression := range []bool{false, true} {
		name := "legacy"
		if expression {
			name = "expression"
		}
		t.Run(name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			billing := &wssRecordingBillingSettler{}
			info := &relaycommon.RelayInfo{
				ChannelMeta:                &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeAli},
				Billing:                    billing,
				GroupModelDiscountSnapshot: &groupdiscount.Snapshot{},
				PriceData:                  hosttypes.PriceData{UsePrice: true, ModelPrice: 0.02, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 0.5}},
			}
			if expression {
				info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: "image_count * 20000", QuotaPerUnit: common.QuotaPerUnit, EstimatedImageCount: common.GetPointer(1)}
			}
			require.Nil(t, PrepareImageBillingForRequest(ctx, info, 4, false))
			assert.Equal(t, 40000, info.PriceData.OriginalQuotaToPreConsume)
			assert.Equal(t, 40000, info.PriceData.QuotaToPreConsume)
			assert.Equal(t, 40000, billing.GetPreConsumedQuota())
		})
	}
}
