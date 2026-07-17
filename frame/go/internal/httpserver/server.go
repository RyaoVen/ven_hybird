// Package httpserver 提供 HTTP 服务器功能。
package httpserver

import (
	"log"
	"time"

	"ven_hybird/internal/config"
	"ven_hybird/internal/ssr"

	"github.com/gofiber/fiber/v2"
)

// Server 是 HTTP 服务器核心结构体。
type Server struct {
	app     *fiber.App           // Fiber 应用实例
	config  config.Config        // 应用配置
	ssr     ssr.Client           // SSR 渲染客户端
	pending *ssr.PendingRegistry // pending 任务注册中心
	hookIDs ssr.HookIDGenerator  // HookID 生成器
}

// New 创建并初始化 HTTP 服务器实例。
func New(
	cfg config.Config,
	client ssr.Client,
	pending *ssr.PendingRegistry,
	hookIDs ssr.HookIDGenerator,
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
		app:     app,
		config:  cfg,
		ssr:     client,
		pending: pending,
		hookIDs: hookIDs,
	}
}

// App 返回底层的 Fiber 应用实例。
func (s *Server) App() *fiber.App {
	return s.app
}
