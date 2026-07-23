package pagepattern

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/pages" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("X-Ven-Internal-Token") != "test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"patterns": []string{"/", "/blog/:slug"},
		})
	}))
	defer server.Close()

	validator, err := Fetch(context.Background(), server.URL, "test-token", 5*time.Second)
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if err := validator.Validate("/blog/:slug"); err != nil {
		t.Fatalf("expected valid pattern, got error: %v", err)
	}
	if err := validator.Validate("/missing"); err == nil {
		t.Fatal("expected error for unknown pattern, got nil")
	}
}

func TestFetch_TokenRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	if _, err := Fetch(context.Background(), server.URL, "wrong-token", 5*time.Second); err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
}
