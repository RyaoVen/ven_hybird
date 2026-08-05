// HTTP 路由注册。
package httpserver

import "github.com/gofiber/fiber/v2"

// RegisterInternalRoutes 注册内部与系统路由。
// 注意：fiber 按注册顺序匹配路由，本方法不含页面兜底，
// 业务页面注册完后再调用 RegisterPageFallback。
func (s *Server) RegisterInternalRoutes() {
	// 内部渲染回调端点：Node.js 工作节点完成渲染后通过此端点回传结果
	s.app.Post("/_internal/render-callback", s.HandleRenderCallback)

	// 健康检查端点：用于负载均衡器和容器编排系统的存活探针
	// 附带页面缓存运行计数（命中/回源/共享），供观测
	s.app.Get("/healthz", func(ctx *fiber.Ctx) error {
		hits, misses, shared := s.pageCache.Stats()
		return ctx.JSON(fiber.Map{
			"status":    "ok",
			"pageCache": fiber.Map{"hits": hits, "misses": misses, "shared": shared},
		})
	})

	// 静态资源服务：将 /assets 路径映射到本地 AssetsDir 目录
	s.app.Static("/assets", s.config.AssetsDir)

	// StaticPage 鉴权中间件：命中带鉴权要求的静态页模式时校验会话，
	// 使 ISR 物化直发路径也受鉴权保护（必须先于 ISR 直发中间件注册）。
	s.app.Use(s.staticAuthMiddleware())

	// ISR 静态直发中间件：命中物化文件直接返回，miss 放行到业务页面与兜底
	// 注册在内部端点之后、业务页面之前（业务页面由后续注册流程挂载）
	s.app.Use(s.isrStore.Middleware())
}

// RegisterPageFallback 注册通用页面渲染路由（兜底），
// 匹配所有未被已注册路由处理的 GET/HEAD 请求，交由 HandlePage 执行 SSR。
// 必须在所有具体页面路由注册之后调用，否则会抢先匹配。
// 幂等：重复调用不会重复注册（hybrid.App.Listen 内部已调用一次）。
func (s *Server) RegisterPageFallback() {
	if s.fallbackRegistered {
		return
	}
	s.fallbackRegistered = true
	s.app.Get("/*", s.HandlePage)
	s.app.Head("/*", s.HandlePage)
}
