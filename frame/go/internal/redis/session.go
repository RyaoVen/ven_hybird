// Redis 版会话 Backend：token → role，带 TTL。
package redis

import (
	"context"
	"errors"
	"log"
	"time"

	"ven_hybird/internal/auth"

	goredis "github.com/redis/go-redis/v9"
)

// 编译期接口断言。
var _ auth.Backend = (*SessionBackend)(nil)

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
func (b *SessionBackend) Set(token string, role string, ttl time.Duration) error {
	if err := b.client.Set(context.Background(), sessionKey(token), role, ttl).Err(); err != nil {
		log.Printf("redis: session set failed: %v", err)
		return err
	}
	return nil
}

// Get 查找会话；未命中或 Redis 故障返回 false（fail-open：按未登录处理）。
func (b *SessionBackend) Get(token string) (string, bool) {
	role, err := b.client.Get(context.Background(), sessionKey(token)).Result()
	if err != nil {
		if !errors.Is(err, goredis.Nil) {
			log.Printf("redis: session get failed: %v", err)
		}
		return "", false
	}
	return role, true
}

// Delete 删除会话（注销）；失败仅记日志。
func (b *SessionBackend) Delete(token string) {
	if err := b.client.Del(context.Background(), sessionKey(token)).Err(); err != nil {
		log.Printf("redis: session delete failed: %v", err)
	}
}
