package httpserver

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ven_hybird/internal/config"
	"ven_hybird/internal/pagepattern"
	"ven_hybird/internal/ssr"

	"github.com/gofiber/fiber/v2"
)

type stubClient struct{}

func (stubClient) Submit(ctx context.Context, task ssr.RenderTask) error { return nil }

type stubHookIDs struct{}

func (stubHookIDs) New() (string, error) { return "hook", nil }

func newTestServer() *Server {
	cfg := config.Config{NodeSubmitTimeout: time.Second, RenderTimeout: 2 * time.Second}
	return New(cfg, stubClient{}, ssr.NewPendingRegistry(4), stubHookIDs{}, pagepattern.NewValidator(nil))
}

func TestRequestLogger(t *testing.T) {
	var buf bytes.Buffer
	original := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(original)

	s := newTestServer()
	s.App().Get("/ping", func(ctx *fiber.Ctx) error { return ctx.SendString("pong") })

	resp, err := s.App().Test(httptest.NewRequest("GET", "/ping", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if out := buf.String(); !strings.Contains(out, "http: GET /ping 200") {
		t.Fatalf("expected request log line, got %q", out)
	}
}

func TestHealthzCacheStats(t *testing.T) {
	s := newTestServer()
	s.RegisterInternalRoutes()

	resp, err := s.App().Test(httptest.NewRequest("GET", "/healthz", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	out := string(body)
	if !strings.Contains(out, `"status":"ok"`) || !strings.Contains(out, `"pageCache"`) {
		t.Fatalf("unexpected healthz body: %s", out)
	}
}
