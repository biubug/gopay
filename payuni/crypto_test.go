package payuni

import (
	"encoding/hex"
	"errors"
	"testing"

	gopay "github.com/biubug/gopay"
)

// 使用官方 PHP SDK README 示例的 key/iv。
func testClient(t *testing.T) *Client {
	t.Helper()
	c, err := New(Config{
		MerID:   "ABC",
		HashKey: "12345678901234567890123456789012",
		HashIV:  "1234567890123456",
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
		{"empty merID", Config{MerID: "", HashKey: "12345678901234567890123456789012", HashIV: "1234567890123456"}, gopay.ErrInvalidConfig},
		{"bad key len", Config{MerID: "ABC", HashKey: "short", HashIV: "1234567890123456"}, gopay.ErrInvalidConfig},
		{"bad iv len", Config{MerID: "ABC", HashKey: "12345678901234567890123456789012", HashIV: "short"}, gopay.ErrInvalidConfig},
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

func TestValidateOutTradeNo(t *testing.T) {
	valid := []string{
		"ORDER1",
		"A1",
		"ORDER_20260818_001",
		"abc123",
		"ABC_123",
		"ORDER123_456",
	}
	for _, s := range valid {
		if err := validateOutTradeNo(s); err != nil {
			t.Errorf("validateOutTradeNo(%q) = %v, want nil", s, err)
		}
	}

	invalid := []string{
		"123456",                           // 纯数字
		"abcdef",                           // 纯英文
		"8",                                // 纯数字
		"A",                                // 纯英文
		"ORDER202608180001234567890123456", // 超 25 位（26 位）
		"ORDER-1",                          // 非法字符
		"订单1",                              // 非 ASCII 非法字符
	}
	for _, s := range invalid {
		if err := validateOutTradeNo(s); err == nil {
			t.Errorf("validateOutTradeNo(%q) = nil, want error", s)
		}
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	c := testClient(t)
	params := map[string]string{
		"MerID":      "ABC",
		"MerTradeNo": "test20220829111528",
		"TradeAmt":   "100",
		"ProdDesc":   "測試商品",
		"ReturnURL":  "https://example.com/api/return?a=1&b=2",
		"NotifyURL":  "https://example.com/api/notify",
		"Timestamp":  "1661419047",
	}
	enc, err := c.encrypt(params)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	dec, err := c.decrypt(enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	for k, v := range params {
		if dec[k] != v {
			t.Errorf("field %q: got %q want %q", k, dec[k], v)
		}
	}
}

func TestEncryptInfoFormat(t *testing.T) {
	c := testClient(t)
	enc, err := c.encrypt(map[string]string{"MerID": "ABC", "Timestamp": "1661419047"})
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	// EncryptInfo 必须是合法 hex。
	if _, err := hex.DecodeString(enc); err != nil {
		t.Fatalf("EncryptInfo is not valid hex: %v", err)
	}
	// 解密可还原。
	dec, err := c.decrypt(enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if dec["MerID"] != "ABC" {
		t.Errorf("MerID: got %q, want ABC", dec["MerID"])
	}
}

// TestGoldenVectorCompat 使用 OpenSSL/Node 生成的参照值，验证 Go 实现的
// AES-256-GCM（16 字节 nonce）与 HashInfo 与 PHP/OpenSSL 字节一致。
func TestGoldenVectorCompat(t *testing.T) {
	c := testClient(t)
	enc, err := c.encrypt(map[string]string{"MerID": "ABC", "Timestamp": "1661419047"})
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	const wantEncryptInfo = "47396636346f6673585365336738474e2f3954792b2b3273334c32553675564e6b3373655a5a53753a3a3a476b3969434d77757a6b6754794b446a6e574d4138513d3d"
	if enc != wantEncryptInfo {
		t.Errorf("EncryptInfo mismatch:\n got %s\nwant %s", enc, wantEncryptInfo)
	}
	gotHash := c.hash(enc)
	const wantHash = "C750EEE31E94D96C9DA2BCF8A55F98C7FE48C684D5AD337894EC4DC9A7EDB34D"
	if gotHash != wantHash {
		t.Errorf("HashInfo mismatch:\n got %s\nwant %s", gotHash, wantHash)
	}
}

func TestHashInfoFormat(t *testing.T) {
	c := testClient(t)
	h := c.hash("0001020304")
	if len(h) != 64 {
		t.Fatalf("HashInfo length = %d, want 64", len(h))
	}
	for _, r := range h {
		if !(r >= '0' && r <= '9' || r >= 'A' && r <= 'F') {
			t.Fatalf("HashInfo contains non-uppercase-hex rune %q in %q", r, h)
		}
	}
}

func TestParseQueryResult(t *testing.T) {
	// 模拟交易查詢回传的明文（PHP http_build_query 嵌套数组表示法）。
	plain := "Status=SUCCESS&Result%5B0%5D%5BMerTradeNo%5D=ORDER1" +
		"&Result%5B0%5D%5BTradeNo%5D=16614190477810373246" +
		"&Result%5B0%5D%5BTradeAmt%5D=100" +
		"&Result%5B0%5D%5BTradeStatus%5D=1" +
		"&Result%5B1%5D%5BMerTradeNo%5D=ORDER2" +
		"&Result%5B1%5D%5BTradeStatus%5D=2"
	records := parseQueryResult(parseQuery(plain))
	if len(records) != 2 {
		t.Fatalf("len(records) = %d, want 2", len(records))
	}
	if records[0]["MerTradeNo"] != "ORDER1" || records[0]["TradeNo"] != "16614190477810373246" || records[0]["TradeAmt"] != "100" || records[0]["TradeStatus"] != "1" {
		t.Errorf("records[0] = %v", records[0])
	}
	if records[1]["MerTradeNo"] != "ORDER2" || records[1]["TradeStatus"] != "2" {
		t.Errorf("records[1] = %v", records[1])
	}
}

func TestVerifyNotifySuccess(t *testing.T) {
	c := testClient(t)
	params := map[string]string{
		"MerID":       "ABC",
		"MerTradeNo":  "order123",
		"TradeNo":     "16614190477810373246",
		"TradeAmt":    "100",
		"TradeStatus": TradeStatusPaid,
		"Timestamp":   "1661419047",
	}
	enc, err := c.encrypt(params)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	notify := buildQueryString(map[string]string{
		"Status":      "SUCCESS",
		"EncryptInfo": enc,
		"HashInfo":    c.hash(enc),
	})

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
	if result.OutTradeNo != "order123" {
		t.Errorf("OutTradeNo = %q, want order123", result.OutTradeNo)
	}
	if result.TradeNo != "16614190477810373246" {
		t.Errorf("TradeNo = %q", result.TradeNo)
	}
}

func TestVerifyNotifyError(t *testing.T) {
	c := testClient(t)
	notify := "Status=ERROR&Message=some%20error"
	result, err := c.VerifyNotify([]byte(notify))
	if err != nil {
		t.Fatalf("VerifyNotify: %v", err)
	}
	if result.Status != "ERROR" {
		t.Errorf("Status = %q, want ERROR", result.Status)
	}
	if result.Message != "some error" {
		t.Errorf("Message = %q, want \"some error\"", result.Message)
	}
}

func TestVerifyNotifyBadSignature(t *testing.T) {
	c := testClient(t)
	enc, err := c.encrypt(map[string]string{"MerID": "ABC", "Timestamp": "1661419047"})
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	notify := buildQueryString(map[string]string{
		"Status":      "SUCCESS",
		"EncryptInfo": enc,
		"HashInfo":    "DEADBEEF",
	})
	_, err = c.VerifyNotify([]byte(notify))
	if err == nil {
		t.Fatal("expected signature error, got nil")
	}
	if !errors.Is(err, gopay.ErrVerifySignature) {
		t.Errorf("err = %v, want ErrVerifySignature", err)
	}
}
