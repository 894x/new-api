package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

const wechatPayOrderTTL = 15 * time.Minute

type WechatPayRequest struct {
	Amount        int64  `json:"amount"`
	PaymentMethod string `json:"payment_method"`
}

type wechatPayGateway interface {
	Prepay(context.Context, string, string, string, int64, string) (string, error)
	Query(context.Context, string) (service.WechatPayTransaction, error)
	ParseNotification(context.Context, *http.Request) (service.WechatPayTransaction, error)
}

var newWechatPayGateway = func(ctx context.Context) (wechatPayGateway, error) {
	return service.NewWechatPayGateway(ctx)
}

var getWechatPayUserGroup = model.GetUserGroup

func RequestWechatPay(c *gin.Context) {
	if !isWechatPayTopUpEnabled() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "当前管理员未配置微信支付"})
		return
	}

	var request WechatPayRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	if request.PaymentMethod != model.PaymentMethodWechatNative {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "不支持的支付渠道"})
		return
	}
	if request.Amount < getMinTopup() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getMinTopup())})
		return
	}

	userID := c.GetInt("id")
	creditedQuota, err := validateTopUpQuota(request.Amount)
	if err == nil {
		err = model.ValidateTopUpQuotaCapacity(userID, creditedQuota)
	}
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}
	group, err := getWechatPayUserGroup(userID, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}

	money := decimal.NewFromFloat(getPayMoney(request.Amount, group)).Round(2)
	moneyCentsDecimal := money.Shift(2)
	if moneyCentsDecimal.LessThan(decimal.NewFromInt(1)) || !moneyCentsDecimal.BigInt().IsInt64() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额无效"})
		return
	}
	moneyCents := moneyCentsDecimal.IntPart()

	notifyURL, err := getWechatPayNotifyURL()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付回调地址无效 user_id=%d error=%q", userID, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "微信支付回调地址必须是公网 HTTPS 地址"})
		return
	}
	gateway, err := newWechatPayGateway(c.Request.Context())
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付 client 初始化失败 user_id=%d error=%q", userID, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "微信支付配置无效"})
		return
	}

	tradeNo := fmt.Sprintf("WX%d%s", time.Now().UnixMilli(), common.GetRandomString(8))
	creditedAmount := request.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		creditedAmount = decimal.NewFromInt(request.Amount).
			Div(decimal.NewFromFloat(common.QuotaPerUnit)).
			IntPart()
	}
	expiresAt := time.Now().Add(wechatPayOrderTTL)
	topUp := &model.TopUp{
		UserId:          userID,
		Amount:          creditedAmount,
		CreditedQuota:   creditedQuota,
		Money:           money.InexactFloat64(),
		MoneyCents:      moneyCents,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodWechatNative,
		PaymentProvider: model.PaymentProviderWechatPay,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付创建本地订单失败 user_id=%d trade_no=%s amount=%d error=%q", userID, tradeNo, request.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	codeURL, err := gateway.Prepay(
		c.Request.Context(),
		tradeNo,
		"账户余额充值",
		notifyURL,
		moneyCents,
		c.ClientIP(),
	)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付 Native 下单失败 user_id=%d trade_no=%s amount=%d money_cents=%d error=%q", userID, tradeNo, request.Amount, moneyCents, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("微信支付 Native 订单创建成功 user_id=%d trade_no=%s amount=%d money_cents=%d", userID, tradeNo, request.Amount, moneyCents))
	common.ApiSuccess(c, gin.H{
		"code_url":    codeURL,
		"trade_no":    tradeNo,
		"money_cents": moneyCents,
		"expires_at":  expiresAt.Unix(),
	})
}

func GetWechatPayStatus(c *gin.Context) {
	tradeNo := strings.TrimSpace(c.Param("trade_no"))
	userID := c.GetInt("id")
	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil || topUp.UserId != userID || topUp.PaymentProvider != model.PaymentProviderWechatPay {
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "data": "订单不存在"})
		return
	}
	if topUp.Status == common.TopUpStatusSuccess {
		common.ApiSuccess(c, gin.H{"status": common.TopUpStatusSuccess, "trade_no": tradeNo})
		return
	}
	if topUp.Status != common.TopUpStatusPending {
		common.ApiSuccess(c, gin.H{"status": topUp.Status, "trade_no": tradeNo})
		return
	}
	if !isWechatPayWebhookEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "error", "data": "微信支付当前不可用"})
		return
	}

	gateway, err := newWechatPayGateway(c.Request.Context())
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付查单 client 初始化失败 user_id=%d trade_no=%s error=%q", userID, tradeNo, err.Error()))
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "error", "data": "微信支付当前不可用"})
		return
	}
	transaction, err := gateway.Query(c.Request.Context(), tradeNo)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付主动查单失败 user_id=%d trade_no=%s error=%q", userID, tradeNo, err.Error()))
		c.JSON(http.StatusBadGateway, gin.H{"message": "error", "data": "查询支付状态失败"})
		return
	}

	if transaction.TradeState == "SUCCESS" {
		if err := validateWechatPayTransaction(transaction, tradeNo); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付主动查单校验失败 user_id=%d trade_no=%s transaction_id=%s error=%q", userID, tradeNo, transaction.TransactionID, err.Error()))
			c.JSON(http.StatusBadGateway, gin.H{"message": "error", "data": "支付结果校验失败"})
			return
		}
		if _, err := model.RechargeWechatPay(tradeNo, transaction.Total, transaction.TransactionID, c.ClientIP()); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付主动查单入账失败 user_id=%d trade_no=%s transaction_id=%s error=%q", userID, tradeNo, transaction.TransactionID, err.Error()))
			c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "data": "充值入账失败，请联系管理员"})
			return
		}
		common.ApiSuccess(c, gin.H{"status": common.TopUpStatusSuccess, "trade_no": tradeNo})
		return
	}

	common.ApiSuccess(c, gin.H{
		"status":      common.TopUpStatusPending,
		"trade_no":    tradeNo,
		"trade_state": transaction.TradeState,
	})
}

func WechatPayNotify(c *gin.Context) {
	if !isWechatPayWebhookEnabled() {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("微信支付 webhook 被拒绝 reason=webhook_disabled path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		wechatPayNotifyReply(c, http.StatusServiceUnavailable, "FAIL", "微信支付未启用")
		return
	}

	gateway, err := newWechatPayGateway(c.Request.Context())
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付 webhook client 初始化失败 client_ip=%s error=%q", c.ClientIP(), err.Error()))
		wechatPayNotifyReply(c, http.StatusServiceUnavailable, "FAIL", "服务配置错误")
		return
	}
	transaction, err := gateway.ParseNotification(c.Request.Context(), c.Request)
	if err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("微信支付 webhook 验签或解密失败 client_ip=%s error=%q", c.ClientIP(), err.Error()))
		wechatPayNotifyReply(c, http.StatusBadRequest, "FAIL", "签名验证失败")
		return
	}
	if transaction.TradeState != "SUCCESS" {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("微信支付 webhook 忽略非成功通知 trade_no=%s transaction_id=%s trade_state=%s client_ip=%s", transaction.TradeNo, transaction.TransactionID, transaction.TradeState, c.ClientIP()))
		wechatPayNotifyReply(c, http.StatusOK, "SUCCESS", "成功")
		return
	}
	if err := validateWechatPayTransaction(transaction, transaction.TradeNo); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付 webhook 交易校验失败 trade_no=%s transaction_id=%s client_ip=%s error=%q", transaction.TradeNo, transaction.TransactionID, c.ClientIP(), err.Error()))
		wechatPayNotifyReply(c, http.StatusBadRequest, "FAIL", "交易校验失败")
		return
	}

	alreadyDone, err := model.RechargeWechatPay(transaction.TradeNo, transaction.Total, transaction.TransactionID, c.ClientIP())
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付 webhook 入账失败 trade_no=%s transaction_id=%s money_cents=%d client_ip=%s error=%q", transaction.TradeNo, transaction.TransactionID, transaction.Total, c.ClientIP(), err.Error()))
		status := http.StatusInternalServerError
		if errors.Is(err, model.ErrPaymentAmountMismatch) || errors.Is(err, model.ErrPaymentMethodMismatch) {
			status = http.StatusBadRequest
		}
		wechatPayNotifyReply(c, status, "FAIL", "充值入账失败")
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("微信支付 webhook 入账成功 trade_no=%s transaction_id=%s money_cents=%d already_done=%t client_ip=%s", transaction.TradeNo, transaction.TransactionID, transaction.Total, alreadyDone, c.ClientIP()))
	wechatPayNotifyReply(c, http.StatusOK, "SUCCESS", "成功")
}

func getWechatPayNotifyURL() (string, error) {
	callbackAddress := strings.TrimRight(strings.TrimSpace(service.GetCallbackAddress()), "/")
	parsed, err := url.Parse(callbackAddress)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("callback address must be an HTTPS origin")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", errors.New("callback address must not include a path")
	}
	return callbackAddress + "/api/user/wechatpay/notify", nil
}

func validateWechatPayTransaction(transaction service.WechatPayTransaction, expectedTradeNo string) error {
	if transaction.AppID != strings.TrimSpace(setting.WechatPayAppID) ||
		transaction.MchID != strings.TrimSpace(setting.WechatPayMchID) {
		return errors.New("merchant identity mismatch")
	}
	if transaction.TradeNo == "" || transaction.TradeNo != expectedTradeNo {
		return errors.New("trade number mismatch")
	}
	if transaction.TransactionID == "" {
		return errors.New("missing transaction id")
	}
	if transaction.TradeState != "SUCCESS" {
		return errors.New("transaction is not successful")
	}
	if transaction.TradeType != "NATIVE" {
		return errors.New("payment trade type mismatch")
	}
	if transaction.Currency != "CNY" || transaction.Total <= 0 {
		return errors.New("invalid transaction amount")
	}
	return nil
}

func wechatPayNotifyReply(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{"code": code, "message": message})
}
