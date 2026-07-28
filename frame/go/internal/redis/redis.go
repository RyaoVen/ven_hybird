// Package redis 提供 Redis 版会话/页面缓存 Backend 与事件 Pub/Sub 传输（集群部署用）。
// 全部实现遵循 fail-open：Redis 错误只记日志——缓存 Get 按 miss、
// session Get 按未登录，不把基础设施故障抛给业务请求路径。
package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// keyPrefix 是框架全部 Redis key 的统一前缀。
const keyPrefix = "ven:"

// 客户端超时（常量先行，后续配置化）。
const (
	dialTimeout  = 3 * time.Second
	readTimeout  = 3 * time.Second
	writeTimeout = 3 * time.Second
	pingTimeout  = 5 * time.Second
)

// NewClient 创建 Redis 客户端并 ping 验证连通性；失败返回 error 由调用方回退内存实现。
func NewClient(addr string, password string, db int) (*goredis.Client, error) {
	client := goredis.NewClient(&goredis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		DialTimeout:  dialTimeout,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
	})
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis: ping %s failed: %w", addr, err)
	}
	return client, nil
}
