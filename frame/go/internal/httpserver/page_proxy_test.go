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
	"ven_hybird/internal/pagecache"
	"ven_hybird/internal/pagepattern"
	"ven_hybird/internal/ssr"

	"github.com/gofiber/fiber/v2"
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

// 去 fail-open：即使配置缺失（启动校验已拦截，这里直接构造模拟），
// 内部回调也不得放行——无令牌一律 401。
func TestRenderCallback_NoTokenConfiguredRejectsAll(t *testing.T) {
	cfg := config.Config{
		NodeSubmitTimeout: 100 * time.Millisecond,
		RenderTimeout:     time.Second,
		InternalToken:     "",
	}
	s := New(cfg, newChanClient(), ssr.NewPendingRegistry(8), ssr.CryptoHookIDGenerator{}, pagepattern.NewValidator(nil))
	s.RegisterInternalRoutes()

	resp := postCallback(t, s, "anything", `{"hookId":"h","requestRoute":"/x","html":"<p/>"}`)
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
		"missing hookId":       `{"requestRoute":"/x","html":"<p/>"}`,
		"route not slash":      `{"hookId":"h","requestRoute":"x","html":"<p/>"}`,
		"html required":        `{"hookId":"h","requestRoute":"/x"}`,
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

// 回调 route 归属校验：RequestRoute 与 Register 时记录的路由不一致 → 404 拒绝。
func TestRenderCallback_RouteMismatch(t *testing.T) {
	s := newProxyTestServer(newChanClient(), time.Second)
	s.RegisterInternalRoutes()

	if _, cleanup, err := s.pending.Register("hook-1", "/news/1"); err != nil {
		t.Fatalf("register failed: %v", err)
	} else {
		defer cleanup()
	}

	resp := postCallback(t, s, "secret", `{"hookId":"hook-1","requestRoute":"/other","html":"<p>fake</p>"}`)
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	// 拒绝后条目即删：路由正确的同名回调也不得再投递（防伪造回调占位）
	if s.pending.Resolve(ssr.RenderCallback{HookID: "hook-1", RequestRoute: "/news/1", HTML: "<p/>"}) {
		t.Fatal("route-mismatched resolve should have removed the entry")
	}
}

func TestRenderCallback_OK(t *testing.T) {
	s := newProxyTestServer(newChanClient(), time.Second)
	s.RegisterInternalRoutes()

	waiter, cleanup, err := s.pending.Register("hook-1", "/x")
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

// TestRender_SSRResponseNoCache 校验 SSR 页面响应头含 Cache-Control: no-cache
// （页面内容每次渲染可变，防止浏览器/中间层缓存部署前的旧页面；与 SSE 同策略）。
func TestRender_SSRResponseNoCache(t *testing.T) {
	client := newChanClient()
	s := newProxyTestServer(client, time.Second)
	s.RegisterPageFallback()

	respCh := make(chan *http.Response, 1)
	go func() {
		resp, err := s.App().Test(httptest.NewRequest("GET", "/news/1", nil))
		if err != nil {
			t.Errorf("request failed: %v", err)
			respCh <- nil
			return
		}
		respCh <- resp
	}()
	task := recvTask(t, client)
	s.pending.Resolve(ssr.RenderCallback{
		HookID:       task.HookID,
		RequestRoute: task.RequestRoute,
		MatchedRoute: "/news/:id",
		HTML:         "<html>news 1</html>",
	})

	resp := <-respCh
	if resp == nil {
		t.Fatal("request failed (see above)")
	}
	defer resp.Body.Close()
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("expected Cache-Control: no-cache, got %q", cc)
	}
}

// ---- stale-while-revalidate ----

// waitCacheEntry 轮询等待缓存条目更新为目标 HTML（异步刷新完成后断言用）。
func waitCacheEntry(t *testing.T, s *Server, key, wantHTML string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if entry, ok := s.pageCache.Get(key); ok && entry.HTML == wantHTML {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("cache entry not updated in time")
}

// 缓存过期后渲染失败：继续发过期缓存（stale）而不是 502，后台异步回源刷新，
// 刷新成功后新内容替换缓存。
func TestRender_StaleServedOnFailureThenAsyncRefresh(t *testing.T) {
	client := newChanClient()
	cfg := config.Config{
		NodeSubmitTimeout:    100 * time.Millisecond,
		RenderTimeout:        time.Second,
		InternalToken:        "secret",
		PageCacheTTL:         60 * time.Millisecond,
		PageCacheStaleWindow: 200 * time.Millisecond,
	}
	s := New(cfg, client, ssr.NewPendingRegistry(8), ssr.CryptoHookIDGenerator{}, pagepattern.NewValidator(nil))
	key, _ := pagecache.Key("/news/1", nil, map[string]any{})

	// 第一次：成功渲染并回填缓存
	respCh := requestFallback(t, s, "/news/1")
	task := recvTask(t, client)
	s.pending.Resolve(ssr.RenderCallback{
		HookID:       task.HookID,
		RequestRoute: task.RequestRoute,
		MatchedRoute: "/news/:id",
		HTML:         "<html>v1</html>",
	})
	status, body := recvResponse(t, respCh)
	if status != 200 || !strings.Contains(body, "v1") {
		t.Fatalf("expected 200 v1, got %d %q", status, body)
	}

	// 缓存过期后再请求：Node 渲染失败 → 发过期缓存（stale）而不是 502
	time.Sleep(80 * time.Millisecond)
	respCh2 := requestFallback(t, s, "/news/1")
	task2 := recvTask(t, client)
	s.pending.Resolve(ssr.RenderCallback{
		HookID:       task2.HookID,
		RequestRoute: task2.RequestRoute,
		Error:        &ssr.RenderError{Code: "RENDER_FAILED", Message: "boom"},
	})
	status2, body2 := recvResponse(t, respCh2)
	if status2 != 200 || !strings.Contains(body2, "v1") {
		t.Fatalf("expected stale 200 v1, got %d %q", status2, body2)
	}

	// 后台异步刷新：提交了第二个任务；成功后缓存更新为新内容
	refreshTask := recvTask(t, client)
	s.pending.Resolve(ssr.RenderCallback{
		HookID:       refreshTask.HookID,
		RequestRoute: refreshTask.RequestRoute,
		MatchedRoute: "/news/:id",
		HTML:         "<html>v2</html>",
	})
	waitCacheEntry(t, s, key, "<html>v2</html>")

	// 第三次请求：命中新缓存，不再回源
	respCh3 := requestFallback(t, s, "/news/1")
	status3, body3 := recvResponse(t, respCh3)
	if status3 != 200 || !strings.Contains(body3, "v2") {
		t.Fatalf("expected 200 v2, got %d %q", status3, body3)
	}
	select {
	case extra := <-client.tasks:
		t.Fatalf("fresh hit should not submit task, got %+v", extra)
	case <-time.After(150 * time.Millisecond):
	}
}

// PAGE_NOT_FOUND 是 Node 权威判定：即使有过期缓存也不发 stale（页面真没了）。
func TestRender_NotFoundDoesNotServeStale(t *testing.T) {
	client := newChanClient()
	cfg := config.Config{
		NodeSubmitTimeout:    100 * time.Millisecond,
		RenderTimeout:        time.Second,
		InternalToken:        "secret",
		PageCacheTTL:         60 * time.Millisecond,
		PageCacheStaleWindow: 200 * time.Millisecond,
	}
	s := New(cfg, client, ssr.NewPendingRegistry(8), ssr.CryptoHookIDGenerator{}, pagepattern.NewValidator(nil))

	respCh := requestFallback(t, s, "/news/1")
	task := recvTask(t, client)
	s.pending.Resolve(ssr.RenderCallback{
		HookID:       task.HookID,
		RequestRoute: task.RequestRoute,
		MatchedRoute: "/news/:id",
		HTML:         "<html>v1</html>",
	})
	if status, _ := recvResponse(t, respCh); status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}

	time.Sleep(80 * time.Millisecond)
	respCh2 := requestFallback(t, s, "/news/1")
	task2 := recvTask(t, client)
	s.pending.Resolve(ssr.RenderCallback{
		HookID:       task2.HookID,
		RequestRoute: task2.RequestRoute,
		Error:        &ssr.RenderError{Code: "PAGE_NOT_FOUND", Message: "page gone"},
	})
	status2, body2 := recvResponse(t, respCh2)
	if status2 != 404 || !strings.Contains(body2, "page gone") {
		t.Fatalf("expected 404 (no stale for not-found), got %d %q", status2, body2)
	}
}

// TestRender_PendingCapacityBackpressure pending 满时新页面请求 503（背压），不挂起等回调。
func TestRender_PendingCapacityBackpressure(t *testing.T) {
	client := newChanClient()
	cfg := config.Config{
		NodeSubmitTimeout: 100 * time.Millisecond,
		RenderTimeout:     time.Second,
		InternalToken:     "secret",
	}
	// 容量 1：第一个请求占满后，第二个请求应 503
	s := New(cfg, client, ssr.NewPendingRegistry(1), ssr.CryptoHookIDGenerator{}, pagepattern.NewValidator(nil))
	s.RegisterPageFallback()

	// 第一个请求：占用唯一 pending 位（提交后不 Resolve，保持挂起）
	req1 := httptest.NewRequest("GET", "/news/1", nil)
	respCh1 := make(chan *http.Response, 1)
	go func() {
		resp, err := s.App().Test(req1)
		if err != nil {
			t.Errorf("request 1 failed: %v", err)
			respCh1 <- nil
			return
		}
		respCh1 <- resp
	}()
	// 等待任务提交（此时 pending 已占用）
	task := recvTask(t, client)

	// 第二个请求：pending 已满 → 503 立即返回
	resp2, err := s.App().Test(httptest.NewRequest("GET", "/news/2", nil))
	if err != nil {
		t.Fatalf("request 2 failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for pending capacity, got %d", resp2.StatusCode)
	}

	// 释放第一个请求（Resolve 回调），避免挂起的测试 goroutine 泄漏
	s.pending.Resolve(ssr.RenderCallback{
		HookID:       task.HookID,
		RequestRoute: task.RequestRoute,
		MatchedRoute: "/news/:id",
		HTML:         "<html>news 1</html>",
	})
	resp1 := <-respCh1
	if resp1 != nil {
		resp1.Body.Close()
	}
}

// TestRender_ClientDisconnect 客户端断开：renderWithQuery 立即停止等待（不等满渲染超时），
// pending 由 defer remove 清理，Node 迟到回调返回 unknown hook（结果丢弃）。
func TestRender_ClientDisconnect(t *testing.T) {
	client := newChanClient()
	// 渲染超时 5s：断开应远早于超时返回
	s := newProxyTestServer(client, 5*time.Second)

	done := make(chan struct{})
	start := time.Now()
	resultCh := make(chan error, 1)
	go func() {
		_, err := s.renderWithQuery("/news/1", nil, map[string]any{}, done)
		resultCh <- err
	}()

	// 等任务提交（pending 已注册），随后模拟客户端断开
	task := recvTask(t, client)
	close(done)

	select {
	case err := <-resultCh:
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("disconnect should return early, waited %s", elapsed)
		}
		var renderErr *renderError
		if !errors.As(err, &renderErr) || renderErr.status != fiber.StatusRequestTimeout {
			t.Fatalf("expected client-disconnect error, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("renderWithQuery did not return after client disconnect")
	}

	// pending 已清理：Node 完成的任务迟到回调 → Resolve 返回 false（unknown hook）
	if s.pending.Resolve(ssr.RenderCallback{
		HookID:       task.HookID,
		RequestRoute: task.RequestRoute,
		MatchedRoute: "/news/:id",
		HTML:         "<html>late</html>",
	}) {
		t.Fatal("late callback should be rejected after client disconnect")
	}
}

// TestRender_NilClientDone 后台渲染（无客户端）：clientDone 传 nil 时只等回调/超时，
// 行为与断开监听无关（nil channel 分支永不触发）。
func TestRender_NilClientDone(t *testing.T) {
	client := newChanClient()
	s := newProxyTestServer(client, time.Second)

	resultCh := make(chan error, 1)
	go func() {
		_, err := s.renderWithQuery("/news/1", nil, map[string]any{}, nil)
		resultCh <- err
	}()
	task := recvTask(t, client)
	s.pending.Resolve(ssr.RenderCallback{
		HookID:       task.HookID,
		RequestRoute: task.RequestRoute,
		MatchedRoute: "/news/:id",
		HTML:         "<html>bg</html>",
	})

	select {
	case err := <-resultCh:
		if err != nil {
			t.Fatalf("background render should succeed, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("background render did not complete")
	}
	// 与断开无关：回调正常被消费，不产生迟到拒绝
	if s.pending.Resolve(ssr.RenderCallback{HookID: task.HookID, RequestRoute: task.RequestRoute, HTML: "<p/>"}) {
		t.Fatal("resolved entry should have been removed already")
	}
}

