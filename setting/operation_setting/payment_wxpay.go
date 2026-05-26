package operation_setting

import (
	"strings"
)

// 微信支付 V3 Native 配置项。所有字段通过管理员后台「系统设置 → 支付设置」录入，
// 私钥/证书以 PEM 字符串形式存入 OptionMap（外层已有加密层）。
var (
	WxMchId         = ""
	WxAppId         = ""
	WxApiV3Key      = ""
	WxCertSerialNo  = ""
	WxPrivateKeyPem = ""
	WxCertPem       = ""
	WxMinTopUp      = 1
)

func IsWxPayConfigured() bool {
	return strings.TrimSpace(WxMchId) != "" &&
		strings.TrimSpace(WxAppId) != "" &&
		strings.TrimSpace(WxApiV3Key) != "" &&
		strings.TrimSpace(WxCertSerialNo) != "" &&
		strings.TrimSpace(WxPrivateKeyPem) != ""
}
