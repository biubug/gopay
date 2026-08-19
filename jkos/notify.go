package jkos

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	gopay "github.com/biubug/gopay"
)

// VerifyNotify 解析 JKO Pay 异步通知（Result URL Callback）。
//
// rawData 为回调的原始 JSON 请求体。
// JKO Pay 回调不携带签名，安全校验依赖 IP 白名单（由调用方在 HTTP 处理层实现）。
// 校验通过并处理业务后应回传 NotifySuccessAck（"OK"）。
func (c *Client) VerifyNotify(rawData []byte) (*gopay.NotifyResult, error) {
	var payload struct {
		Transaction transaction `json:"transaction"`
	}
	if err := json.Unmarshal(rawData, &payload); err != nil {
		return nil, fmt.Errorf("jkos: decode notify: %w", err)
	}

	tx := payload.Transaction
	if tx.PlatformOrderID == "" {
		return nil, fmt.Errorf("%w: missing transaction.platform_order_id", gopay.ErrVerifySignature)
	}

	result := &gopay.NotifyResult{
		Channel:     gopay.ChannelJkos,
		OutTradeNo:  tx.PlatformOrderID,
		TradeNo:     tx.TradeNo,
		Amount:      strconv.Itoa(tx.FinalPrice),
		Currency:    "TWD",
		PayType:     tx.ChannelType,
		PayTime:     tx.TransTime,
		TradeState:  mapTradeState(tx.Status),
		Status:      strconv.Itoa(tx.Status),
		Message:     getStatusMsg(tx.Status),
	}
	result.Paid = result.TradeState == gopay.TradeStatePaid

	// 支付时间未返回时填当前时间。
	if result.PayTime == "" {
		result.PayTime = time.Now().Format("2006-01-02 15:04:05")
	}

	return result, nil
}

// mapTradeState 将 JKO Pay 交易状态映射为统一状态。
func mapTradeState(status int) gopay.TradeState {
	switch status {
	case TradeStatusPaid:
		return gopay.TradeStatePaid
	case TradeStatusUnpaid:
		return gopay.TradeStateUnpaid
	case TradeStatusProcessing, TradeStatusNotExist:
		return gopay.TradeStateUnknown
	default:
		return gopay.TradeStateUnknown
	}
}
