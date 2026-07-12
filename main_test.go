package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sidun-av/glance-homeassistant/internal/hass"
)

func fakeHAServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/template":
			fmt.Fprint(w, `[
				{"id":"living_room","name":"Living Room","entities":["sensor.lr_temp","light.lr_main"]},
				{"id":"hallway","name":"Hallway","entities":["binary_sensor.front_door","binary_sensor.hall_motion"]}
			]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/states":
			fmt.Fprint(w, `[
				{"entity_id":"sensor.lr_temp","state":"21.4","attributes":{"friendly_name":"LR Temp","device_class":"temperature"}},
				{"entity_id":"light.lr_main","state":"on","attributes":{"friendly_name":"LR Main","icon":"mdi:track-light"}},
				{"entity_id":"binary_sensor.front_door","state":"off","attributes":{"friendly_name":"Front Door","device_class":"door"}},
				{"entity_id":"binary_sensor.hall_motion","state":"on","attributes":{"friendly_name":"Hall Motion","device_class":"motion"}}
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
		Temperature:   TemperatureConfig{Range: "24h", MaxPoints: 5, ChartHeight: 130, ChartStyle: "sparkline"},
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
	if !strings.Contains(body, `class="track-light"`) {
		t.Errorf("body missing the light's real HA icon glyph")
	}
	if !strings.Contains(body, `<div class="ha-chart-axis">`) {
		t.Errorf("body missing the temperature chart's axis labels row")
	}
	if !strings.Contains(body, "Front Door") {
		t.Errorf("body missing Front Door contact badge")
	}
	// Combined into one substring, not separate data-room/data-occupied
	// checks — the widget's static CSS also contains data-occupied="true"
	// on its own (as part of its [data-occupied="true"] attribute
	// selectors), so a bare check would pass even if Hallway's own <div>
	// never got the attribute. Hallway has no lights in this fixture, so
	// data-lit="false" is the expected value alongside it.
	if !strings.Contains(body, `data-room="Hallway" data-lit="false" data-occupied="true">`) {
		t.Errorf("body missing Hallway's occupied state on its own element")
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
		Rooms []map[string]any `json:"rooms"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(payload.Rooms) != 2 {
		t.Errorf("payload = %+v, want 2 rooms with live-updatable data (Living Room's light, Hallway's contact+motion)", payload)
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
	// Hour-only, no minutes — matches Glance's own WEATHER widget style
	// ("6am 2pm 10pm"), not a bare "HH:MM" clock.
	if labels[0] != "12am" {
		t.Errorf("labels[0] = %q, want %q (hour-only, no minutes)", labels[0], "12am")
	}
	if labels[2] != "12pm" {
		t.Errorf("labels[2] = %q, want %q (hour-only, no minutes)", labels[2], "12pm")
	}
}

func TestSizeClassForWeight(t *testing.T) {
	cases := []struct {
		weight int
		want   string
	}{
		{0, ""},
		{2, ""},
		{3, "ha-size-md"},
		{4, "ha-size-md"},
		{5, "ha-size-lg"},
		{9, "ha-size-lg"},
	}
	for _, c := range cases {
		if got := sizeClassForWeight(c.weight); got != c.want {
			t.Errorf("sizeClassForWeight(%d) = %q, want %q", c.weight, got, c.want)
		}
	}
}

func TestRoomCardView_ComputesLitAndOccupiedFromEntities(t *testing.T) {
	card := hass.RoomCard{
		Room:      "Bedroom",
		Lights:    []hass.Light{{EntityID: "light.a", On: false}, {EntityID: "light.b", On: true, Icon: "mdi:led-strip-variant"}},
		Occupancy: []hass.SensorEntity{{Room: "Bedroom", Name: "Bed Motion", Attention: true, Label: "Occupied"}},
		Weight:    3,
	}
	view := roomCardView(card)

	if !view.Lit {
		t.Error("Lit = false, want true (one light is on)")
	}
	if !view.Occupied {
		t.Error("Occupied = false, want true (occupancy sensor attention)")
	}
	if len(view.Lights) != 2 || view.Lights[1].EntityID != "light.b" || !view.Lights[1].On {
		t.Errorf("Lights = %+v", view.Lights)
	}
	if view.SizeClass != "ha-size-md" {
		t.Errorf("SizeClass = %q, want ha-size-md", view.SizeClass)
	}
}

func TestRoomCardView_AllLightsOffAndNoOccupancyIsNotLitOrOccupied(t *testing.T) {
	card := hass.RoomCard{
		Room:   "Office",
		Lights: []hass.Light{{EntityID: "light.a", On: false}},
		Weight: 1,
	}
	view := roomCardView(card)
	if view.Lit {
		t.Error("Lit = true, want false (no light is on)")
	}
	if view.Occupied {
		t.Error("Occupied = true, want false (no occupancy sensor)")
	}
}
