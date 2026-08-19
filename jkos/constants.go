package jkos

// 环境基础地址（必须以斜杆结尾，与官方 SDK 一致）。
const (
	// ProductionBaseURL 正式环境基础地址。
	ProductionBaseURL = "https://onlinepay.jkopay.com/"
	// SandboxBaseURL 沙箱/测试环境基础地址。
	SandboxBaseURL = "https://uat-onlinepay.jkopay.app/"
)

// 请求接口路径。
const (
	// ModeEntry 下单（platform/entry）。
	ModeEntry = "platform/entry"
	// ModeInquiry 查询订单（platform/inquiry）。
	ModeInquiry = "platform/inquiry"
	// ModeRefund 退款（platform/refund）。
	ModeRefund = "platform/refund"
)

// 接口返回 result 代码。
const (
	// ResultSuccess 成功。
	ResultSuccess = "000"
)

// 交易状态 status（JKO Pay 定义，数值类型）。
const (
	// TradeStatusPaid 交易成功。
	TradeStatusPaid = 0
	// TradeStatusProcessing 付款处理中。
	TradeStatusProcessing = 1
	// TradeStatusUnpaid 此订单号尚未付款。
	TradeStatusUnpaid = 101
	// TradeStatusNotExist 此订单号不存在。
	TradeStatusNotExist = 102
)

// NotifySuccessAck 收到异步通知并处理完成后，应回传给 JKO Pay 的应答字符串。
const NotifySuccessAck = "OK"

// resultErrorMsg 接口返回 result 代码对应的错误描述。
var resultErrorMsg = map[string]string{
	"000": "成功",
	"100": "訂單不存在",
	"101": "此訂單號已付款",
	"103": "退款金額錯誤",
	"105": "remain_amount 或 refund_amount 金額不正確",
	"108": "店家收款額度已達上限或用戶交易已達限額",
	"113": "退款金額大於店家累計未請款金額",
	"200": "參數錯誤",
	"201": "參數錯誤",
	"922": "退款總金額超過原訂單金額",
	"999": "其他錯誤",
}

// getResultMsg 返回 result 代码对应的错误描述，未知代码返回空字符串。
func getResultMsg(code string) string {
	return resultErrorMsg[code]
}

// statusMsg 交易状态代码对应的描述。
var statusMsg = map[int]string{
	TradeStatusPaid:       "交易成功",
	TradeStatusProcessing: "付款處理中",
	TradeStatusUnpaid:     "此訂單號尚未付款",
	TradeStatusNotExist:   "此訂單號不存在",
}

// getStatusMsg 返回交易状态代码对应的描述，未知代码返回空字符串。
func getStatusMsg(code int) string {
	return statusMsg[code]
}
