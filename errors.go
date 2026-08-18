package gopay

import "errors"

var (
	// ErrNotImplemented 功能尚未实现。
	ErrNotImplemented = errors.New("gopay: not implemented")
	// ErrInvalidConfig 渠道配置无效。
	ErrInvalidConfig = errors.New("gopay: invalid config")
	// ErrInvalidRequest 请求参数无效。
	ErrInvalidRequest = errors.New("gopay: invalid request")
	// ErrVerifySignature 回调/响应签名校验失败。
	ErrVerifySignature = errors.New("gopay: signature verification failed")
	// ErrAPIResponse 上游接口返回错误。
	ErrAPIResponse = errors.New("gopay: upstream api returned an error")
)