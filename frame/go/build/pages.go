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

	for _, p := range []struct {
		pattern string
		roles   []string
		handler hybrid.PageHandler
	}{
		{"/login", nil, emptyPage},
		{"/403", nil, emptyPage},
		{"/home", nil, homePage},
		{"/about", nil, aboutPage},
		{"/blog/:id", []string{"guest"}, blogPage},
	} {
		if err := a.Page(p.pattern, p.roles, p.handler); err != nil {
			return err
		}
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
