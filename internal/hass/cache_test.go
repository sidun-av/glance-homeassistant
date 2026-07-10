package hass

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestAreaCache_ServesFromCacheWithinTTL(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		fmt.Fprint(w, `[{"id":"living_room","name":"Living Room","entities":[]}]`)
	}))
	defer server.Close()

	client := New(server.URL, "test-token")
	cache := NewAreaCache(client, time.Minute)

	if _, err := cache.Get(context.Background()); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, err := cache.Get(context.Background()); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("HTTP calls = %d, want 1 (second Get should be served from cache)", got)
	}
}

func TestAreaCache_RefetchesAfterTTL(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		fmt.Fprint(w, `[{"id":"living_room","name":"Living Room","entities":[]}]`)
	}))
	defer server.Close()

	client := New(server.URL, "test-token")
	cache := NewAreaCache(client, 10*time.Millisecond)

	if _, err := cache.Get(context.Background()); err != nil {
		t.Fatalf("Get: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	if _, err := cache.Get(context.Background()); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("HTTP calls = %d, want 2 (cache should refetch after TTL)", got)
	}
}

func TestAreaCache_ServesStaleOnTransientError(t *testing.T) {
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, `[{"id":"living_room","name":"Living Room","entities":[]}]`)
	}))
	defer server.Close()

	client := New(server.URL, "test-token")
	cache := NewAreaCache(client, 10*time.Millisecond)

	rooms, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	if len(rooms) != 1 {
		t.Fatalf("len(rooms) = %d, want 1", len(rooms))
	}

	fail.Store(true)
	time.Sleep(30 * time.Millisecond)

	staleRooms, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("second Get (should serve stale, not error): %v", err)
	}
	if len(staleRooms) != 1 || staleRooms[0].Name != "Living Room" {
		t.Errorf("staleRooms = %+v, want the previously cached value", staleRooms)
	}
}

func TestAreaCache_ReturnsErrorWhenNoCacheYetAndFetchFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := New(server.URL, "test-token")
	cache := NewAreaCache(client, time.Minute)

	if _, err := cache.Get(context.Background()); err == nil {
		t.Fatal("expected error on first Get when HA is unreachable and no cache exists, got nil")
	}
}
