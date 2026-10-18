package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestDevServer is a local development harness, not a test: it runs the
// widget server against a fake Home Assistant with a generic demo home and
// a stored House Control plan, so the widget, the Plan | 3D switch, the
// Dock invite and the 3D page can be looked at in a browser with no real
// Home Assistant anywhere. It is skipped unless GLANCE_HA_DEVSERVER holds
// an address to listen on:
//
//	GLANCE_HA_DEVSERVER=:8099 go test -run TestDevServer -timeout 0 .
//
// then open http://localhost:8099/harness (?theme=light for Glance's light
// palette). /harness mounts /widget the way Glance does, by fetching it and
// assigning it with innerHTML into Glance's widget box markup. Optional
// GLANCE_HA_DEVSERVER_CSS names a directory with Glance's own stylesheets
// (bundle.css, …), served under /harness/css/; without it the page uses a
// small stand-in for Glance's theme variables.
//
// The fake Home Assistant keeps state in memory and only logs service
// calls; it is the only "Home Assistant" this server talks to. Ctrl-C
// stops it.
func TestDevServer(t *testing.T) {
	addr := os.Getenv("GLANCE_HA_DEVSERVER")
	if addr == "" {
		t.Skip("set GLANCE_HA_DEVSERVER=:8099 to run the development harness")
	}
	ha := newDevHA()
	haSrv := httptest.NewServer(ha)
	defer haSrv.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(cfgPath, []byte(devConfigYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	// Set after loading, so no HA_*/HOUSE_* in the environment can point
	// this harness at anything real.
	cfg.HomeAssistant.URL, cfg.HomeAssistant.Token = haSrv.URL, "dev-token"
	cfg.PublicURL = "/ha-widget"
	cfg.LayoutFile = ""
	cfg.House.PlanFile = filepath.Join(dir, "house-plan.json")
	cfg.House.Secret = ""
	cfg.House.Lang = "ru"
	if v := os.Getenv("GLANCE_HA_DEVSERVER_LANG"); v == "en" {
		cfg.House.Lang = v
	}
	if err := os.WriteFile(cfg.House.PlanFile, []byte(devPlanJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	a := newApp(cfg)
	mux := newMux(cfg, a)
	mux.HandleFunc("/harness", devHarnessPage)
	if css := os.Getenv("GLANCE_HA_DEVSERVER_CSS"); css != "" {
		mux.Handle("/harness/css/", http.StripPrefix("/harness/css/", http.FileServer(http.Dir(css))))
	}

	srv := &http.Server{Addr: addr, Handler: devLog(mux)}
	go func() {
		log.Printf("dev server on %s: open /harness (fake Home Assistant at %s)", addr, haSrv.URL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("dev server: %v", err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
	srv.Close()
}

// devLog logs every request the harness serves, so a browser session can
// be checked for what it asked (or did not ask) for.
func devLog(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("dev: %s %s", r.Method, r.URL.RequestURI())
		h.ServeHTTP(w, r)
	})
}

const devConfigYAML = `
home_assistant:
  url: http://ha.invalid
  token: placeholder
public_url: /ha-widget
title: Home
layout: floorplan
floorplan:
  aspect_ratio: "1.6"
  grid:
    - "living living kitchen"
    - "living living hall"
  rooms:
    living: Living Room
    kitchen: Kitchen
    hall: Hallway
devices:
  domains: [climate, vacuum]
temperature:
  chart_style: bars
live:
  poll_interval: 5s
`

const devPlanJSON = `{
  "version": 1,
  "name": "Demo",
  "wallHeight": 2.7,
  "interiorWallHeight": 1.25,
  "camera": {"azimuth": 30, "elevation": 50},
  "rooms": [
    {"id": "living", "name": "Гостиная", "rect": [0, 0, 6, 5], "floor": "walnut"},
    {"id": "kitchen", "name": "Кухня", "rect": [6, 0, 3, 3], "floor": "tile"},
    {"id": "hall", "name": "Прихожая", "rect": [6, 3, 3, 2], "floor": "stone"}
  ],
  "openings": [
    {"type": "door", "from": [6, 3.6], "to": [6, 4.5]},
    {"type": "window", "from": [2, 0], "to": [4, 0]}
  ],
  "furniture": [
    {"type": "sofa", "pos": [3, 4.4], "rot": 180, "width": 2.1},
    {"type": "kitchen", "pos": [7.5, 0.4], "width": 2.4}
  ],
  "lights": [
    {"id": "living.lamp", "name": "Торшер", "room": "living", "type": "floor", "pos": [0.6, 0.6],
     "capabilities": ["brightness", "temp"], "link": {"service": "homeassistant", "id": "light.demo_lamp"}},
    {"id": "kitchen.plug", "name": "Подсветка", "room": "kitchen", "type": "strip", "pos": [7.5, 0.2], "length": 1.5,
     "link": {"service": "homeassistant", "id": "switch.demo_plug"}},
    {"id": "hall.sim", "name": "Simulated", "room": "hall", "type": "ceiling", "pos": [7.5, 4]}
  ],
  "acs": [
    {"id": "living.heater", "name": "Обогреватель", "room": "living", "wall": "west", "pos": [0, 2.5], "y": 2.2,
     "link": {"service": "homeassistant", "id": "climate.demo_heater"},
     "sensor": {"service": "homeassistant", "id": "sensor.demo_temp"}}
  ],
  "vacuums": [
    {"id": "living.robot", "name": "Робот", "room": "living", "pos": [4.5, 1.2],
     "link": {"service": "homeassistant", "id": "vacuum.demo_robot"}}
  ]
}`

// devHA is a fake Home Assistant with a generic demo home.
type devHA struct {
	mu     sync.Mutex
	states map[string]map[string]any // entity_id → {"state", "attributes"}
}

func newDevHA() *devHA {
	st := func(state string, attrs map[string]any) map[string]any {
		return map[string]any{"state": state, "attributes": attrs}
	}
	return &devHA{states: map[string]map[string]any{
		"light.demo_lamp": st("on", map[string]any{"friendly_name": "Demo Lamp", "brightness": 180,
			"supported_color_modes": []string{"color_temp"}, "color_mode": "color_temp", "color_temp_kelvin": 3000,
			"min_color_temp_kelvin": 2200, "max_color_temp_kelvin": 6500}),
		"switch.demo_plug":    st("off", map[string]any{"friendly_name": "Demo Plug"}),
		"climate.demo_heater": st("heat", map[string]any{"friendly_name": "Demo Heater", "hvac_modes": []string{"off", "heat"}, "temperature": 21, "current_temperature": 19.5, "min_temp": 7, "max_temp": 30, "target_temp_step": 0.5}),
		"sensor.demo_temp":    st("20.4", map[string]any{"friendly_name": "Demo Temperature", "device_class": "temperature", "unit_of_measurement": "°C"}),
		"vacuum.demo_robot": st("docked", map[string]any{"friendly_name": "Demo Robot", "fan_speed": "Standard",
			"fan_speed_list": []string{"Quiet", "Standard", "Strong", "Turbo"}, "supported_features": 4 | 8 | 16 | 32 | 512 | 8192}),
		"sensor.demo_robot_battery": st("87", map[string]any{"friendly_name": "Demo Robot Battery", "device_class": "battery", "unit_of_measurement": "%"}),
		"sun.sun":                   st("above_horizon", map[string]any{"next_rising": time.Now().Add(20 * time.Hour).UTC().Format(time.RFC3339), "next_setting": time.Now().Add(4 * time.Hour).UTC().Format(time.RFC3339)}),
	}}
}

func (h *devHA) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/states":
		out := []map[string]any{}
		for id, s := range h.states {
			out = append(out, map[string]any{"entity_id": id, "state": s["state"], "attributes": s["attributes"],
				"last_changed": time.Now().UTC().Format(time.RFC3339)})
		}
		json.NewEncoder(w).Encode(out)
	case r.Method == http.MethodPost && r.URL.Path == "/api/template":
		var body struct{ Template string }
		json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "text/plain")
		if strings.Contains(body.Template, "ns.b") { // house.BatteryTemplate
			io.WriteString(w, "vacuum.demo_robot\tsensor.demo_robot_battery\n")
			return
		}
		io.WriteString(w, `[
			{"id":"living","name":"Living Room","entities":["light.demo_lamp","climate.demo_heater","sensor.demo_temp","vacuum.demo_robot"]},
			{"id":"kitchen","name":"Kitchen","entities":["switch.demo_plug"]},
			{"id":"hall","name":"Hallway","entities":[]}
		]`)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/history/period/"):
		io.WriteString(w, "[]")
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/services/"):
		b, _ := io.ReadAll(r.Body)
		log.Printf("fake HA: service %s %s (state not changed)", strings.TrimPrefix(r.URL.Path, "/api/services/"), b)
		io.WriteString(w, "[]")
	default:
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"message":"not found"}`)
	}
}

// devHarnessPage is Glance's page skeleton around one extension widget:
// the widget's HTML is fetched from /widget and mounted with innerHTML,
// once, the way Glance's page.js does it.
func devHarnessPage(w http.ResponseWriter, r *http.Request) {
	light := r.URL.Query().Get("theme") == "light"
	scheme, theme := "dark", ""
	if light {
		scheme = "light"
		theme = `:root{--scheme:100% -;--bgh:240;--bgs:13%;--bgl:95%;--color-primary:hsl(240,50%,55%);--cm:1.5}`
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, devHarnessHTML, scheme, devStandInCSS(light), theme)
}

// devStandInCSS approximates Glance's theme variables when its own
// stylesheets are not served (they are only used when present).
func devStandInCSS(light bool) string {
	if light {
		return `:root{--color-background:#f1f1f5;--color-widget-background:#ffffff;--color-widget-background-highlight:#ececf2;--color-widget-content-border:#d8d8e2;--color-text-base:#3a3a48;--color-text-highlight:#14141c;--color-text-subdue:#7a7a8c;--color-primary:hsl(240,50%,55%);--color-negative:#d14d4d;--font-size-base:1.3rem;--font-size-h6:1.1rem}`
	}
	return `:root{--color-background:#1b1c22;--color-widget-background:#22232b;--color-widget-background-highlight:#2b2c36;--color-widget-content-border:#363746;--color-text-base:#b8b9c6;--color-text-highlight:#eceef6;--color-text-subdue:#7d7f8e;--color-primary:hsl(43,50%,70%);--color-negative:#e06a6a;--font-size-base:1.3rem;--font-size-h6:1.1rem}`
}

const devHarnessHTML = `<!doctype html>
<html data-scheme="%s"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Widget harness</title>
<style>%s</style>
<link rel="stylesheet" href="/harness/css/bundle.css">
<style>%s</style>
<style>
html{font-size:62.5%%}
body{margin:0;background:var(--color-background);color:var(--color-text-base);font:1.4rem/1.4 system-ui,sans-serif}
.dev-col{max-width:960px;margin:20px auto;padding:0 16px}
.widget-header h2{margin:0 0 8px;font-size:1.4rem;letter-spacing:.06em;color:var(--color-text-highlight)}
.widget-content{background:var(--color-widget-background);border:1px solid var(--color-widget-content-border);border-radius:5px;padding:15px}
</style>
</head><body><div class="body-content"><div class="content-bounds"><div class="dev-col">
<div class="widget widget-type-extension"><div class="widget-header"><h2 class="uppercase">Home</h2></div>
<div class="widget-content" id="mount">loading…</div></div></div></div></div>
<script>
fetch('/widget').then(function(r){return r.text()}).then(function(h){document.getElementById('mount').innerHTML=h});
</script>
</body></html>`
