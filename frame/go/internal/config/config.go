// Package config 提供从环境变量加载配置的功能。
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config 是 VenHybird 网关的配置结构体，配置值优先从环境变量读取。
type Config struct {
	ListenAddr        string        // HTTP 监听地址，如 ":8080"，环境变量: VEN_LISTEN_ADDR
	NodeWorkerURL     string        // Node.js SSR 工作节点 URL，环境变量: VEN_NODE_WORKER_URL
	NodeSubmitTimeout time.Duration // 任务提交超时，环境变量: VEN_NODE_SUBMIT_TIMEOUT
	RenderTimeout     time.Duration // 渲染总超时，环境变量: VEN_RENDER_TIMEOUT
	InternalToken     string        // 内部认证令牌，环境变量: VEN_INTERNAL_TOKEN
	MaxPendingRenders int           // 最大并发 pending 数，环境变量: VEN_MAX_PENDING_RENDERS
	AssetsDir         string        // 静态资源目录，环境变量: VEN_ASSETS_DIR
	IsrDir            string        // ISR 物化文件目录，环境变量: VEN_ISR_DIR
	IsrEnabled        bool          // 是否启用 ISR（dev 置 false），环境变量: VEN_ISR_ENABLED
	RedisAddr         string        // Redis 地址（空 = 关闭，回退内存实现），环境变量: VEN_REDIS_ADDR
	RedisPassword     string        // Redis 密码，环境变量: VEN_REDIS_PASSWORD
	RedisDB           int           // Redis 数据库编号，环境变量: VEN_REDIS_DB
	SessionTTL        time.Duration // 会话有效期，环境变量: VEN_SESSION_TTL
	PageCacheTTL      time.Duration // 动态页内存缓存有效期，环境变量: VEN_PAGE_CACHE_TTL
	EventQuietWindow  time.Duration // 事件总线 debounce 静默窗口，环境变量: VEN_EVENT_QUIET_WINDOW
	EventMaxWait      time.Duration // 事件总线持续变更最大等待（强制 flush），环境变量: VEN_EVENT_MAX_WAIT
}

// Load 从环境变量加载配置并校验合法性。
func Load() (Config, error) {
	config := Config{
		ListenAddr:        getenv("VEN_LISTEN_ADDR", ":8080"),
		NodeWorkerURL:     getenv("VEN_NODE_WORKER_URL", "http://127.0.0.1:3000"),
		NodeSubmitTimeout: duration("VEN_NODE_SUBMIT_TIMEOUT", 5*time.Second),
		RenderTimeout:     duration("VEN_RENDER_TIMEOUT", 20*time.Second),
		InternalToken:     getenv("VEN_INTERNAL_TOKEN", "development-token"),
		MaxPendingRenders: integer("VEN_MAX_PENDING_RENDERS", 100),
		AssetsDir:         getenv("VEN_ASSETS_DIR", "../node/build"),
		IsrDir:            getenv("VEN_ISR_DIR", "./isr-pages"),
		IsrEnabled:        boolean("VEN_ISR_ENABLED", true),
		RedisAddr:         getenv("VEN_REDIS_ADDR", ""),
		RedisPassword:     getenv("VEN_REDIS_PASSWORD", ""),
		RedisDB:           integer("VEN_REDIS_DB", 0),
		SessionTTL:        duration("VEN_SESSION_TTL", 24*time.Hour),
		PageCacheTTL:      duration("VEN_PAGE_CACHE_TTL", time.Minute),
		EventQuietWindow:  duration("VEN_EVENT_QUIET_WINDOW", 5*time.Second),
		EventMaxWait:      duration("VEN_EVENT_MAX_WAIT", 30*time.Second),
	}

	// 业务规则校验：渲染总超时必须大于任务提交超时，
	// 否则可能出现提交成功但还没等到回调就已超时的情况
	if config.RenderTimeout <= config.NodeSubmitTimeout {
		return Config{}, fmt.Errorf("VEN_RENDER_TIMEOUT must be greater than VEN_NODE_SUBMIT_TIMEOUT")
	}
	// 最大并发渲染数必须大于零，否则系统将无法处理任何渲染请求
	if config.MaxPendingRenders < 1 {
		return Config{}, fmt.Errorf("VEN_MAX_PENDING_RENDERS must be greater than zero")
	}
	// 会话/页面缓存有效期必须大于零
	if config.SessionTTL <= 0 {
		return Config{}, fmt.Errorf("VEN_SESSION_TTL must be greater than zero")
	}
	if config.PageCacheTTL <= 0 {
		return Config{}, fmt.Errorf("VEN_PAGE_CACHE_TTL must be greater than zero")
	}
	// 持续变更最大等待必须大于静默窗口，否则静默窗口永远不生效
	if config.EventQuietWindow <= 0 {
		return Config{}, fmt.Errorf("VEN_EVENT_QUIET_WINDOW must be greater than zero")
	}
	if config.EventMaxWait <= config.EventQuietWindow {
		return Config{}, fmt.Errorf("VEN_EVENT_MAX_WAIT must be greater than VEN_EVENT_QUIET_WINDOW")
	}

	return config, nil
}

// getenv 获取环境变量值，未设置时返回 fallback。
func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// duration 从环境变量解析 Duration 值，解析失败返回 fallback。
func duration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

// integer 从环境变量解析整数值，解析失败返回 fallback。
func integer(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

// boolean 从环境变量解析布尔值（"false"/"0" 为 false，其余非空为 true）。
func boolean(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value != "false" && value != "0"
}
