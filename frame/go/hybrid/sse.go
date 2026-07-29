// SSE 实时推送的胶水层：/_internal/sse 端点 + 页面数据重算。
package hybrid

import (
	"bufio"
	"log"

	"ven_hybird/internal/isr"

	"github.com/gofiber/fiber/v2"
)

// sseRoute 是 SSE 订阅端点（内部路由，entry-client 的 SPA router 自动订阅）。
const sseRoute = "/_internal/sse"

// registerSSE 注册 SSE 订阅端点。
// 请求流程：route 参数解析页面（Page/StaticPage 均可订阅）→ 与页面同套 cookie 鉴权
// （拒绝时与页面守卫同形：401/403 裸 JSON，401 带 X-Ven-Login-Path）→ 升级 text/event-stream。
func (a *App) registerSSE() {
	a.server.App().Get(sseRoute, func(ctx *fiber.Ctx) error {
		route := ctx.Query("route")
		if route == "" {
			return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "route is required"})
		}
		pattern, params, levels, ok := a.resolvePage(route)
		if !ok {
			return ctx.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "page not found"})
		}
		if _, status, reason := a.authCheck(ctx, levels); status != fiber.StatusOK {
			log.Printf("sse: denied %s reason=%s route=%s", ctx.IP(), reason, route)
			if status == fiber.StatusUnauthorized {
				ctx.Set(loginPathHeader, a.loginRedirect)
			}
			return ctx.Status(status).JSON(fiber.Map{"error": reason})
		}

		// route 参数本身不是页面 query（数据随 query 变化，订阅时带上当前 query）
		query := ctx.Queries()
		delete(query, "route")
		conn := a.hub.Subscribe(pattern, route, params, query)

		ctx.Set(fiber.HeaderContentType, "text/event-stream")
		ctx.Set(fiber.HeaderCacheControl, "no-cache")
		raw := ctx.Context().Conn()
		ctx.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
			// 写循环在 handler 返回后运行：注销必须挂在写循环退出时，不能在 handler 里 defer
			defer a.hub.Unsubscribe(conn)
			conn.Stream(w, raw)
		})
		return nil
	})
}

// resolvePage 把具体路径解析为已注册页面：pattern + 路径参数 + 鉴权等级。
// Page 与 StaticPage 都可订阅；未注册路径返回 ok=false。
func (a *App) resolvePage(route string) (pattern string, params map[string]string, levels []int64, ok bool) {
	for p := range a.pageHandlers {
		if decl := a.pageDecl(p); decl != nil {
			if extracted, matched := decl.Match(route); matched {
				return p, extracted, a.authLevelsOf(p), true
			}
		}
	}
	for p := range a.staticHandlers {
		if decl := a.pageDecl(p); decl != nil {
			if extracted, matched := decl.Match(route); matched {
				return p, extracted, nil, true // StaticPage 公开无鉴权
			}
		}
	}
	return "", nil, nil, false
}

// pageDecl 解析并缓存页面 pattern 的声明。
func (a *App) pageDecl(pattern string) *isr.Declaration {
	if decl, ok := a.pageDecls[pattern]; ok {
		return decl
	}
	decl, err := isr.ParseDeclaration(pattern, 0, false)
	if err != nil {
		return nil
	}
	a.pageDecls[pattern] = decl
	return decl
}

// authLevelsOf 返回 Page pattern 的鉴权等级。
func (a *App) authLevelsOf(pattern string) []int64 {
	for _, p := range a.pages {
		if p.Pattern == pattern {
			return p.AuthLevels
		}
	}
	return nil
}

// pageData 是 SSE 推送的重取数实现：以无请求静态源执行页面数据函数。
// Page/StaticPage 共用；ok=false 表示跳过（handler 失败或 NotFound）。
func (a *App) pageData(pattern string, params, query map[string]string) (any, bool) {
	handler, ok := a.staticHandlers[pattern]
	if !ok {
		handler, ok = a.pageHandlers[pattern]
	}
	if !ok {
		return nil, false
	}
	c := newStaticPageCtx(params, query)
	if err := handler(c); err != nil {
		log.Printf("sse: data func %s failed: %v", pattern, err)
		return nil, false
	}
	if c.responded {
		return nil, false // NotFound，跳过
	}
	return c.data, true
}
