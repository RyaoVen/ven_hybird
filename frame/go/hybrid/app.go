// Package hybrid 是用户引用的胶水层，把 internal/httpserver 的页面渲染能力
// 反包成固定格式的注册函数。
package hybrid

import (
	"ven_hybird/internal/httpserver"
)

// App 是 hybrid 框架应用，底层基于 *httpserver.Server。
type App struct {
	server        *httpserver.Server
	pages         []page
	loginRedirect string // 401 时的登录跳转目标
}

// defaultLoginRedirect 是默认的登录跳转路径。
const defaultLoginRedirect = "/login"

// New 创建 hybrid 应用，注入已构建好的 *httpserver.Server。
func New(server *httpserver.Server) *App {
	return &App{
		server:        server,
		loginRedirect: defaultLoginRedirect,
	}
}

// Server 返回底层的 *httpserver.Server。
func (a *App) Server() *httpserver.Server {
	return a.server
}

// SetLoginRedirect 配置 401（未登录）时的登录跳转目标，默认 /login。
func (a *App) SetLoginRedirect(path string) {
	a.loginRedirect = path
}

// RegisterRole 注册一个角色（权限等级），可指定继承的父角色。
// 需在 Page() 调用前完成角色注册，否则 Page() 解析 role 会失败。
func (a *App) RegisterRole(role string, parents []string) error {
	return a.server.RegisterRole(role, parents)
}

// InvalidatePage 使指定路径的页面缓存失效（业务数据变更后手动调用）。
func (a *App) InvalidatePage(path string) {
	a.server.InvalidatePage(path)
}
