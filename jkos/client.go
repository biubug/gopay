// Package jkos 实现街口支付（JKO Pay）支付渠道，满足 gopay.PaymentClient 接口。
//
// 签名算法与官方 PHP SDK 一致：
//   - digest = lowercase hex( HMAC-SHA256(payload, secretKey) )
//   - POST 请求 payload 为 JSON 请求体
//   - GET  请求 payload 为 query string
package jkos

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	gopay "github.com/biubug/gopay"
)

// 编译期断言：*Client 实现了 gopay.PaymentClient。
var _ gopay.PaymentClient = (*Client)(nil)

const (
	defaultTimeout   = 30 * time.Second
	defaultUserAgent = "gopay-sdk/1.0"
)

// Config JKO Pay 客户端配置。
type Config struct {
	// StoreID 特店編號（商店编号）。
	StoreID string
	// APIKey 串接金鑰（API Key）。
	APIKey string
	// SecretKey 签名密钥（HMAC-SHA256 密钥）。
	SecretKey string
	// Sandbox 是否使用测试环境。
	Sandbox bool
	// Timeout HTTP 请求超时，为 0 时使用默认 30 秒。
	Timeout time.Duration
	// HTTPClient 自定义 HTTP 客户端，为 nil 时使用默认客户端。
	HTTPClient *http.Client
}

// Client JKO Pay 客户端。
type Client struct {
	storeID   string
	apiKey    string
	secretKey []byte
	baseURL   string
	http      *http.Client
}

// New 创建 JKO Pay 客户端。
func New(cfg Config) (*Client, error) {
	cfg.StoreID = strings.TrimSpace(cfg.StoreID)
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	cfg.SecretKey = strings.TrimSpace(cfg.SecretKey)

	if cfg.StoreID == "" {
		return nil, fmt.Errorf("%w: StoreID is empty", gopay.ErrInvalidConfig)
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("%w: APIKey is empty", gopay.ErrInvalidConfig)
	}
	if cfg.SecretKey == "" {
		return nil, fmt.Errorf("%w: SecretKey is empty", gopay.ErrInvalidConfig)
	}

	baseURL := ProductionBaseURL
	if cfg.Sandbox {
		baseURL = SandboxBaseURL
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		to := cfg.Timeout
		if to <= 0 {
			to = defaultTimeout
		}
		httpClient = &http.Client{Timeout: to}
	}

	return &Client{
		storeID:   cfg.StoreID,
		apiKey:    cfg.APIKey,
		secretKey: []byte(cfg.SecretKey),
		baseURL:   baseURL,
		http:      httpClient,
	}, nil
}

// CreateOrder 创建支付单（platform/entry），返回支付跳转地址。
func (c *Client) CreateOrder(req *gopay.CreateOrderRequest) (*gopay.CreateOrderResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: CreateOrderRequest is nil", gopay.ErrInvalidRequest)
	}
	if req.OutTradeNo == "" {
		return nil, fmt.Errorf("%w: OutTradeNo is required", gopay.ErrInvalidRequest)
	}
	if req.Amount == "" {
		return nil, fmt.Errorf("%w: Amount is required", gopay.ErrInvalidRequest)
	}

	// JKO Pay 仅支持 TWD。
	currency := strings.ToUpper(req.Currency)
	if currency == "" {
		currency = "TWD"
	} else if currency != "TWD" {
		return nil, fmt.Errorf("%w: Currency %q is not supported by JKO Pay (only TWD)", gopay.ErrInvalidRequest, req.Currency)
	}

	amount, err := parseAmount(req.Amount)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid Amount %q: %v", gopay.ErrInvalidRequest, req.Amount, err)
	}

	body := map[string]interface{}{
		"store_id":          c.storeID,
		"platform_order_id": req.OutTradeNo,
		"total_price":       amount,
		"final_price":       amount,
		"unredeem":          0,
		"currency":          currency,
	}
	// result_url 为异步通知地址。
	if req.NotifyURL != "" {
		body["result_url"] = req.NotifyURL
	}
	// result_display_url 为支付完成后浏览器跳转地址。
	if req.ReturnURL != "" {
		body["result_display_url"] = req.ReturnURL
	}
	// Subject 作为 result_display_url 不适用，忽略。
	mergeExtra(body, req.Extra)

	rawBody, err := c.doPost(context.Background(), ModeEntry, body)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Result  string `json:"result"`
		Message string `json:"message"`
		ResultObject struct {
			PaymentURL string `json:"payment_url"`
		} `json:"result_object"`
	}
	if err := json.Unmarshal([]byte(rawBody), &resp); err != nil {
		return nil, fmt.Errorf("jkos: decode response: %w", err)
	}
	if resp.Result != ResultSuccess {
		return nil, fmt.Errorf("%w: result=%s message=%s", gopay.ErrAPIResponse, resp.Result, resp.Message)
	}
	if resp.ResultObject.PaymentURL == "" {
		return nil, fmt.Errorf("%w: missing payment_url in response", gopay.ErrAPIResponse)
	}

	return &gopay.CreateOrderResponse{
		Channel:     gopay.ChannelJkos,
		ResultType:  gopay.ResultTypeURL,
		RedirectURL: resp.ResultObject.PaymentURL,
		RawBody:     rawBody,
	}, nil
}

// QueryOrder 查询订单状态（platform/inquiry）。
func (c *Client) QueryOrder(req *gopay.QueryOrderRequest) (*gopay.QueryOrderResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: QueryOrderRequest is nil", gopay.ErrInvalidRequest)
	}
	if req.OutTradeNo == "" {
		return nil, fmt.Errorf("%w: OutTradeNo is required (JKO Pay does not support querying by TradeNo)", gopay.ErrInvalidRequest)
	}

	q := url.Values{}
	q.Set("platform_order_ids", req.OutTradeNo)

	rawBody, err := c.doGet(context.Background(), ModeInquiry, q)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Result  string `json:"result"`
		Message string `json:"message"`
		ResultObject struct {
			Transactions []transaction `json:"transactions"`
		} `json:"result_object"`
	}
	if err := json.Unmarshal([]byte(rawBody), &resp); err != nil {
		return nil, fmt.Errorf("jkos: decode response: %w", err)
	}
	if resp.Result != ResultSuccess {
		return nil, fmt.Errorf("%w: result=%s message=%s", gopay.ErrAPIResponse, resp.Result, resp.Message)
	}

	out := &gopay.QueryOrderResponse{
		Channel: gopay.ChannelJkos,
		RawBody: rawBody,
	}
	if len(resp.ResultObject.Transactions) > 0 {
		tx := resp.ResultObject.Transactions[0]
		out.OutTradeNo = tx.PlatformOrderID
		out.TradeNo = tx.TradeNo
		out.TradeState = mapTradeState(tx.Status)
		out.Amount = strconv.Itoa(tx.FinalPrice)
	}
	return out, nil
}

// RefundOrder 退款（platform/refund），按商户订单号退款。
func (c *Client) RefundOrder(req *gopay.RefundOrderRequest) (*gopay.RefundOrderResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: RefundOrderRequest is nil", gopay.ErrInvalidRequest)
	}
	if req.OutTradeNo == "" {
		return nil, fmt.Errorf("%w: OutTradeNo is required (JKO Pay refunds by platform_order_id)", gopay.ErrInvalidRequest)
	}
	if req.Amount == "" {
		return nil, fmt.Errorf("%w: Amount is required", gopay.ErrInvalidRequest)
	}

	amount, err := parseAmount(req.Amount)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid Amount %q: %v", gopay.ErrInvalidRequest, req.Amount, err)
	}

	body := map[string]interface{}{
		"platform_order_id": req.OutTradeNo,
		"refund_amount":     amount,
	}
	mergeExtra(body, req.Extra)

	rawBody, err := c.doPost(context.Background(), ModeRefund, body)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Result  string `json:"result"`
		Message string `json:"message"`
		ResultObject struct {
			RefundTradeNo string `json:"refund_tradeNo"`
			RefundTime    string `json:"refund_time"`
		} `json:"result_object"`
	}
	if err := json.Unmarshal([]byte(rawBody), &resp); err != nil {
		return nil, fmt.Errorf("jkos: decode response: %w", err)
	}
	if resp.Result != ResultSuccess {
		return nil, fmt.Errorf("%w: result=%s message=%s", gopay.ErrAPIResponse, resp.Result, resp.Message)
	}

	return &gopay.RefundOrderResponse{
		Channel: gopay.ChannelJkos,
		Success: true,
		Message: getResultMsg(resp.Result),
		RawBody: rawBody,
	}, nil
}

// NotifyAck 返回 JKO Pay 回调成功后的应答字符串（"OK"）。
func (c *Client) NotifyAck() string {
	return NotifySuccessAck
}

// transaction JKO Pay 交易信息。
type transaction struct {
	PlatformOrderID string `json:"platform_order_id"`
	Status          int    `json:"status"`
	TradeNo         string `json:"tradeNo"`
	TransTime       string `json:"trans_time"`
	FinalPrice      int    `json:"final_price"`
	ChannelType     string `json:"channel_type"`
}

// sign 计算 HMAC-SHA256 签名，返回小写十六进制字符串。
// payload 为请求的原始字节（POST 为 JSON body，GET 为 query string）。
func (c *Client) sign(payload []byte) string {
	mac := hmac.New(sha256.New, c.secretKey)
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// doPost 发送 JSON POST 请求并携带签名头。
func (c *Client) doPost(ctx context.Context, path string, body map[string]interface{}) (string, error) {
	payload, err := jsonMarshal(body)
	if err != nil {
		return "", fmt.Errorf("jkos: marshal request: %w", err)
	}

	endpoint := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Api-Key", c.apiKey)
	req.Header.Set("digest", c.sign(payload))
	req.Header.Set("User-Agent", defaultUserAgent)

	return c.doRequest(req, endpoint)
}

// doGet 发送 GET 请求并携带签名头。
// queryString 为已编码的 query string，签名基于该字符串计算。
func (c *Client) doGet(ctx context.Context, path string, q url.Values) (string, error) {
	queryString := q.Encode()
	endpoint := c.baseURL + path
	if queryString != "" {
		endpoint += "?" + queryString
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Api-Key", c.apiKey)
	req.Header.Set("digest", c.sign([]byte(queryString)))
	req.Header.Set("User-Agent", defaultUserAgent)

	return c.doRequest(req, endpoint)
}

// doRequest 执行 HTTP 请求并返回响应体字符串。
func (c *Client) doRequest(req *http.Request, endpoint string) (string, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("jkos: request %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("jkos: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return string(body), fmt.Errorf("jkos: unexpected http status %d: %s", resp.StatusCode, string(body))
	}
	return string(body), nil
}

// jsonMarshal 将值序列化为 JSON，禁用 HTML 转义以匹配 PHP json_encode 行为，
// 且不追加末尾换行。
func jsonMarshal(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// parseAmount 将字符串金额解析为整数。
// 支持 "100"、"100.0" 等格式。
func parseAmount(s string) (int, error) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return int(f), nil
}

// mergeExtra 将 Extra 参数合并到请求 body 中。
func mergeExtra(body map[string]interface{}, extra map[string]string) {
	for k, v := range extra {
		body[k] = v
	}
}
