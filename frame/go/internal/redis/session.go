// Redis 版会话 Backend：token → 会话（role + userID），带 TTL。
package redis

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"ven_hybird/internal/auth"

	goredis "github.com/redis/go-redis/v9"
)

// 编译期接口断言。
var _ auth.Backend = (*SessionBackend)(nil)

// sessionValueSep 是会话值里 role 与 userID 的分隔符（\x1f Unit Separator，
// 角色名与用户主键正常不含控制字符）。
// 兼容性：旧格式值（纯 role、无分隔符）解析为 userID=""，与旧 GrantAuth 行为一致——
// 因此不换 key 前缀、无会话丢失。
const sessionValueSep = "\x1f"

// SessionBackend 实现 auth.Backend：会话跨实例共享。
type SessionBackend struct {
	client *goredis.Client
}

// NewSessionBackend 创建 Redis 会话 Backend。
func NewSessionBackend(client *goredis.Client) *SessionBackend {
	return &SessionBackend{client: client}
}

func sessionKey(token string) string {
	return keyPrefix + "session:" + token
}

// Set 写入会话（TTL 过期）；失败记日志并返回 error（登录路径可见失败，不静默吞掉）。
func (b *SessionBackend) Set(token, role, userID string, ttl time.Duration) error {
	if err := b.client.Set(context.Background(), sessionKey(token), role+sessionValueSep+userID, ttl).Err(); err != nil {
		log.Printf("redis: session set failed: %v", err)
		return err
	}
	return nil
}

// Get 查找会话；未命中或 Redis 故障返回 false（fail-open：按未登录处理）。
// 旧格式值（无分隔符）按纯 role 读，userID 为空。
func (b *SessionBackend) Get(token string) (string, string, bool) {
	value, err := b.client.Get(context.Background(), sessionKey(token)).Result()
	if err != nil {
		if !errors.Is(err, goredis.Nil) {
			log.Printf("redis: session get failed: %v", err)
		}
		return "", "", false
	}
	role, userID, _ := strings.Cut(value, sessionValueSep)
	return role, userID, true
}

// Delete 删除会话（注销）；失败仅记日志。
func (b *SessionBackend) Delete(token string) {
	if err := b.client.Del(context.Background(), sessionKey(token)).Err(); err != nil {
		log.Printf("redis: session delete failed: %v", err)
	}
}
