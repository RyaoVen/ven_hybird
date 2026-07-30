package auth

import (
	"testing"
	"time"
)

func TestSessionStore_GrantAndRole(t *testing.T) {
	store := NewSessionStore(NewMemoryBackend(), time.Hour)

	token, err := store.Grant("admin", "u-1")
	if err != nil {
		t.Fatalf("grant failed: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	role, userID, ok := store.Lookup(token)
	if !ok || role != "admin" || userID != "u-1" {
		t.Fatalf("expected (admin, u-1, true), got (%q, %q, %v)", role, userID, ok)
	}
}

// 旧 GrantAuth 路径（userID 为空）Lookup 得 userID 为空字符串。
func TestSessionStore_GrantWithoutUser(t *testing.T) {
	store := NewSessionStore(NewMemoryBackend(), time.Hour)

	token, err := store.Grant("admin", "")
	if err != nil {
		t.Fatalf("grant failed: %v", err)
	}
	role, userID, ok := store.Lookup(token)
	if !ok || role != "admin" || userID != "" {
		t.Fatalf("expected (admin, \"\", true), got (%q, %q, %v)", role, userID, ok)
	}
}

func TestSessionStore_UnknownToken(t *testing.T) {
	store := NewSessionStore(NewMemoryBackend(), time.Hour)

	if _, _, ok := store.Lookup("forged-token"); ok {
		t.Fatal("expected false for unknown token")
	}
	if _, _, ok := store.Lookup(""); ok {
		t.Fatal("expected false for empty token")
	}
}

func TestSessionStore_Revoke(t *testing.T) {
	store := NewSessionStore(NewMemoryBackend(), time.Hour)

	token, err := store.Grant("user", "u-2")
	if err != nil {
		t.Fatalf("grant failed: %v", err)
	}
	store.Revoke(token)

	if _, _, ok := store.Lookup(token); ok {
		t.Fatal("expected false after revoke")
	}
}

func TestMemoryBackend_LazyExpiry(t *testing.T) {
	backend := NewMemoryBackend()
	if err := backend.Set("token", "user", "u-3", 20*time.Millisecond); err != nil {
		t.Fatalf("set failed: %v", err)
	}

	if _, _, ok := backend.Get("token"); !ok {
		t.Fatal("expected token to be valid before expiry")
	}

	time.Sleep(30 * time.Millisecond)
	if _, _, ok := backend.Get("token"); ok {
		t.Fatal("expected token to be expired")
	}
}
