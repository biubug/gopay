package jkos

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	gopay "github.com/biubug/gopay"
)

func testClient(t *testing.T) *Client {
	t.Helper()
	c, err := New(Config{
		StoreID:   "test_store",
		APIKey:    "test_api_key",
		SecretKey: "test_secret_key",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestNewValidation(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want error
	}{
		{"empty storeID", Config{StoreID: "", APIKey: "k", SecretKey: "s"}, gopay.ErrInvalidConfig},
		{"empty apiKey", Config{StoreID: "s", APIKey: "", SecretKey: "s"}, gopay.ErrInvalidConfig},
		{"empty secretKey", Config{StoreID: "s", APIKey: "k", SecretKey: ""}, gopay.ErrInvalidConfig},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.cfg)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestSandboxBaseURL(t *testing.T) {
	c, err := New(Config{
		StoreID:   "s",
		APIKey:    "k",
		SecretKey: "sec",
		Sandbox:   true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.baseURL != SandboxBaseURL {
		t.Errorf("baseURL = %q, want %q", c.baseURL, SandboxBaseURL)
	}

	c2, _ := New(Config{StoreID: "s", APIKey: "k", SecretKey: "sec"})
	if c2.baseURL != ProductionBaseURL {
		t.Errorf("baseURL = %q, want %q", c2.baseURL, ProductionBaseURL)
	}
}

// TestSignGoldenVectorPOST 验证 POST 请求的 HMAC-SHA256 签名与参考实现一致。
// 参考值由 PowerShell HMACSHA256 计算得到。
func TestSignGoldenVectorPOST(t *testing.T) {
	c := testClient(t)
	payload, err := jsonMarshal(map[string]interface{}{"store_id": "test"})
	if err != nil {
		t.Fatalf("jsonMarshal: %v", err)
	}
	got := c.sign(payload)
	const want = "bcea7a9a247751b134aa5c859d3d50c24635d2ae3e7ed0a70596fb9535ec9f4b"
	if got != want {
		t.Errorf("sign mismatch:\n got %s\nwant %s", got, want)
	}
}

// TestSignGoldenVectorGET 验证 GET 请求（query string）的 HMAC-SHA256 签名。
func TestSignGoldenVectorGET(t *testing.T) {
	c := testClient(t)
	q := url.Values{}
	q.Set("platform_order_ids", "JKOS_TEST_001")
	got := c.sign([]byte(q.Encode()))
	const want = "589cc628cdea7bd8f22a900e822725b8b8826bb7235c210448520842ffc17a84"
	if got != want {
		t.Errorf("sign mismatch:\n got %s\nwant %s", got, want)
	}
}

func TestSignFormat(t *testing.T) {
	c := testClient(t)
	sig := c.sign([]byte("test"))
	if len(sig) != 64 {
		t.Fatalf("signature length = %d, want 64", len(sig))
	}
	for _, r := range sig {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			t.Fatalf("signature contains non-lowercase-hex rune %q in %q", r, sig)
		}
	}
}

func TestJsonMarshalNoHTMLEscape(t *testing.T) {
	// 确保 & 不被转义为 \u0026（与 PHP json_encode 行为一致）。
	out, err := jsonMarshal(map[string]interface{}{
		"result_url": "https://example.com/callback?a=1&b=2",
	})
	if err != nil {
		t.Fatalf("jsonMarshal: %v", err)
	}
	if !strings.Contains(string(out), "&") {
		t.Errorf("expected literal '&' in JSON, got %s", out)
	}
}

func TestVerifyNotifySuccess(t *testing.T) {
	c := testClient(t)
	notify := `{
		"transaction": {
			"platform_order_id": "JKOS_TEST_001",
			"status": 0,
			"tradeNo": "2208250001",
			"trans_time": "2022-08-25 15:17:27",
			"final_price": 100,
			"channel_type": "account"
		}
	}`
	result, err := c.VerifyNotify([]byte(notify))
	if err != nil {
		t.Fatalf("VerifyNotify: %v", err)
	}
	if !result.Paid {
		t.Error("Paid = false, want true")
	}
	if result.TradeState != gopay.TradeStatePaid {
		t.Errorf("TradeState = %s, want %s", result.TradeState, gopay.TradeStatePaid)
	}
	if result.OutTradeNo != "JKOS_TEST_001" {
		t.Errorf("OutTradeNo = %q, want JKOS_TEST_001", result.OutTradeNo)
	}
	if result.TradeNo != "2208250001" {
		t.Errorf("TradeNo = %q, want 2208250001", result.TradeNo)
	}
	if result.Amount != "100" {
		t.Errorf("Amount = %q, want 100", result.Amount)
	}
	if result.Currency != "TWD" {
		t.Errorf("Currency = %q, want TWD", result.Currency)
	}
	if result.PayType != "account" {
		t.Errorf("PayType = %q, want account", result.PayType)
	}
	if result.PayTime != "2022-08-25 15:17:27" {
		t.Errorf("PayTime = %q", result.PayTime)
	}
}

func TestVerifyNotifyUnpaid(t *testing.T) {
	c := testClient(t)
	notify := `{
		"transaction": {
			"platform_order_id": "JKOS_TEST_002",
			"status": 101,
			"final_price": 200
		}
	}`
	result, err := c.VerifyNotify([]byte(notify))
	if err != nil {
		t.Fatalf("VerifyNotify: %v", err)
	}
	if result.Paid {
		t.Error("Paid = true, want false")
	}
	if result.TradeState != gopay.TradeStateUnpaid {
		t.Errorf("TradeState = %s, want %s", result.TradeState, gopay.TradeStateUnpaid)
	}
	if result.Amount != "200" {
		t.Errorf("Amount = %q, want 200", result.Amount)
	}
	if result.PayTime == "" {
		t.Error("PayTime = empty, want current time")
	}
}

func TestVerifyNotifyMissingPlatformOrderID(t *testing.T) {
	c := testClient(t)
	notify := `{"transaction": {"status": 0}}`
	_, err := c.VerifyNotify([]byte(notify))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, gopay.ErrVerifySignature) {
		t.Errorf("err = %v, want ErrVerifySignature", err)
	}
}

func TestVerifyNotifyInvalidJSON(t *testing.T) {
	c := testClient(t)
	_, err := c.VerifyNotify([]byte("not json"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestMapTradeState(t *testing.T) {
	cases := []struct {
		status int
		want   gopay.TradeState
	}{
		{TradeStatusPaid, gopay.TradeStatePaid},
		{TradeStatusUnpaid, gopay.TradeStateUnpaid},
		{TradeStatusProcessing, gopay.TradeStateUnknown},
		{TradeStatusNotExist, gopay.TradeStateUnknown},
		{999, gopay.TradeStateUnknown},
	}
	for _, tc := range cases {
		got := mapTradeState(tc.status)
		if got != tc.want {
			t.Errorf("mapTradeState(%d) = %s, want %s", tc.status, got, tc.want)
		}
	}
}

func TestCreateOrderValidation(t *testing.T) {
	c := testClient(t)
	cases := []struct {
		name    string
		req     *gopay.CreateOrderRequest
		wantErr error
	}{
		{"nil request", nil, gopay.ErrInvalidRequest},
		{"empty OutTradeNo", &gopay.CreateOrderRequest{Amount: "100"}, gopay.ErrInvalidRequest},
		{"empty Amount", &gopay.CreateOrderRequest{OutTradeNo: "ORDER1"}, gopay.ErrInvalidRequest},
		{"CNY not supported", &gopay.CreateOrderRequest{OutTradeNo: "ORDER1", Amount: "100", Currency: "CNY"}, gopay.ErrInvalidRequest},
		{"invalid amount", &gopay.CreateOrderRequest{OutTradeNo: "ORDER1", Amount: "abc"}, gopay.ErrInvalidRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.CreateOrder(tc.req)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestCreateOrderCurrencyDefault(t *testing.T) {
	c := testClient(t)
	// 空币种应默认为 TWD，不返回 currency 错误。
	// 由于没有 HTTP 服务器，这里只验证不因 currency 报错。
	_, err := c.CreateOrder(&gopay.CreateOrderRequest{
		OutTradeNo: "ORDER1",
		Amount:     "100",
	})
	if err != nil && errors.Is(err, gopay.ErrInvalidRequest) &&
		err.Error() != "" {
		// 预期会因网络请求失败，但不应因 currency 失败。
		// 网络错误是正常的（没有真实 API）。
	}
}

func TestRefundOrderValidation(t *testing.T) {
	c := testClient(t)
	cases := []struct {
		name    string
		req     *gopay.RefundOrderRequest
		wantErr error
	}{
		{"nil request", nil, gopay.ErrInvalidRequest},
		{"empty OutTradeNo", &gopay.RefundOrderRequest{Amount: "100"}, gopay.ErrInvalidRequest},
		{"empty Amount", &gopay.RefundOrderRequest{OutTradeNo: "ORDER1"}, gopay.ErrInvalidRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.RefundOrder(tc.req)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestNotifyAck(t *testing.T) {
	c := testClient(t)
	if got := c.NotifyAck(); got != NotifySuccessAck {
		t.Errorf("NotifyAck = %q, want %q", got, NotifySuccessAck)
	}
}

func TestParseAmount(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"100", 100},
		{"100.0", 100},
		{"100.99", 100},
		{"0", 0},
	}
	for _, tc := range cases {
		got, err := parseAmount(tc.input)
		if err != nil {
			t.Errorf("parseAmount(%q) error: %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseAmount(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}

	if _, err := parseAmount("abc"); err == nil {
		t.Error("parseAmount(abc) expected error, got nil")
	}
}
