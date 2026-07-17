// Package ssr 提供服务端渲染的核心类型定义。
package ssr

// PageBootstrap 是页面渲染的启动数据，传递给 Node.js 工作节点。
type PageBootstrap struct {
	Route        string            `json:"route"`        // 页面路由路径
	Params       map[string]string `json:"params"`       // 路径参数
	Query        map[string]string `json:"query"`        // 查询参数
	InitialState any               `json:"initialState"` // 初始状态数据
}

// RenderTask 是提交给 Node.js 工作节点的渲染任务。
type RenderTask struct {
	HookID       string        `json:"hookId"`       // 任务唯一标识
	RequestRoute string        `json:"requestRoute"` // 原始请求路由
	Payload      PageBootstrap `json:"payload"`      // 页面启动数据
}

// RenderError 表示渲染过程中的错误。
type RenderError struct {
	Code    string `json:"code"`    // 错误码
	Message string `json:"message"` // 错误描述
}

// RenderCallback 是 Node.js 工作节点回传的渲染结果。
type RenderCallback struct {
	HookID       string       `json:"hookId"`                 // 任务唯一标识
	RequestRoute string       `json:"requestRoute"`           // 原始请求路由
	MatchedRoute string       `json:"matchedRoute,omitempty"` // 实际匹配的路由
	PageName     string       `json:"pageName,omitempty"`     // 页面组件名
	HTML         string       `json:"html"`                   // 渲染的 HTML 内容
	Error        *RenderError `json:"error,omitempty"`        // 渲染错误
	Duration     int64        `json:"duration,omitempty"`     // 渲染耗时(ms)
}
