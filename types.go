// Package gopay 提供聚合支付客户端抽象接口与通用数据结构。
//
// 上层业务通过 PaymentClient 接口与具体支付渠道解耦。新增支付渠道时，
// 只需在对应子包（如 payuni）中实现该接口，无需改动上层调用代码。
package gopay

// 渠道标识常量。
const (
	// ChannelPayUni 統一金流 PAYUNi。
	ChannelPayUni = "payuni"
	// ChannelJkos 街口支付 JKO Pay。
	ChannelJkos = "jkos"
)

// TradeState 统一交易状态。
type TradeState string

const (
	// TradeStateUnknown 未知状态。
	TradeStateUnknown TradeState = "UNKNOWN"
	// TradeStateUnpaid 待付款 / 未付款。
	TradeStateUnpaid TradeState = "UNPAID"
	// TradeStatePaid 已付款 / 交易成功。
	TradeStatePaid TradeState = "PAID"
	// TradeStateFailed 付款失败。
	TradeStateFailed TradeState = "FAILED"
	// TradeStateCanceled 已取消。
	TradeStateCanceled TradeState = "CANCELED"
	// TradeStateRefunded 已退款。
	TradeStateRefunded TradeState = "REFUNDED"
)

// ResultType 创建支付单返回结果的类型。
type ResultType string

const (
	// ResultTypeURL 返回跳转地址。
	ResultTypeURL ResultType = "url"
	// ResultTypeHTML 返回自动提交的表单 HTML。
	ResultTypeHTML ResultType = "html"
	// ResultTypeRaw 后台类交易，返回结构化原始数据。
	ResultTypeRaw ResultType = "raw"
)

// PaymentClient 聚合支付客户端统一接口。
//
// 所有支付渠道均需实现该接口，上层调用只依赖此接口。
type PaymentClient interface {
	// CreateOrder 创建支付单，返回支付跳转地址或自动提交表单 HTML。
	CreateOrder(req *CreateOrderRequest) (*CreateOrderResponse, error)
	// QueryOrder 查询订单状态。
	QueryOrder(req *QueryOrderRequest) (*QueryOrderResponse, error)
	// RefundOrder 退款（预留实现）。
	RefundOrder(req *RefundOrderRequest) (*RefundOrderResponse, error)
	// VerifyNotify 校验异步回调/同步回传通知的签名并解析数据。
	VerifyNotify(rawData []byte) (*NotifyResult, error)
	// NotifyAck 返回回调成功后应答给渠道的字符串。
	// 调用方在校验通过并处理完业务后，将此字符串作为 HTTP 响应体返回。
	NotifyAck() string
}

// CreateOrderRequest 创建支付单请求。
type CreateOrderRequest struct {
	// OutTradeNo 商户订单号，必填。
	OutTradeNo string
	// Amount 订单金额，字符串表示以避免浮点精度问题，必填。
	Amount string
	// Currency 币种代码（如 TWD、CNY、USD），为空时使用渠道默认币种。
	// 注意：PAYUNi 渠道仅支持 TWD。
	Currency string
	// Subject 商品/订单描述。
	Subject string
	// ReturnURL 支付完成后浏览器同步跳转地址。
	ReturnURL string
	// NotifyURL 异步通知地址。
	NotifyURL string
	// PayType 渠道内支付方式（如 payuni 的 upp/atm/cvs/credit），
	// 为空时使用渠道默认方式（payuni 默认为整合式支付页 upp）。
	PayType string
	// Extra 渠道扩展参数，透传给上游。
	Extra map[string]string
}

// CreateOrderResponse 创建支付单响应。
type CreateOrderResponse struct {
	// Channel 渠道标识，如 gopay.ChannelPayUni。
	Channel string
	// PayType 实际使用的支付方式。
	PayType string
	// ResultType 结果类型（url/html/raw）。
	ResultType ResultType
	// RedirectURL 当 ResultType == ResultTypeURL 时有值。
	RedirectURL string
	// FormHTML 当 ResultType == ResultTypeHTML 时有值。
	FormHTML string
	// RawBody 上游原始响应报文，便于调试。
	RawBody string
}

// QueryOrderRequest 查询订单请求。
type QueryOrderRequest struct {
	// OutTradeNo 商户订单号。
	OutTradeNo string
	// TradeNo 渠道平台交易号。
	TradeNo string
	// Extra 渠道扩展参数。
	Extra map[string]string
}

// QueryOrderResponse 查询订单响应。
type QueryOrderResponse struct {
	// Channel 渠道标识。
	Channel string
	// OutTradeNo 商户订单号。
	OutTradeNo string
	// TradeNo 渠道平台交易号。
	TradeNo string
	// TradeState 统一交易状态。
	TradeState TradeState
	// Amount 订单金额。
	Amount string
	// RawBody 上游原始响应报文。
	RawBody string
}

// RefundOrderRequest 退款请求。
type RefundOrderRequest struct {
	// TradeNo 渠道平台交易号（需退款的原始交易）。
	// 部分渠道（如 PAYUNi）按渠道交易号退款时必填。
	TradeNo string
	// OutTradeNo 商户订单号。
	// 部分渠道（如 JKO Pay）按商户订单号退款时必填。
	OutTradeNo string
	// Amount 退款金额，部分渠道支持空值为全额退款。
	Amount string
	// CloseType 渠道关账/退款类型，含义由渠道定义（payuni 参考官方文件）。
	CloseType string
	// Extra 渠道扩展参数。
	Extra map[string]string
}

// RefundOrderResponse 退款响应。
type RefundOrderResponse struct {
	// Channel 渠道标识。
	Channel string
	// Success 是否成功。
	Success bool
	// Message 结果信息。
	Message string
	// RawBody 上游原始响应报文。
	RawBody string
}

// NotifyResult 回调/通知解析结果。
type NotifyResult struct {
	// Channel 渠道标识。
	Channel string
	// Status 通知/交易状态（如 SUCCESS / ERROR）。
	Status string
	// Message 提示信息。
	Message string
	// OutTradeNo 商户订单号。
	OutTradeNo string
	// TradeNo 渠道平台交易号。
	TradeNo string
	// TradeState 统一交易状态。
	TradeState TradeState
	// Amount 订单金额。
	Amount string
	// Currency 币种代码（如 TWD），渠道未返回时为空。
	Currency string
	// PayType 支付方式（渠道原始值，如 PAYUNi 的 PaymentType），渠道未返回时为空。
	PayType string
	// PayTime 支付时间（渠道原始字符串，如 PAYUNi 的 PayTime），渠道未返回时为空。
	PayTime string
	// Paid 是否支付成功（TradeState == TradeStatePaid 的便捷判断）。
	Paid bool
}
