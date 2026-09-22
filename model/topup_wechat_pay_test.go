package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createWechatPayTestOrder(t *testing.T, userID int, tradeNo string, money float64, provider string) *TopUp {
	t.Helper()
	topUp := &TopUp{
		UserId:          userID,
		Amount:          2,
		CreditedQuota:   2 * 500000,
		Money:           money,
		MoneyCents:      int64(money*100 + 0.5),
		TradeNo:         tradeNo,
		PaymentMethod:   PaymentMethodWechatNative,
		PaymentProvider: provider,
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, topUp.Insert())
	return topUp
}

func TestRechargeWechatPayRequiresExactAmountAndCreditsOnce(t *testing.T) {
	truncateTables(t)

	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })

	user := insertUserForPaymentGuardTest(t, 601, 0)
	order := createWechatPayTestOrder(t, user.Id, "WXNATIVEAMOUNT", 10.01, PaymentProviderWechatPay)
	// Settlement must use the quota frozen at order creation, even if the
	// display conversion changes while the payment is in flight.
	common.QuotaPerUnit = 123

	alreadyDone, err := RechargeWechatPay(order.TradeNo, 1000, "wx-txn-wrong", "127.0.0.1")
	require.ErrorIs(t, err, ErrPaymentAmountMismatch)
	assert.False(t, alreadyDone)
	assert.Equal(t, 0, getUserQuotaForPaymentGuardTest(t, user.Id))
	assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, order.TradeNo))

	alreadyDone, err = RechargeWechatPay(order.TradeNo, 1001, "wx-txn-1", "127.0.0.1")
	require.NoError(t, err)
	assert.False(t, alreadyDone)
	assert.Equal(t, 2*500000, getUserQuotaForPaymentGuardTest(t, user.Id))

	reloaded := GetTopUpByTradeNo(order.TradeNo)
	require.NotNil(t, reloaded)
	assert.Equal(t, common.TopUpStatusSuccess, reloaded.Status)
	assert.Equal(t, "wx-txn-1", reloaded.ProviderTransactionId)

	alreadyDone, err = RechargeWechatPay(order.TradeNo, 1001, "wx-txn-1", "127.0.0.1")
	require.NoError(t, err)
	assert.True(t, alreadyDone)
	assert.Equal(t, 2*500000, getUserQuotaForPaymentGuardTest(t, user.Id))

	_, err = RechargeWechatPay(order.TradeNo, 1001, "wx-txn-other", "127.0.0.1")
	require.ErrorIs(t, err, ErrPaymentTransactionMismatch)
	assert.Equal(t, 2*500000, getUserQuotaForPaymentGuardTest(t, user.Id))
}

func TestRechargeWechatPayRejectsForeignProvider(t *testing.T) {
	truncateTables(t)

	user := insertUserForPaymentGuardTest(t, 602, 0)
	order := createWechatPayTestOrder(t, user.Id, "WXNATIVEFOREIGN", 10, PaymentProviderStripe)

	_, err := RechargeWechatPay(order.TradeNo, 1000, "wx-txn-2", "127.0.0.1")
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)
	assert.Equal(t, 0, getUserQuotaForPaymentGuardTest(t, user.Id))
	assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, order.TradeNo))
}

func TestRechargeWechatPayCreditsLargeSnapshotOnceAndEnforcesWalletCeiling(t *testing.T) {
	for _, tc := range []struct {
		name    string
		balance int
		wantErr bool
	}{
		{"exact ceiling", common.MaxWalletQuota - 4_294_500_000, false},
		{"over ceiling", common.MaxWalletQuota - 4_294_500_000 + 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			user := insertUserForPaymentGuardTest(t, 603, tc.balance)
			order := createWechatPayTestOrder(t, user.Id, "WXLARGEQUOTA", 10, PaymentProviderWechatPay)
			require.NoError(t, DB.Model(order).Update("credited_quota", 4_294_500_000).Error)
			_, err := RechargeWechatPay(order.TradeNo, 1000, "wx-large", "127.0.0.1")
			if tc.wantErr {
				require.ErrorIs(t, err, ErrTopUpQuotaLimitExceeded)
				assert.Equal(t, tc.balance, getUserQuotaForPaymentGuardTest(t, user.Id))
				assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, order.TradeNo))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, common.MaxWalletQuota, getUserQuotaForPaymentGuardTest(t, user.Id))
			alreadyDone, err := RechargeWechatPay(order.TradeNo, 1000, "wx-large", "127.0.0.1")
			require.NoError(t, err)
			assert.True(t, alreadyDone)
			var reloaded User
			require.NoError(t, DB.First(&reloaded, user.Id).Error)
			assert.Equal(t, common.MaxWalletQuota, reloaded.Quota)
			assert.EqualValues(t, 1, reloaded.QuotaVersion)
		})
	}
}
