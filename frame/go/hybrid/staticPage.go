// 静态页 ISR 的胶水层 API：StaticPage 注册与 DataChange 显式失效。
package hybrid

import (
	"log"

	"ven_hybird/internal/isr"

	"github.com/gofiber/fiber/v2"
)

// StaticPage 注册一个静态页（ISR）：声明即注册 fiber 路由（GET+HEAD，公开无鉴权）。
// dynamicUrl 为完整路由模式（支持多层动态，如 /blog/:id、/:user/blog/:id，纯静态页如 /about）。
// maxPages 为该模式物化文件数上限（0 = 不限）；
// smartLoad 开启时全局 DataChange 按访问热度预重渲染 Top-N，超出上限走 SSR；
// 关闭且设置上限时按 LRU 懒删除（淘汰最久未访问文件）。
//
// 请求流程：ISR 中间件命中物化文件直接返回（不经过本 handler）；
// miss 时执行 handler 取数 → RenderPage（内部完成落盘与上限治理）。
func (a *App) StaticPage(dynamicUrl string, maxPages int, smartLoad bool, h PageHandler) error {
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
	a.staticHandlers[dynamicUrl] = h

	handler := func(ctx *fiber.Ctx) error {
		c := newPageCtx(ctx)
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
// 删除文件与内存缓存后写日志；smartLoad 声明在全局更新时异步按热门预重渲染。
func (a *App) DataChange(pattern string, params ...string) error {
	_, hot, err := a.server.InvalidateStatic(pattern, params)
	if err != nil {
		return err
	}
	if len(hot) > 0 {
		go a.prerenderHot(pattern, hot)
	}
	return nil
}

// prerenderHot 串行预重渲染热门路径（单批次互斥，异步执行）。
func (a *App) prerenderHot(template string, paths []string) {
	a.prerenderMu.Lock()
	defer a.prerenderMu.Unlock()

	decl := a.server.StaticDecl(template)
	handler := a.staticHandlers[template]
	if decl == nil || handler == nil {
		return
	}
	rendered := 0
	for _, path := range paths {
		params, ok := decl.Match(path)
		if !ok {
			continue
		}
		c := newStaticPageCtx(params, map[string]string{})
		if err := handler(c); err != nil {
			log.Printf("isr: prerender %s handler failed: %v", path, err)
			continue
		}
		if c.responded {
			continue // handler 判定 NotFound，跳过
		}
		if err := a.server.RenderStaticPath(path, c.data); err != nil {
			log.Printf("isr: prerender %s failed: %v", path, err)
			continue
		}
		rendered++
	}
	log.Printf("isr: pre-rendered %d/%d hot pages for %s", rendered, len(paths), template)
}
