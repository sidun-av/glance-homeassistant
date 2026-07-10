package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func fakeHAServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/template":
			fmt.Fprint(w, `[
				{"id":"living_room","name":"Living Room","entities":["sensor.lr_temp","light.lr_main"]},
				{"id":"hallway","name":"Hallway","entities":["binary_sensor.front_door"]}
			]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/states":
			fmt.Fprint(w, `[
				{"entity_id":"sensor.lr_temp","state":"21.4","attributes":{"friendly_name":"LR Temp","device_class":"temperature"}},
				{"entity_id":"light.lr_main","state":"on","attributes":{"friendly_name":"LR Main"}},
				{"entity_id":"binary_sensor.front_door","state":"off","attributes":{"friendly_name":"Front Door","device_class":"door"}}
			]`)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/history/period/"):
			now := time.Now().UTC().Format(time.RFC3339)
			fmt.Fprintf(w, `[[{"entity_id":"sensor.lr_temp","state":"21.4","last_changed":"%s"}]]`, now)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func testConfig(haURL string) *Config {
	pause := true
	return &Config{
		HomeAssistant: HomeAssistantConfig{URL: haURL, Token: "test-token"},
		PublicURL:     "/ha-widget",
		Title:         "Home",
		Temperature:   TemperatureConfig{Range: "24h", MaxPoints: 5, ChartHeight: 34, ChartStyle: "sparkline"},
		Live:          LiveConfig{PollInterval: "10s", PauseWhenHidden: &pause},
		Sensors: SensorsConfig{
			ContactDeviceClasses: []string{"door", "window", "garage_door", "opening"},
			MotionDeviceClasses:  []string{"motion", "occupancy"},
		},
	}
}

func TestWidgetHandler_EndToEnd(t *testing.T) {
	ha := fakeHAServer(t)
	defer ha.Close()

	cfg := testConfig(ha.URL)
	mux := newMux(cfg, newApp(cfg))

	req := httptest.NewRequest(http.MethodGet, "/widget", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("Widget-Title") != "Home" {
		t.Errorf("Widget-Title = %q, want Home", rec.Header().Get("Widget-Title"))
	}
	if rec.Header().Get("Widget-Content-Type") != "html" {
		t.Errorf("Widget-Content-Type = %q, want html", rec.Header().Get("Widget-Content-Type"))
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Living Room") {
		t.Errorf("body missing Living Room")
	}
	if !strings.Contains(body, "Front Door") {
		t.Errorf("body missing Front Door")
	}
	if !strings.Contains(body, `data-live-url="/ha-widget/live.json"`) {
		t.Errorf("body missing correct live URL")
	}
}

func TestWidgetHandler_HomeAssistantUnavailable(t *testing.T) {
	ha := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ha.Close()

	cfg := testConfig(ha.URL)
	mux := newMux(cfg, newApp(cfg))

	req := httptest.NewRequest(http.MethodGet, "/widget", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (service owns its degraded state)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Home Assistant unavailable") {
		t.Errorf("body = %s, want unavailable message", rec.Body.String())
	}
}

func TestLiveHandler_EndToEnd(t *testing.T) {
	ha := fakeHAServer(t)
	defer ha.Close()

	cfg := testConfig(ha.URL)
	mux := newMux(cfg, newApp(cfg))

	req := httptest.NewRequest(http.MethodGet, "/live.json", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want *", rec.Header().Get("Access-Control-Allow-Origin"))
	}

	var payload struct {
		Lights  []map[string]any `json:"lights"`
		Sensors []map[string]any `json:"sensors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(payload.Lights) != 1 || len(payload.Sensors) != 1 {
		t.Errorf("payload = %+v, want 1 light room and 1 sensor", payload)
	}
}

func TestHealthzHandler(t *testing.T) {
	cfg := testConfig("http://unused")
	mux := newMux(cfg, newApp(cfg))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestLiveURL(t *testing.T) {
	cases := []struct {
		publicURL string
		want      string
	}{
		{"", "/live.json"},
		{"/ha-widget", "/ha-widget/live.json"},
		{"/ha-widget/", "/ha-widget/live.json"},
	}
	for _, c := range cases {
		if got := liveURL(c.publicURL); got != c.want {
			t.Errorf("liveURL(%q) = %q, want %q", c.publicURL, got, c.want)
		}
	}
}

func TestSparseAxisLabels(t *testing.T) {
	base := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	timestamps := []time.Time{
		base,
		base.Add(6 * time.Hour),
		base.Add(12 * time.Hour),
		base.Add(18 * time.Hour),
		base.Add(24 * time.Hour),
	}
	labels := sparseAxisLabels(timestamps)
	if len(labels) != 5 {
		t.Fatalf("len(labels) = %d, want 5", len(labels))
	}
	if labels[0] == "" || labels[2] == "" || labels[4] == "" {
		t.Errorf("labels = %v, want first/middle/last populated", labels)
	}
	if labels[1] != "" || labels[3] != "" {
		t.Errorf("labels = %v, want the rest empty", labels)
	}
}
