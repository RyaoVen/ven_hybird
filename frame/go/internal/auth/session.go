// 会话存储与鉴权 cookie。
// 设计：服务端维护 token → role 的会话缓存，鉴权时拿 cookie 里的令牌来缓存比对。
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

const (
	// AuthCookieName 是 HttpOnly 令牌 cookie，后端鉴权的唯一依据。
	AuthCookieName = "ven_auth"
	// RoleCookieName 是 JS 可读的角色 cookie，供前端路由守卫使用。
	RoleCookieName = "ven_role"
)

// Backend 会话存储后端接口。
// 会话值为 role + userID（用户身份）；Redis 等外部存储自行处理序列化。
// 实现此接口即可替换存储，鉴权逻辑不动。
type Backend interface {
	Set(token, role, userID string, ttl time.Duration) error
	Get(token string) (role, userID string, ok bool)
	Delete(token string)
}

// SessionStore 管理会话令牌的发放、查找与注销。
// 令牌生成与过期语义在这里，后端只做 KV 存取。
type SessionStore struct {
	backend Backend
	ttl     time.Duration
}

// NewSessionStore 创建会话存储，backend 为底层 KV 实现，ttl 为会话有效期。
func NewSessionStore(backend Backend, ttl time.Duration) *SessionStore {
	return &SessionStore{backend: backend, ttl: ttl}
}

// Grant 为角色生成一个随机会话令牌并存入后端，userID 为业务用户主键（可为空）。
// 令牌为 24 字节加密随机数的 base64url 编码（32 字符，无填充）。
func (s *SessionStore) Grant(role, userID string) (string, error) {
	buffer := make([]byte, 24)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(buffer)
	if err := s.backend.Set(token, role, userID, s.ttl); err != nil {
		return "", err
	}
	return token, nil
}

// Lookup 查找令牌对应的会话（角色与用户身份），不存在或已过期返回 false。
func (s *SessionStore) Lookup(token string) (role, userID string, ok bool) {
	if token == "" {
		return "", "", false
	}
	return s.backend.Get(token)
}

// Revoke 注销令牌（登出）。
func (s *SessionStore) Revoke(token string) {
	if token != "" {
		s.backend.Delete(token)
	}
}

// TTL 返回会话有效期。
func (s *SessionStore) TTL() time.Duration {
	return s.ttl
}

// memoryEntry 是内存后端里的会话条目。
type memoryEntry struct {
	role      string
	userID    string
	expiresAt time.Time
}

// MemoryBackend 是 Backend 的内存实现（RWMutex + map + 惰性过期）。
type MemoryBackend struct {
	mu       sync.RWMutex
	sessions map[string]memoryEntry
}

// NewMemoryBackend 创建内存会话后端。
func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{sessions: make(map[string]memoryEntry)}
}

// Set 存储 token → 会话（role + userID），ttl 后过期。
func (m *MemoryBackend) Set(token, role, userID string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[token] = memoryEntry{role: role, userID: userID, expiresAt: time.Now().Add(ttl)}
	return nil
}

// Get 查找 token，过期条目惰性删除并返回 false。
func (m *MemoryBackend) Get(token string) (string, string, bool) {
	m.mu.RLock()
	entry, ok := m.sessions[token]
	m.mu.RUnlock()
	if !ok {
		return "", "", false
	}
	if time.Now().After(entry.expiresAt) {
		m.mu.Lock()
		// 双重检查，避免删掉他人刚写入的新条目
		if current, exists := m.sessions[token]; exists && current.expiresAt.Equal(entry.expiresAt) {
			delete(m.sessions, token)
		}
		m.mu.Unlock()
		return "", "", false
	}
	return entry.role, entry.userID, true
}

// Delete 删除 token。
func (m *MemoryBackend) Delete(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, token)
}

// SetAuthCookies 同时下发令牌 cookie（HttpOnly）与角色 cookie（JS 可读）。
func SetAuthCookies(ctx *fiber.Ctx, token, role string, ttl time.Duration) {
	expires := time.Now().Add(ttl)
	ctx.Cookie(&fiber.Cookie{
		Name:     AuthCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HTTPOnly: true,
		SameSite: fiber.CookieSameSiteLaxMode,
	})
	ctx.Cookie(&fiber.Cookie{
		Name:     RoleCookieName,
		Value:    role,
		Path:     "/",
		Expires:  expires,
		HTTPOnly: false,
		SameSite: fiber.CookieSameSiteLaxMode,
	})
}

// ClearAuthCookies 清除两个鉴权 cookie（登出）。
func ClearAuthCookies(ctx *fiber.Ctx) {
	for _, name := range []string{AuthCookieName, RoleCookieName} {
		ctx.Cookie(&fiber.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			Expires:  time.Now().Add(-time.Hour),
			HTTPOnly: name == AuthCookieName,
			SameSite: fiber.CookieSameSiteLaxMode,
		})
	}
}
