package router

import (
	"bytes"
	"io/fs"
	"path/filepath"
	"strings"
)

type Page struct {
	Name    string
	Route   string
	Enabled bool
}

type PageRouter struct {
	pages        map[string]Page
	dynamicPages []Page
	rootPath     string
}

func NewPageRouter(rootPath string) *PageRouter {
	router := &PageRouter{
		pages:        make(map[string]Page),
		dynamicPages: make([]Page, 0),
		rootPath:     rootPath,
	}
	router.generateRouter()
	return router
}

// RegisterPage 注册页面
func (router *PageRouter) RegisterPage(page Page) {
	if page.Enabled {
		router.pages[page.Route] = page
	}
	return
}

func (router *PageRouter) RegisterPages(pages ...Page) {
	for _, page := range pages {
		router.RegisterPage(page)
	}
}

func (router *PageRouter) RegisterDynamicPage(page Page) {
	if page.Enabled {
		router.dynamicPages = append(router.dynamicPages, page)
	}
	return
}

func (router *PageRouter) RegisterDynamicPages(pages ...Page) {
	for _, page := range pages {
		router.RegisterDynamicPage(page)
	}
}

// GetPages 获取页面
func (router *PageRouter) GetPages() []Page {
	result := make([]Page, 0, len(router.pages))
	for _, page := range router.pages {
		result = append(result, page)
	}
	return result
}
func (router *PageRouter) GetPageByRoute(route string) (page Page, ok bool) {
	page, ok = router.pages[route]
	if !ok {
		for _, page := range router.dynamicPages {
			if matchRoute(page.Route, route) {
				return page, true
			}
		}
	}
	return
}

func GeneratePage(root string) (pages []Page, dynamicPages []Page) {
	routes, dynamicRouters, err := dirPath(root)
	if err != nil {
		panic(err)
	}
	return generatePageFromPath(routes), generatePageFromPath(dynamicRouters)
}
func (router *PageRouter) generateRouter() {
	routes, dynamicRouters, err := dirPath(router.rootPath)
	if err != nil {
		panic(err)
	}
	for _, page := range generatePageFromPath(routes) {
		router.RegisterPage(page)
	}
	for _, page := range generatePageFromPath(dynamicRouters) {
		router.RegisterDynamicPages(page)
	}
}

// 动态路由匹配
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
func dirPath(root string) (routes []string, dynamicRouters []string, err error) {

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // 忽略错误，继续遍历
		}
		if !d.IsDir() && strings.EqualFold(d.Name(), "page") {

			if rel, e := filepath.Rel(root, path); e == nil {
				rel = filepath.ToSlash(rel)
				dynamic := strings.Split(rel, "/")
				dynamic = dynamic[:len(dynamic)-1]
				//动态路由判断
				if len(dynamic) > 0 && len(dynamic[len(dynamic)-1]) > 0 && dynamic[len(dynamic)-1][0] == '[' && dynamic[len(dynamic)-1][len(dynamic[len(dynamic)-1])-1] == ']' {
					//转移动态路由目录名格式
					seg := dynamic[len(dynamic)-1]
					if len(seg) >= 2 && seg[0] == '[' && seg[len(seg)-1] == ']' {
						dynamic[len(dynamic)-1] = ":" + seg[1:len(seg)-1]
					}
					//整合追加
					rel = strings.Join(dynamic, "/")
					rel = strings.TrimPrefix(rel, "/")
					dynamicRouters = append(dynamicRouters, rel)
				} else {
					//静态追加
					rel = strings.Join(dynamic, "/")
					rel = strings.TrimPrefix(rel, "/")
					routes = append(routes, rel)
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return routes, dynamicRouters, nil
}

func generatePageFromPath(routes []string) []Page {
	var pages []Page
	for _, route := range routes {
		a := strings.Split(route, "/")
		pages = append(pages, Page{
			Name:    a[len(a)-1],
			Route:   route,
			Enabled: true,
		})
	}
	return pages
}
