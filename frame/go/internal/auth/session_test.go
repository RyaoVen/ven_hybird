package auth

import (
	"testing"
	"time"
)

func TestSessionStore_GrantAndRole(t *testing.T) {
	store := NewSessionStore(NewMemoryBackend(), time.Hour)

	token, err := store.Grant("admin")
	if err != nil {
		t.Fatalf("grant failed: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	role, ok := store.Role(token)
	if !ok || role != "admin" {
		t.Fatalf("expected (admin, true), got (%q, %v)", role, ok)
	}
}

func TestSessionStore_UnknownToken(t *testing.T) {
	store := NewSessionStore(NewMemoryBackend(), time.Hour)

	if _, ok := store.Role("forged-token"); ok {
		t.Fatal("expected false for unknown token")
	}
	if _, ok := store.Role(""); ok {
		t.Fatal("expected false for empty token")
	}
}

func TestSessionStore_Revoke(t *testing.T) {
	store := NewSessionStore(NewMemoryBackend(), time.Hour)

	token, err := store.Grant("user")
	if err != nil {
		t.Fatalf("grant failed: %v", err)
	}
	store.Revoke(token)

	if _, ok := store.Role(token); ok {
		t.Fatal("expected false after revoke")
	}
}

func TestMemoryBackend_LazyExpiry(t *testing.T) {
	backend := NewMemoryBackend()
	if err := backend.Set("token", "user", 20*time.Millisecond); err != nil {
		t.Fatalf("set failed: %v", err)
	}

	if _, ok := backend.Get("token"); !ok {
		t.Fatal("expected token to be valid before expiry")
	}

	time.Sleep(30 * time.Millisecond)
	if _, ok := backend.Get("token"); ok {
		t.Fatal("expected token to be expired")
	}
}
