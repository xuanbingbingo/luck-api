package controller

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/wxpay"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// 微信支付商品描述：合规起见永远固定为「开发者额度包」，
// 避免出现 AI/OpenAI/GPT/Claude 等触发支付平台风控的关键词。
const wxPayDescription = "开发者额度包"

type WxPayRequest struct {
	Amount int64 `json:"amount"`
}

// RequestWxPay 用户点击「微信支付」按钮后调用。
// 1. 校验 + 计算人民币金额
// 2. 创建本地 TopUp 订单（Pending）
// 3. 调微信支付 V3 Native 下单接口，拿 code_url
// 4. 返回 code_url 给前端，前端用它渲染二维码
func RequestWxPay(c *gin.Context) {
	if !operation_setting.IsWxPayConfigured() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "管理员未配置微信支付"})
		return
	}

	var req WxPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	if req.Amount < getMinTopup() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getMinTopup())})
		return
	}

	userId := c.GetInt("id")
	group, err := model.GetUserGroup(userId, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}

	payMoney := getPayMoney(req.Amount, group)
	if payMoney < 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}

	// 微信支付 amount.total 单位是「分」，且只能是整数。
	// 用 decimal 库做精确换算避免浮点丢精度。
	amountCents := decimal.NewFromFloat(payMoney).Mul(decimal.NewFromInt(100)).IntPart()
	if amountCents <= 0 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "金额换算异常"})
		return
	}

	// 生成 out_trade_no，最大 32 字符。
	// 格式：LUCK{userId}T{unix}{random6}，例：LUCK42T1716700000abc123
	tradeNo := fmt.Sprintf("LUCK%dT%d%s", userId, time.Now().Unix(), common.GetRandomString(6))
	if len(tradeNo) > 32 {
		tradeNo = tradeNo[:32]
	}

	notifyURL := strings.TrimRight(service.GetCallbackAddress(), "/") + "/api/user/wxpay/notify"

	// 先调微信下单。下单成功才落库，避免大量孤儿 Pending 订单。
	codeURL, err := wxpay.NativePrepay(c.Request.Context(), tradeNo, amountCents, wxPayDescription, notifyURL)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付 下单失败 user_id=%d trade_no=%s amount=%d money=%.2f error=%q",
			userId, tradeNo, req.Amount, payMoney, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "微信支付下单失败，请稍后再试"})
		return
	}

	// 与 epay 保持一致：tokens 显示模式下 amount 存换算后的「美元数量」
	amount := req.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dAmount := decimal.NewFromInt(amount)
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		amount = dAmount.Div(dQuotaPerUnit).IntPart()
	}

	topUp := &model.TopUp{
		UserId:          userId,
		Amount:          amount,
		Money:           payMoney,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodWxPay,
		PaymentProvider: model.PaymentProviderWxPay,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付 创建充值订单失败 user_id=%d trade_no=%s amount=%d error=%q",
			userId, tradeNo, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("微信支付 充值订单创建成功 user_id=%d trade_no=%s amount=%d money=%.2f cents=%d",
		userId, tradeNo, req.Amount, payMoney, amountCents))

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"trade_no":     tradeNo,
			"code_url":     codeURL,
			"money_cny":    payMoney,
			"display_unit": operation_setting.GetQuotaDisplayType(),
			"amount":       req.Amount, // 美元数量（或 tokens）
		},
	})
}

// WxPayNotify 接收微信异步回调。
// 流程：解密 + 验签 → 找单 → 防重 → 加 quota → 返回 200。
// 返回非 200 微信会重试（最多 8 次，间隔递增）。
func WxPayNotify(c *gin.Context) {
	if !operation_setting.IsWxPayConfigured() {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("微信支付 webhook 被拒绝 reason=not_configured client_ip=%s", c.ClientIP()))
		c.JSON(http.StatusBadRequest, gin.H{"code": "FAIL", "message": "未配置"})
		return
	}

	transaction, err := wxpay.ParseNotify(c.Request.Context(), c.Request)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付 webhook 解析失败 client_ip=%s error=%q", c.ClientIP(), err.Error()))
		// 返回非 200 让微信重试
		c.JSON(http.StatusBadRequest, gin.H{"code": "FAIL", "message": "解析失败"})
		return
	}

	if transaction.OutTradeNo == nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("微信支付 webhook 缺少 out_trade_no client_ip=%s", c.ClientIP()))
		c.JSON(http.StatusOK, gin.H{"code": "SUCCESS", "message": "OK"})
		return
	}
	tradeNo := *transaction.OutTradeNo

	tradeState := ""
	if transaction.TradeState != nil {
		tradeState = *transaction.TradeState
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("微信支付 webhook 收到 trade_no=%s trade_state=%s client_ip=%s",
		tradeNo, tradeState, c.ClientIP()))

	// 只处理支付成功事件；其他状态（NOTPAY/CLOSED/...）直接 ack 让微信不再重试
	if tradeState != "SUCCESS" {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("微信支付 忽略非成功状态 trade_no=%s trade_state=%s",
			tradeNo, tradeState))
		c.JSON(http.StatusOK, gin.H{"code": "SUCCESS", "message": "OK"})
		return
	}

	if err := completeWxPayTopUp(c, tradeNo, transaction); err != nil {
		// completeWxPayTopUp 内部已经打日志了
		// 业务错误也返回 SUCCESS：微信重试也救不了（订单不存在、provider 不匹配等是逻辑问题）
		c.JSON(http.StatusOK, gin.H{"code": "SUCCESS", "message": "OK"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": "SUCCESS", "message": "OK"})
}

// completeWxPayTopUp 完成订单：状态机推进 + 加 quota。
// 通过 trade_no 锁防止并发回调 + 主动查询的双重加 quota。
func completeWxPayTopUp(c *gin.Context, tradeNo string, transaction interface{}) error {
	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)

	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("微信支付 订单不存在 trade_no=%s", tradeNo))
		return fmt.Errorf("订单不存在")
	}
	if topUp.PaymentProvider != model.PaymentProviderWxPay {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("微信支付 订单网关不匹配 trade_no=%s order_provider=%s",
			tradeNo, topUp.PaymentProvider))
		return fmt.Errorf("订单网关不匹配")
	}
	if topUp.Status != common.TopUpStatusPending {
		// 已经处理过了（可能 notify 来了一次，前端查询又来一次），直接幂等返回
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("微信支付 订单已处理 trade_no=%s current_status=%s",
			tradeNo, topUp.Status))
		return nil
	}

	topUp.Status = common.TopUpStatusSuccess
	topUp.CompleteTime = time.Now().Unix()
	if err := topUp.Update(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付 更新订单状态失败 trade_no=%s error=%q",
			tradeNo, err.Error()))
		return err
	}

	// 计算要加的 quota（与 epay 同算法）
	dAmount := decimal.NewFromInt(topUp.Amount)
	dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
	quotaToAdd := int(dAmount.Mul(dQuotaPerUnit).IntPart())

	if err := model.IncreaseUserQuota(topUp.UserId, quotaToAdd, true); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付 加 quota 失败 trade_no=%s user_id=%d quota=%d error=%q",
			tradeNo, topUp.UserId, quotaToAdd, err.Error()))
		return err
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("微信支付 充值成功 trade_no=%s user_id=%d quota_to_add=%d money=%.2f",
		tradeNo, topUp.UserId, quotaToAdd, topUp.Money))
	model.RecordTopupLog(topUp.UserId,
		fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%f", logger.LogQuota(quotaToAdd), topUp.Money),
		c.ClientIP(), topUp.PaymentMethod, model.PaymentProviderWxPay)
	return nil
}

// QueryWxPayOrder 前端轮询用：主动查微信订单状态。
// 若发现已支付但本地还是 Pending，会主动补 quota（防 notify 丢失/延迟）。
func QueryWxPayOrder(c *gin.Context) {
	tradeNo := c.Param("trade_no")
	if tradeNo == "" {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "缺少 trade_no"})
		return
	}

	// 只能查自己的订单
	userId := c.GetInt("id")
	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "订单不存在"})
		return
	}
	if topUp.UserId != userId {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "无权查询此订单"})
		return
	}

	// 本地已经是成功状态，直接返回
	if topUp.Status == common.TopUpStatusSuccess {
		c.JSON(http.StatusOK, gin.H{
			"message": "success",
			"data": gin.H{
				"trade_no": tradeNo,
				"status":   "paid",
				"paid":     true,
			},
		})
		return
	}

	if !operation_setting.IsWxPayConfigured() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "微信支付未配置"})
		return
	}

	// 主动查微信侧状态
	result, err := wxpay.QueryOrder(c.Request.Context(), tradeNo)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付 主动查询失败 trade_no=%s error=%q",
			tradeNo, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "查询失败"})
		return
	}

	// 已支付但本地未更新 → 补加 quota（防 notify 漏单）
	if result.Paid && topUp.Status == common.TopUpStatusPending {
		if err := completeWxPayTopUp(c, tradeNo, nil); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付 主动查询触发补偿失败 trade_no=%s error=%q",
				tradeNo, err.Error()))
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"trade_no":    tradeNo,
			"status":      strings.ToLower(result.TradeState),
			"paid":        result.Paid,
			"total_cents": result.TotalAmount,
		},
	})
}
