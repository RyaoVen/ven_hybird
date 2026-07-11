package handler

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"ven_hybird/types"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

/**
 * HTTPRequestConfig - HTTP请求客户端配置
 * @property {string} BaseURL - 基础请求地址
 * @property {time.Duration} Timeout - 请求超时时间
 * @property {int} MaxConnsPerHost - 每个主机的最大连接数
 * @property {time.Duration} MaxIdleConnDuration - 穱闲连接保持时间
 */
type HTTPRequestConfig struct {
	BaseURL             string
	Timeout             time.Duration
	MaxConnsPerHost     int
	MaxIdleConnDuration time.Duration
}

/**
 * HTTPResponseConfig - HTTP响应服务端配置
 * @property {int} Port - 服务监听端口
 * @property {bool} Prefork - 是否启用多进程模式
 * @property {bool} DisableStartupMessage - 是否禁用启动消息
 */
type HTTPResponseConfig struct {
	Port                  int
	Prefork               bool
	DisableStartupMessage bool
}

/**
 * HTTPRequest - 封装fasthttp客户端
 * @property {*fasthttp.Client} client - fasthttp客户端实例
 * @property {HTTPRequestConfig} config - 配置信息
 */
type HTTPRequest struct {
	client *fasthttp.Client
	config HTTPRequestConfig
}

/**
 * NewHTTPRequest - 创建HTTPRequest实例
 * @param {HTTPRequestConfig} config - HTTP请求配置
 * @returns {*HTTPRequest} HTTPRequest实例
 */
func NewHTTPRequest(config HTTPRequestConfig) *HTTPRequest {
	// 统一填充默认值到 config 本身，确保后续逻辑一致性
	if config.MaxConnsPerHost == 0 {
		config.MaxConnsPerHost = 100
	}
	if config.MaxIdleConnDuration == 0 {
		config.MaxIdleConnDuration = 30 * time.Second
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	client := &fasthttp.Client{
		MaxConnsPerHost:     config.MaxConnsPerHost,
		MaxIdleConnDuration: config.MaxIdleConnDuration,
		ReadTimeout:         config.Timeout,
		WriteTimeout:        config.Timeout,
	}

	return &HTTPRequest{
		client: client,
		config: config,
	}
}

/**
 * RequestResult - 请求结果
 * @property {[]byte} Body - 响应体
 * @property {map[string]string} Headers - 响应头
 * @property {int} Status - HTTP状态码
 */
type RequestResult struct {
	Body    []byte
	Headers map[string]string
	Status  int
}

/**
 * DoRequest - 执行HTTP请求
 * @param {context.Context} ctx - 上下文（用于取消请求）
 * @param {string} method - 请求方法 (GET, POST, PUT, DELETE等)
 * @param {string} path - 请求路径
 * @param {map[string]string} params - 查询参数
 * @param {[]byte} body - 请求体
 * @param {map[string]string} headers - 请求头
 * @returns {*RequestResult, error} 请求结果和错误
 */
func (r *HTTPRequest) DoRequest(ctx context.Context, method, path string, params map[string]string, body []byte, headers map[string]string) (*RequestResult, error) {
	fullURL := r.buildURL(path, params)

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer func() {
		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(resp)
	}()

	req.SetRequestURI(fullURL)
	req.Header.SetMethod(method)

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	if len(body) > 0 {
		req.SetBody(body)
	}

	// 使用 context 控制请求取消
	// 如果 context 已取消，直接返回错误
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// 计算截止时间：优先使用 context 的 deadline，否则使用配置的超时时间
	deadline := time.Now().Add(r.config.Timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}

	// 使用 DoDeadline 支持截止时间控制
	err := r.client.DoDeadline(req, resp, deadline)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	responseHeaders := make(map[string]string)
	resp.Header.VisitAll(func(key, value []byte) {
		responseHeaders[string(key)] = string(value)
	})

	responseBody := make([]byte, len(resp.Body()))
	copy(responseBody, resp.Body())

	return &RequestResult{
		Body:    responseBody,
		Headers: responseHeaders,
		Status:  resp.StatusCode(),
	}, nil
}

/**
 * Get - 执行GET请求
 * @param {context.Context} ctx - 上下文
 * @param {string} path - 请求路径
 * @param {map[string]string} params - 查询参数
 * @param {map[string]string} headers - 请求头
 * @returns {*RequestResult, error} 请求结果和错误
 */
func (r *HTTPRequest) Get(ctx context.Context, path string, params map[string]string, headers map[string]string) (*RequestResult, error) {
	return r.DoRequest(ctx, fasthttp.MethodGet, path, params, nil, headers)
}

/**
 * Post - 执行POST请求
 * @param {context.Context} ctx - 上下文
 * @param {string} path - 请求路径
 * @param {map[string]string} params - 查询参数
 * @param {[]byte} body - 请求体
 * @param {map[string]string} headers - 请求头
 * @returns {*RequestResult, error} 请求结果和错误
 */
func (r *HTTPRequest) Post(ctx context.Context, path string, params map[string]string, body []byte, headers map[string]string) (*RequestResult, error) {
	return r.DoRequest(ctx, fasthttp.MethodPost, path, params, body, headers)
}

/**
 * buildURL - 构建完整URL
 * @param {string} path - 请求路径（自动补全前导斜杠）
 * @param {map[string]string} params - 查询参数
 * @returns {string} 完整URL
 */
func (r *HTTPRequest) buildURL(path string, params map[string]string) string {
	baseURL := strings.TrimSuffix(r.config.BaseURL, "/")

	// 确保 path 以 / 开头，避免拼接错误
	if len(path) > 0 && path[0] != '/' {
		path = "/" + path
	}

	fullPath := path

	if len(params) > 0 {
		query := url.Values{}
		for key, value := range params {
			query.Set(key, value)
		}
		fullPath = fmt.Sprintf("%s?%s", path, query.Encode())
	}

	return fmt.Sprintf("%s%s", baseURL, fullPath)
}

/**
 * HTTPResponse - 封装fiber应用
 * @property {*fiber.App} app - fiber应用实例
 * @property {HTTPResponseConfig} config - 配置信息
 */
type HTTPResponse struct {
	app    *fiber.App
	config HTTPResponseConfig
}

/**
 * NewHTTPResponse - 创建HTTPResponse实例
 * @param {HTTPResponseConfig} config - HTTP响应配置
 * @returns {*HTTPResponse} HTTPResponse实例
 */
func NewHTTPResponse(config HTTPResponseConfig) *HTTPResponse {
	app := fiber.New(fiber.Config{
		Prefork:               config.Prefork,
		DisableStartupMessage: config.DisableStartupMessage,
	})

	return &HTTPResponse{
		app:    app,
		config: config,
	}
}

/**
 * GetApp - 获取fiber.App实例
 * @returns {*fiber.App} fiber应用实例
 */
func (r *HTTPResponse) GetApp() *fiber.App {
	return r.app
}

/**
 * Start - 启动服务
 * @returns {error} 启动错误
 */
func (r *HTTPResponse) Start() error {
	addr := fmt.Sprintf(":%d", r.config.Port)
	return r.app.Listen(addr)
}

/**
 * Stop - 停止服务
 * @returns {error} 停止错误
 */
func (r *HTTPResponse) Stop() error {
	return r.app.Shutdown()
}

/**
 * RegisterRoute - 注册路由
 * @param {string} method - 请求方法 (GET, POST, PUT, DELETE, PATCH等)
 * @param {string} path - 路由路径
 * @param {fiber.Handler} handler - 路由处理函数
 */
func (r *HTTPResponse) RegisterRoute(method, path string, handler fiber.Handler) {
	switch strings.ToUpper(method) {
	case fiber.MethodGet:
		r.app.Get(path, handler)
	case fiber.MethodPost:
		r.app.Post(path, handler)
	case fiber.MethodPut:
		r.app.Put(path, handler)
	case fiber.MethodDelete:
		r.app.Delete(path, handler)
	case fiber.MethodPatch:
		r.app.Patch(path, handler)
	default:
		r.app.All(path, handler)
	}
}

/**
 * Use - 添加中间件
 * @param {...any} args - 中间件参数
 */
func (r *HTTPResponse) Use(args ...any) {
	r.app.Use(args...)
}

/**
 * HTTPHandler - 服务端通信层处理器
 * @property {*HTTPRequest} request - HTTP请求客户端
 * @property {*HTTPResponse} response - HTTP响应服务端
 */
type HTTPHandler struct {
	request  *HTTPRequest
	response *HTTPResponse
}

/**
 * NewHTTPHandler - 创建HTTPHandler实例
 * @param {HTTPRequestConfig} requestConfig - HTTP请求配置
 * @param {HTTPResponseConfig} responseConfig - HTTP响应配置
 * @returns {*HTTPHandler} HTTPHandler实例
 */
func NewHTTPHandler(requestConfig HTTPRequestConfig, responseConfig HTTPResponseConfig) *HTTPHandler {
	request := NewHTTPRequest(requestConfig)
	response := NewHTTPResponse(responseConfig)

	return &HTTPHandler{
		request:  request,
		response: response,
	}
}

/**
 * GetRequest - 获取HTTPRequest实例
 * @returns {*HTTPRequest} HTTPRequest实例
 */
func (h *HTTPHandler) GetRequest() *HTTPRequest {
	return h.request
}

/**
 * GetResponse - 获取HTTPResponse实例
 * @returns {*HTTPResponse} HTTPResponse实例
 */
func (h *HTTPHandler) GetResponse() *HTTPResponse {
	return h.response
}

/**
 * GetPageHTML - 根据静态页面实例返回HTML
 * @param {context.Context} ctx - 上下文（由调用方控制超时和取消）
 * @param {types.StaticPage} page - 静态页面实例
 * @returns {string, error} HTML内容和错误
 */
func (h *HTTPHandler) GetPageHTML(ctx context.Context, page types.StaticPage) (string, error) {
	if !page.Enabled {
		return "", fmt.Errorf("page is disabled")
	}

	result, err := h.request.Get(ctx, page.Route, nil, nil)
	if err != nil {
		return "", fmt.Errorf("get page html failed: %w", err)
	}

	return string(result.Body), nil
}

/**
 * GetDynamicPageHTML - 根据动态页面实例与参数返回HTML
 *
 * 占位符规则：
 *   - page.Params 存储占位符名称，格式为 ":id"（以冒号开头）
 *   - page.Route 中对应段也使用 ":id" 格式，如 "/user/:id"
 *   - params map 的 key 应为参数名（不带冒号），即 "id"
 *
 * @param {context.Context} ctx - 上下文（由调用方控制超时和取消）
 * @param {types.DynamicPage} page - 动态页面实例
 * @param {map[string]string} params - 动态参数映射（key 为参数名，不带冒号）
 * @returns {string, error} HTML内容和错误
 */
func (h *HTTPHandler) GetDynamicPageHTML(ctx context.Context, page types.DynamicPage, params map[string]string) (string, error) {
	if !page.Enabled {
		return "", fmt.Errorf("page is disabled")
	}

	// 将路由按 / 分段，逐段检查和替换占位符
	// 这避免了 strings.Replace 可能误伤非路由段的问题
	segments := strings.Split(page.Route, "/")
	for i, seg := range segments {
		// 检查是否是占位符段（以 : 开头）
		if strings.HasPrefix(seg, ":") {
			paramName := seg[1:] // 去掉冒号得到参数名，如 ":id" → "id"
			if value, ok := params[paramName]; ok {
				// 对值进行 URL escape，防止特殊字符（/、空格、?等）破坏路由
				segments[i] = url.PathEscape(value)
			}
			// 如果没有提供参数值，保留占位符段
		}
	}

	route := strings.Join(segments, "/")

	result, err := h.request.Get(ctx, route, nil, nil)
	if err != nil {
		return "", fmt.Errorf("get dynamic page html failed: %w", err)
	}

	return string(result.Body), nil
}
