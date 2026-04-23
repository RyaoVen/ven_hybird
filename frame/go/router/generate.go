package router

import (
	"bytes"
	"io/fs"
	"path/filepath"
	"strings"
	"ven_hybird/types"
)

// PageRouter 页面路由管理器
// 负责管理静态路由和动态路由页面的注册与查找
type PageRouter struct {
	pages        map[string]types.StaticPage
	dynamicPages []types.DynamicPage
	rootPath     string
}

// NewPageRouter 创建并初始化一个新的页面路由管理器
// 参数:
//   - rootPath: 页面文件的根目录路径
//
// 返回:
//   - *PageRouter: 初始化完成的路由管理器实例
func NewPageRouter(rootPath string) *PageRouter {
	router := &PageRouter{
		pages:        make(map[string]types.StaticPage),
		dynamicPages: make([]types.DynamicPage, 0),
		rootPath:     rootPath,
	}
	router.generateRouter()
	return router
}

// RegisterPage 注册单个静态页面到路由管理器
// 参数:
//   - page: 要注册的页面信息
//
// 注意: 仅当 page.Enabled 为 true 时才会注册
func (router *PageRouter) RegisterPage(page types.StaticPage) {
	if page.Enabled {
		router.pages[page.Route] = page
	}
	return
}

// RegisterPages 批量注册多个静态页面
// 参数:
//   - pages: 可变数量的页面信息
func (router *PageRouter) RegisterPages(pages ...types.StaticPage) {
	for _, page := range pages {
		router.RegisterPage(page)
	}
}

// RegisterDynamicPage 注册单个动态页面到路由管理器
// 参数:
//   - page: 要注册的动态页面信息
//
// 注意: 仅当 page.Enabled 为 true 时才会注册
func (router *PageRouter) RegisterDynamicPage(page types.DynamicPage) {
	if page.Enabled {
		router.dynamicPages = append(router.dynamicPages, page)
	}
	return
}

// RegisterDynamicPages 批量注册多个动态页面
// 参数:
//   - pages: 可变数量的动态页面信息
func (router *PageRouter) RegisterDynamicPages(pages ...types.DynamicPage) {
	for _, page := range pages {
		router.RegisterDynamicPage(page)
	}
}

// GetPages 获取所有已注册的静态页面列表
// 返回:
//   - []types.StaticPage: 静态页面切片
func (router *PageRouter) GetPages() []types.StaticPage {
	result := make([]types.StaticPage, 0, len(router.pages))
	for _, page := range router.pages {
		result = append(result, page)
	}
	return result
}

// GetStaticPageByRoute 根据路由路径查找对应的静态页面
// 参数:
//   - route: 请求的路由路径
//
// 返回:
//   - page: 找到的静态页面信息
//   - ok: 是否找到匹配的页面
func (router *PageRouter) GetStaticPageByRoute(route string) (page types.StaticPage, ok bool) {
	page, ok = router.pages[route]
	return
}

// GetDynamicPageByRoute 根据路由路径查找对应的动态页面
// 参数:
//   - route: 请求的路由路径
//
// 返回:
//   - page: 找到的动态页面信息
//   - ok: 是否找到匹配的页面
func (router *PageRouter) GetDynamicPageByRoute(route string) (page types.DynamicPage, ok bool) {
	for _, page := range router.dynamicPages {
		if matchRoute(page.Route, route) {
			return page, true
		}
	}
	return
}

// GeneratePage 扫描指定目录生成静态页面和动态页面列表
// 参数:
//   - root: 页面文件的根目录路径
//
// 返回:
//   - pages: 生成的静态页面列表
//   - dynamicPages: 生成的动态页面列表
func GeneratePage(root string) (pages []types.StaticPage, dynamicPages []types.DynamicPage) {
	routes, dynamicRouters, dynamicParams, err := dirPath(root)
	if err != nil {
		panic(err)
	}
	return generateStaticPageFromPath(routes), generateDynamicPageFromPath(dynamicRouters, dynamicParams)
}

// generateRouter 内部方法：扫描根目录并自动注册所有页面路由
// 会自动识别静态路由和动态路由（目录名包含 [param] 格式）
func (router *PageRouter) generateRouter() {
	routes, dynamicRouters, dynamicParams, err := dirPath(router.rootPath)
	if err != nil {
		panic(err)
	}
	for _, page := range generateStaticPageFromPath(routes) {
		router.RegisterPage(page)
	}
	for _, page := range generateDynamicPageFromPath(dynamicRouters, dynamicParams) {
		router.RegisterDynamicPage(page)
	}
}

// matchRoute 动态路由匹配
// 参数:
//   - definedRoute: 定义的路由模式（如 "/user/:id"）
//   - requestRoute: 实际请求的路由路径
//
// 返回:
//   - bool: 是否匹配成功
//
// 说明: 支持以冒号开头的动态参数匹配，如 "/user/:id" 可匹配 "/user/123"
func matchRoute(definedRoute string, requestRoute string) bool {
	definedParts := bytes.Split([]byte(definedRoute), []byte("/"))
	requestParts := bytes.Split([]byte(requestRoute), []byte("/"))

	if len(definedParts) != len(requestParts) {
		return false
	}
	for i := 0; i < len(definedParts); i++ {
		if len(definedParts[i]) > 0 && definedParts[i][0] == ':' {
			continue
		}
		if !bytes.Equal(definedParts[i], requestParts[i]) {
			return false
		}
	}
	return true
}

// dirPath 扫描指定目录，查找所有文件名为 "page" 的页面文件，返回其相对路径列表
// 参数:
//   - root: 要扫描的根目录路径
//
// 返回:
//   - routes: 静态路由路径列表
//   - dynamicRouters: 动态路由路径列表（目录名格式为 [param] 会被转换为 :param）
//   - dynamicParams: 动态路由对应的参数列表
//   - err: 扫描过程中的错误
func dirPath(root string) (routes []string, dynamicRouters []string, dynamicParams [][]string, err error) {

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // 忽略错误，继续遍历
		}
		if !d.IsDir() && strings.EqualFold(d.Name(), "page") {

			if rel, e := filepath.Rel(root, path); e == nil {
				rel = filepath.ToSlash(rel)
				segments := strings.Split(rel, "/")
				segments = segments[:len(segments)-1]

				hasParam := false
				params := make([]string, 0)

				for i, seg := range segments {
					if len(seg) >= 2 && seg[0] == '[' && seg[len(seg)-1] == ']' {
						hasParam = true
						paramName := ":" + seg[1:len(seg)-1]
						segments[i] = paramName
						params = append(params, paramName)
					}
				}

				rel = strings.Join(segments, "/")
				rel = strings.TrimPrefix(rel, "/")

				if hasParam {
					dynamicRouters = append(dynamicRouters, rel)
					dynamicParams = append(dynamicParams, params)
				} else {
					routes = append(routes, rel)
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, nil, err
	}
	return routes, dynamicRouters, dynamicParams, nil
}

// generateStaticPageFromPath 将静态路由路径列表转换为静态页面信息列表
// 参数:
//   - routes: 路由路径字符串切片
//
// 返回:
//   - []types.StaticPage: 生成的静态页面信息切片，每个页面的 Name 取路径最后一段，Enabled 默认为 true
func generateStaticPageFromPath(routes []string) []types.StaticPage {
	var pages []types.StaticPage
	for _, route := range routes {
		a := strings.Split(route, "/")
		pages = append(pages, types.StaticPage{
			Name:    a[len(a)-1],
			Route:   route,
			Enabled: true,
		})
	}
	return pages
}

// generateDynamicPageFromPath 将动态路由路径列表和参数列表转换为动态页面信息列表
// 参数:
//   - routes: 路由路径字符串切片
//   - params: 参数列表（每个路由对应的参数数组）
//
// 返回:
//   - []types.DynamicPage: 生成的动态页面信息切片，每个页面的 Name 取路径最后一段，Enabled 默认为 true
func generateDynamicPageFromPath(routes []string, params [][]string) []types.DynamicPage {
	var pages []types.DynamicPage
	for i, route := range routes {
		a := strings.Split(route, "/")
		pages = append(pages, types.DynamicPage{
			Name:    a[len(a)-1],
			Route:   route,
			Enabled: true,
			Params:  params[i],
		})
	}
	return pages
}
