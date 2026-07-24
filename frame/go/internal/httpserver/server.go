// Package httpserver 提供 HTTP 服务器功能。
package httpserver

import (
	"log"
	"time"

	"ven_hybird/internal/auth"
	"ven_hybird/internal/config"
	"ven_hybird/internal/pagecache"
	"ven_hybird/internal/pagepattern"
	"ven_hybird/internal/ssr"

	"github.com/gofiber/fiber/v2"
)

// Server 是 HTTP 服务器核心结构体。
type Server struct {
	app       *fiber.App             // Fiber 应用实例
	config    config.Config          // 应用配置
	ssr       ssr.Client             // SSR 渲染客户端
	pending   *ssr.PendingRegistry   // pending 任务注册中心
	hookIDs   ssr.HookIDGenerator    // HookID 生成器
	auth      *auth.Registry         // 权限等级注册表
	sessions  *auth.SessionStore     // 会话存储（token → role）
	patterns  *pagepattern.Validator // 页面 pattern 校验器
	pageCache *pagecache.Store       // 页面渲染结果缓存
}

// New 创建并初始化 HTTP 服务器实例。
// patterns 为 Node 页面路由模式校验器，启动时通过 pagepattern.Fetch 拉取构建。
func New(
	cfg config.Config,
	client ssr.Client,
	pending *ssr.PendingRegistry,
	hookIDs ssr.HookIDGenerator,
	patterns *pagepattern.Validator,
) *Server {
	app := fiber.New(fiber.Config{
		AppName:               "VenHybird",
		ReadTimeout:           10 * time.Second,
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

	return &Server{
		app:       app,
		config:    cfg,
		ssr:       client,
		pending:   pending,
		hookIDs:   hookIDs,
		auth:      auth.NewRegistry(),
		sessions:  auth.NewSessionStore(auth.NewMemoryBackend(), sessionTTL),
		patterns:  patterns,
		pageCache: pagecache.NewStore(pagecache.NewMemoryBackend(pageCacheCapacity), pageCacheTTL),
	}
}

// sessionTTL 是会话有效期（常量先行，后续再配置化）。
const sessionTTL = 24 * time.Hour

// 页面缓存参数（常量先行，后续再配置化）。
const (
	pageCacheTTL      = time.Minute // 页面缓存有效期
	pageCacheCapacity = 1000        // 页面缓存最大条目数（均值 ~30KB/页 → ~30MB）
)

// InvalidatePage 使指定路径的全部缓存变体（任意 query/data）失效。
// 手动失效入口：业务数据变更后调用；未来 ISR/DataChange 事件也挂在这里。
func (s *Server) InvalidatePage(path string) {
	s.pageCache.InvalidatePrefix(path + "|")
}

// ValidatePagePattern 校验页面 pattern 是否合法。
func (s *Server) ValidatePagePattern(pattern string) error {
	return s.patterns.Validate(pattern)
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
