package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sidun-av/glance-homeassistant/internal/hass"
	"github.com/sidun-av/glance-homeassistant/internal/render"
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
			if r.URL.Query().Get("filter_entity_id") == "sun.sun" {
				fmt.Fprintf(w, `[[{"entity_id":"sun.sun","state":"above_horizon","last_changed":"%s"}]]`, now)
			} else {
				fmt.Fprintf(w, `[[{"entity_id":"sensor.lr_temp","state":"21.4","last_changed":"%s"}]]`, now)
			}
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
	// data-lit="false" is the expected value alongside it, and no
	// temperature sensor, so data-chart="false" (see Fix 1 in template.go —
	// only chart-bearing cards grow to fill row space).
	if !strings.Contains(body, `data-room="Hallway" data-lit="false" data-occupied="true" data-chart="false">`) {
		t.Errorf("body missing Hallway's occupied/chart state on its own element")
	}
	if !strings.Contains(body, `data-live-url="/ha-widget/live.json"`) {
		t.Errorf("body missing correct live URL")
	}
}

func TestWidgetHandler_BarsChartStyleIncludesDaytimeBand(t *testing.T) {
	ha := fakeHAServer(t)
	defer ha.Close()

	cfg := testConfig(ha.URL)
	cfg.Temperature.ChartStyle = "bars"
	mux := newMux(cfg, newApp(cfg))

	req := httptest.NewRequest(http.MethodGet, "/widget", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `class="ha-bar-daylight`) {
		t.Errorf("body missing a daytime band div (ha-bar-daylight) — fakeHAServer reports sun.sun as above_horizon for the whole window, so at least one column must have it")
	}
	if strings.Contains(body, `class="ha-room-chart"`) {
		t.Errorf("body contains a ha-room-chart SVG — the bars chart_style is plain HTML (ha-bar-cols) now, not SVG (a light-bulb icon <svg> elsewhere in the card is fine and expected)")
	}
}

// TestWidgetHandler_BarsChartStyle_DaylightBandExtendsPastCurrentIndexUsingSunState
// is the regression test for the "daylight band only ever covers a tiny
// sliver" bug: FetchSunHistory only has data up to "now", and
// nanOutFuture blanked isDaytime for every later-today bucket — even
// though sunrise/sunset for the rest of today is fully deterministic, not
// unknown the way future temperature is. This mocks sun.sun's CURRENT
// state (/api/states/sun.sun, not the history endpoint) with a
// next_setting shortly after the next bucket past "now", and asserts the
// daylight highlight covers more columns than currentIdx alone (not
// stopping exactly at it). Since the Weather port, the highlight is one
// nested inset:0 div per daytime column rather than one positioned div per
// contiguous run, so the assertion counts columns instead of parsing
// left/right percentages.
func TestWidgetHandler_BarsChartStyle_DaylightBandExtendsPastCurrentIndexUsingSunState(t *testing.T) {
	now := time.Now()
	// next_setting is placed 4h01m out, which is past the END of the bucket
	// after the one containing now: bucket ends fall every 2h, so the bucket
	// containing now ends within 2h and the one after it within 4h. That
	// guarantees at least one future bucket is still daytime whatever
	// wall-clock time the test happens to run at. next_rising is later
	// still, which is what puts SolarNoon on its above-the-horizon branch.
	nextSetting := now.Add(4*time.Hour + time.Minute)
	nextRising := now.Add(12 * time.Hour)

	// The window is derived from the sun state now (see
	// hass.BuildCenteredDayBuckets), so the expected column layout has to be
	// derived the same way rather than assumed to be a calendar day.
	sun := hass.SunState{State: "above_horizon", NextRising: nextRising, NextSetting: nextSetting}
	center, ok := sun.SolarNoon()
	if !ok {
		t.Fatal("SolarNoon returned !ok for a fully populated sun state")
	}
	timestamps, currentIdx, _ := hass.BuildCenteredDayBuckets(now, center, 12)

	ha := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/template":
			fmt.Fprint(w, `[{"id":"kitchen","name":"Kitchen","entities":["sensor.kitchen_temp"]}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/states":
			fmt.Fprint(w, `[{"entity_id":"sensor.kitchen_temp","state":"21.4","attributes":{"friendly_name":"Kitchen Temp","device_class":"temperature"}}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/states/sun.sun":
			fmt.Fprintf(w, `{"state":"above_horizon","attributes":{"next_rising":"%s","next_setting":"%s"}}`,
				nextRising.UTC().Format(time.RFC3339), nextSetting.UTC().Format(time.RFC3339))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/history/period/"):
			nowStr := now.UTC().Format(time.RFC3339)
			if r.URL.Query().Get("filter_entity_id") == "sun.sun" {
				// Single history point, above_horizon for the whole
				// elapsed part of the window (StepForwardFill falls back to
				// the first known point for timestamps before it) — isolates
				// this test to the FUTURE portion, which is what the fix
				// changes.
				fmt.Fprintf(w, `[[{"entity_id":"sun.sun","state":"above_horizon","last_changed":"%s"}]]`, nowStr)
			} else {
				fmt.Fprintf(w, `[[{"entity_id":"sensor.kitchen_temp","state":"21.4","last_changed":"%s"}]]`, nowStr)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ha.Close()

	cfg := testConfig(ha.URL)
	cfg.Temperature.ChartStyle = "bars"
	mux := newMux(cfg, newApp(cfg))

	req := httptest.NewRequest(http.MethodGet, "/widget", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()

	// Past history is above_horizon the whole way (see server mock above),
	// so every bucket up to and including currentIdx is daytime; the
	// sun-state fix then keeps every later bucket that still falls before
	// next_setting daytime too. The pre-fix behavior stopped dead at
	// currentIdx.
	wantCols := 0
	for i, ts := range timestamps {
		if i <= currentIdx || ts.Before(nextSetting) {
			wantCols++
		}
	}
	if wantCols <= currentIdx+1 {
		t.Fatalf("test setup is not exercising the fix: next_setting must fall past at least one future bucket (currentIdx=%d, want>%d daytime columns, got %d)", currentIdx, currentIdx+1, wantCols)
	}
	gotCols := strings.Count(body, `<div class="ha-bar-daylight`)
	if gotCols != wantCols {
		t.Errorf("daylight column count = %d, want %d (currentIdx=%d) — the band must extend past currentIdx using sun.sun's next_setting, not stop exactly at it:\n%s", gotCols, wantCols, currentIdx, body)
	}
	// A single unbroken run, anchored at column 0 and rounded off at its
	// end. Counted against the markup only — matching the bare class name
	// would also hit the ".ha-bar-daylight-sunrise{...}" rule in the style
	// block this same body carries.
	if got := strings.Count(body, `-sunrise"`) + strings.Count(body, "-sunrise "); got != 1 {
		t.Errorf("sunrise-rounded column count = %d, want 1 (the run starts at column 0 and is never broken)", got)
	}
	if got := strings.Count(body, `-sunset"`); got != 1 {
		t.Errorf("sunset-rounded column count = %d, want 1", got)
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

func TestLiveHandler_ReachableAtPublicURLPrefix(t *testing.T) {
	// Some reverse proxies (e.g. Nginx Proxy Manager's Custom Locations,
	// configured with just a forward host/port and no trailing slash on
	// proxy_pass) forward the request's full original path instead of
	// stripping the location prefix. When public_url is "/ha-widget", the
	// browser's bootstrap script requests "/ha-widget/live.json" — if the
	// proxy doesn't strip the prefix, that's the literal path this service
	// receives, and it must still resolve instead of 404ing (which the
	// bootstrap script's fetch().catch() swallows silently, so lights and
	// sensors would just never update, with no visible error).
	ha := fakeHAServer(t)
	defer ha.Close()

	cfg := testConfig(ha.URL)
	cfg.PublicURL = "/ha-widget"
	mux := newMux(cfg, newApp(cfg))

	req := httptest.NewRequest(http.MethodGet, "/ha-widget/live.json", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (must be reachable whether or not the reverse proxy strips the public_url prefix)", rec.Code)
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

func TestSparseAxisLabels_FiveTimestampsGivesThreeTierZeroOneLabels(t *testing.T) {
	base := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	timestamps := []time.Time{
		base,
		base.Add(6 * time.Hour),
		base.Add(12 * time.Hour),
		base.Add(18 * time.Hour),
		base.Add(24 * time.Hour),
	}
	labels := sparseAxisLabels(timestamps)

	// The default (narrow-card) view only shows tier <= 1 — this must
	// still be exactly first/middle/last, matching the widget's original
	// fixed behavior, regardless of how many higher-tier candidates exist.
	var shownByDefault []render.AxisLabel
	for _, l := range labels {
		if l.Tier <= 1 {
			shownByDefault = append(shownByDefault, l)
		}
	}
	if len(shownByDefault) != 3 {
		t.Fatalf("tier<=1 labels = %+v, want 3 (first/middle/last)", shownByDefault)
	}
	// Hour-only, no minutes — matches Glance's own WEATHER widget style
	// ("6am 2pm 10pm"), not a bare "HH:MM" clock.
	if shownByDefault[0].Text != "12am" || shownByDefault[0].Tier != 0 {
		t.Errorf("first default label = %+v, want {12am, tier 0}", shownByDefault[0])
	}
	if shownByDefault[1].Text != "12pm" || shownByDefault[1].Tier != 1 {
		t.Errorf("middle default label = %+v, want {12pm, tier 1}", shownByDefault[1])
	}
	// 24h later == the same wall-clock hour — expected, not a bug (see
	// AxisLabelsRow's doc comment history).
	if shownByDefault[2].Text != "12am" || shownByDefault[2].Tier != 0 {
		t.Errorf("last default label = %+v, want {12am, tier 0}", shownByDefault[2])
	}
}

func TestSparseAxisLabels_MorePointsRevealHigherTiers(t *testing.T) {
	base := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	var timestamps []time.Time
	for i := 0; i < 61; i++ {
		timestamps = append(timestamps, base.Add(time.Duration(i)*24*time.Minute))
	}
	labels := sparseAxisLabels(timestamps)

	maxTier := 0
	for _, l := range labels {
		if l.Tier > maxTier {
			maxTier = l.Tier
		}
	}
	if maxTier != 3 {
		t.Errorf("max tier = %d, want 3 (a wide enough range should offer the finest dyadic tier)", maxTier)
	}
	if len(labels) != axisLabelIntervals+1 {
		t.Errorf("len(labels) = %d, want %d (61 evenly spaced timestamps land on all 9 dyadic candidates)", len(labels), axisLabelIntervals+1)
	}
}

func TestSparseAxisLabels_EmptyTimestampsReturnsNil(t *testing.T) {
	if labels := sparseAxisLabels(nil); labels != nil {
		t.Errorf("labels = %+v, want nil", labels)
	}
}

// TestBarColumnTimeLabels_EveryColumnGetsAFullLabel pins the Weather port:
// weather.html emits `index $.TimeLabels $i` for every one of its twelve
// columns and lets CSS decide which are visible at rest (:nth-child(3),
// (7), (11)) while hover reveals the rest — so a sparse Go-side slice with
// only three non-empty entries would leave nine columns with nothing to
// reveal. The abbreviated "12a"/"8a"/"4p" form this used to emit was a
// space-saving invention that Weather doesn't have; the labels that stay
// visible at rest are the inset columns 3/7/11, which have neighbours on
// both sides to overflow into, so the full "12am"/"8am"/"4pm" form no
// longer collides with anything.
func TestBarColumnTimeLabels_EveryColumnGetsAFullLabel(t *testing.T) {
	base := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	var timestamps []time.Time
	for i := 0; i < 12; i++ {
		timestamps = append(timestamps, base.Add(time.Duration(i)*2*time.Hour))
	}
	labels := barColumnTimeLabels(timestamps)

	want := []string{"12am", "2am", "4am", "6am", "8am", "10am", "12pm", "2pm", "4pm", "6pm", "8pm", "10pm"}
	if len(labels) != len(want) {
		t.Fatalf("len(labels) = %d, want %d", len(labels), len(want))
	}
	for i, w := range want {
		if labels[i] != w {
			t.Errorf("labels[%d] = %q, want %q", i, labels[i], w)
		}
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

// TestProjectYesterday_FillsOnlyTheNotYetHappenedTail pins the split: every
// bucket up to and including currentIdx is a measurement and must survive
// untouched, and every bucket after it is a projection.
func TestProjectYesterday_FillsOnlyTheNotYetHappenedTail(t *testing.T) {
	values := []float64{20, 21, 22, math.NaN(), math.NaN(), math.NaN()}
	dayEarlier := []float64{10, 11, 12, 13, 14, 15}

	projectYesterday(values, dayEarlier, 2)

	want := []float64{20, 21, 22, 13, 14, 15}
	for i := range want {
		if values[i] != want[i] {
			t.Errorf("values[%d] = %v, want %v (full result %v)", i, values[i], want[i], values)
		}
	}
}

func TestProjectYesterday_LeavesGapsWhereYesterdayIsUnknown(t *testing.T) {
	values := []float64{20, math.NaN(), math.NaN()}
	dayEarlier := []float64{10, math.NaN(), 12}

	projectYesterday(values, dayEarlier, 0)

	if !math.IsNaN(values[1]) {
		t.Errorf("values[1] = %v, want NaN — a sensor with no history that far back must leave an empty column", values[1])
	}
	if values[2] != 12 {
		t.Errorf("values[2] = %v, want 12", values[2])
	}
	if values[0] != 20 {
		t.Errorf("values[0] = %v, want the measured value untouched", values[0])
	}
}

// A shorter dayEarlier slice must not panic or truncate the measured half.
func TestProjectYesterday_ToleratesShorterProjectionSlice(t *testing.T) {
	values := []float64{20, math.NaN(), math.NaN()}
	projectYesterday(values, []float64{10, 11}, 0)
	if values[1] != 11 {
		t.Errorf("values[1] = %v, want 11", values[1])
	}
	if !math.IsNaN(values[2]) {
		t.Errorf("values[2] = %v, want it left as NaN", values[2])
	}
}
