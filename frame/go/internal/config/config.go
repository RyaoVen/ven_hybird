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
	ListenAddr           string        // HTTP 监听地址，如 ":8080"，环境变量: VEN_LISTEN_ADDR
	NodeWorkerURL        string        // Node.js SSR 工作节点 URL，环境变量: VEN_NODE_WORKER_URL
	NodeSubmitTimeout    time.Duration // 任务提交超时，环境变量: VEN_NODE_SUBMIT_TIMEOUT
	RenderTimeout        time.Duration // 渲染总超时，环境变量: VEN_RENDER_TIMEOUT
	InternalToken        string        // 内部认证令牌，环境变量: VEN_INTERNAL_TOKEN
	MaxPendingRenders    int           // 最大并发 pending 数（背压阈值，默认对齐 Node maxConcurrentRenders），环境变量: VEN_MAX_PENDING_RENDERS
	AssetsDir            string        // 静态资源目录，环境变量: VEN_ASSETS_DIR
	IsrDir               string        // ISR 物化文件目录，环境变量: VEN_ISR_DIR
	IsrEnabled           bool          // 是否启用 ISR（dev 置 false），环境变量: VEN_ISR_ENABLED
	RedisAddr            string        // Redis 地址（空 = 关闭，回退内存实现），环境变量: VEN_REDIS_ADDR
	RedisPassword        string        // Redis 密码，环境变量: VEN_REDIS_PASSWORD
	RedisDB              int           // Redis 数据库编号，环境变量: VEN_REDIS_DB
	SessionTTL           time.Duration // 会话有效期，环境变量: VEN_SESSION_TTL
	CookieSecure         bool          // 鉴权 cookie 是否带 Secure 标志（仅 HTTPS 发送；本地 http 开发置 false），环境变量: VEN_COOKIE_SECURE
	PageCacheTTL         time.Duration // 动态页内存缓存有效期，环境变量: VEN_PAGE_CACHE_TTL
	EventQuietWindow     time.Duration // 事件总线 debounce 静默窗口，环境变量: VEN_EVENT_QUIET_WINDOW
	EventMaxWait         time.Duration // 事件总线持续变更最大等待（强制 flush），环境变量: VEN_EVENT_MAX_WAIT
	PatternsFile         string        // Node 页面模式持久化文件（Node 不可达时回退启动），环境变量: VEN_PATTERNS_FILE
	PatternRefresh       time.Duration // Node 页面模式主动刷新间隔（0 = 关闭；Node 路由表变化后自动感知），环境变量: VEN_PATTERN_REFRESH
	PageCacheStaleWindow time.Duration // 过期缓存保留窗口（stale-while-revalidate；0 = 关闭），环境变量: VEN_PAGE_CACHE_STALE_WINDOW
	NodeCircuitThreshold int           // Node 熔断连续失败阈值，环境变量: VEN_NODE_CIRCUIT_THRESHOLD
	NodeCircuitHalfOpen  time.Duration // Node 熔断半开探测间隔，环境变量: VEN_NODE_CIRCUIT_HALF_OPEN
	EventMaxPending      int           // 事件总线待处理批次容量上限（防内存无界增长；超出丢弃新事件，允许丢），环境变量: VEN_EVENT_MAX_PENDING
	SSEMaxConns          int           // SSE 实时推送最大连接数（超出拒绝新订阅，预关闭连接），环境变量: VEN_SSE_MAX_CONNS
}

// Load 从环境变量加载配置并校验合法性。
func Load() (Config, error) {
	config := Config{
		ListenAddr:           getenv("VEN_LISTEN_ADDR", ":8080"),
		NodeWorkerURL:        getenv("VEN_NODE_WORKER_URL", "http://127.0.0.1:3000"),
		NodeSubmitTimeout:    duration("VEN_NODE_SUBMIT_TIMEOUT", 5*time.Second),
		RenderTimeout:        duration("VEN_RENDER_TIMEOUT", 20*time.Second),
		InternalToken:        internalToken(),
		MaxPendingRenders:    integer("VEN_MAX_PENDING_RENDERS", 4),
		AssetsDir:            getenv("VEN_ASSETS_DIR", "../node/build"),
		IsrDir:               getenv("VEN_ISR_DIR", "./isr-pages"),
		IsrEnabled:           boolean("VEN_ISR_ENABLED", true),
		RedisAddr:            getenv("VEN_REDIS_ADDR", ""),
		RedisPassword:        getenv("VEN_REDIS_PASSWORD", ""),
		RedisDB:              integer("VEN_REDIS_DB", 0),
		SessionTTL:           duration("VEN_SESSION_TTL", 24*time.Hour),
		CookieSecure:         boolean("VEN_COOKIE_SECURE", true),
		PageCacheTTL:         duration("VEN_PAGE_CACHE_TTL", time.Minute),
		EventQuietWindow:     duration("VEN_EVENT_QUIET_WINDOW", 5*time.Second),
		EventMaxWait:         duration("VEN_EVENT_MAX_WAIT", 30*time.Second),
		PatternsFile:         getenv("VEN_PATTERNS_FILE", "./node-patterns.json"),
		PatternRefresh:       duration("VEN_PATTERN_REFRESH", 30*time.Second),
		PageCacheStaleWindow: duration("VEN_PAGE_CACHE_STALE_WINDOW", 5*time.Minute),
		NodeCircuitThreshold: integer("VEN_NODE_CIRCUIT_THRESHOLD", 5),
		NodeCircuitHalfOpen:  duration("VEN_NODE_CIRCUIT_HALF_OPEN", 10*time.Second),
		EventMaxPending:      integer("VEN_EVENT_MAX_PENDING", 1024),
		SSEMaxConns:          integer("VEN_SSE_MAX_CONNS", 1000),
	}

	// 内部令牌是内部通道（渲染回调/页面模式拉取）的唯一凭据，安全关键：
	// 空值或开发默认值一律拒绝启动，防止内部通道在无令牌/弱令牌下带病运行。
	// 本地开发需显式设置 VEN_INTERNAL_TOKEN（强随机串，见 env.local.example 注释）。
	if config.InternalToken == "" {
		return Config{}, fmt.Errorf("VEN_INTERNAL_TOKEN must not be empty: set a strong secret explicitly")
	}
	if config.InternalToken == "development-token" {
		return Config{}, fmt.Errorf("VEN_INTERNAL_TOKEN must not be the default \"development-token\": set a strong secret explicitly")
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
	// Node 熔断参数：阈值必须为正，半开间隔必须大于零（否则熔断永远无法恢复）
	if config.NodeCircuitThreshold < 1 {
		return Config{}, fmt.Errorf("VEN_NODE_CIRCUIT_THRESHOLD must be greater than zero")
	}
	if config.NodeCircuitHalfOpen <= 0 {
		return Config{}, fmt.Errorf("VEN_NODE_CIRCUIT_HALF_OPEN must be greater than zero")
	}
	// stale 保留窗口不能为负（0 = 关闭 stale 兜底，合法）
	if config.PageCacheStaleWindow < 0 {
		return Config{}, fmt.Errorf("VEN_PAGE_CACHE_STALE_WINDOW must not be negative")
	}
	// 内存上限必须为正：无上限 = 无界增长（bus pending / SSE 连接表正是本次治理对象）
	if config.EventMaxPending < 1 {
		return Config{}, fmt.Errorf("VEN_EVENT_MAX_PENDING must be greater than zero")
	}
	if config.SSEMaxConns < 1 {
		return Config{}, fmt.Errorf("VEN_SSE_MAX_CONNS must be greater than zero")
	}

	return config, nil
}

// internalToken 读取内部令牌：未设置时回退开发默认值（Load 校验会拒绝），
// 显式置空则保留空值（同样被 Load 校验拒绝）——空/默认两态都不可绕过启动校验。
func internalToken() string {
	if value, ok := os.LookupEnv("VEN_INTERNAL_TOKEN"); ok {
		return value
	}
	return "development-token"
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
