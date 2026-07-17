// HTTP 路由注册。
package httpserver

import "github.com/gofiber/fiber/v2"

// RegisterRoutes 注册所有 HTTP 路由。
func (s *Server) RegisterRoutes() {
	// 内部渲染回调端点：Node.js 工作节点完成渲染后通过此端点回传结果
	s.app.Post("/_internal/render-callback", s.HandleRenderCallback)

	// 健康检查端点：用于负载均衡器和容器编排系统的存活探针
	s.app.Get("/healthz", func(ctx *fiber.Ctx) error {
		return ctx.JSON(fiber.Map{"status": "ok"})
	})

	// 静态资源服务：将 /assets 路径映射到本地 AssetsDir 目录
	s.app.Static("/assets", s.config.AssetsDir)

	// API 路由组：当前为占位实现，所有 /api/* 请求返回 404
	// 预留给后续的 API 接口扩展
	api := s.app.Group("/api")
	api.All("/*", func(ctx *fiber.Ctx) error {
		return ctx.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "api route not found"})
	})

	// 认证路由组：当前为占位实现，所有 /auth/* 请求返回 404
	// 预留给后续的认证功能扩展
	auth := s.app.Group("/auth")
	auth.All("/*", func(ctx *fiber.Ctx) error {
		return ctx.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "auth route not found"})
	})

	// 通用页面渲染路由（兜底）：匹配所有未被上述路由处理的 GET/HEAD 请求
	// 交由 HandlePage 处理器执行 SSR 渲染流程
	s.app.Get("/*", s.HandlePage)
	s.app.Head("/*", s.HandlePage)
}
