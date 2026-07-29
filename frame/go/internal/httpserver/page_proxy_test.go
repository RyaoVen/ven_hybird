package httpserver

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ven_hybird/internal/config"
	"ven_hybird/internal/pagepattern"
	"ven_hybird/internal/ssr"
)

// chanClient 是 channel 收任务的假 SSR client：测试拿到任务后自行注入回调。
// submitErr 非 nil 时 Submit 直接返回该错误（模拟提交失败路径）。
type chanClient struct {
	tasks     chan ssr.RenderTask
	submitErr error
}

func newChanClient() *chanClient {
	return &chanClient{tasks: make(chan ssr.RenderTask, 4)}
}

func (c *chanClient) Submit(ctx context.Context, task ssr.RenderTask) error {
	if c.submitErr != nil {
		return c.submitErr
	}
	select {
	case c.tasks <- task:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func newProxyTestServer(client ssr.Client, renderTimeout time.Duration) *Server {
	cfg := config.Config{
		NodeSubmitTimeout: 100 * time.Millisecond,
		RenderTimeout:     renderTimeout,
		InternalToken:     "secret",
	}
	return New(cfg, client, ssr.NewPendingRegistry(8), ssr.CryptoHookIDGenerator{}, pagepattern.NewValidator(nil))
}

// ---- HandleRenderCallback 状态码矩阵 ----

func postCallback(t *testing.T, s *Server, token string, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequest("POST", "/_internal/render-callback", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Ven-Internal-Token", token)
	}
	resp, err := s.App().Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func TestRenderCallback_WrongToken(t *testing.T) {
	s := newProxyTestServer(newChanClient(), time.Second)
	s.RegisterInternalRoutes()

	resp := postCallback(t, s, "wrong", `{"hookId":"h","requestRoute":"/x","html":"<p/>"}`)
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestRenderCallback_BadBody(t *testing.T) {
	s := newProxyTestServer(newChanClient(), time.Second)
	s.RegisterInternalRoutes()

	resp := postCallback(t, s, "secret", `{not json`)
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestRenderCallback_InvalidFields(t *testing.T) {
	s := newProxyTestServer(newChanClient(), time.Second)
	s.RegisterInternalRoutes()

	cases := map[string]string{
		"missing hookId":      `{"requestRoute":"/x","html":"<p/>"}`,
		"route not slash":     `{"hookId":"h","requestRoute":"x","html":"<p/>"}`,
		"html required":       `{"hookId":"h","requestRoute":"/x"}`,
		"missing requestRoute": `{"hookId":"h","html":"<p/>"}`,
	}
	for name, body := range cases {
		resp := postCallback(t, s, "secret", body)
		if resp.StatusCode != 400 {
			t.Errorf("%s: expected 400, got %d", name, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func TestRenderCallback_UnknownHook(t *testing.T) {
	s := newProxyTestServer(newChanClient(), time.Second)
	s.RegisterInternalRoutes()

	resp := postCallback(t, s, "secret", `{"hookId":"ghost","requestRoute":"/x","html":"<p/>"}`)
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestRenderCallback_OK(t *testing.T) {
	s := newProxyTestServer(newChanClient(), time.Second)
	s.RegisterInternalRoutes()

	waiter, cleanup, err := s.pending.Register("hook-1")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	defer cleanup()

	resp := postCallback(t, s, "secret", `{"hookId":"hook-1","requestRoute":"/x","html":"<p>ok</p>"}`)
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
	select {
	case callback := <-waiter:
		if callback.HTML != "<p>ok</p>" {
			t.Fatalf("unexpected callback html: %q", callback.HTML)
		}
	case <-time.After(time.Second):
		t.Fatal("callback not delivered to waiter")
	}
}

// ---- renderWithQuery（兜底页渲染）状态码 ----

// requestFallback 发起兜底页请求并返回接收任务/响应的通道对。
func requestFallback(t *testing.T, s *Server, path string) chan *http.Response {
	t.Helper()
	s.RegisterPageFallback()
	respCh := make(chan *http.Response, 1)
	go func() {
		resp, err := s.App().Test(httptest.NewRequest("GET", path, nil))
		if err != nil {
			t.Errorf("request failed: %v", err)
			respCh <- nil
			return
		}
		respCh <- resp
	}()
	return respCh
}

// recvTask 取一个提交给 Node 的渲染任务。
func recvTask(t *testing.T, client *chanClient) ssr.RenderTask {
	t.Helper()
	select {
	case task := <-client.tasks:
		return task
	case <-time.After(2 * time.Second):
		t.Fatal("no render task submitted")
		return ssr.RenderTask{}
	}
}

func recvResponse(t *testing.T, respCh chan *http.Response) (int, string) {
	t.Helper()
	select {
	case resp := <-respCh:
		if resp == nil {
			t.Fatal("request failed (see above)")
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	case <-time.After(3 * time.Second):
		t.Fatal("no response")
		return 0, ""
	}
}

func TestRender_PageNotFound(t *testing.T) {
	client := newChanClient()
	s := newProxyTestServer(client, time.Second)
	respCh := requestFallback(t, s, "/news/1")

	task := recvTask(t, client)
	s.pending.Resolve(ssr.RenderCallback{
		HookID:       task.HookID,
		RequestRoute: task.RequestRoute,
		Error:        &ssr.RenderError{Code: "PAGE_NOT_FOUND", Message: "page gone"},
	})

	status, body := recvResponse(t, respCh)
	if status != 404 || !strings.Contains(body, "page gone") {
		t.Fatalf("expected 404 with message, got %d %q", status, body)
	}
}

func TestRender_RenderError(t *testing.T) {
	client := newChanClient()
	s := newProxyTestServer(client, time.Second)
	respCh := requestFallback(t, s, "/news/1")

	task := recvTask(t, client)
	s.pending.Resolve(ssr.RenderCallback{
		HookID:       task.HookID,
		RequestRoute: task.RequestRoute,
		Error:        &ssr.RenderError{Code: "RENDER_FAILED", Message: "boom"},
	})

	status, _ := recvResponse(t, respCh)
	if status != 502 {
		t.Fatalf("expected 502, got %d", status)
	}
}

func TestRender_CallbackTimeout(t *testing.T) {
	client := newChanClient()
	s := newProxyTestServer(client, 150*time.Millisecond)
	respCh := requestFallback(t, s, "/news/1")

	_ = recvTask(t, client) // 不注入回调，等渲染超时

	status, body := recvResponse(t, respCh)
	if status != 504 || !strings.Contains(body, "timed out") {
		t.Fatalf("expected 504, got %d %q", status, body)
	}
}

func TestRender_SubmitTimeout(t *testing.T) {
	client := newChanClient()
	client.submitErr = context.DeadlineExceeded
	s := newProxyTestServer(client, time.Second)
	respCh := requestFallback(t, s, "/news/1")

	status, _ := recvResponse(t, respCh)
	if status != 504 {
		t.Fatalf("expected 504, got %d", status)
	}
}

func TestRender_SubmitRejected(t *testing.T) {
	client := newChanClient()
	client.submitErr = errors.New("worker down")
	s := newProxyTestServer(client, time.Second)
	respCh := requestFallback(t, s, "/news/1")

	status, _ := recvResponse(t, respCh)
	if status != 502 {
		t.Fatalf("expected 502, got %d", status)
	}
}

func TestRender_SuccessThenCacheHit(t *testing.T) {
	client := newChanClient()
	s := newProxyTestServer(client, time.Second)
	respCh := requestFallback(t, s, "/news/1")

	task := recvTask(t, client)
	s.pending.Resolve(ssr.RenderCallback{
		HookID:       task.HookID,
		RequestRoute: task.RequestRoute,
		MatchedRoute: "/news/:id",
		HTML:         "<html>news 1</html>",
	})

	status, body := recvResponse(t, respCh)
	if status != 200 || !strings.Contains(body, "news 1") {
		t.Fatalf("expected 200 with html, got %d %q", status, body)
	}

	// 二次请求同路径：缓存命中直接返回，不再向 Node 提交任务
	respCh2 := requestFallback(t, s, "/news/1")
	status2, body2 := recvResponse(t, respCh2)
	if status2 != 200 || !strings.Contains(body2, "news 1") {
		t.Fatalf("expected cached 200, got %d %q", status2, body2)
	}
	select {
	case extra := <-client.tasks:
		t.Fatalf("cache hit should not submit task, got %+v", extra)
	case <-time.After(150 * time.Millisecond):
	}
}
