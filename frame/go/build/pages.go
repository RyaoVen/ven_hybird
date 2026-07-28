// Package build 业务层：角色与页面的注册入口，以及各页面的业务 handler。
package build

import (
	"ven_hybird/hybrid"
)

// Register 注册业务角色与页面。
// pattern 必须与 Node 页面路由权威列表（nodePagesPattern）一致，否则启动即失败。
func Register(a *hybrid.App) error {
	// CookieAuth 实装后按会话缓存鉴权；先注册 guest 角色让 demo 登录可放行
	if err := a.RegisterRole("guest", nil); err != nil {
		return err
	}

	registerAuthRoutes(a)

	// 静态页（ISR 物化）：公开、内容稳定，落盘后由中间件直发
	for _, p := range []struct {
		pattern   string
		maxPages  int
		smartLoad bool
		handler   hybrid.PageHandler
	}{
		{"/login", 1, false, emptyPage},
		{"/403", 1, false, emptyPage},
		{"/home", 1, false, homePage},
		{"/about", 1, false, aboutPage},
		{"/news/:id", 10, true, newsPage},
	} {
		if err := a.StaticPage(p.pattern, p.maxPages, p.smartLoad, p.handler); err != nil {
			return err
		}
	}

	// 动态页（鉴权 + 内存缓存）
	if err := a.Page("/blog/:id", []string{"guest"}, blogPage); err != nil {
		return err
	}
	return nil
}

// emptyPage 无业务数据的页面（登录页、错误页等）。
func emptyPage(c *hybrid.PageCtx) error {
	return nil
}

// homePage 首页，返回欢迎数据。
func homePage(c *hybrid.PageCtx) error {
	return c.JSON(map[string]any{
		"title":   "VenHybird",
		"message": "hello from go",
	})
}

// aboutPage 关于页，返回固定数据。
func aboutPage(c *hybrid.PageCtx) error {
	return c.JSON(map[string]any{
		"page": "about",
	})
}

// blogPage 博客详情页，读取路径参数并返回文章数据。
func blogPage(c *hybrid.PageCtx) error {
	id := c.Param("id")
	if id == "" {
		return c.NotFound()
	}
	return c.JSON(map[string]any{
		"id":    id,
		"title": "blog post " + id,
	})
}

// newsPage 新闻详情页（静态页 demo），读取路径参数并返回新闻数据。
func newsPage(c *hybrid.PageCtx) error {
	id := c.Param("id")
	if id == "" {
		return c.NotFound()
	}
	return c.JSON(map[string]any{
		"id":    id,
		"title": "news " + id,
	})
}
