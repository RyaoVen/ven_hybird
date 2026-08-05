package hybrid

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ven_hybird/internal/ssr"

	"github.com/gofiber/fiber/v2"
)

// startRealServer 起真实监听（SSE 流式响应无法走 app.Test），返回地址。
func startRealServer(t *testing.T, app *App) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = app.Server().App().Listener(ln) }()
	t.Cleanup(func() {
		app.Close() // drain SSE 连接后再关 HTTP
		_ = app.Server().App().ShutdownWithTimeout(time.Second)
	})
	time.Sleep(50 * time.Millisecond)
	return ln.Addr().String()
}

// openSSE 建立 SSE 连接并读完响应头（此时订阅已在 hub 注册），返回帧读取器。
func openSSE(t *testing.T, addr, url string) (*bufio.Reader, net.Conn) {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\n\r\n", url, addr)
	reader := bufio.NewReader(conn)
	status, err := reader.ReadString('\n')
	if err != nil || !strings.Contains(status, "200") {
		t.Fatalf("bad SSE status: %q err=%v", status, err)
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line == "\r\n" {
			break
		}
	}
	return reader, conn
}

// readPushPayload 在期限内读到 event: page-data 帧并解析载荷。
func readPushPayload(t *testing.T, reader *bufio.Reader, conn net.Conn, timeout time.Duration) ssr.PageBootstrap {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	deadline := time.Now().Add(timeout)
	var eventLine, dataLine string
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read frame failed: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, "event: ") {
			eventLine = line
		}
		if strings.HasPrefix(line, "data: ") {
			dataLine = line
			break
		}
	}
	if eventLine != "event: page-data" {
		t.Fatalf("expected page-data event, got %q", eventLine)
	}
	var payload ssr.PageBootstrap
	if err := json.Unmarshal([]byte(strings.TrimPrefix(dataLine, "data: ")), &payload); err != nil {
		t.Fatalf("decode payload failed: %v", err)
	}
	return payload
}

func TestSSE_MissingRoute(t *testing.T) {
	app, _, _, _ := setupTestApp(t)
	resp, err := app.Server().App().Test(httptest.NewRequest("GET", "/_internal/sse", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestSSE_UnknownRoute(t *testing.T) {
	app, _, _, _ := setupTestApp(t)
	resp, err := app.Server().App().Test(httptest.NewRequest("GET", "/_internal/sse?route=/nope", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestSSE_AuthRequired(t *testing.T) {
	app, _, _, _ := setupTestApp(t)
	if err := app.RegisterRole("admin", nil); err != nil {
		t.Fatal(err)
	}
	mustPage(t, app, "/admin/:id", []string{"admin"}, func(c *PageCtx) error {
		return c.JSON(fiber.Map{"id": c.Param("id")})
	})
	resp, err := app.Server().App().Test(httptest.NewRequest("GET", "/_internal/sse?route=/admin/1", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	if resp.Header.Get(loginPathHeader) == "" {
		t.Fatal("expected login path header on 401")
	}
}

func TestSSE_PushStaticPage(t *testing.T) {
	app, _, _, _ := setupTestApp(t)
	shortDebounce(app)
	mustStaticPage(t, app, "/news/:id", 10, false, nil, func(c *PageCtx) error {
		return c.JSON(fiber.Map{"id": c.Param("id")})
	})
	addr := startRealServer(t, app)

	reader, conn := openSSE(t, addr, "/_internal/sse?route=/news/1")
	if err := app.DataChange("/news/:id", "1"); err != nil {
		t.Fatal(err)
	}
	payload := readPushPayload(t, reader, conn, 3*time.Second)
	if payload.Route != "/news/1" || payload.Params["id"] != "1" {
		t.Fatalf("payload mismatch: %+v", payload)
	}
	state, ok := payload.InitialState.(map[string]any)
	if !ok || state["id"] != "1" {
		t.Fatalf("initialState mismatch: %+v", payload.InitialState)
	}
}

func TestSSE_PushDynamicPage(t *testing.T) {
	app, _, _, _ := setupTestApp(t)
	mustPage(t, app, "/ssr/:name", nil, func(c *PageCtx) error {
		return c.JSON(fiber.Map{"name": c.Param("name")})
	})
	addr := startRealServer(t, app)

	reader, conn := openSSE(t, addr, "/_internal/sse?route=/ssr/world")
	// InvalidatePage 联动：动态页精确路径推送
	app.InvalidatePage("/ssr/world")
	payload := readPushPayload(t, reader, conn, 3*time.Second)
	state, ok := payload.InitialState.(map[string]any)
	if !ok || state["name"] != "world" {
		t.Fatalf("initialState mismatch: %+v", payload.InitialState)
	}
}

func TestSSE_PushQueryAware(t *testing.T) {
	app, _, _, _ := setupTestApp(t)
	mustStaticPage(t, app, "/news/:id", 10, false, nil, func(c *PageCtx) error {
		return c.JSON(fiber.Map{"id": c.Param("id"), "tab": c.Query("tab")})
	})
	addr := startRealServer(t, app)

	// 订阅带 query：推送载荷必须带同一 query（数据随 query 变化）
	reader, conn := openSSE(t, addr, "/_internal/sse?route=/news/1&tab=2")
	app.InvalidatePage("/news/1")
	payload := readPushPayload(t, reader, conn, 3*time.Second)
	if payload.Query["tab"] != "2" {
		t.Fatalf("expected query preserved in payload, got %+v", payload.Query)
	}
	state, ok := payload.InitialState.(map[string]any)
	if !ok || state["tab"] != "2" {
		t.Fatalf("expected query-aware data, got %+v", payload.InitialState)
	}
}

// 确认 404 页走 JSON（避免流式挂起 app.Test）。
func TestSSE_NotFoundIsJSON(t *testing.T) {
	app, _, _, _ := setupTestApp(t)
	resp, err := app.Server().App().Test(httptest.NewRequest("GET", "/_internal/sse?route=/nope", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "page not found") {
		t.Fatalf("expected JSON error, got %q", body)
	}
}
