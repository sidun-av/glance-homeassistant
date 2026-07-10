package hass

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestFetchStates_ParsesEntities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/states" {
			t.Errorf("path = %s, want /api/states", r.URL.Path)
		}
		fmt.Fprint(w, `[
			{"entity_id":"sensor.living_room_temp","state":"21.4","attributes":{"friendly_name":"Living Room Temperature","device_class":"temperature"}},
			{"entity_id":"light.living_room_main","state":"on","attributes":{"friendly_name":"Living Room Main Light"}},
			{"entity_id":"binary_sensor.front_door","state":"off","attributes":{"friendly_name":"Front Door","device_class":"door"}},
			{"entity_id":"sensor.unnamed_thing","state":"5","attributes":{}}
		]`)
	}))
	defer server.Close()

	client := New(server.URL, "test-token")
	states, err := client.FetchStates(context.Background())
	if err != nil {
		t.Fatalf("FetchStates: %v", err)
	}
	if len(states) != 4 {
		t.Fatalf("len(states) = %d, want 4", len(states))
	}

	temp := states["sensor.living_room_temp"]
	if temp.Domain != "sensor" || temp.DeviceClass != "temperature" || temp.State != "21.4" {
		t.Errorf("temp entity = %+v", temp)
	}
	if temp.FriendlyName != "Living Room Temperature" {
		t.Errorf("temp.FriendlyName = %q", temp.FriendlyName)
	}

	light := states["light.living_room_main"]
	if light.Domain != "light" || light.State != "on" {
		t.Errorf("light entity = %+v", light)
	}

	door := states["binary_sensor.front_door"]
	if door.Domain != "binary_sensor" || door.DeviceClass != "door" {
		t.Errorf("door entity = %+v", door)
	}

	unnamed := states["sensor.unnamed_thing"]
	if unnamed.FriendlyName != "sensor.unnamed_thing" {
		t.Errorf("unnamed.FriendlyName = %q, want fallback to entity_id", unnamed.FriendlyName)
	}
}

func TestFetchStates_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := New(server.URL, "test-token")
	_, err := client.FetchStates(context.Background())
	if err == nil {
		t.Fatal("expected error for 401 response, got nil")
	}
}

func TestFetchHistory_ParsesAndFiltersNumericStates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/api/history/period/") {
			t.Errorf("path = %s, want prefix /api/history/period/", r.URL.Path)
		}
		filter := r.URL.Query().Get("filter_entity_id")
		if !strings.Contains(filter, "sensor.living_room_temp") || !strings.Contains(filter, "sensor.bedroom_temp") {
			t.Errorf("filter_entity_id = %q, missing expected entity ids", filter)
		}
		if r.URL.Query().Get("end_time") == "" {
			t.Error("end_time query param missing")
		}
		fmt.Fprint(w, `[
			[
				{"entity_id":"sensor.living_room_temp","state":"20.1","last_changed":"2026-07-10T08:00:00Z"},
				{"entity_id":"sensor.living_room_temp","state":"20.4","last_changed":"2026-07-10T09:00:00Z"}
			],
			[
				{"entity_id":"sensor.bedroom_temp","state":"unavailable","last_changed":"2026-07-10T08:30:00Z"},
				{"entity_id":"sensor.bedroom_temp","state":"19.5","last_changed":"2026-07-10T09:30:00Z"}
			]
		]`)
	}))
	defer server.Close()

	client := New(server.URL, "test-token")
	start := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	history, err := client.FetchHistory(context.Background(), []string{"sensor.living_room_temp", "sensor.bedroom_temp"}, start, end)
	if err != nil {
		t.Fatalf("FetchHistory: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("len(history) = %d, want 2", len(history))
	}

	lr := history["sensor.living_room_temp"]
	if len(lr) != 2 || lr[0].Value != 20.1 || lr[1].Value != 20.4 {
		t.Errorf("living room history = %+v", lr)
	}

	br := history["sensor.bedroom_temp"]
	if len(br) != 1 || br[0].Value != 19.5 {
		t.Errorf("bedroom history = %+v, want 1 point (unavailable filtered out)", br)
	}
}

func TestFetchHistory_EmptyEntityListSkipsRequest(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	client := New(server.URL, "test-token")
	history, err := client.FetchHistory(context.Background(), nil, time.Now(), time.Now())
	if err != nil {
		t.Fatalf("FetchHistory: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("len(history) = %d, want 0", len(history))
	}
	if called {
		t.Error("expected no HTTP request for an empty entity list")
	}
}

func TestFetchHistory_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := New(server.URL, "test-token")
	_, err := client.FetchHistory(context.Background(), []string{"sensor.x"}, time.Now(), time.Now())
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}
