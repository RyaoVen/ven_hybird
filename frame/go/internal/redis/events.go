// Redis Pub/Sub 版事件传输：DataChange 变更事件跨实例广播。
package redis

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"ven_hybird/internal/event"

	goredis "github.com/redis/go-redis/v9"
)

// 编译期接口断言。
var _ event.Transport = (*EventTransport)(nil)

// changeChannel 是变更事件广播频道。
const changeChannel = keyPrefix + "events:change"

// EventTransport 实现 event.Transport（Redis Pub/Sub，允许丢消息）。
type EventTransport struct {
	client *goredis.Client
}

// NewEventTransport 创建 Redis 事件传输。
func NewEventTransport(client *goredis.Client) *EventTransport {
	return &EventTransport{client: client}
}

// Publish 广播一次变更事件；失败返回 error（调用方记日志即可，允许丢）。
func (t *EventTransport) Publish(ev event.ChangeEvent) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return t.client.Publish(context.Background(), changeChannel, data).Err()
}

// Subscribe 订阅变更事件并投递给 handler（goroutine 随进程生命周期）。
// 接收时间覆盖 EnqueuedAt：以本实例收到时刻作为本地 debounce 基准。
func (t *EventTransport) Subscribe(handler func(ev event.ChangeEvent)) {
	pubsub := t.client.Subscribe(context.Background(), changeChannel)
	go func() {
		for msg := range pubsub.Channel() {
			var ev event.ChangeEvent
			if err := json.Unmarshal([]byte(msg.Payload), &ev); err != nil {
				log.Printf("redis: change event decode failed: %v", err)
				continue
			}
			ev.EnqueuedAt = time.Now()
			handler(ev)
		}
	}()
}
