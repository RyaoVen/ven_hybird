// Package httpserver 提供 HTTP 服务器功能。
package httpserver

import (
	"log"
	"time"

	"ven_hybird/internal/auth"
	"ven_hybird/internal/config"
	"ven_hybird/internal/pagepattern"
	"ven_hybird/internal/ssr"

	"github.com/gofiber/fiber/v2"
)

// Server 是 HTTP 服务器核心结构体。
type Server struct {
	app      *fiber.App             // Fiber 应用实例
	config   config.Config          // 应用配置
	ssr      ssr.Client             // SSR 渲染客户端
	pending  *ssr.PendingRegistry   // pending 任务注册中心
	hookIDs  ssr.HookIDGenerator    // HookID 生成器
	auth     *auth.Registry         // 权限等级注册表
	patterns *pagepattern.Validator // 页面 pattern 校验器
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
		app:      app,
		config:   cfg,
		ssr:      client,
		pending:  pending,
		hookIDs:  hookIDs,
		auth:     auth.NewRegistry(),
		patterns: patterns,
	}
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

// CookieAuth 从请求 cookie 中解析用户角色（当前为 stub）。
func (s *Server) CookieAuth(ctx *fiber.Ctx) (role string, ok bool) {
	return auth.CookieAuth(ctx)
}

// CheckAuth 检查用户角色是否满足页面所需的任意等级。
func (s *Server) CheckAuth(role string, pageLevels []int64) (bool, error) {
	return s.auth.Check(role, pageLevels)
}

// App 返回底层的 Fiber 应用实例。
func (s *Server) App() *fiber.App {
	return s.app
}
