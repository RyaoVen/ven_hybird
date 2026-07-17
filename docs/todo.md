# 服务端通信层封装 - 待办事项

## 模块概述
在 `go/http/handler.go` 中实现服务端通信层封装，包含 HTTP 请求客户端和 HTTP 响应服务端的封装。

---

## 一、配置接口定义

### 1.1 HTTPRequestConfig 配置接口
- [x] 定义 `HTTPRequestConfig` 结构体
- [x] 字段：BaseURL (基础请求地址)
- [x] 字段：Timeout (请求超时时间)
- [x] 字段：MaxConnsPerHost (每个主机最大连接数)
- [x] 字段：MaxIdleConnDuration (空闲连接保持时间)

### 1.2 HTTPResponseConfig 配置接口
- [x] 定义 `HTTPResponseConfig` 结构体
- [x] 字段：Port (服务监听端口)
- [x] 字段：Prefork (是否启用多进程模式)
- [x] 字段：DisableStartupMessage (是否禁用启动消息)

---

## 二、HTTPRequest 结构体实现

### 2.1 结构体定义
- [x] 包含 `*fasthttp.Client` 客户端实例
- [x] 包含配置字段

### 2.2 请求方法封装
- [x] 实现 `DoRequest` 方法
  - 入参：ctx、method、path、params、body、headers
  - 返回：RequestResult (Body, Headers, Status)、error
- [x] 支持设置请求方法 (GET/POST 等)
- [x] 支持设置请求头
- [x] 处理查询参数序列化
- [x] 处理请求体
- [x] 解析响应头并返回
- [x] 实现 `Get` 快捷方法
- [x] 实现 `Post` 快捷方法

---

## 三、HTTPResponse 结构体实现

### 3.1 结构体定义
- [x] 包含 `*fiber.App` 实例
- [x] 包含配置字段

### 3.2 服务端方法封装
- [x] 实现服务启动方法 `Start()`
- [x] 实现服务停止方法 `Stop()`
- [x] 实现路由注册方法 `RegisterRoute()`
- [x] 实现中间件注册方法 `Use()`
- [x] 实现 `GetApp()` 获取原始 fiber 实例

---

## 四、HTTPHandler 类型实现

### 4.1 结构体定义
- [x] 内置 `*HTTPRequest` 字段
- [x] 内置 `*HTTPResponse` 字段

### 4.2 创建函数
- [x] 实现 `NewHTTPHandler(requestConfig, responseConfig) (*HTTPHandler, error)`
- [x] 初始化 HTTPRequest 实例
- [x] 初始化 HTTPResponse 实例
- [x] 实现 `GetRequest()` 获取请求实例
- [x] 实现 `GetResponse()` 获取响应实例

---

## 五、页面 HTML 获取方法

### 5.1 GetPageHTML 方法
- [x] 入参：page (types.StaticPage)
- [x] 返回：html (string)
- [x] 实现逻辑：根据静态页面配置获取对应 HTML 内容

### 5.2 GetDynamicPageHTML 方法
- [x] 入参：page (types.DynamicPage)、params (map[string]string)
- [x] 返回：html (string)
- [x] 实现逻辑：根据动态页面配置与参数获取对应 HTML 内容

---

## 六、错误处理

- [x] 返回 error 类型处理错误
- [x] 使用 fmt.Errorf 包装错误信息

---

## 已完成 ✅

所有功能已实现，编译通过。

## 依赖包
- `github.com/gofiber/fiber/v2` - HTTP 服务端框架
- `github.com/valyala/fasthttp` - 高性能 HTTP 客户端

## 文件位置
- `go/http/handler.go` - 主要实现文件