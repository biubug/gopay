package payuni

// 环境基础地址。
const (
	// ProductionBaseURL 正式环境基础地址。
	ProductionBaseURL = "https://api.payuni.com.tw/api/"
	// SandboxBaseURL 沙箱/测试环境基础地址。
	SandboxBaseURL = "https://sandbox-api.payuni.com.tw/api/"
)

// 请求 mode -> 接口路径（与官方 SDK 的 UniversalTrade 一致）。
const (
	// ModeUPP 整合式支付页（幕前，返回自动提交表单）。
	ModeUPP = "upp"
	// ModeATM 虚拟账号幕後。
	ModeATM = "atm"
	// ModeCVS 超商代碼幕後。
	ModeCVS = "cvs"
	// ModeCredit 信用卡幕後。
	ModeCredit = "credit"
	// ModeLinePay LINE Pay 幕後。
	ModeLinePay = "linepay"
	// ModeAfteeDirect AFTEE 幕後。
	ModeAfteeDirect = "aftee_direct"
	// ModeTradeQuery 交易查詢。
	ModeTradeQuery = "trade/query"
	// ModeTradeClose 交易請退款（信用卡請款/退款）。
	ModeTradeClose = "trade/close"
	// ModeTradeCancel 交易取消授權。
	ModeTradeCancel = "trade/cancel"
	// ModeCancelCVS 交易取消超商代碼。
	ModeCancelCVS = "cancel_cvs"
	// ModeCreditBindQuery 信用卡 Token 查詢。
	ModeCreditBindQuery = "credit_bind/query"
	// ModeCreditBindCancel 信用卡 Token 取消。
	ModeCreditBindCancel = "credit_bind/cancel"
	// ModeTradeRefundICash 愛金卡退款。
	ModeTradeRefundICash = "trade/common/refund/icash"
	// ModeTradeRefundAftee 後支付退款。
	ModeTradeRefundAftee = "trade/common/refund/aftee"
	// ModeTradeRefundLinePay LINE Pay 退款。
	ModeTradeRefundLinePay = "trade/common/refund/linepay"
	// ModeTradeConfirmAftee 後支付確認。
	ModeTradeConfirmAftee = "trade/common/confirm/aftee"
)

// 交易状态 TradeStatus（PAYUNi 定义）。
const (
	// TradeStatusUnpaid 未付款。
	TradeStatusUnpaid = "0"
	// TradeStatusPaid 已付款。
	TradeStatusPaid = "1"
	// TradeStatusFailed 付款失敗。
	TradeStatusFailed = "2"
	// TradeStatusCanceled 已取消。
	TradeStatusCanceled = "3"
	// TradeStatusRefunded 已退款。
	TradeStatusRefunded = "6"
)

// 关账类型 CloseType（含义以官方文件为准）。
const (
	// CloseTypeRefund 退款，RefundOrder 默认值。
	CloseTypeRefund = "2"
)

// NotifySuccessAck 收到异步通知并通过校验后，应回传给 PAYUNi 的应答字符串。
const NotifySuccessAck = "1|OK"

// defaultVersion 默认接口版本号。
const defaultVersion = "1.0"

// queryVersion 交易查詢接口版本号（官方文件固定 2.0）。
const queryVersion = "2.0"