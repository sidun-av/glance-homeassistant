package hass

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchAreas_ParsesRooms(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/template" {
			t.Errorf("path = %s, want /api/template", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer test-token")
		}
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, `[{"id":"living_room","name":"Living Room","entities":["sensor.living_room_temp","light.living_room_main"]},{"id":"bedroom","name":"Bedroom","entities":["light.bedroom_main"]}]`)
	}))
	defer server.Close()

	client := New(server.URL, "test-token")
	rooms, err := client.FetchAreas(context.Background())
	if err != nil {
		t.Fatalf("FetchAreas: %v", err)
	}
	if len(rooms) != 2 {
		t.Fatalf("len(rooms) = %d, want 2", len(rooms))
	}
	if rooms[0].ID != "living_room" || rooms[0].Name != "Living Room" {
		t.Errorf("rooms[0] = %+v, want id=living_room name=\"Living Room\"", rooms[0])
	}
	if len(rooms[0].EntityIDs) != 2 || rooms[0].EntityIDs[0] != "sensor.living_room_temp" {
		t.Errorf("rooms[0].EntityIDs = %v", rooms[0].EntityIDs)
	}
	if rooms[1].Name != "Bedroom" {
		t.Errorf("rooms[1].Name = %q, want Bedroom", rooms[1].Name)
	}
}

func TestFetchAreas_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := New(server.URL, "test-token")
	_, err := client.FetchAreas(context.Background())
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %v, want it to mention status 500", err)
	}
}

func TestFetchAreas_MalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not json`)
	}))
	defer server.Close()

	client := New(server.URL, "test-token")
	_, err := client.FetchAreas(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed response, got nil")
	}
}
