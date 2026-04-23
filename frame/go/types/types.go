package types

// StaticPage 表示一个静态页面路由信息
// Name: 页面名称
// Route: 页面路由路径
// Enabled: 是否启用该页面
type StaticPage struct {
	Name    string
	Route   string
	Enabled bool
}

// DynamicPage 表示一个动态页面路由信息
// Name: 页面名称
// Route: 页面路由路径
// Enabled: 是否启用该页面
// Params: 动态参数列表（如 [":id", ":slug"]）
type DynamicPage struct {
	Name    string
	Route   string
	Enabled bool
	Params  []string
}
