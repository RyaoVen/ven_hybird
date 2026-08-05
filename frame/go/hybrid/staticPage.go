// 静态页 ISR 的胶水层 API：StaticPage 注册与 DataChange 显式失效。
package hybrid

import (
	"fmt"
	"log"
	"net/url"
	"time"

	"ven_hybird/internal/event"
	"ven_hybird/internal/isr"

	"github.com/gofiber/fiber/v2"
)

// StaticPage 注册一个静态页（ISR）：声明即注册 fiber 路由（GET+HEAD）。
// dynamicUrl 为完整路由模式（支持多层动态，如 /blog/:id、/:user/blog/:id，纯静态页如 /about）。
// maxPages 为该模式物化文件数上限（0 = 不限）；
// smartLoad 开启时全局 DataChange 按访问热度预重渲染 Top-N，超出上限走 SSR；
// 关闭且设置上限时按 LRU 懒删除（淘汰最久未访问文件）。
// roles 为该页的鉴权角色要求（与 Page 一致：nil/空 = 公开；非空 = 需登录且角色匹配）——
// ISR 物化直发路径由 httpserver 中间件校验，miss 回源路径由本 handler 校验，两路都生效。
//
// 请求流程：StaticPage 鉴权中间件（命中带 roles 的模式先校验会话）→
// ISR 中间件命中物化文件直接返回；miss 时执行 handler 取数 → RenderPage（内部完成落盘与上限治理）。
func (a *App) StaticPage(dynamicUrl string, maxPages int, smartLoad bool, roles []string, h PageHandler) error {
	if err := checkPagePatternAllowed(dynamicUrl); err != nil {
		return err
	}
	decl, err := isr.ParseDeclaration(dynamicUrl, maxPages, smartLoad)
	if err != nil {
		return err
	}
	if err := a.server.RegisterStaticPage(decl); err != nil {
		return err
	}
	levels, err := a.server.ResolveRoles(roles)
	if err != nil {
		return fmt.Errorf("hybrid: resolve roles for static page %q failed: %w", dynamicUrl, err)
	}
	a.server.RegisterStaticPageAuth(dynamicUrl, levels)
	a.staticHandlers[dynamicUrl] = h

	handler := func(ctx *fiber.Ctx) error {
		// 鉴权：公开页直接放行；带 roles 的页在静态直发路径已由中间件校验，
		// 此处兜住 miss 回源路径（未命中物化文件时走本 handler）。
		// 直发中间件已拦截未授权，故这里仅需防御性再查一次（幂等）。
		if userRole, status, reason := a.authCheck(ctx, levels); status != fiber.StatusOK {
			log.Printf("auth: denied %s %s reason=%s role=%s static=%s", ctx.Method(), ctx.Path(), reason, userRole, dynamicUrl)
			if isDataOnly(ctx) {
				if status == fiber.StatusUnauthorized {
					ctx.Set(loginPathHeader, a.loginRedirect)
				}
				return ctx.Status(status).JSON(fiber.Map{"error": reason})
			}
			if status == fiber.StatusUnauthorized {
				return ctx.Redirect(a.loginRedirect+"?next="+url.QueryEscape(ctx.OriginalURL()), fiber.StatusFound)
			}
			ctx.Status(fiber.StatusForbidden)
			return a.server.RenderPageAs(ctx, forbiddenPageRoute, nil)
		}

		c := newPageCtx(ctx, a.server)
		if err := h(c); err != nil {
			return err
		}
		if c.responded {
			return nil
		}
		if c.render || ctx.Get(dataOnlyHeader) != "true" {
			return a.server.RenderPage(ctx, c.data)
		}
		return ctx.JSON(c.data)
	}
	a.server.App().Get(dynamicUrl, handler)
	a.server.App().Head(dynamicUrl, handler)
	return nil
}

// DataChange 显式声明数据变更：使受影响页面失效。
// pattern 必须是已注册的 StaticPage 模式；params 按动态段从左到右连续填充——
// 不给 = 全局页更新（整个模式失效），给满 = 局部单页，给一部分 = 子树。
//
// 语义：同步校验后永远异步入队到事件总线，即时返回，不阻塞调用方。
// 总线在静默窗口（默认 5s，持续变更最多等 30s）后合批处理：
// 先删物化文件与内存缓存，再由 smartLoad 声明按热门后台再生落盘；
// 未再生的页面下次访问时懒回源。
func (a *App) DataChange(pattern string, params ...string) error {
	decl := a.server.StaticDecl(pattern)
	if decl == nil {
		return fmt.Errorf("static page not declared: %s", pattern)
	}
	if _, err := decl.BuildMatcher(params); err != nil {
		return err
	}
	a.bus.Enqueue(event.ChangeEvent{Pattern: pattern, Params: params, EnqueuedAt: time.Now()})
	return nil
}

// renderStatic 是事件总线 ② 再生阶段的回源渲染：
// 执行数据函数取数 → SSR 渲染 → 返回 HTML（不落盘；落盘由总线经跨代检查后执行）。
// ok=false 表示跳过（handler 失败、NotFound 或渲染失败），页面留待访问时懒回源。
func (a *App) renderStatic(template string, path string) (string, bool) {
	decl := a.server.StaticDecl(template)
	handler := a.staticHandlers[template]
	if decl == nil || handler == nil {
		return "", false
	}
	params, ok := decl.Match(path)
	if !ok {
		return "", false
	}
	c := newStaticPageCtx(params, map[string]string{}, a.server)
	if err := handler(c); err != nil {
		log.Printf("isr: regen %s handler failed: %v", path, err)
		return "", false
	}
	if c.responded {
		return "", false // handler 判定 NotFound，跳过
	}
	html, err := a.server.RenderStaticHTML(path, c.data)
	if err != nil {
		log.Printf("isr: regen %s render failed: %v", path, err)
		return "", false
	}
	return html, true
}
