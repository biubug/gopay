package payuni

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// phpURLEncode 模拟 PHP http_build_query 内部使用的 urlencode 行为：
// 字母数字与 "-" "_" "." 保持不变，空格转 "+"，其余字符按 UTF-8 字节大写百分号编码。
func phpURLEncode(s string) string {
	const hexDigits = "0123456789ABCDEF"
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.':
			b.WriteByte(c)
		case c == ' ':
			b.WriteByte('+')
		default:
			b.WriteByte('%')
			b.WriteByte(hexDigits[c>>4])
			b.WriteByte(hexDigits[c&0x0f])
		}
	}
	return b.String()
}

// buildQueryString 构造 query string；key/value 均按 PHP 风格编码，
// key 按字典序排序以保证输出确定性（服务端使用 parse_str 解析，顺序不影响语义）。
func buildQueryString(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(m))
	for _, k := range keys {
		pairs = append(pairs, phpURLEncode(k)+"="+phpURLEncode(m[k]))
	}
	return strings.Join(pairs, "&")
}

// parseQuery 按 PHP parse_str 语义解析 query string：仅以 "&" 分隔，
// "+" 与 "%XX" 解码为对应字节。
func parseQuery(s string) map[string]string {
	m := make(map[string]string)
	for _, pair := range strings.Split(s, "&") {
		if pair == "" {
			continue
		}
		kv := strings.SplitN(pair, "=", 2)
		k, err := url.QueryUnescape(kv[0])
		if err != nil {
			k = kv[0]
		}
		v := ""
		if len(kv) == 2 {
			if vv, err := url.QueryUnescape(kv[1]); err == nil {
				v = vv
			}
		}
		m[k] = v
	}
	return m
}

// encrypt 使用 AES-256-GCM 加密并生成 EncryptInfo。
// 格式与官方 SDK 保持一致：hex( base64(ciphertext) + ":::" + base64(gcm tag) )。
func (c *Client) encrypt(params map[string]string) (string, error) {
	block, err := aes.NewCipher(c.hashKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, len(c.hashIV))
	if err != nil {
		return "", err
	}
	nonce := make([]byte, len(c.hashIV))
	copy(nonce, c.hashIV)

	// Seal 返回 ciphertext || tag(16)。
	sealed := gcm.Seal(nil, nonce, []byte(buildQueryString(params)), nil)
	tagLen := gcm.Overhead()
	ciphertext := sealed[:len(sealed)-tagLen]
	tag := sealed[len(sealed)-tagLen:]

	combined := base64.StdEncoding.EncodeToString(ciphertext) + ":::" + base64.StdEncoding.EncodeToString(tag)
	return hex.EncodeToString([]byte(combined)), nil
}

// decrypt 解密 EncryptInfo 并解析为字段 map。
func (c *Client) decrypt(encryptInfo string) (map[string]string, error) {
	raw, err := hex.DecodeString(encryptInfo)
	if err != nil {
		return nil, fmt.Errorf("payuni: invalid EncryptInfo hex: %w", err)
	}
	parts := strings.SplitN(string(raw), ":::", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("payuni: invalid EncryptInfo format")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("payuni: decode ciphertext: %w", err)
	}
	tag, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("payuni: decode tag: %w", err)
	}

	block, err := aes.NewCipher(c.hashKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, len(c.hashIV))
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, len(c.hashIV))
	copy(nonce, c.hashIV)

	sealed := make([]byte, 0, len(ciphertext)+len(tag))
	sealed = append(sealed, ciphertext...)
	sealed = append(sealed, tag...)

	plaintext, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, fmt.Errorf("payuni: gcm open: %w", err)
	}
	return parseQuery(string(plaintext)), nil
}

// hash 计算 HashInfo：大写 hex( sha256(HashKey + EncryptInfo + HashIV) )。
func (c *Client) hash(encryptInfo string) string {
	sum := sha256.Sum256([]byte(string(c.hashKey) + encryptInfo + string(c.hashIV)))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}