# gopay

Go 聚合支付 SDK，通过统一的 `PaymentClient` 接口屏蔽不同支付渠道的差异。

目前实现渠道：

- [x] PAYUNi 統一金流（整合式支付页 upp / 交易查询 / 退款 / 通用交易）
- [x] JKO Pay 街口支付（下单 / 查询 / 退款 / 异步通知）
- [ ] 后续渠道（结构已预留，新增渠道无需改动上层调用代码）

> 详细 API 文档见 [docs/API.md](./docs/API.md)（含全部参数表、必传说明、调用案例）。

## 特性

- 统一抽象接口 `PaymentClient`，上层只依赖接口，切换/新增渠道零改动。
- PAYUNi 加解密与签名严格对齐官方 SDK：
  - `EncryptInfo = hex( base64(AES-256-GCM ciphertext) + ":::" + base64(GCM tag) )`
  - `HashInfo = 大写 hex( sha256(HashKey + EncryptInfo + HashIV) )`
- JKO Pay 签名与官方 PHP SDK 一致：`digest = lowercase hex( HMAC-SHA256(payload, SecretKey) )`。
- 仅依赖 Go 标准库，零第三方依赖。
- 金额使用字符串，避免浮点精度问题。

## 安装

> 模块路径：`github.com/biubug/gopay`。

```bash
go get github.com/biubug/gopay
```

## 快速开始

```go
package main

import (
	"fmt"
	"io"
	"net/http"

	gopay "github.com/biubug/gopay"
	"github.com/biubug/gopay/payuni"
)

func main() {
	// 创建 PAYUNi 客户端。Sandbox=true 使用测试环境。
	client, err := payuni.New(payuni.Config{
		MerID:   "ABC",                                 // 商店代號
		HashKey: "12345678901234567890123456789012",    // Hash Key（32 字节）
		HashIV:  "1234567890123456",                    // Hash IV（16 字节）
		Sandbox: true,
	})
	if err != nil {
		panic(err)
	}

	// 将具体客户端赋给统一接口，上层只依赖 gopay.PaymentClient。
	var pc gopay.PaymentClient = client

	// 1. 创建支付单（整合式支付页，返回自动提交表单 HTML）
	resp, err := pc.CreateOrder(&gopay.CreateOrderRequest{
		OutTradeNo: "ORDER202608180001",
		Amount:     "100",
		Subject:    "測試商品",
		ReturnURL:  "https://your.site/api/payuni/return",
		NotifyURL:  "https://your.site/api/payuni/notify",
	})
	if err != nil {
		panic(err)
	}
	// 将 resp.FormHTML 直接输出给浏览器，用户会被自动导向 PAYUNi 支付页。
	_ = resp.FormHTML

	// 2. 查询订单
	q, err := pc.QueryOrder(&gopay.QueryOrderRequest{OutTradeNo: "ORDER202608180001"})
	if err != nil {
		panic(err)
	}
	fmt.Printf("交易状态: %s\n", q.TradeState) // PAID / UNPAID / FAILED ...

	// 3. 退款（trade/close，默认 CloseType=2）
	_, _ = pc.RefundOrder(&gopay.RefundOrderRequest{TradeNo: "平台交易号"})
}

// 异步通知处理：校验签名并解析。
func notifyHandler(pc gopay.PaymentClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 读取原始通知数据：GET（同步回传）用 query string，POST（异步通知）用表单 body。
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
			// 校验失败，回 200 且不处理业务。
			w.WriteHeader(http.StatusOK)
			return
		}
		if result.Paid {
			// 更新订单为已付款...
		}
		// 校验通过后回传应答。
		w.Write([]byte(pc.NotifyAck())) // "OK"
	}
}
```

### JKO Pay 示例

```go
package main

import (
	"fmt"

	gopay "github.com/biubug/gopay"
	"github.com/biubug/gopay/jkos"
)

func main() {
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

	// 创建支付单（返回支付跳转地址）
	resp, err := pc.CreateOrder(&gopay.CreateOrderRequest{
		OutTradeNo: "ORDER202608190001",
		Amount:     "100",
		NotifyURL:  "https://your.site/api/jkos/notify",
		ReturnURL:  "https://your.site/api/jkos/return",
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(resp.RedirectURL) // 前端跳转此 URL 完成支付

	// 查询订单（必须按商户订单号）
	q, err := pc.QueryOrder(&gopay.QueryOrderRequest{OutTradeNo: "ORDER202608190001"})
	if err != nil {
		panic(err)
	}
	fmt.Printf("交易状态: %s\n", q.TradeState)

	// 退款（按商户订单号）
	_, err = pc.RefundOrder(&gopay.RefundOrderRequest{
		OutTradeNo: "ORDER202608190001",
		Amount:     "100",
	})
	if err != nil {
		panic(err)
	}
}
```

> JKO Pay 异步通知为 JSON body 且不携带签名，安全校验依赖 IP 白名单（由调用方在 HTTP 层实现）。详细处理方式见 [docs/API.md](./docs/API.md#异步通知处理通用模式)。

## 目录结构

```
gopay/
├── types.go          # PaymentClient 接口 + 通用请求/响应 + 统一状态
├── errors.go         # 统一错误
├── docs/
│   └── API.md        # 详细 API 文档（参数表 / 必传 / 调用案例）
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

## 新增支付渠道

1. 在 `gopay` 下新建子包（如 `gopay/newebpay`）。
2. 实现 `gopay.PaymentClient` 接口的 5 个方法（`CreateOrder` / `QueryOrder` / `RefundOrder` / `VerifyNotify` / `NotifyAck`）。
3. 新增 `newebpay.New(cfg)` 构造器。

上层代码仅需把构造器从 `payuni.New(...)` 换成 `newebpay.New(...)`，其余逻辑不涉及。

## 通用接口

```go
type PaymentClient interface {
	CreateOrder(req *CreateOrderRequest) (*CreateOrderResponse, error)
	QueryOrder(req *QueryOrderRequest) (*QueryOrderResponse, error)
	RefundOrder(req *RefundOrderRequest) (*RefundOrderResponse, error)
	VerifyNotify(rawData []byte) (*NotifyResult, error)
	NotifyAck() string
}
```

说明：

- `CreateOrderResponse.ResultType` 区分结果：`url`（跳转地址）/ `html`（自动提交表单）/ `raw`（后台类交易原始数据）。
- `TradeState` 为跨渠道统一状态：`UNPAID / PAID / FAILED / CANCELED / REFUNDED / UNKNOWN`。
- `VerifyNotify` 校验失败时返回 `gopay.ErrVerifySignature`（可通过 `errors.Is` 判断）。

## PAYUNi 高级用法

若需调用 `CreateOrder` 未覆盖的接口（如 ATM / 超商代碼 / LINE Pay / AFTEE 幕後等），
可使用底层通用方法：

```go
decrypted, rawBody, err := client.UniversalTrade(
	map[string]string{"MerTradeNo": "ORDER202608180001", "TradeAmt": "100"},
	payuni.ModeATM, // 接口路径，见 payuni.Mode* 常量
	"1.0",
)
```

## 测试

```bash
go test ./...
```

测试覆盖：

- PAYUNi：与 OpenSSL/PHP 的跨语言黄金向量校验，确保 AES-256-GCM 加解密与 HashInfo 字节一致。
- JKO Pay：HMAC-SHA256 签名黄金向量（POST/GET）、JSON 序列化（禁用 HTML 转义）、配置校验、回调解析、状态映射等。

## License

[MIT](./LICENSE)