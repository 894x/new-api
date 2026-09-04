package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/setting"

	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth/verifiers"
	"github.com/wechatpay-apiv3/wechatpay-go/core/notify"
	"github.com/wechatpay-apiv3/wechatpay-go/core/option"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
	"github.com/wechatpay-apiv3/wechatpay-go/utils"
)

type WechatPayTransaction struct {
	AppID         string
	MchID         string
	TradeNo       string
	TransactionID string
	TradeState    string
	TradeType     string
	Currency      string
	Total         int64
}

type WechatPayGateway struct {
	appID         string
	mchID         string
	nativeService native.NativeApiService
	notifyHandler *notify.Handler
}

func NewWechatPayGateway(ctx context.Context) (*WechatPayGateway, error) {
	appID := strings.TrimSpace(setting.WechatPayAppID)
	mchID := strings.TrimSpace(setting.WechatPayMchID)
	merchantSerial := strings.TrimSpace(setting.WechatPayMerchantSerialNumber)
	apiV3Key := strings.TrimSpace(setting.WechatPayAPIv3Key)
	privateKeyPEM := strings.TrimSpace(setting.WechatPayMerchantPrivateKey)
	publicKeyID := strings.TrimSpace(setting.WechatPayPublicKeyID)
	publicKeyPEM := strings.TrimSpace(setting.WechatPayPublicKey)
	if appID == "" || mchID == "" || merchantSerial == "" || apiV3Key == "" ||
		privateKeyPEM == "" || publicKeyID == "" || publicKeyPEM == "" {
		return nil, errors.New("微信支付配置不完整")
	}

	privateKey, err := utils.LoadPrivateKey(privateKeyPEM)
	if err != nil {
		return nil, errors.New("微信支付商户私钥无效")
	}
	publicKey, err := utils.LoadPublicKey(publicKeyPEM)
	if err != nil {
		return nil, errors.New("微信支付公钥无效")
	}

	client, err := core.NewClient(ctx, option.WithWechatPayPublicKeyAuthCipher(
		mchID,
		merchantSerial,
		privateKey,
		publicKeyID,
		publicKey,
	))
	if err != nil {
		return nil, err
	}
	notifyHandler, err := notify.NewRSANotifyHandler(
		apiV3Key,
		verifiers.NewSHA256WithRSAPubkeyVerifier(publicKeyID, *publicKey),
	)
	if err != nil {
		return nil, errors.New("微信支付 API v3 密钥无效")
	}

	return &WechatPayGateway{
		appID:         appID,
		mchID:         mchID,
		nativeService: native.NativeApiService{Client: client},
		notifyHandler: notifyHandler,
	}, nil
}

func (g *WechatPayGateway) Prepay(
	ctx context.Context,
	tradeNo string,
	description string,
	notifyURL string,
	totalCents int64,
	clientIP string,
) (string, error) {
	expiresAt := time.Now().Add(15 * time.Minute)
	currency := "CNY"
	request := native.PrepayRequest{
		Appid:       core.String(g.appID),
		Mchid:       core.String(g.mchID),
		Description: core.String(description),
		OutTradeNo:  core.String(tradeNo),
		TimeExpire:  core.Time(expiresAt),
		NotifyUrl:   core.String(notifyURL),
		Amount: &native.Amount{
			Total:    core.Int64(totalCents),
			Currency: core.String(currency),
		},
	}
	if strings.TrimSpace(clientIP) != "" {
		request.SceneInfo = &native.SceneInfo{PayerClientIp: core.String(clientIP)}
	}

	response, _, err := g.nativeService.Prepay(ctx, request)
	if err != nil {
		return "", err
	}
	if response == nil || response.CodeUrl == nil || strings.TrimSpace(*response.CodeUrl) == "" {
		return "", errors.New("微信支付未返回二维码地址")
	}
	return *response.CodeUrl, nil
}

func (g *WechatPayGateway) Query(ctx context.Context, tradeNo string) (WechatPayTransaction, error) {
	transaction, _, err := g.nativeService.QueryOrderByOutTradeNo(ctx, native.QueryOrderByOutTradeNoRequest{
		OutTradeNo: core.String(tradeNo),
		Mchid:      core.String(g.mchID),
	})
	if err != nil {
		return WechatPayTransaction{}, err
	}
	return normalizeWechatPayTransaction(transaction), nil
}

func (g *WechatPayGateway) ParseNotification(ctx context.Context, request *http.Request) (WechatPayTransaction, error) {
	transaction := new(payments.Transaction)
	if _, err := g.notifyHandler.ParseNotifyRequest(ctx, request, transaction); err != nil {
		return WechatPayTransaction{}, err
	}
	return normalizeWechatPayTransaction(transaction), nil
}

func normalizeWechatPayTransaction(transaction *payments.Transaction) WechatPayTransaction {
	if transaction == nil {
		return WechatPayTransaction{}
	}

	result := WechatPayTransaction{}
	if transaction.Appid != nil {
		result.AppID = *transaction.Appid
	}
	if transaction.Mchid != nil {
		result.MchID = *transaction.Mchid
	}
	if transaction.OutTradeNo != nil {
		result.TradeNo = *transaction.OutTradeNo
	}
	if transaction.TransactionId != nil {
		result.TransactionID = *transaction.TransactionId
	}
	if transaction.TradeState != nil {
		result.TradeState = *transaction.TradeState
	}
	if transaction.TradeType != nil {
		result.TradeType = *transaction.TradeType
	}
	if transaction.Amount != nil {
		if transaction.Amount.Currency != nil {
			result.Currency = *transaction.Amount.Currency
		}
		if transaction.Amount.Total != nil {
			result.Total = *transaction.Amount.Total
		}
	}
	return result
}
