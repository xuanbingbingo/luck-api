// Package wxpay 封装微信支付 V3 Native 客户端。
//
// 使用懒加载 + 配置指纹缓存：admin 在后台改完 mch_id / 证书 / APIv3 密钥后，
// 下一次调用 GetClient 会检测到指纹变化并自动重建客户端，不需要重启进程。
package wxpay

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth/verifiers"
	"github.com/wechatpay-apiv3/wechatpay-go/core/downloader"
	"github.com/wechatpay-apiv3/wechatpay-go/core/notify"
	"github.com/wechatpay-apiv3/wechatpay-go/core/option"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
	"github.com/wechatpay-apiv3/wechatpay-go/utils"
)

var (
	cachedClient      *core.Client
	cachedFingerprint string
	clientMu          sync.Mutex
)

// 配置指纹：mch_id + 证书序列号 + ApiV3 密钥 + 私钥 PEM 长度。
// 配置变更后会触发客户端重建。
func configFingerprint() string {
	return operation_setting.WxMchId + "|" +
		operation_setting.WxCertSerialNo + "|" +
		operation_setting.WxApiV3Key + "|" +
		fmt.Sprintf("%d", len(operation_setting.WxPrivateKeyPem))
}

// GetClient 拿到 V3 客户端（懒加载 + 配置变更自动重建）。
// 调用方负责保证 IsWxPayConfigured()。
func GetClient(ctx context.Context) (*core.Client, error) {
	if !operation_setting.IsWxPayConfigured() {
		return nil, fmt.Errorf("微信支付未配置")
	}

	fingerprint := configFingerprint()

	clientMu.Lock()
	defer clientMu.Unlock()

	if cachedClient != nil && cachedFingerprint == fingerprint {
		return cachedClient, nil
	}

	privateKey, err := utils.LoadPrivateKey(operation_setting.WxPrivateKeyPem)
	if err != nil {
		return nil, fmt.Errorf("加载微信支付私钥失败: %w", err)
	}

	// WithWechatPayAutoAuthCipher 会自动：
	// 1. 注册 downloader（用于下载平台证书）
	// 2. 配置签名器（用商户私钥签名）
	// 3. 配置加解密器（AES-256-GCM with APIv3 Key）
	client, err := core.NewClient(
		ctx,
		option.WithWechatPayAutoAuthCipher(
			operation_setting.WxMchId,
			operation_setting.WxCertSerialNo,
			privateKey,
			operation_setting.WxApiV3Key,
		),
	)
	if err != nil {
		return nil, fmt.Errorf("创建微信支付客户端失败: %w", err)
	}

	cachedClient = client
	cachedFingerprint = fingerprint
	return client, nil
}

// NativePrepay 发起 Native 扫码下单，返回 code_url（二维码内容字符串）。
// amountCents 单位是分（微信支付 V3 强制要求）；商品描述传给微信后会显示在支付页面上。
func NativePrepay(ctx context.Context, tradeNo string, amountCents int64, description, notifyURL string) (string, error) {
	client, err := GetClient(ctx)
	if err != nil {
		return "", err
	}

	svc := native.NativeApiService{Client: client}
	resp, _, err := svc.Prepay(ctx, native.PrepayRequest{
		Appid:       core.String(operation_setting.WxAppId),
		Mchid:       core.String(operation_setting.WxMchId),
		Description: core.String(description),
		OutTradeNo:  core.String(tradeNo),
		NotifyUrl:   core.String(notifyURL),
		Amount: &native.Amount{
			Total:    core.Int64(amountCents),
			Currency: core.String("CNY"),
		},
	})
	if err != nil {
		return "", fmt.Errorf("微信支付下单失败: %w", err)
	}
	if resp == nil || resp.CodeUrl == nil {
		return "", fmt.Errorf("微信支付返回 code_url 为空")
	}
	return *resp.CodeUrl, nil
}

// OrderQueryResult 订单查询结果（简化）。
type OrderQueryResult struct {
	TradeState    string // SUCCESS / REFUND / NOTPAY / CLOSED / REVOKED / USERPAYING / PAYERROR
	TransactionId string // 微信流水号（仅 PAID 后有值）
	TotalAmount   int64  // 总金额（分）
	Paid          bool   // TradeState == SUCCESS
}

// QueryOrder 按 out_trade_no 查订单状态。前端轮询用。
func QueryOrder(ctx context.Context, tradeNo string) (*OrderQueryResult, error) {
	client, err := GetClient(ctx)
	if err != nil {
		return nil, err
	}

	svc := native.NativeApiService{Client: client}
	resp, _, err := svc.QueryOrderByOutTradeNo(ctx, native.QueryOrderByOutTradeNoRequest{
		OutTradeNo: core.String(tradeNo),
		Mchid:      core.String(operation_setting.WxMchId),
	})
	if err != nil {
		return nil, fmt.Errorf("查询微信订单失败: %w", err)
	}

	result := &OrderQueryResult{}
	if resp.TradeState != nil {
		result.TradeState = *resp.TradeState
		result.Paid = result.TradeState == "SUCCESS"
	}
	if resp.TransactionId != nil {
		result.TransactionId = *resp.TransactionId
	}
	if resp.Amount != nil && resp.Amount.Total != nil {
		result.TotalAmount = *resp.Amount.Total
	}
	return result, nil
}

// ParseNotify 解密 + 验签 V3 异步回调，返回 Transaction 结构。
// 必须先调用过 GetClient 才能用（否则 downloader 未注册）。
func ParseNotify(ctx context.Context, req *http.Request) (*payments.Transaction, error) {
	// 触发 client 初始化（如果还没初始化，会自动注册 downloader）
	if _, err := GetClient(ctx); err != nil {
		return nil, err
	}

	certVisitor := downloader.MgrInstance().GetCertificateVisitor(operation_setting.WxMchId)
	handler, err := notify.NewRSANotifyHandler(
		operation_setting.WxApiV3Key,
		verifiers.NewSHA256WithRSAVerifier(certVisitor),
	)
	if err != nil {
		return nil, fmt.Errorf("创建回调处理器失败: %w", err)
	}

	transaction := new(payments.Transaction)
	if _, err := handler.ParseNotifyRequest(ctx, req, transaction); err != nil {
		return nil, fmt.Errorf("解析微信支付回调失败: %w", err)
	}
	return transaction, nil
}
