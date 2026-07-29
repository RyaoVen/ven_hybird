// Package httpserver 提供 HTTP 服务器功能。
package httpserver

import (
	"context"
	"log"
	"sync"
	"time"

	"ven_hybird/internal/auth"
	"ven_hybird/internal/config"
	"ven_hybird/internal/event"
	"ven_hybird/internal/isr"
	"ven_hybird/internal/pagecache"
	"ven_hybird/internal/pagepattern"
	"ven_hybird/internal/redis"
	"ven_hybird/internal/ssr"

	"github.com/gofiber/fiber/v2"
)

// Server 是 HTTP 服务器核心结构体。
type Server struct {
	app       *fiber.App           // Fiber 应用实例
	config    config.Config        // 应用配置
	ssr       ssr.Client           // SSR 渲染客户端
	pending   *ssr.PendingRegistry // pending 任务注册中心
	hookIDs   ssr.HookIDGenerator  // HookID 生成器
	auth      *auth.Registry       // 权限等级注册表
	sessions  *auth.SessionStore   // 会话存储（token → role）
	pageCache *pagecache.Store     // 页面渲染结果缓存
	isrStore  *isr.Store           // ISR 文件层

	eventTransport event.Transport // 事件跨实例传输（nil = 单实例；Redis 配置后由 hybrid 挂到事件总线）

	patternMu   sync.RWMutex           // 保护 patterns 指针（校验失败重拉时换入新校验器）
	patterns    *pagepattern.Validator // 页面 pattern 校验器
	lastRefetch time.Time              // 上次 pattern 重拉时间（节流用）

	staticDecls        map[string]*isr.Declaration // StaticPage 声明（按模板字符串）
	fallbackRegistered bool                        // 页面兜底路由是否已注册（RegisterPageFallback 幂等标记）
}

// 默认值：Config 由字面量构造（测试）未设这些字段时回退到与 config.Load 相同的默认。
const (
	defaultSessionTTL   = 24 * time.Hour // 会话有效期
	defaultPageCacheTTL = time.Minute    // 页面缓存有效期
)

// New 创建并初始化 HTTP 服务器实例。
// patterns 为 Node 页面路由模式校验器，启动时通过 pagepattern.Fetch 拉取构建。
func New(
	cfg config.Config,
	client ssr.Client,
	pending *ssr.PendingRegistry,
	hookIDs ssr.HookIDGenerator,
	patterns *pagepattern.Validator,
) *Server {
	// 零值回退默认（字面量构造 Config 时不带时间字段的场景）
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = defaultSessionTTL
	}
	if cfg.PageCacheTTL <= 0 {
		cfg.PageCacheTTL = defaultPageCacheTTL
	}
	app := fiber.New(fiber.Config{
		AppName:               "VenHybird",
		ReadTimeout:           60 * time.Second, // 大于浏览器 keep-alive 空闲，消除 408 噪音
		WriteTimeout:          cfg.RenderTimeout + 5*time.Second,
		IdleTimeout:           120 * time.Second,
		DisableStartupMessage: false,
		BodyLimit:             10 * 1024 * 1024, // 10MB
		// 全局错误处理器：记录错误日志并返回统一的 500 错误响应
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			log.Printf("http error: %s %s: %v", ctx.Method(), ctx.Path(), err)
			return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		},
	})
	app.Use(requestLogger())

	// 会话/页面缓存后端：配置 Redis 则跨实例共享，连接失败回退内存（fail-open）
	sessionBackend := auth.Backend(auth.NewMemoryBackend())
	pageBackend := pagecache.Backend(pagecache.NewMemoryBackend(pageCacheCapacity))
	var eventTransport event.Transport
	if cfg.RedisAddr != "" {
		if redisClient, err := redis.NewClient(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB); err != nil {
			log.Printf("redis: connect failed, fallback to memory backends: %v", err)
		} else {
			log.Printf("redis: session/page cache backends connected (%s db=%d)", cfg.RedisAddr, cfg.RedisDB)
			sessionBackend = redis.NewSessionBackend(redisClient)
			pageBackend = redis.NewPageBackend(redisClient)
			eventTransport = redis.NewEventTransport(redisClient)
		}
	}

	s := &Server{
		app:            app,
		config:         cfg,
		ssr:            client,
		pending:        pending,
		hookIDs:        hookIDs,
		auth:           auth.NewRegistry(),
		sessions:       auth.NewSessionStore(sessionBackend, cfg.SessionTTL),
		patterns:       patterns,
		pageCache:      pagecache.NewStore(pageBackend, cfg.PageCacheTTL),
		isrStore:       isr.NewStore(cfg.IsrDir, cfg.IsrEnabled),
		eventTransport: eventTransport,
		staticDecls:    make(map[string]*isr.Declaration),
	}
	// 启动重载 ISR：变更事件不做持久化，停机期间漏收的失效无补偿通道，
	// 重启不沿用上次运行的物化产物（清空后懒回源重新物化）
	s.reloadISR()
	return s
}

// reloadISR 启动重载：清空上次运行的物化产物（不沿用旧文件，懒回源重新物化）。
func (s *Server) reloadISR() {
	if cleared := s.isrStore.ClearAll(); cleared > 0 {
		log.Printf("isr: startup reload cleared %d materialized files", cleared)
	}
}

// Config 返回服务器使用的配置（值拷贝，只读用途）。
func (s *Server) Config() config.Config {
	return s.config
}

// pageCacheCapacity 是页面缓存最大条目数（均值 ~30KB/页 → ~30MB）。
const pageCacheCapacity = 1000

// InvalidatePage 使指定路径的全部缓存变体（任意 query/data）失效。
// 手动失效入口：业务数据变更后调用；未来 ISR/DataChange 事件也挂在这里。
func (s *Server) InvalidatePage(path string) {
	s.pageCache.InvalidatePrefix(path + "|")
}

// ValidatePagePattern 校验页面 pattern 是否合法。
// 校验失败时节流重拉一次 Node 页面列表（Node 可能新增了页面），再校验一次。
func (s *Server) ValidatePagePattern(pattern string) error {
	s.patternMu.RLock()
	err := s.patterns.Validate(pattern)
	s.patternMu.RUnlock()
	if err == nil {
		return nil
	}
	if !s.refetchPatterns() {
		return err
	}
	s.patternMu.RLock()
	defer s.patternMu.RUnlock()
	return s.patterns.Validate(pattern)
}

// patternRefetchInterval 是 pattern 重拉的最小间隔（节流）。
const patternRefetchInterval = 10 * time.Second

// refetchPatterns 从 Node 重拉页面列表并换入新校验器。
// 节流：距上次重拉不足 patternRefetchInterval 时直接返回 false。
func (s *Server) refetchPatterns() bool {
	s.patternMu.Lock()
	defer s.patternMu.Unlock()
	if time.Since(s.lastRefetch) < patternRefetchInterval {
		return false
	}
	s.lastRefetch = time.Now()
	validator, err := pagepattern.Fetch(context.Background(), s.config.NodeWorkerURL, s.config.InternalToken, s.config.NodeSubmitTimeout)
	if err != nil {
		log.Printf("refetch page patterns failed: %v", err)
		return false
	}
	s.patterns = validator
	log.Printf("refetched page patterns from node")
	return true
}

// RegisterRole 注册一个角色（权限等级）。
func (s *Server) RegisterRole(role string, parents []string) error {
	return s.auth.Register(role, parents)
}

// ResolveRoles 把角色名列表解析为页面所需的等级列表。
func (s *Server) ResolveRoles(roles []string) ([]int64, error) {
	return s.auth.Resolve(roles)
}

// GrantAuth 放行函数：为已注册的角色生成会话令牌并下发鉴权 cookie。
// 业务层在用户校验（登录）通过后调用；未注册的角色拒绝放行。
func (s *Server) GrantAuth(ctx *fiber.Ctx, role string) error {
	if _, err := s.auth.Resolve([]string{role}); err != nil {
		return err
	}
	token, err := s.sessions.Grant(role)
	if err != nil {
		return err
	}
	auth.SetAuthCookies(ctx, token, role, s.sessions.TTL())
	return nil
}

// RevokeAuth 注销当前请求的会话并清除鉴权 cookie（登出）。
func (s *Server) RevokeAuth(ctx *fiber.Ctx) {
	s.sessions.Revoke(ctx.Cookies(auth.AuthCookieName))
	auth.ClearAuthCookies(ctx)
}

// CookieAuth 从请求的 ven_auth cookie 中解析用户角色：
// 拿令牌到会话缓存里比对，不存在或已过期返回 false。
func (s *Server) CookieAuth(ctx *fiber.Ctx) (role string, ok bool) {
	return s.sessions.Role(ctx.Cookies(auth.AuthCookieName))
}

// CheckAuth 检查用户角色是否满足页面所需的任意等级。
func (s *Server) CheckAuth(role string, pageLevels []int64) (bool, error) {
	return s.auth.Check(role, pageLevels)
}

// App 返回底层的 Fiber 应用实例。
func (s *Server) App() *fiber.App {
	return s.app
}

// EventTransport 返回事件跨实例传输（未配置 Redis 时为 nil，hybrid 接线事件总线用）。
func (s *Server) EventTransport() event.Transport {
	return s.eventTransport
}
