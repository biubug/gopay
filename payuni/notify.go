package payuni

import (
	"fmt"
	"time"

	gopay "github.com/biubug/gopay"
)

// VerifyNotify 校验并解析 PAYUNi 异步通知/同步回传数据。
//
// rawData 为回调的原始 form/query 数据（application/x-www-form-urlencoded 或 query string）。
// 校验通过后返回解析结果；校验失败返回 error，上层应回以 HTTP 200 且不处理业务，
// 校验通过并处理业务后应回传 NotifySuccessAck（"1|OK"）。
func (c *Client) VerifyNotify(rawData []byte) (*gopay.NotifyResult, error) {
	fields := parseQuery(string(rawData))

	result := &gopay.NotifyResult{
		Channel: gopay.ChannelPayUni,
		Status:  fields["Status"],
		Message: fields["Message"],
	}

	encryptInfo := fields["EncryptInfo"]
	hashInfo := fields["HashInfo"]

	// 无 EncryptInfo：仅有 Status/Message 的错误通知，直接返回。
	if encryptInfo == "" {
		if result.Status == "" {
			return nil, fmt.Errorf("%w: missing EncryptInfo and Status", gopay.ErrVerifySignature)
		}
		return result, nil
	}

	if hashInfo == "" {
		return nil, fmt.Errorf("%w: missing HashInfo", gopay.ErrVerifySignature)
	}
	if c.hash(encryptInfo) != hashInfo {
		return nil, fmt.Errorf("%w: hash mismatch", gopay.ErrVerifySignature)
	}

	decrypted, err := c.decrypt(encryptInfo)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", gopay.ErrVerifySignature, err)
	}

	// 外层未带 Status 时，从解密结果取交易级 Status。
	if result.Status == "" {
		result.Status = decrypted["Status"]
	}
	if result.Message == "" {
		result.Message = decrypted["Message"]
	}

	result.TradeNo = decrypted["TradeNo"]
	result.OutTradeNo = decrypted["MerTradeNo"]
	result.Amount = decrypted["TradeAmt"]
	result.TradeState = mapTradeState(decrypted["TradeStatus"])
	result.Paid = result.TradeState == gopay.TradeStatePaid
	// 以下字段渠道未返回时为空字符串（零值），无需额外判断。
	// PAYUNi 仅支持 TWD，币种固定返回 TWD。
	result.Currency = "TWD"
	result.PayType = decrypted["PaymentType"]
	// 支付时间未返回时填当前时间。
	result.PayTime = decrypted["PayTime"]
	if result.PayTime == "" {
		result.PayTime = time.Now().Format("2006-01-02 15:04:05")
	}

	return result, nil
}

// mapTradeState 将 PAYUNi TradeStatus 映射为统一状态。
func mapTradeState(status string) gopay.TradeState {
	switch status {
	case TradeStatusIssued, TradeStatusUnpaid:
		return gopay.TradeStateUnpaid
	case TradeStatusPaid:
		return gopay.TradeStatePaid
	case TradeStatusFailed:
		return gopay.TradeStateFailed
	case TradeStatusCanceled, TradeStatusExpired:
		return gopay.TradeStateCanceled
	default:
		// TradeStatusPending（訂單待確認）等其余状态返回 UNKNOWN。
		return gopay.TradeStateUnknown
	}
}