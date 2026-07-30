package hybrid

import (
	"fmt"
	"log"
	"net/url"

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

// loginPathHeader 是 401 响应携带登录跳转目标的响应头（SPA router 读取它决定跳哪里）。
const loginPathHeader = "X-Ven-Login-Path"

// forbiddenPageRoute 是 403 错误页的路由（原地渲染，不跳转）。
const forbiddenPageRoute = "/403"

// isDataOnly 判断请求是否只要数据（走 JSON 响应而非页面跳转/渲染）。
func isDataOnly(ctx *fiber.Ctx) bool {
	return ctx.Get(dataOnlyHeader) == "true"
}

// Page 注册一个页面路由（GET + HEAD）。
// 注册流程：/api 前缀检查 → 校验 pattern → 解析 role 为 AuthLevels → 在 fiber 上注册路由；失败返回 error。
// 请求处理流程：（有 role 要求时）cookie 鉴权 + 权限校验 → 执行 handler → 截流 JSON → 按请求头决定 SSR/JSON。
// role 为空的页面默认公开，不做任何鉴权。
// 鉴权失败：data-only 返回 401/403 裸 JSON；HTML 导航 401 跳登录页、403 原地渲染错误页。
func (a *App) Page(pattern string, role []string, h PageHandler) error {
	if err := checkPagePatternAllowed(pattern); err != nil {
		return err
	}
	if err := a.server.ValidatePagePattern(pattern); err != nil {
		return fmt.Errorf("hybrid: page pattern %q invalid: %w", pattern, err)
	}

	levels, err := a.server.ResolveRoles(role)
	if err != nil {
		return fmt.Errorf("hybrid: resolve roles for page %q failed: %w", pattern, err)
	}

	a.pages = append(a.pages, page{Pattern: pattern, AuthLevels: levels})
	a.pageHandlers[pattern] = h

	handler := func(ctx *fiber.Ctx) error {
		// 1. 鉴权：仅当页面有 role 要求时执行，公开页面直接放行
		userRole, status, reason := a.authCheck(ctx, levels)
		if status != fiber.StatusOK {
			log.Printf("auth: denied %s %s reason=%s role=%s pattern=%s", ctx.Method(), ctx.Path(), reason, userRole, pattern)
			if isDataOnly(ctx) {
				if status == fiber.StatusUnauthorized {
					ctx.Set(loginPathHeader, a.loginRedirect)
				}
				return ctx.Status(status).JSON(fiber.Map{"error": reason})
			}
			if status == fiber.StatusUnauthorized {
				// HTML 导航：302 跳登录页，next 为原始路径（含 query）
				return ctx.Redirect(a.loginRedirect+"?next="+url.QueryEscape(ctx.OriginalURL()), fiber.StatusFound)
			}
			// HTML 导航：原地渲染 403 错误页（URL 不跳转）
			ctx.Status(fiber.StatusForbidden)
			return a.server.RenderPageAs(ctx, forbiddenPageRoute, nil)
		}

		// 2. 执行用户 handler
		c := newPageCtx(ctx, a.server)
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
	}
	a.server.App().Get(pattern, handler)
	a.server.App().Head(pattern, handler)
	return nil
}
