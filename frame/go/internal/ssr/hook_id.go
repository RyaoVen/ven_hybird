// HookID 生成器，用于标识渲染任务。
package ssr

import (
	"crypto/rand"
	"encoding/base64"
)

// HookIDGenerator 定义 HookID 生成器接口。
type HookIDGenerator interface {
	// New 生成一个唯一的 HookID。
	New() (string, error)
}

// CryptoHookIDGenerator 基于加密安全随机数生成 HookID。
type CryptoHookIDGenerator struct{}

// New 生成 32 字符的 Base64 URL 安全随机字符串。
func (CryptoHookIDGenerator) New() (string, error) {
	// 分配 24 字节缓冲区，编码后将产生 32 个 Base64 字符
	buffer := make([]byte, 24)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	// 使用 RawURLEncoding：不含填充 "="，使用 URL 安全字符集（- 和 _）
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
