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
// 请求处理流程：cookie 鉴权 → 权限校验 → 执行 handler → 截流 JSON → 按请求头决定 SSR/JSON → 日志。
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
		// 1. cookie 鉴权
		userRole, ok := a.server.CookieAuth(ctx)
		if !ok {
			return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}

		// 2. 权限校验（页面有 role 要求时才校验）
		if len(levels) > 0 {
			allowed, err := a.server.CheckAuth(userRole, levels)
			if err != nil || !allowed {
				return ctx.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
			}
		}

		// 3. 执行用户 handler
		c := newPageCtx(ctx, a)
		if err := h(c); err != nil {
			return err
		}
		if c.responded {
			return nil
		}

		// 4. 根据请求头或显式 Render() 决定输出格式
		if c.render || ctx.Get(dataOnlyHeader) != "true" {
			return a.server.RenderPage(ctx, c.data)
		}
		return ctx.JSON(c.data)
	})
}
