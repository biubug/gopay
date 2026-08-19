# gopay 详细文档

Go 聚合支付 SDK，通过统一的 `PaymentClient` 接口屏蔽不同支付渠道的差异。

- 模块路径：`github.com/biubug/gopay`
- Go 版本：`>= 1.21`
- 依赖：仅 Go 标准库，零第三方依赖

## 目录

- [安装](#安装)
- [架构总览](#架构总览)
- [通用接口 PaymentClient](#通用接口-paymentclient)
- [通用数据结构](#通用数据结构)
  - [CreateOrderRequest](#createorderrequest)
  - [CreateOrderResponse](#createorderresponse)
  - [QueryOrderRequest](#queryorderrequest)
  - [QueryOrderResponse](#queryorderresponse)
  - [RefundOrderRequest](#refundorderrequest)
  - [RefundOrderResponse](#refundorderresponse)
  - [NotifyResult](#notifyresult)
- [统一枚举](#统一枚举)
- [统一错误](#统一错误)
- [PAYUNi 渠道](#payuni-渠道)
  - [配置](#payuni-配置)
  - [创建支付单 CreateOrder](#payuni-createorder)
  - [查询订单 QueryOrder](#payuni-queryorder)
  - [退款 RefundOrder](#payuni-refundorder)
  - [通用交易 UniversalTrade](#payuni-universaltrade)
  - [异步通知 VerifyNotify](#payuni-verifynotify)
  - [常量](#payuni-常量)
  - [完整调用案例](#payuni-完整调用案例)
- [JKO Pay 渠道](#jkos-渠道)
  - [配置](#jkos-配置)
  - [创建支付单 CreateOrder](#jkos-createorder)
  - [查询订单 QueryOrder](#jkos-queryorder)
  - [退款 RefundOrder](#jkos-refundorder)
  - [异步通知 VerifyNotify](#jkos-verifynotify)
  - [常量](#jkos-常量)
  - [完整调用案例](#jkos-完整调用案例)
- [异步通知处理通用模式](#异步通知处理通用模式)
- [错误处理](#错误处理)
- [测试](#测试)

---

## 安装

```bash
go get github.com/biubug/gopay
```

本地开发可使用 `replace` 指向本地目录：

```go
// go.mod
require github.com/biubug/gopay v0.0.0

replace github.com/biubug/gopay => ../gopay
```

## 架构总览

```
gopay/
├── types.go          # PaymentClient 接口 + 通用请求/响应 + 统一状态
├── errors.go         # 统一错误
├── payuni/           # PAYUNi 渠道实现
│   ├── client.go     # 配置 / New / CreateOrder / QueryOrder / RefundOrder / UniversalTrade
│   ├── crypto.go     # AES-256-GCM 加解密 / HashInfo / query 编解码
│   ├── notify.go     # VerifyNotify / 状态映射
│   └── constants.go  # 地址 / mode / 交易状态 / 关账类型
└── jkos/             # JKO Pay 渠道实现
    ├── client.go     # 配置 / New / CreateOrder / QueryOrder / RefundOrder
    ├── notify.go     # VerifyNotify / 状态映射
    ├── constants.go  # 地址 / mode / result 代码 / 交易状态
    └── client_test.go
```

上层调用只依赖 `gopay.PaymentClient` 接口，切换/新增渠道时仅需替换构造器。

---

## 通用接口 PaymentClient

所有支付渠道均需实现该接口，定义于 [types.go](file:///e:/go-project/src/gopay/types.go)：

```go
type PaymentClient interface {
    // CreateOrder 创建支付单，返回支付跳转地址或自动提交表单 HTML。
    CreateOrder(req *CreateOrderRequest) (*CreateOrderResponse, error)
    // QueryOrder 查询订单状态。
    QueryOrder(req *QueryOrderRequest) (*QueryOrderResponse, error)
    // RefundOrder 退款。
    RefundOrder(req *RefundOrderRequest) (*RefundOrderResponse, error)
    // VerifyNotify 校验异步回调/同步回传通知的签名并解析数据。
    VerifyNotify(rawData []byte) (*NotifyResult, error)
    // NotifyAck 返回回调成功后应答给渠道的字符串。
    NotifyAck() string
}
```

---

## 通用数据结构

> 下面所有请求/响应结构均定义于 [types.go](file:///e:/go-project/src/gopay/types.go)。
> “必传”列以通用接口视角说明；各渠道实现可能进一步收紧校验（见各渠道章节）。

### CreateOrderRequest

创建支付单请求。

| 字段 | 类型 | 必传 | 说明 |
| --- | --- | --- | --- |
| `OutTradeNo` | `string` | 是 | 商户订单号。PAYUNi 要求长度 ≤ 25，仅允许字母、数字、下划线，且不能全为数字或全为字母。 |
| `Amount` | `string` | 是 | 订单金额，字符串表示以避免浮点精度问题。PAYUNi 原样透传；JKO Pay 会 `parseAmount` 转为整数（截断小数）。 |
| `Currency` | `string` | 否 | 币种代码（如 `TWD`/`CNY`/`USD`）。为空时使用渠道默认币种。当前两渠道均仅支持 `TWD`，传非 `TWD` 会报错。 |
| `Subject` | `string` | 否 | 商品/订单描述。PAYUNi 映射为 `ProdDesc`；JKO Pay 未使用该字段（会被忽略）。 |
| `ReturnURL` | `string` | 否 | 支付完成后浏览器同步跳转地址。PAYUNi 映射为 `ReturnURL`；JKO Pay 映射为 `result_display_url`。 |
| `NotifyURL` | `string` | 否 | 异步通知地址。PAYUNi 映射为 `NotifyURL`；JKO Pay 映射为 `result_url`。 |
| `PayType` | `string` | 否 | 渠道内支付方式。PAYUNi 的 `CreateOrder` 仅允许空或 `upp`，其他后台方式需用 `UniversalTrade`；JKO Pay 未使用。 |
| `Extra` | `map[string]string` | 否 | 渠道扩展参数，原样透传给上游。会覆盖同名默认字段。 |

### CreateOrderResponse

创建支付单响应。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `Channel` | `string` | 渠道标识，如 `gopay.ChannelPayUni` / `gopay.ChannelJkos`。 |
| `PayType` | `string` | 实际使用的支付方式。PAYUNi 为 `upp`；JKO Pay 为空。 |
| `ResultType` | `ResultType` | 结果类型：`url`（跳转地址）/ `html`（自动提交表单）/ `raw`（原始数据）。 |
| `RedirectURL` | `string` | 当 `ResultType == ResultTypeURL` 时有值（JKO Pay 返回此字段）。 |
| `FormHTML` | `string` | 当 `ResultType == ResultTypeHTML` 时有值，直接输出给浏览器即可自动跳转（PAYUNi 返回此字段）。 |
| `RawBody` | `string` | 上游原始响应报文，便于调试。PAYUNi 的 `CreateOrder` 不发请求，该字段为空。 |

### QueryOrderRequest

查询订单请求。

| 字段 | 类型 | 必传 | 说明 |
| --- | --- | --- | --- |
| `OutTradeNo` | `string` | 二选一 | 商户订单号。JKO Pay 必须用此字段查询（不支持 `TradeNo`）。 |
| `TradeNo` | `string` | 二选一 | 渠道平台交易号。PAYUNi 支持用此字段或 `OutTradeNo` 查询，至少传一个。 |
| `Extra` | `map[string]string` | 否 | 渠道扩展参数。 |

> PAYUNi：`OutTradeNo` 与 `TradeNo` 至少传一个；JKO Pay：必须传 `OutTradeNo`。

### QueryOrderResponse

查询订单响应。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `Channel` | `string` | 渠道标识。 |
| `OutTradeNo` | `string` | 商户订单号。 |
| `TradeNo` | `string` | 渠道平台交易号。 |
| `TradeState` | `TradeState` | 统一交易状态，见 [统一枚举](#统一枚举)。 |
| `Amount` | `string` | 订单金额。 |
| `RawBody` | `string` | 上游原始响应报文。 |

### RefundOrderRequest

退款请求。

| 字段 | 类型 | 必传 | 说明 |
| --- | --- | --- | --- |
| `TradeNo` | `string` | PAYUNi 必传 | 渠道平台交易号。PAYUNi 按此退款。 |
| `OutTradeNo` | `string` | JKO Pay 必传 | 商户订单号。JKO Pay 按此退款。 |
| `Amount` | `string` | JKO Pay 必传；PAYUNi 选传 | 退款金额。PAYUNi 为空表示全额退款（由渠道决定）；JKO Pay 必传。 |
| `CloseType` | `string` | 否 | 渠道关账/退款类型。PAYUNi 默认 `CloseTypeRefund`(`"2"`)。 |
| `Extra` | `map[string]string` | 否 | 渠道扩展参数。 |

### RefundOrderResponse

退款响应。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `Channel` | `string` | 渠道标识。 |
| `Success` | `bool` | 是否成功（当前实现：无 error 即视为 `true`）。 |
| `Message` | `string` | 结果信息。PAYUNi 取解密后的 `Message`；JKO Pay 取 `result` 代码对应的中文描述。 |
| `RawBody` | `string` | 上游原始响应报文。 |

### NotifyResult

回调/通知解析结果。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `Channel` | `string` | 渠道标识。 |
| `Status` | `string` | 通知/交易状态（渠道原始值，如 PAYUNi 的 `SUCCESS`/`ERROR`，JKO Pay 的状态码数字字符串）。 |
| `Message` | `string` | 提示信息。 |
| `OutTradeNo` | `string` | 商户订单号。 |
| `TradeNo` | `string` | 渠道平台交易号。 |
| `TradeState` | `TradeState` | 统一交易状态。 |
| `Amount` | `string` | 订单金额。 |
| `Currency` | `string` | 币种代码，当前两渠道固定返回 `TWD`。 |
| `PayType` | `string` | 支付方式（渠道原始值，如 PAYUNi 的 `PaymentType`、JKO Pay 的 `channel_type`）。 |
| `PayTime` | `string` | 支付时间（渠道原始字符串）。渠道未返回时填当前时间。 |
| `Paid` | `bool` | 是否支付成功（`TradeState == TradeStatePaid` 的便捷判断）。 |

---

## 统一枚举

定义于 [types.go](file:///e:/go-project/src/gopay/types.go)。

### 渠道标识

| 常量 | 值 | 说明 |
| --- | --- | --- |
| `ChannelPayUni` | `"payuni"` | 統一金流 PAYUNi |
| `ChannelJkos` | `"jkos"` | 街口支付 JKO Pay |

### TradeState 统一交易状态

| 常量 | 值 | 说明 |
| --- | --- | --- |
| `TradeStateUnknown` | `"UNKNOWN"` | 未知状态 |
| `TradeStateUnpaid` | `"UNPAID"` | 待付款 / 未付款 |
| `TradeStatePaid` | `"PAID"` | 已付款 / 交易成功 |
| `TradeStateFailed` | `"FAILED"` | 付款失败 |
| `TradeStateCanceled` | `"CANCELED"` | 已取消 |
| `TradeStateRefunded` | `"REFUNDED"` | 已退款 |

### ResultType 结果类型

| 常量 | 值 | 说明 |
| --- | --- | --- |
| `ResultTypeURL` | `"url"` | 返回跳转地址 |
| `ResultTypeHTML` | `"html"` | 返回自动提交的表单 HTML |
| `ResultTypeRaw` | `"raw"` | 后台类交易，返回结构化原始数据 |

---

## 统一错误

定义于 [errors.go](file:///e:/go-project/src/gopay/errors.go)，全部支持 `errors.Is` 判断。

| 错误变量 | 含义 |
| --- | --- |
| `ErrNotImplemented` | 功能尚未实现 |
| `ErrInvalidConfig` | 渠道配置无效 |
| `ErrInvalidRequest` | 请求参数无效 |
| `ErrVerifySignature` | 回调/响应签名校验失败 |
| `ErrAPIResponse` | 上游接口返回错误 |

---

## PAYUNi 渠道

> 包路径：`github.com/biubug/gopay/payuni`
> 实现文件：[client.go](file:///e:/go-project/src/gopay/payuni/client.go) / [crypto.go](file:///e:/go-project/src/gopay/payuni/crypto.go) / [notify.go](file:///e:/go-project/src/gopay/payuni/notify.go) / [constants.go](file:///e:/go-project/src/gopay/payuni/constants.go)

### PAYUNi 配置

`payuni.Config` 定义于 [client.go](file:///e:/go-project/src/gopay/payuni/client.go#L32)：

| 字段 | 类型 | 必传 | 说明 |
| --- | --- | --- | --- |
| `MerID` | `string` | 是 | 商店代號。 |
| `HashKey` | `string` | 是 | 串接 Hash Key（AES-256 密钥），必须 **32 字节**。 |
| `HashIV` | `string` | 是 | 串接 Hash IV（GCM nonce），必须 **16 字节**。 |
| `Sandbox` | `bool` | 否 | `true` 使用沙箱环境，`false`（默认）使用正式环境。 |
| `Timeout` | `time.Duration` | 否 | HTTP 请求超时，为 0 时使用默认 30 秒。 |
| `HTTPClient` | `*http.Client` | 否 | 自定义 HTTP 客户端，为 `nil` 时使用默认客户端。 |

构造函数：

```go
func New(cfg Config) (*Client, error)
```

校验失败返回包装了 `gopay.ErrInvalidConfig` 的错误。

加解密与签名规则（与官方 PHP/.NET SDK 一致）：

- `EncryptInfo = hex( base64(AES-256-GCM ciphertext) + ":::" + base64(GCM tag) )`
- `HashInfo = 大写 hex( sha256(HashKey + EncryptInfo + HashIV) )`
- 请求体编码模拟 PHP `http_build_query`，key 按字典序排序。

### PAYUNi CreateOrder

创建支付单（整合式支付页 `upp`），返回**自动提交表单 HTML**。

> 方法签名：`func (c *Client) CreateOrder(req *gopay.CreateOrderRequest) (*gopay.CreateOrderResponse, error)`

参数校验：

- `OutTradeNo` 必填，且需通过 `validateOutTradeNo`：长度 ≤ 25、仅字母/数字/下划线、不能全为数字或全为字母。
- `Amount` 必填。
- `PayType` 仅允许空或 `"upp"`，其他后台方式（ATM/CVS/信用卡等）请使用 [`UniversalTrade`](#payuni-universaltrade)。
- `Currency` 仅支持 `TWD`（为空默认 `TWD`）。

请求映射到 PAYUNi 字段：

| 通用字段 | PAYUNi 字段 | 备注 |
| --- | --- | --- |
| `OutTradeNo` | `MerTradeNo` | |
| `Amount` | `TradeAmt` | |
| `Currency` | `Currency` | 固定 `TWD` |
| `Subject` | `ProdDesc` | 为空不传 |
| `ReturnURL` | `ReturnURL` | 为空不传 |
| `NotifyURL` | `NotifyURL` | 为空不传 |
| —（自动） | `MerID` | 取配置 |
| —（自动） | `Timestamp` | `time.Now().Unix()` 秒 |
| `Extra` | 透传 | 合并覆盖 |

返回结果：

- `Channel = gopay.ChannelPayUni`
- `PayType = ModeUPP`(`"upp"`)
- `ResultType = gopay.ResultTypeHTML`
- `FormHTML` 为自动提交表单，直接 `w.Write([]byte(resp.FormHTML))` 输出给浏览器即可。

### PAYUNi QueryOrder

查询订单（`trade/query`），接口版本固定 `2.0`。

> 方法签名：`func (c *Client) QueryOrder(req *gopay.QueryOrderRequest) (*gopay.QueryOrderResponse, error)`

参数校验：`OutTradeNo` 与 `TradeNo` 至少传一个。

请求映射：

| 通用字段 | PAYUNi 字段 |
| --- | --- |
| `OutTradeNo` | `MerTradeNo` |
| `TradeNo` | `TradeNo` |
| `Extra` | 透传 |

响应解析：PAYUNi 交易查詢返回 `Result[0][字段]`、`Result[1][字段]` 形式的嵌套数组（PHP `http_build_query` 风格），SDK 取**第一笔**记录填充 `OutTradeNo`/`TradeNo`/`TradeState`/`Amount`。

### PAYUNi RefundOrder

退款（`trade/close`），接口版本 `1.0`，默认 `CloseType=2`（退款）。

> 方法签名：`func (c *Client) RefundOrder(req *gopay.RefundOrderRequest) (*gopay.RefundOrderResponse, error)`

参数校验：`TradeNo` 必填。

请求映射：

| 通用字段 | PAYUNi 字段 | 备注 |
| --- | --- | --- |
| `TradeNo` | `TradeNo` | 必填 |
| `Amount` | `TradeAmt` | 为空不传（由渠道决定是否全额退款） |
| `CloseType` | `CloseType` | 为空默认 `CloseTypeRefund`(`"2"`) |
| `Extra` | 透传 | |

返回 `Message` 取自解密后的 `Message` 字段。

### PAYUNi UniversalTrade

底层通用交易调用，映射到 PAYUNi 任意接口（对应官方 `UniversalTrade`）。

> 方法签名：
> ```go
> func (c *Client) UniversalTrade(params map[string]string, mode, version string) (map[string]string, string, error)
> ```

参数说明：

| 参数 | 类型 | 必传 | 说明 |
| --- | --- | --- | --- |
| `params` | `map[string]string` | 是 | EncryptInfo 的业务字段。`MerID`/`Timestamp` 未提供时自动补全。 |
| `mode` | `string` | 是 | 接口路径，见 [PAYUNi 常量](#payuni-常量)（如 `ModeATM`/`ModeCVS`/`ModeCredit` 等）。 |
| `version` | `string` | 是 | API 版本号（如 `"1.0"`/`"2.0"`）。 |

返回：解密后的业务字段 `map` 与原始响应报文。

适用场景：调用 `CreateOrder` 未覆盖的后台接口，如 ATM、超商代碼、信用卡幕後、LINE Pay、AFTEE、交易取消、信用卡 Token 查詢/取消、各渠道专属退款等。

### PAYUNi VerifyNotify

校验并解析 PAYUNi 异步通知/同步回传数据。

> 方法签名：`func (c *Client) VerifyNotify(rawData []byte) (*gopay.NotifyResult, error)`

`rawData` 为回调原始 `application/x-www-form-urlencoded` 或 query string 字节。

处理流程：

1. 解析 `Status`/`Message`/`EncryptInfo`/`HashInfo`。
2. 无 `EncryptInfo` 且有 `Status`：当作纯状态通知直接返回。
3. 校验 `HashInfo == hash(EncryptInfo)`，失败返回 `gopay.ErrVerifySignature`。
4. AES-256-GCM 解密 `EncryptInfo`，解析字段。
5. 填充 `NotifyResult`：`TradeNo`、`MerTradeNo`→`OutTradeNo`、`TradeAmt`→`Amount`、`TradeStatus`→`TradeState`、`PaymentType`→`PayType`、`PayTime`，`Currency` 固定 `TWD`，`Paid = (TradeState == TradeStatePaid)`。
6. `PayTime` 为空时填当前时间。

### PAYUNi 常量

定义于 [constants.go](file:///e:/go-project/src/gopay/payuni/constants.go)。

#### 环境地址

| 常量 | 值 |
| --- | --- |
| `ProductionBaseURL` | `https://api.payuni.com.tw/api/` |
| `SandboxBaseURL` | `https://sandbox-api.payuni.com.tw/api/` |

#### 接口路径 Mode

| 常量 | 值 | 说明 |
| --- | --- | --- |
| `ModeUPP` | `upp` | 整合式支付页（幕前） |
| `ModeATM` | `atm` | 虚拟账号幕後 |
| `ModeCVS` | `cvs` | 超商代碼幕後 |
| `ModeCredit` | `credit` | 信用卡幕後 |
| `ModeLinePay` | `linepay` | LINE Pay 幕後 |
| `ModeAfteeDirect` | `aftee_direct` | AFTEE 幕後 |
| `ModeTradeQuery` | `trade/query` | 交易查詢 |
| `ModeTradeClose` | `trade/close` | 交易請退款 |
| `ModeTradeCancel` | `trade/cancel` | 交易取消授權 |
| `ModeCancelCVS` | `cancel_cvs` | 交易取消超商代碼 |
| `ModeCreditBindQuery` | `credit_bind/query` | 信用卡 Token 查詢 |
| `ModeCreditBindCancel` | `credit_bind/cancel` | 信用卡 Token 取消 |
| `ModeTradeRefundICash` | `trade/common/refund/icash` | 愛金卡退款 |
| `ModeTradeRefundAftee` | `trade/common/refund/aftee` | 後支付退款 |
| `ModeTradeRefundLinePay` | `trade/common/refund/linepay` | LINE Pay 退款 |
| `ModeTradeConfirmAftee` | `trade/common/confirm/aftee` | 後支付確認 |

#### 交易状态 TradeStatus（PAYUNi 定义）

| 常量 | 值 | 含义 | 映射统一状态 |
| --- | --- | --- | --- |
| `TradeStatusIssued` | `"0"` | 取號成功 | `TradeStateUnpaid` |
| `TradeStatusUnpaid` | `"9"` | 未付款 | `TradeStateUnpaid` |
| `TradeStatusPaid` | `"1"` | 已付款 | `TradeStatePaid` |
| `TradeStatusFailed` | `"2"` | 付款失敗 | `TradeStateFailed` |
| `TradeStatusCanceled` | `"3"` | 付款取消 | `TradeStateCanceled` |
| `TradeStatusExpired` | `"4"` | 交易逾期 | `TradeStateCanceled` |
| `TradeStatusPending` | `"8"` | 訂單待確認 | `TradeStateUnknown` |

#### 关账类型 CloseType

| 常量 | 值 | 说明 |
| --- | --- | --- |
| `CloseTypeRefund` | `"2"` | 退款（`RefundOrder` 默认值） |

#### 应答与版本

| 常量 | 值 | 说明 |
| --- | --- | --- |
| `NotifySuccessAck` | `"OK"` | 回调成功应答字符串（`NotifyAck()` 返回值） |
| `defaultVersion` | `"1.0"` | 默认接口版本（未导出） |
| `queryVersion` | `"2.0"` | 交易查詢接口版本（未导出） |

### PAYUNi 完整调用案例

```go
package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	gopay "github.com/biubug/gopay"
	"github.com/biubug/gopay/payuni"
)

func main() {
	// 1. 创建 PAYUNi 客户端（Sandbox=true 使用测试环境）
	client, err := payuni.New(payuni.Config{
		MerID:   "U02982161",                            // 商店代號
		HashKey: "12345678901234567890123456789012",     // Hash Key（32 字节）
		HashIV:  "1234567890123456",                    // Hash IV（16 字节）
		Sandbox: true,
	})
	if err != nil {
		panic(err)
	}

	// 上层只依赖统一接口
	var pc gopay.PaymentClient = client

	// 2. 创建支付单（整合式支付页，返回自动提交表单 HTML）
	resp, err := pc.CreateOrder(&gopay.CreateOrderRequest{
		OutTradeNo: "ORDER202608190001", // 必填，≤25 位，字母+数字混合
		Amount:     "100",              // 必填
		Currency:   "TWD",              // 可选，默认 TWD
		Subject:    "測試商品",          // 可选
		ReturnURL:  "https://your.site/api/payuni/return",
		NotifyURL:  "https://your.site/api/payuni/notify",
	})
	if err != nil {
		panic(err)
	}
	fmt.Printf("ResultType=%s\n", resp.ResultType) // html
	// resp.FormHTML 直接输出给浏览器，会自动跳转 PAYUNi 支付页

	// 3. 查询订单（按商户订单号）
	q, err := pc.QueryOrder(&gopay.QueryOrderRequest{
		OutTradeNo: "ORDER202608190001",
	})
	if err != nil {
		panic(err)
	}
	fmt.Printf("TradeState=%s Amount=%s TradeNo=%s\n", q.TradeState, q.Amount, q.TradeNo)

	// 查询订单（按平台交易号）
	q2, err := pc.QueryOrder(&gopay.QueryOrderRequest{
		TradeNo: "P202608190001",
	})
	if err != nil {
		panic(err)
	}
	_ = q2

	// 4. 退款（按平台交易号，默认 CloseType=2 退款）
	rf, err := pc.RefundOrder(&gopay.RefundOrderRequest{
		TradeNo: "P202608190001",
		Amount:  "100", // 可选，部分退款时必传
	})
	if err != nil {
		panic(err)
	}
	fmt.Printf("Refund Success=%v Message=%s\n", rf.Success, rf.Message)

	// 5. 调用 CreateOrder 未覆盖的接口（如 ATM 虚拟账号）
	decrypted, rawBody, err := client.UniversalTrade(
		map[string]string{
			"MerTradeNo": "ORDER202608190002",
			"TradeAmt":   "200",
		},
		payuni.ModeATM, // 接口路径
		"1.0",          // API 版本
	)
	if err != nil {
		panic(err)
	}
	fmt.Printf("ATM result: %+v\nraw=%s\n", decrypted, rawBody)
}

// 异步通知 HTTP 处理器
func payuniNotifyHandler(pc gopay.PaymentClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// PAYUNi 通知：GET（同步回传）用 RawQuery，POST（异步通知）用表单 body
		var raw []byte
		if r.Method == http.MethodGet {
			raw = []byte(r.URL.RawQuery)
		} else {
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				w.WriteHeader(http.StatusOK)
				return
			}
			raw = body
		}

		result, err := pc.VerifyNotify(raw)
		if err != nil {
			if errors.Is(err, gopay.ErrVerifySignature) {
				// 签名校验失败
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		if result.Paid {
			// 更新订单为已付款（result.OutTradeNo / result.TradeNo / result.Amount）
		}
		// 校验通过并处理业务后回传应答
		w.Write([]byte(pc.NotifyAck())) // "OK"
	}
}
```

---

## JKO Pay 渠道

> 包路径：`github.com/biubug/gopay/jkos`
> 实现文件：[client.go](file:///e:/go-project/src/gopay/jkos/client.go) / [notify.go](file:///e:/go-project/src/gopay/jkos/notify.go) / [constants.go](file:///e:/go-project/src/gopay/jkos/constants.go)

### JKO Pay 配置

`jkos.Config` 定义于 [client.go](file:///e:/go-project/src/gopay/jkos/client.go#L36)：

| 字段 | 类型 | 必传 | 说明 |
| --- | --- | --- | --- |
| `StoreID` | `string` | 是 | 特店編號（商店编号）。 |
| `APIKey` | `string` | 是 | 串接金鑰（API Key），通过 `Api-Key` 请求头发送。 |
| `SecretKey` | `string` | 是 | 签名密钥（HMAC-SHA256 密钥），用于计算 `digest` 头。 |
| `Sandbox` | `bool` | 否 | `true` 使用沙箱环境，`false`（默认）使用正式环境。 |
| `Timeout` | `time.Duration` | 否 | HTTP 请求超时，为 0 时使用默认 30 秒。 |
| `HTTPClient` | `*http.Client` | 否 | 自定义 HTTP 客户端，为 `nil` 时使用默认客户端。 |

构造函数：

```go
func New(cfg Config) (*Client, error)
```

校验失败返回包装了 `gopay.ErrInvalidConfig` 的错误。

签名规则（与官方 PHP SDK 一致）：

- `digest = lowercase hex( HMAC-SHA256(payload, SecretKey) )`
- POST 请求：`payload` 为 JSON 请求体（`json_encode` 禁用 HTML 转义，不追加末尾换行）
- GET 请求：`payload` 为 query string
- HTTP 头：`Api-Key: <APIKey>`、`digest: <签名>`、`Content-Type: application/json`

### JKO Pay CreateOrder

创建支付单（`platform/entry`），返回**支付跳转地址**。

> 方法签名：`func (c *Client) CreateOrder(req *gopay.CreateOrderRequest) (*gopay.CreateOrderResponse, error)`

参数校验：

- `OutTradeNo` 必填。
- `Amount` 必填，会被 `parseAmount` 转为整数（截断小数）。
- `Currency` 仅支持 `TWD`（为空默认 `TWD`）。

请求映射到 JKO Pay 字段：

| 通用字段 | JKO Pay 字段 | 备注 |
| --- | --- | --- |
| `OutTradeNo` | `platform_order_id` | |
| `Amount` | `total_price` 和 `final_price` | 同值，整数 |
| `Currency` | `currency` | 固定 `TWD` |
| `NotifyURL` | `result_url` | 异步通知地址 |
| `ReturnURL` | `result_display_url` | 同步跳转地址 |
| `Subject` | — | JKO Pay 未使用，忽略 |
| —（自动） | `store_id` | 取配置 |
| —（自动） | `unredeem` | 固定 `0` |
| `Extra` | 透传 | 合并覆盖 |

返回结果：

- `Channel = gopay.ChannelJkos`
- `ResultType = gopay.ResultTypeURL`
- `RedirectURL` 为 JKO Pay 返回的 `payment_url`，前端跳转即可。
- `RawBody` 为原始 JSON 响应。

### JKO Pay QueryOrder

查询订单状态（`platform/inquiry`），GET 请求。

> 方法签名：`func (c *Client) QueryOrder(req *gopay.QueryOrderRequest) (*gopay.QueryOrderResponse, error)`

参数校验：`OutTradeNo` 必填（JKO Pay 不支持按 `TradeNo` 查询）。

请求参数：

| 通用字段 | JKO Pay 参数 | 备注 |
| --- | --- | --- |
| `OutTradeNo` | `platform_order_ids` | query string |

响应解析：取 `result_object.transactions[0]` 填充：

| 通用响应字段 | JKO Pay 字段 |
| --- | --- |
| `OutTradeNo` | `platform_order_id` |
| `TradeNo` | `tradeNo` |
| `TradeState` | `status` 经 `mapTradeState` 映射 |
| `Amount` | `final_price`（转字符串） |

### JKO Pay RefundOrder

退款（`platform/refund`），按商户订单号退款。

> 方法签名：`func (c *Client) RefundOrder(req *gopay.RefundOrderRequest) (*gopay.RefundOrderResponse, error)`

参数校验：

- `OutTradeNo` 必填。
- `Amount` 必填，会被 `parseAmount` 转为整数。

请求映射：

| 通用字段 | JKO Pay 字段 |
| --- | --- |
| `OutTradeNo` | `platform_order_id` |
| `Amount` | `refund_amount` |
| `Extra` | 透传 |

返回 `Message` 为 `result` 代码对应的中文描述（见 [result 代码](#jkos-result-代码)）。

### JKO Pay VerifyNotify

解析 JKO Pay 异步通知（Result URL Callback）。

> 方法签名：`func (c *Client) VerifyNotify(rawData []byte) (*gopay.NotifyResult, error)`

`rawData` 为回调原始 JSON 请求体。

**安全提示**：JKO Pay 回调**不携带签名**，安全校验依赖 IP 白名单（由调用方在 HTTP 处理层实现）。

通知结构：

```json
{
  "transaction": {
    "platform_order_id": "ORDER202608190001",
    "status": 0,
    "tradeNo": "2208250001",
    "trans_time": "2022-08-25 15:17:27",
    "final_price": 100,
    "channel_type": "account"
  }
}
```

字段映射：

| NotifyResult 字段 | JKO Pay 字段 |
| --- | --- |
| `OutTradeNo` | `transaction.platform_order_id` |
| `TradeNo` | `transaction.tradeNo` |
| `Amount` | `transaction.final_price`（转字符串） |
| `Currency` | 固定 `TWD` |
| `PayType` | `transaction.channel_type` |
| `PayTime` | `transaction.trans_time`（为空填当前时间） |
| `TradeState` | `transaction.status` 经 `mapTradeState` 映射 |
| `Status` | `transaction.status`（转字符串） |
| `Message` | `status` 代码对应描述 |
| `Paid` | `TradeState == TradeStatePaid` |

校验：缺失 `transaction.platform_order_id` 返回 `gopay.ErrVerifySignature`；JSON 解析失败返回包装错误。

### JKO Pay 常量

定义于 [constants.go](file:///e:/go-project/src/gopay/jkos/constants.go)。

#### 环境地址

| 常量 | 值 |
| --- | --- |
| `ProductionBaseURL` | `https://onlinepay.jkopay.com/` |
| `SandboxBaseURL` | `https://uat-onlinepay.jkopay.app/` |

> 基础地址必须以斜杠结尾，与官方 SDK 一致。

#### 接口路径 Mode

| 常量 | 值 | 说明 |
| --- | --- | --- |
| `ModeEntry` | `platform/entry` | 下单 |
| `ModeInquiry` | `platform/inquiry` | 查询订单 |
| `ModeRefund` | `platform/refund` | 退款 |

#### result 代码

| 常量/值 | 说明 |
| --- | --- |
| `ResultSuccess` = `"000"` | 成功 |
| `"100"` | 訂單不存在 |
| `"101"` | 此訂單號已付款 |
| `"103"` | 退款金額錯誤 |
| `"105"` | `remain_amount` 或 `refund_amount` 金額不正確 |
| `"108"` | 店家收款額度已達上限或用戶交易已達限額 |
| `"113"` | 退款金額大於店家累計未請款金額 |
| `"200"` / `"201"` | 參數錯誤 |
| `"922"` | 退款總金額超過原訂單金額 |
| `"999"` | 其他錯誤 |

#### 交易状态 status（JKO Pay 定义，数值类型）

| 常量 | 值 | 含义 | 映射统一状态 |
| --- | --- | --- | --- |
| `TradeStatusPaid` | `0` | 交易成功 | `TradeStatePaid` |
| `TradeStatusProcessing` | `1` | 付款處理中 | `TradeStateUnknown` |
| `TradeStatusUnpaid` | `101` | 此訂單號尚未付款 | `TradeStateUnpaid` |
| `TradeStatusNotExist` | `102` | 此訂單號不存在 | `TradeStateUnknown` |

#### 应答

| 常量 | 值 | 说明 |
| --- | --- | --- |
| `NotifySuccessAck` | `"OK"` | 回调成功应答字符串（`NotifyAck()` 返回值） |

### JKO Pay 完整调用案例

```go
package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	gopay "github.com/biubug/gopay"
	"github.com/biubug/gopay/jkos"
)

func main() {
	// 1. 创建 JKO Pay 客户端（Sandbox=true 使用测试环境）
	client, err := jkos.New(jkos.Config{
		StoreID:   "your_store_id",
		APIKey:    "your_api_key",
		SecretKey: "your_secret_key",
		Sandbox:   true,
	})
	if err != nil {
		panic(err)
	}

	var pc gopay.PaymentClient = client

	// 2. 创建支付单（返回支付跳转地址）
	resp, err := pc.CreateOrder(&gopay.CreateOrderRequest{
		OutTradeNo: "ORDER202608190001", // 必填
		Amount:     "100",              // 必填，会转整数
		Currency:   "TWD",              // 可选，默认 TWD
		NotifyURL:  "https://your.site/api/jkos/notify", // result_url
		ReturnURL:  "https://your.site/api/jkos/return", // result_display_url
	})
	if err != nil {
		panic(err)
	}
	fmt.Printf("RedirectURL=%s\n", resp.RedirectURL)
	// 前端跳转到 resp.RedirectURL 完成支付

	// 3. 查询订单（必须按商户订单号）
	q, err := pc.QueryOrder(&gopay.QueryOrderRequest{
		OutTradeNo: "ORDER202608190001",
	})
	if err != nil {
		panic(err)
	}
	fmt.Printf("TradeState=%s Amount=%s TradeNo=%s\n", q.TradeState, q.Amount, q.TradeNo)

	// 4. 退款（按商户订单号）
	rf, err := pc.RefundOrder(&gopay.RefundOrderRequest{
		OutTradeNo: "ORDER202608190001", // 必填
		Amount:     "100",              // 必填
	})
	if err != nil {
		panic(err)
	}
	fmt.Printf("Refund Success=%v Message=%s\n", rf.Success, rf.Message)
}

// 异步通知 HTTP 处理器
func jkosNotifyHandler(pc gopay.PaymentClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// JKO Pay 通知为 JSON body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusOK)
			return
		}

		// 建议在此校验来源 IP（白名单），回调本身无签名
		result, err := pc.VerifyNotify(body)
		if err != nil {
			if errors.Is(err, gopay.ErrVerifySignature) {
				// 缺失关键字段或解析失败
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		if result.Paid {
			// 更新订单为已付款（result.OutTradeNo / result.TradeNo / result.Amount）
		}
		// 处理完业务后回传应答
		w.Write([]byte(pc.NotifyAck())) // "OK"
	}
}
```

---

## 异步通知处理通用模式

两个渠道的回调处理逻辑统一遵循：**读原始数据 → `VerifyNotify` → 业务处理 → `NotifyAck`**。

| 渠道 | 通知数据格式 | 原始数据读取方式 | 签名校验 | 应答字符串 |
| --- | --- | --- | --- | --- |
| PAYUNi | `application/x-www-form-urlencoded`（POST）或 query string（GET 同步回传） | POST 用 body；GET 用 `r.URL.RawQuery` | HashInfo + AES 解密 | `NotifyAck()` → `"OK"` |
| JKO Pay | JSON body（POST） | `io.ReadAll(r.Body)` | 无签名，依赖 IP 白名单 | `NotifyAck()` → `"OK"` |

通用回调处理骨架：

```go
func notifyHandler(pc gopay.PaymentClient, isPayuni bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var raw []byte
		if isPayuni && r.Method == http.MethodGet {
			raw = []byte(r.URL.RawQuery)
		} else {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusOK)
				return
			}
			raw = body
		}

		result, err := pc.VerifyNotify(raw)
		if err != nil {
			w.WriteHeader(http.StatusOK)
			return
		}
		if result.Paid {
			// TODO: 更新订单为已付款
			// result.OutTradeNo / result.TradeNo / result.Amount / result.PayTime
		}
		w.Write([]byte(pc.NotifyAck()))
	}
}
```

---

## 错误处理

所有自定义错误均支持 `errors.Is` 判断：

```go
resp, err := pc.CreateOrder(req)
switch {
case errors.Is(err, gopay.ErrInvalidConfig):
    // 配置错误（如 HashKey 长度不对）
case errors.Is(err, gopay.ErrInvalidRequest):
    // 请求参数错误（如 OutTradeNo 为空、币种不支持）
case errors.Is(err, gopay.ErrVerifySignature):
    // 签名校验失败（响应/回调 HashInfo 不匹配）
case errors.Is(err, gopay.ErrAPIResponse):
    // 上游返回错误（Status=ERROR、result!=000）
case err != nil:
    // 其他错误（网络、JSON 解析等）
}
```

错误信息格式示例：

- 配置：`gopay: invalid config: HashKey must be 32 bytes (got 31)`
- 请求：`gopay: invalid request: OutTradeNo exceeds 25 characters (got 26)`
- 请求：`gopay: invalid request: OutTradeNo must not be all digits`
- 签名：`gopay: signature verification failed: hash mismatch`
- 上游：`gopay: upstream api returned an error: status=ERROR message=...`

---

## 测试

```bash
go test ./...
```

测试覆盖：

- **PAYUNi**：与 OpenSSL/PHP 的跨语言黄金向量校验，确保 AES-256-GCM 加解密与 HashInfo 字节一致。
- **JKO Pay**：HMAC-SHA256 签名黄金向量（POST/GET）、JSON 序列化（禁用 HTML 转义）、配置校验、回调解析、状态映射、参数校验等。

---

## License

[MIT](../LICENSE)
