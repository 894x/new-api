package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type fakeWechatPayGateway struct {
	transaction   service.WechatPayTransaction
	parseError    error
	prepayTradeNo string
	prepayTotal   int64
	prepayNotify  string
}

func (f *fakeWechatPayGateway) Prepay(_ context.Context, tradeNo string, _ string, notifyURL string, total int64, _ string) (string, error) {
	f.prepayTradeNo = tradeNo
	f.prepayTotal = total
	f.prepayNotify = notifyURL
	return "weixin://wxpay/example", nil
}

func (f *fakeWechatPayGateway) Query(context.Context, string) (service.WechatPayTransaction, error) {
	return f.transaction, nil
}

func (f *fakeWechatPayGateway) ParseNotification(context.Context, *http.Request) (service.WechatPayTransaction, error) {
	return f.transaction, f.parseError
}

func setupWechatPayControllerTest(t *testing.T) {
	t.Helper()
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}))
	model.DB = db
	model.LOG_DB = db

	originalQuotaPerUnit := common.QuotaPerUnit
	originalRedisEnabled := common.RedisEnabled
	originalLogConsumeEnabled := common.LogConsumeEnabled
	originalBatchUpdateEnabled := common.BatchUpdateEnabled
	common.QuotaPerUnit = 500000
	common.RedisEnabled = false
	common.LogConsumeEnabled = true
	common.BatchUpdateEnabled = false

	paymentSetting := operation_setting.GetPaymentSetting()
	originalCompliance := paymentSetting.ComplianceConfirmed
	originalTerms := paymentSetting.ComplianceTermsVersion
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion

	originalAppID := setting.WechatPayAppID
	originalMchID := setting.WechatPayMchID
	originalSerial := setting.WechatPayMerchantSerialNumber
	originalAPIv3Key := setting.WechatPayAPIv3Key
	originalPrivateKey := setting.WechatPayMerchantPrivateKey
	originalPublicKeyID := setting.WechatPayPublicKeyID
	originalPublicKey := setting.WechatPayPublicKey
	setting.WechatPayAppID = "wx-app"
	setting.WechatPayMchID = "1900000001"
	setting.WechatPayMerchantSerialNumber = "merchant-serial"
	setting.WechatPayAPIv3Key = "01234567890123456789012345678901"
	setting.WechatPayMerchantPrivateKey = "private-key"
	setting.WechatPayPublicKeyID = "PUB_KEY_ID_1"
	setting.WechatPayPublicKey = "public-key"

	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.QuotaPerUnit = originalQuotaPerUnit
		common.RedisEnabled = originalRedisEnabled
		common.LogConsumeEnabled = originalLogConsumeEnabled
		common.BatchUpdateEnabled = originalBatchUpdateEnabled
		paymentSetting.ComplianceConfirmed = originalCompliance
		paymentSetting.ComplianceTermsVersion = originalTerms
		setting.WechatPayAppID = originalAppID
		setting.WechatPayMchID = originalMchID
		setting.WechatPayMerchantSerialNumber = originalSerial
		setting.WechatPayAPIv3Key = originalAPIv3Key
		setting.WechatPayMerchantPrivateKey = originalPrivateKey
		setting.WechatPayPublicKeyID = originalPublicKeyID
		setting.WechatPayPublicKey = originalPublicKey
	})
}

func TestWechatPayNotifyRejectsAmountMismatchBeforeCredit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupWechatPayControllerTest(t)

	user := &model.User{Id: 701, Username: "wx_notify_user", Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(user).Error)
	order := &model.TopUp{
		UserId:          user.Id,
		Amount:          2,
		CreditedQuota:   2 * 500000,
		Money:           10.01,
		MoneyCents:      1001,
		TradeNo:         "WXNOTIFYAMOUNT",
		PaymentMethod:   model.PaymentMethodWechatNative,
		PaymentProvider: model.PaymentProviderWechatPay,
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, order.Insert())

	fake := &fakeWechatPayGateway{transaction: service.WechatPayTransaction{
		AppID:         setting.WechatPayAppID,
		MchID:         setting.WechatPayMchID,
		TradeNo:       order.TradeNo,
		TransactionID: "wx-transaction-701",
		TradeState:    "SUCCESS",
		TradeType:     "NATIVE",
		Currency:      "CNY",
		Total:         1000,
	}}
	originalFactory := newWechatPayGateway
	newWechatPayGateway = func(context.Context) (wechatPayGateway, error) { return fake, nil }
	t.Cleanup(func() { newWechatPayGateway = originalFactory })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/wechatpay/notify", strings.NewReader("{}"))
	WechatPayNotify(ctx)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	var reloaded model.User
	require.NoError(t, model.DB.First(&reloaded, user.Id).Error)
	assert.Zero(t, reloaded.Quota)
	assert.Equal(t, common.TopUpStatusPending, model.GetTopUpByTradeNo(order.TradeNo).Status)
}

func TestValidateWechatPayTransactionRequiresMerchantIdentityAndCNY(t *testing.T) {
	originalAppID := setting.WechatPayAppID
	originalMchID := setting.WechatPayMchID
	setting.WechatPayAppID = "wx-app"
	setting.WechatPayMchID = "1900000001"
	t.Cleanup(func() {
		setting.WechatPayAppID = originalAppID
		setting.WechatPayMchID = originalMchID
	})

	valid := service.WechatPayTransaction{
		AppID:         "wx-app",
		MchID:         "1900000001",
		TradeNo:       "WXORDER",
		TransactionID: "wx-transaction",
		TradeState:    "SUCCESS",
		TradeType:     "NATIVE",
		Currency:      "CNY",
		Total:         100,
	}
	require.NoError(t, validateWechatPayTransaction(valid, "WXORDER"))

	wrongMerchant := valid
	wrongMerchant.MchID = "other"
	require.Error(t, validateWechatPayTransaction(wrongMerchant, "WXORDER"))

	wrongCurrency := valid
	wrongCurrency.Currency = "USD"
	require.Error(t, validateWechatPayTransaction(wrongCurrency, "WXORDER"))
}

func TestGetWechatPayStatusSettlesSuccessfulProviderOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupWechatPayControllerTest(t)

	user := &model.User{Id: 702, Username: "wx_query_user", Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(user).Error)
	order := &model.TopUp{
		UserId:          user.Id,
		Amount:          2,
		CreditedQuota:   2 * 500000,
		Money:           10.01,
		MoneyCents:      1001,
		TradeNo:         "WXQUERYSETTLE",
		PaymentMethod:   model.PaymentMethodWechatNative,
		PaymentProvider: model.PaymentProviderWechatPay,
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, order.Insert())

	fake := &fakeWechatPayGateway{transaction: service.WechatPayTransaction{
		AppID:         setting.WechatPayAppID,
		MchID:         setting.WechatPayMchID,
		TradeNo:       order.TradeNo,
		TransactionID: "wx-transaction-702",
		TradeState:    "SUCCESS",
		TradeType:     "NATIVE",
		Currency:      "CNY",
		Total:         1001,
	}}
	originalFactory := newWechatPayGateway
	newWechatPayGateway = func(context.Context) (wechatPayGateway, error) { return fake, nil }
	t.Cleanup(func() { newWechatPayGateway = originalFactory })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/wechatpay/status/"+order.TradeNo, nil)
	ctx.Set("id", user.Id)
	ctx.Params = gin.Params{{Key: "trade_no", Value: order.TradeNo}}
	GetWechatPayStatus(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var reloaded model.User
	require.NoError(t, model.DB.First(&reloaded, user.Id).Error)
	assert.Equal(t, 2*500000, reloaded.Quota)
	assert.Equal(t, common.TopUpStatusSuccess, model.GetTopUpByTradeNo(order.TradeNo).Status)
}

func TestRequestWechatPayStoresAndSubmitsTheSameFenAmount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupWechatPayControllerTest(t)

	originalPrice := operation_setting.Price
	originalMinTopUp := operation_setting.MinTopUp
	originalCallback := operation_setting.CustomCallbackAddress
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalDiscount := operation_setting.GetPaymentSetting().AmountDiscount
	operation_setting.Price = 10.01
	operation_setting.MinTopUp = 1
	operation_setting.CustomCallbackAddress = "https://pay.example.com"
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	t.Cleanup(func() {
		operation_setting.Price = originalPrice
		operation_setting.MinTopUp = originalMinTopUp
		operation_setting.CustomCallbackAddress = originalCallback
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
		operation_setting.GetPaymentSetting().AmountDiscount = originalDiscount
	})

	user := &model.User{Id: 703, Username: "wx_create_user", Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, model.DB.Create(user).Error)
	fake := new(fakeWechatPayGateway)
	originalFactory := newWechatPayGateway
	originalGroupLookup := getWechatPayUserGroup
	newWechatPayGateway = func(context.Context) (wechatPayGateway, error) { return fake, nil }
	getWechatPayUserGroup = func(int, bool) (string, error) { return "default", nil }
	t.Cleanup(func() {
		newWechatPayGateway = originalFactory
		getWechatPayUserGroup = originalGroupLookup
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/user/wechatpay/pay",
		strings.NewReader(`{"amount":1,"payment_method":"wechat_native"}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", user.Id)
	RequestWechatPay(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.NotEmpty(t, fake.prepayTradeNo)
	assert.EqualValues(t, 1001, fake.prepayTotal)
	assert.Equal(t, "https://pay.example.com/api/user/wechatpay/notify", fake.prepayNotify)
	stored := model.GetTopUpByTradeNo(fake.prepayTradeNo)
	require.NotNil(t, stored)
	assert.Equal(t, 500000, stored.CreditedQuota)
	assert.EqualValues(t, 1001, stored.MoneyCents)
	assert.InDelta(t, 10.01, stored.Money, 0.000001)
}

func TestRequestWechatPayUsesCNYAsTheDirectTopUpUnit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupWechatPayControllerTest(t)

	originalPrice := operation_setting.Price
	originalMinTopUp := operation_setting.MinTopUp
	originalExchangeRate := operation_setting.USDExchangeRate
	originalCallback := operation_setting.CustomCallbackAddress
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalDiscount := operation_setting.GetPaymentSetting().AmountDiscount
	operation_setting.Price = 7.3
	operation_setting.MinTopUp = 1
	operation_setting.USDExchangeRate = 7.3
	operation_setting.CustomCallbackAddress = "https://pay.example.com"
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	t.Cleanup(func() {
		operation_setting.Price = originalPrice
		operation_setting.MinTopUp = originalMinTopUp
		operation_setting.USDExchangeRate = originalExchangeRate
		operation_setting.CustomCallbackAddress = originalCallback
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
		operation_setting.GetPaymentSetting().AmountDiscount = originalDiscount
	})

	user := &model.User{Id: 704, Username: "wx_cny_user", Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, model.DB.Create(user).Error)
	fake := new(fakeWechatPayGateway)
	originalFactory := newWechatPayGateway
	originalGroupLookup := getWechatPayUserGroup
	newWechatPayGateway = func(context.Context) (wechatPayGateway, error) { return fake, nil }
	getWechatPayUserGroup = func(int, bool) (string, error) { return "default", nil }
	t.Cleanup(func() {
		newWechatPayGateway = originalFactory
		getWechatPayUserGroup = originalGroupLookup
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/user/wechatpay/pay",
		strings.NewReader(`{"amount":100,"payment_method":"wechat_native"}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", user.Id)
	RequestWechatPay(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.EqualValues(t, 10_000, fake.prepayTotal)
	stored := model.GetTopUpByTradeNo(fake.prepayTradeNo)
	require.NotNil(t, stored)
	assert.Equal(t, 6_849_315, stored.CreditedQuota)
	assert.EqualValues(t, 10_000, stored.MoneyCents)
}
