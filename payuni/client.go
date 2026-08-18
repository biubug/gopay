// Package payuni 实现統一金流 PAYUNi 支付渠道，满足 gopay.PaymentClient 接口。
//
// 加解密与签名逻辑严格对齐官方 PHP/.NET SDK：
//   - EncryptInfo = hex( base64(AES-256-GCM ciphertext) + ":::" + base64(GCM tag) )
//   - HashInfo    = 大写 hex( sha256(HashKey + EncryptInfo + HashIV) )
package payuni

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
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

// Config PAYUNi 客户端配置。
type Config struct {
	// MerID 商店代號。
	MerID string
	// HashKey 串接的 Hash Key（AES-256 密钥，32 字节）。
	HashKey string
	// HashIV 串接的 Hash IV（GCM nonce，16 字节）。
	HashIV string
	// Sandbox 是否使用测试环境。
	Sandbox bool
	// Timeout HTTP 请求超时，为 0 时使用默认 30 秒。
	Timeout time.Duration
	// HTTPClient 自定义 HTTP 客户端，为 nil 时使用默认客户端。
	HTTPClient *http.Client
}

// Client PAYUNi 客户端。
type Client struct {
	merID   string
	hashKey []byte
	hashIV  []byte
	baseURL string
	http    *http.Client
}

// New 创建 PAYUNi 客户端。
func New(cfg Config) (*Client, error) {
	cfg.MerID = strings.TrimSpace(cfg.MerID)
	cfg.HashKey = strings.TrimSpace(cfg.HashKey)
	cfg.HashIV = strings.TrimSpace(cfg.HashIV)

	if cfg.MerID == "" {
		return nil, fmt.Errorf("%w: MerID is empty", gopay.ErrInvalidConfig)
	}
	if len(cfg.HashKey) != 32 {
		return nil, fmt.Errorf("%w: HashKey must be 32 bytes (got %d)", gopay.ErrInvalidConfig, len(cfg.HashKey))
	}
	if len(cfg.HashIV) != 16 {
		return nil, fmt.Errorf("%w: HashIV must be 16 bytes (got %d)", gopay.ErrInvalidConfig, len(cfg.HashIV))
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
		merID:   cfg.MerID,
		hashKey: []byte(cfg.HashKey),
		hashIV:  []byte(cfg.HashIV),
		baseURL: baseURL,
		http:    httpClient,
	}, nil
}

// CreateOrder 创建支付单（整合式支付页 upp，返回自动提交表单 HTML）。
func (c *Client) CreateOrder(req *gopay.CreateOrderRequest) (*gopay.CreateOrderResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: CreateOrderRequest is nil", gopay.ErrInvalidRequest)
	}
	if req.OutTradeNo == "" {
		return nil, fmt.Errorf("%w: OutTradeNo is required", gopay.ErrInvalidRequest)
	}
	if err := validateOutTradeNo(req.OutTradeNo); err != nil {
		return nil, err
	}
	if req.Amount == "" {
		return nil, fmt.Errorf("%w: Amount is required", gopay.ErrInvalidRequest)
	}
	if req.PayType != "" && req.PayType != ModeUPP {
		return nil, fmt.Errorf("%w: PayType %q is not supported by CreateOrder (only upp); use UniversalTrade for backend modes", gopay.ErrInvalidRequest, req.PayType)
	}

	params := map[string]string{
		"MerID":      c.merID,
		"MerTradeNo": req.OutTradeNo,
		"TradeAmt":   req.Amount,
		"Timestamp":  strconv.FormatInt(time.Now().Unix(), 10),
	}
	if req.Subject != "" {
		params["ProdDesc"] = req.Subject
	}
	if req.ReturnURL != "" {
		params["ReturnURL"] = req.ReturnURL
	}
	if req.NotifyURL != "" {
		params["NotifyURL"] = req.NotifyURL
	}
	mergeParams(params, req.Extra)

	encryptInfo, hashInfo, isPlatForm, err := c.prepareEncryptInfo(params)
	if err != nil {
		return nil, err
	}

	fields := map[string]string{
		"MerID":       c.merID,
		"Version":     defaultVersion,
		"EncryptInfo": encryptInfo,
		"HashInfo":    hashInfo,
	}
	if isPlatForm != "" {
		fields["IsPlatForm"] = isPlatForm
	}

	return &gopay.CreateOrderResponse{
		Channel:    gopay.ChannelPayUni,
		PayType:    ModeUPP,
		ResultType: gopay.ResultTypeHTML,
		FormHTML:   c.buildForm(c.baseURL+ModeUPP, fields),
	}, nil
}

// QueryOrder 查询订单（trade/query）。
func (c *Client) QueryOrder(req *gopay.QueryOrderRequest) (*gopay.QueryOrderResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: QueryOrderRequest is nil", gopay.ErrInvalidRequest)
	}
	if req.OutTradeNo == "" && req.TradeNo == "" {
		return nil, fmt.Errorf("%w: OutTradeNo or TradeNo is required", gopay.ErrInvalidRequest)
	}

	params := map[string]string{}
	if req.OutTradeNo != "" {
		params["MerTradeNo"] = req.OutTradeNo
	}
	if req.TradeNo != "" {
		params["TradeNo"] = req.TradeNo
	}
	mergeParams(params, req.Extra)

	decrypted, rawBody, err := c.universalTrade(params, ModeTradeQuery, queryVersion)
	if err != nil {
		return nil, err
	}

	resp := &gopay.QueryOrderResponse{
		Channel: gopay.ChannelPayUni,
		Raw:     decrypted,
		RawBody: rawBody,
	}
	// 交易查詢回傳統一為 Result 陣列，單筆查詢取第一筆。
	if records := parseQueryResult(decrypted); len(records) > 0 {
		resp.OutTradeNo = records[0]["MerTradeNo"]
		resp.TradeNo = records[0]["TradeNo"]
		resp.TradeState = mapTradeState(records[0]["TradeStatus"])
		resp.Amount = records[0]["TradeAmt"]
	}
	return resp, nil
}

// parseQueryResult 从解密后的扁平字段中提取 Result 数组。
// PAYUNi 交易查詢回傳使用 PHP http_build_query 的嵌套数组表示法，
// 形如 Result[0][MerTradeNo]=...、Result[1][TradeNo]=...。
func parseQueryResult(fields map[string]string) []map[string]string {
	records := map[int]map[string]string{}
	maxIdx := -1
	for k, v := range fields {
		idx, key, ok := parseResultKey(k)
		if !ok {
			continue
		}
		if records[idx] == nil {
			records[idx] = make(map[string]string)
		}
		records[idx][key] = v
		if idx > maxIdx {
			maxIdx = idx
		}
	}
	if maxIdx < 0 {
		return nil
	}
	result := make([]map[string]string, 0, maxIdx+1)
	for i := 0; i <= maxIdx; i++ {
		if records[i] != nil {
			result = append(result, records[i])
		}
	}
	return result
}

// parseResultKey 解析形如 "Result[0][MerTradeNo]" 的 key，
// 返回下标、字段名与是否解析成功。
func parseResultKey(s string) (int, string, bool) {
	rest, ok := strings.CutPrefix(s, "Result[")
	if !ok {
		return 0, "", false
	}
	closeIdx := strings.IndexByte(rest, ']')
	if closeIdx <= 0 {
		return 0, "", false
	}
	idx, err := strconv.Atoi(rest[:closeIdx])
	if err != nil {
		return 0, "", false
	}
	field, ok := strings.CutPrefix(rest[closeIdx+1:], "[")
	if !ok {
		return 0, "", false
	}
	field, ok = strings.CutSuffix(field, "]")
	if !ok {
		return 0, "", false
	}
	return idx, field, true
}

// RefundOrder 退款（trade/close，默认 CloseType=2 退款）。
func (c *Client) RefundOrder(req *gopay.RefundOrderRequest) (*gopay.RefundOrderResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: RefundOrderRequest is nil", gopay.ErrInvalidRequest)
	}
	if req.TradeNo == "" {
		return nil, fmt.Errorf("%w: TradeNo is required", gopay.ErrInvalidRequest)
	}

	closeType := req.CloseType
	if closeType == "" {
		closeType = CloseTypeRefund
	}

	params := map[string]string{
		"TradeNo":   req.TradeNo,
		"CloseType": closeType,
	}
	if req.Amount != "" {
		params["TradeAmt"] = req.Amount
	}
	mergeParams(params, req.Extra)

	decrypted, rawBody, err := c.universalTrade(params, ModeTradeClose, defaultVersion)
	if err != nil {
		return nil, err
	}

	return &gopay.RefundOrderResponse{
		Channel: gopay.ChannelPayUni,
		Success: true,
		Message: decrypted["Message"],
		Raw:     decrypted,
		RawBody: rawBody,
	}, nil
}

// UniversalTrade 通用交易调用，映射到 PAYUNi 任意接口（与官方 UniversalTrade 对应）。
//
// params 为 EncryptInfo 的业务字段（MerID/Timestamp 未提供时会自动补全），
// mode 为接口路径（见本包 Mode* 常量），version 为 API 版本号。
// 返回解密后的业务字段 map 与原始响应报文。
func (c *Client) UniversalTrade(params map[string]string, mode, version string) (map[string]string, string, error) {
	return c.universalTrade(params, mode, version)
}

// universalTrade 通用调用实现。
func (c *Client) universalTrade(params map[string]string, mode, version string) (map[string]string, string, error) {
	if params == nil {
		params = map[string]string{}
	}
	if params["MerID"] == "" {
		params["MerID"] = c.merID
	}
	if params["Timestamp"] == "" {
		params["Timestamp"] = strconv.FormatInt(time.Now().Unix(), 10)
	}

	encryptInfo, hashInfo, isPlatForm, err := c.prepareEncryptInfo(params)
	if err != nil {
		return nil, "", err
	}

	form := url.Values{
		"MerID":       {c.merID},
		"Version":     {version},
		"EncryptInfo": {encryptInfo},
		"HashInfo":    {hashInfo},
	}
	if isPlatForm != "" {
		form.Set("IsPlatForm", isPlatForm)
	}

	rawBody, err := c.doPost(context.Background(), c.baseURL+mode, form)
	if err != nil {
		return nil, rawBody, err
	}
	return c.parseResponse(rawBody)
}

// prepareEncryptInfo 生成 EncryptInfo/HashInfo，并抽出 IsPlatForm
// （代理商模式需作为外层参数而非加密字段）。
func (c *Client) prepareEncryptInfo(params map[string]string) (encryptInfo, hashInfo, isPlatForm string, err error) {
	isPlatForm = params["IsPlatForm"]

	encryptParams := params
	if isPlatForm != "" {
		encryptParams = make(map[string]string, len(params)-1)
		for k, v := range params {
			if k != "IsPlatForm" {
				encryptParams[k] = v
			}
		}
	}

	encryptInfo, err = c.encrypt(encryptParams)
	if err != nil {
		return "", "", "", err
	}
	return encryptInfo, c.hash(encryptInfo), isPlatForm, nil
}

// parseResponse 解析上游 JSON 响应，校验签名并解密。
func (c *Client) parseResponse(rawBody string) (map[string]string, string, error) {
	var resp struct {
		Status      string `json:"Status"`
		Message     string `json:"Message"`
		EncryptInfo string `json:"EncryptInfo"`
		HashInfo    string `json:"HashInfo"`
	}
	if err := json.Unmarshal([]byte(rawBody), &resp); err != nil {
		return nil, rawBody, fmt.Errorf("payuni: decode response: %w", err)
	}

	if resp.Status == "ERROR" {
		return nil, rawBody, fmt.Errorf("%w: status=ERROR message=%s", gopay.ErrAPIResponse, resp.Message)
	}
	if resp.EncryptInfo == "" {
		return nil, rawBody, fmt.Errorf("payuni: missing EncryptInfo in response")
	}
	if c.hash(resp.EncryptInfo) != resp.HashInfo {
		return nil, rawBody, fmt.Errorf("%w: hash mismatch", gopay.ErrVerifySignature)
	}

	decrypted, err := c.decrypt(resp.EncryptInfo)
	if err != nil {
		return nil, rawBody, err
	}
	return decrypted, rawBody, nil
}

// doPost 发送 form-urlencoded POST 请求。
func (c *Client) doPost(ctx context.Context, endpoint string, form url.Values) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("payuni: post %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("payuni: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return string(body), fmt.Errorf("payuni: unexpected http status %d", resp.StatusCode)
	}
	return string(body), nil
}

// buildForm 生成自动提交到 PAYUNi 的 HTML 表单。
func (c *Client) buildForm(action string, fields map[string]string) string {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html><head><meta charset=\"utf-8\"></head><body>")
	b.WriteString("<form id=\"payuni_form\" action=\"")
	b.WriteString(htmlEscape(action))
	b.WriteString("\" method=\"post\">")

	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString("<input type=\"hidden\" name=\"")
		b.WriteString(htmlEscape(k))
		b.WriteString("\" value=\"")
		b.WriteString(htmlEscape(fields[k]))
		b.WriteString("\">")
	}
	b.WriteString("</form><script>document.getElementById('payuni_form').submit();</script>")
	b.WriteString("</body></html>")
	return b.String()
}

func htmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&#39;",
	)
	return r.Replace(s)
}

func mergeParams(dst, src map[string]string) {
	for k, v := range src {
		dst[k] = v
	}
}

// validateOutTradeNo 校验 PAYUNi 商户订单号（MerTradeNo）：
// 长度不得超过 25 位，仅允许数字、英文与下划线，且不能全为数字或全为英文。
func validateOutTradeNo(s string) error {
	if len(s) > 25 {
		return fmt.Errorf("%w: OutTradeNo exceeds 25 characters (got %d)", gopay.ErrInvalidRequest, len(s))
	}

	allDigit := true
	allLetter := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			allLetter = false
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
			allDigit = false
		case c == '_':
			allDigit = false
			allLetter = false
		default:
			return fmt.Errorf("%w: OutTradeNo contains invalid character %q; only letters, digits and underscore are allowed", gopay.ErrInvalidRequest, c)
		}
	}

	if allDigit {
		return fmt.Errorf("%w: OutTradeNo must not be all digits", gopay.ErrInvalidRequest)
	}
	if allLetter {
		return fmt.Errorf("%w: OutTradeNo must not be all letters", gopay.ErrInvalidRequest)
	}
	return nil
}
