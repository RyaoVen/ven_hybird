package hybrid

import (
	"log"

	"github.com/gofiber/fiber/v2"
)

// PageHandler 是页面处理器签名。
type PageHandler func(c *PageCtx) error

// page 记录已注册页面的元数据。
type page struct {
	Pattern    string
	AuthLevels []int64
}

const dataOnlyHeader = "X-Ven-Data-Only"

// Page 注册一个页面路由。
// 注册流程：校验 pattern → 解析 role 为 AuthLevels → 在 fiber 上注册 GET 路由。
// 请求处理流程：（有 role 要求时）cookie 鉴权 + 权限校验 → 执行 handler → 截流 JSON → 按请求头决定 SSR/JSON。
// role 为空的页面默认公开，不做任何鉴权。
func (a *App) Page(pattern string, role []string, h PageHandler) {
	if err := a.server.ValidatePagePattern(pattern); err != nil {
		log.Fatalf("hybrid: page pattern %q invalid: %v", pattern, err)
	}

	levels, err := a.server.ResolveRoles(role)
	if err != nil {
		log.Fatalf("hybrid: resolve roles for page %q failed: %v", pattern, err)
	}

	a.pages = append(a.pages, page{Pattern: pattern, AuthLevels: levels})

	a.server.App().Get(pattern, func(ctx *fiber.Ctx) error {
		// 1. 鉴权：仅当页面有 role 要求时执行，公开页面直接放行
		if len(levels) > 0 {
			userRole, ok := a.server.CookieAuth(ctx)
			if !ok {
				log.Printf("auth: denied %s %s reason=unauthenticated pattern=%s", ctx.Method(), ctx.Path(), pattern)
				return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
			}
			allowed, err := a.server.CheckAuth(userRole, levels)
			if err != nil || !allowed {
				log.Printf("auth: denied %s %s reason=forbidden role=%s pattern=%s", ctx.Method(), ctx.Path(), userRole, pattern)
				return ctx.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
			}
		}

		// 2. 执行用户 handler
		c := newPageCtx(ctx)
		if err := h(c); err != nil {
			return err
		}
		if c.responded {
			return nil
		}

		// 3. 根据请求头或显式 Render() 决定输出格式
		if c.render || ctx.Get(dataOnlyHeader) != "true" {
			return a.server.RenderPage(ctx, c.data)
		}
		return ctx.JSON(c.data)
	})
}
