package config

import (
	"os"
	"testing"
	"time"
)

// 测试用强令牌：Load 拒绝空值/默认值，所有用例需显式配置。
const testInternalToken = "test-internal-token-9f2c"

// VEN_COOKIE_SECURE 默认 true：鉴权 cookie 仅 HTTPS 发送（安全默认）。
func TestLoad_CookieSecureDefaultTrue(t *testing.T) {
	t.Setenv("VEN_INTERNAL_TOKEN", testInternalToken)
	t.Setenv("VEN_COOKIE_SECURE", "") // 显式置空，避免本机环境变量干扰断言
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if !cfg.CookieSecure {
		t.Error("VEN_COOKIE_SECURE 默认应为 true")
	}
}

// VEN_COOKIE_SECURE=false：本地 http 开发可关掉 Secure（否则 http 不传 cookie 登不上）。
func TestLoad_CookieSecureEnvDisable(t *testing.T) {
	t.Setenv("VEN_INTERNAL_TOKEN", testInternalToken)
	t.Setenv("VEN_COOKIE_SECURE", "false")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.CookieSecure {
		t.Error("VEN_COOKIE_SECURE=false 时应关闭 Secure")
	}
}

// 内部令牌为安全关键配置：未显式设置（回退 development-token）时拒绝启动。
func TestLoad_InternalTokenDefaultRejected(t *testing.T) {
	old, had := os.LookupEnv("VEN_INTERNAL_TOKEN")
	os.Unsetenv("VEN_INTERNAL_TOKEN")
	t.Cleanup(func() {
		if had {
			os.Setenv("VEN_INTERNAL_TOKEN", old)
		} else {
			os.Unsetenv("VEN_INTERNAL_TOKEN")
		}
	})
	if _, err := Load(); err == nil {
		t.Fatal("默认 development-token 应拒绝启动")
	}
}

// 显式置空同样拒绝启动：内部通道不允许无令牌运行（fail-open 已移除）。
func TestLoad_InternalTokenEmptyRejected(t *testing.T) {
	t.Setenv("VEN_INTERNAL_TOKEN", "")
	if _, err := Load(); err == nil {
		t.Fatal("空令牌应拒绝启动")
	}
}

// 显式配置强令牌后正常启动，令牌按原值生效。
func TestLoad_InternalTokenCustomAccepted(t *testing.T) {
	t.Setenv("VEN_INTERNAL_TOKEN", testInternalToken)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.InternalToken != testInternalToken {
		t.Errorf("InternalToken = %q, want %q", cfg.InternalToken, testInternalToken)
	}
}

// 高可用相关配置（pattern 持久化路径/熔断参数/stale 窗口）按环境变量生效。
func TestLoad_HARelatedConfig(t *testing.T) {
	t.Setenv("VEN_INTERNAL_TOKEN", testInternalToken)
	t.Setenv("VEN_PATTERNS_FILE", "/tmp/patterns.json")
	t.Setenv("VEN_NODE_CIRCUIT_THRESHOLD", "3")
	t.Setenv("VEN_NODE_CIRCUIT_HALF_OPEN", "5s")
	t.Setenv("VEN_PAGE_CACHE_STALE_WINDOW", "2m")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.PatternsFile != "/tmp/patterns.json" {
		t.Errorf("PatternsFile = %q, want /tmp/patterns.json", cfg.PatternsFile)
	}
	if cfg.NodeCircuitThreshold != 3 {
		t.Errorf("NodeCircuitThreshold = %d, want 3", cfg.NodeCircuitThreshold)
	}
	if cfg.NodeCircuitHalfOpen != 5*time.Second {
		t.Errorf("NodeCircuitHalfOpen = %v, want 5s", cfg.NodeCircuitHalfOpen)
	}
	if cfg.PageCacheStaleWindow != 2*time.Minute {
		t.Errorf("PageCacheStaleWindow = %v, want 2m", cfg.PageCacheStaleWindow)
	}
}

// 熔断阈值必须为正：0 拒绝启动（否则一次失败即熔断，语义错误）。
func TestLoad_CircuitThresholdZeroRejected(t *testing.T) {
	t.Setenv("VEN_INTERNAL_TOKEN", testInternalToken)
	t.Setenv("VEN_NODE_CIRCUIT_THRESHOLD", "0")
	if _, err := Load(); err == nil {
		t.Fatal("VEN_NODE_CIRCUIT_THRESHOLD=0 应拒绝启动")
	}
}

// stale 窗口不能为负；0 是合法值（关闭 stale 兜底）。
func TestLoad_StaleWindowNegativeRejected(t *testing.T) {
	t.Setenv("VEN_INTERNAL_TOKEN", testInternalToken)
	t.Setenv("VEN_PAGE_CACHE_STALE_WINDOW", "-1m")
	if _, err := Load(); err == nil {
		t.Fatal("负的 stale 窗口应拒绝启动")
	}
	t.Setenv("VEN_PAGE_CACHE_STALE_WINDOW", "0")
	if cfg, err := Load(); err != nil || cfg.PageCacheStaleWindow != 0 {
		t.Fatalf("stale 窗口 0 应合法（关闭兜底），got cfg=%+v err=%v", cfg, err)
	}
}

// 内存上限（事件总线 pending / SSE 连接数）默认有界，按环境变量覆盖。
func TestLoad_MemoryBoundsConfig(t *testing.T) {
	t.Setenv("VEN_INTERNAL_TOKEN", testInternalToken)
	t.Setenv("VEN_EVENT_MAX_PENDING", "")
	t.Setenv("VEN_SSE_MAX_CONNS", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.EventMaxPending != 1024 {
		t.Errorf("EventMaxPending 默认应为 1024，got %d", cfg.EventMaxPending)
	}
	if cfg.SSEMaxConns != 1000 {
		t.Errorf("SSEMaxConns 默认应为 1000，got %d", cfg.SSEMaxConns)
	}

	t.Setenv("VEN_EVENT_MAX_PENDING", "256")
	t.Setenv("VEN_SSE_MAX_CONNS", "500")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.EventMaxPending != 256 || cfg.SSEMaxConns != 500 {
		t.Errorf("环境变量未生效: EventMaxPending=%d SSEMaxConns=%d", cfg.EventMaxPending, cfg.SSEMaxConns)
	}
}

// 内存上限必须为正：0 拒绝启动（无上限 = 无界增长，正是治理对象）。
func TestLoad_MemoryBoundsZeroRejected(t *testing.T) {
	t.Setenv("VEN_INTERNAL_TOKEN", testInternalToken)
	t.Setenv("VEN_EVENT_MAX_PENDING", "0")
	if _, err := Load(); err == nil {
		t.Fatal("VEN_EVENT_MAX_PENDING=0 应拒绝启动")
	}
	t.Setenv("VEN_EVENT_MAX_PENDING", "1024")
	t.Setenv("VEN_SSE_MAX_CONNS", "0")
	if _, err := Load(); err == nil {
		t.Fatal("VEN_SSE_MAX_CONNS=0 应拒绝启动")
	}
}
