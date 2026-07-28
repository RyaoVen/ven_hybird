// Redis 版页面缓存 Backend：Entry JSON 存取 + SCAN 前缀删除。
package redis

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"ven_hybird/internal/pagecache"

	goredis "github.com/redis/go-redis/v9"
)

// 编译期接口断言。
var _ pagecache.Backend = (*PageBackend)(nil)

// scanCount 是 DeletePrefix 每批 SCAN 的数量。
const scanCount = 200

// PageBackend 实现 pagecache.Backend：页面缓存跨实例共享。
type PageBackend struct {
	client *goredis.Client
}

// NewPageBackend 创建 Redis 页面缓存 Backend。
func NewPageBackend(client *goredis.Client) *PageBackend {
	return &PageBackend{client: client}
}

func pageKey(key string) string {
	return keyPrefix + "page:" + key
}

// Get 查找缓存条目；未命中、反序列化失败或 Redis 故障均返回 false（fail-open：按 miss 处理）。
func (b *PageBackend) Get(key string) (*pagecache.Entry, bool) {
	data, err := b.client.Get(context.Background(), pageKey(key)).Bytes()
	if err != nil {
		if !errors.Is(err, goredis.Nil) {
			log.Printf("redis: page cache get failed: %v", err)
		}
		return nil, false
	}
	var entry pagecache.Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		log.Printf("redis: page cache entry decode failed: %v", err)
		return nil, false
	}
	return &entry, true
}

// Set 写入缓存条目；失败记日志并返回 error（调用方忽略即可，缓存本是优化）。
func (b *PageBackend) Set(key string, entry *pagecache.Entry, ttl time.Duration) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if err := b.client.Set(context.Background(), pageKey(key), data, ttl).Err(); err != nil {
		log.Printf("redis: page cache set failed: %v", err)
		return err
	}
	return nil
}

// Delete 删除指定 key；失败仅记日志。
func (b *PageBackend) Delete(key string) {
	if err := b.client.Del(context.Background(), pageKey(key)).Err(); err != nil {
		log.Printf("redis: page cache delete failed: %v", err)
	}
}

// DeletePrefix 删除所有以 prefix 开头的条目：SCAN MATCH 分批 + DEL。
// 调用频率极低（仅 DataChange/InvalidatePage），SCAN 全量扫的成本可接受；
// Redis 故障时中断并记日志（残留条目靠 TTL 自然过期）。
func (b *PageBackend) DeletePrefix(prefix string) {
	ctx := context.Background()
	pattern := pageKey(escapeGlob(prefix)) + "*"
	var cursor uint64
	for {
		keys, next, err := b.client.Scan(ctx, cursor, pattern, scanCount).Result()
		if err != nil {
			log.Printf("redis: page cache scan failed: %v", err)
			return
		}
		if len(keys) > 0 {
			if err := b.client.Del(ctx, keys...).Err(); err != nil {
				log.Printf("redis: page cache batch delete failed: %v", err)
			}
		}
		cursor = next
		if cursor == 0 {
			return
		}
	}
}

// globEscaper 转义 Redis MATCH 的 glob 元字符，保证前缀按字面匹配。
var globEscaper = strings.NewReplacer(
	`\`, `\\`,
	`*`, `\*`,
	`?`, `\?`,
	`[`, `\[`,
	`]`, `\]`,
)

func escapeGlob(s string) string {
	return globEscaper.Replace(s)
}
