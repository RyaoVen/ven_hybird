// SSR 渲染任务提交的 HTTP 客户端。
package ssr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client 定义渲染任务提交接口。
type Client interface {
	// Submit 向 Node.js 工作节点提交渲染任务。
	Submit(ctx context.Context, task RenderTask) error
}

// NodeClient 是 Client 接口的 HTTP 实现。
type NodeClient struct {
	baseURL string       // Node.js 工作节点基础 URL
	client  *http.Client // 带超时的 HTTP 客户端
	token   string       // 内部认证令牌
}

// NewNodeClient 创建 Node.js SSR 工作节点的 HTTP 客户端。
func NewNodeClient(baseURL string, timeout time.Duration, token string) *NodeClient {
	return &NodeClient{
		// 移除末尾斜杠，确保拼接路径时不会出现双斜杠
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: timeout},
		token:   token,
	}
}

// Submit 向 Node.js 工作节点提交渲染任务，期望返回 202。
func (c *NodeClient) Submit(ctx context.Context, task RenderTask) error {
	// 参数校验：HookID 必须非空，RequestRoute 必须以 "/" 开头
	if task.HookID == "" || task.RequestRoute == "" || !strings.HasPrefix(task.RequestRoute, "/") {
		return fmt.Errorf("invalid render task")
	}

	// 将渲染任务序列化为 JSON 请求体
	body, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("encode render task: %w", err)
	}

	// 构造 POST 请求，目标地址为 /render 端点
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/render", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create render request: %w", err)
	}

	// 设置请求头
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	// 仅在配置了内部令牌时才发送认证头
	if c.token != "" {
		request.Header.Set("X-Ven-Internal-Token", c.token)
	}

	// 发送 HTTP 请求
	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("submit render task: %w", err)
	}
	defer response.Body.Close()

	// Node 端接受任务后应返回 202 Accepted 状态码
	// 其他状态码视为拒绝，读取响应体作为错误信息
	if response.StatusCode != http.StatusAccepted {
		// 限制读取量（最多 4KB），防止恶意响应体导致内存问题
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("render worker returned %d: %s", response.StatusCode, strings.TrimSpace(string(message)))
	}

	return nil
}
