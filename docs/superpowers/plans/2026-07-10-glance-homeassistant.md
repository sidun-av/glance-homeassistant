# glance-homeassistant Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go extension-widget backend for Glance that shows Home Assistant room
temperature (sparkline or bar-chart style), light on/off state per room, and contact/motion
sensor state, with lights/sensors updating live in the browser while the tab is open.

**Architecture:** A stateless Go HTTP service (`internal/hass` talks to Home Assistant's REST
API; `internal/render` produces themed HTML/SVG/JSON) exposing three routes: `/widget` (full
render, called by Glance on its own `cache:` schedule), `/live.json` (lights+sensors only, polled
client-side every ~10s via a bootstrapped `<img onerror>` script while the tab is open/visible),
and `/healthz`. No database, no background workers, no persistent connections — every request is
self-contained; the only in-process state is a short-TTL cache of the room→entity mapping.

**Tech Stack:** Go 1.23 stdlib (`net/http`, `encoding/json`, `html`), `gopkg.in/yaml.v3` for
config, no other dependencies. Docker (distroless static base image), GitHub Actions CI → GHCR.

## Global Constraints

- Module path: `github.com/sidun-av/glance-homeassistant`, Go 1.23, matching
  `glance-grafana-sparkline`'s toolchain.
- Every HTTP call to Home Assistant uses a bounded `context` timeout (10s for `/widget`'s
  combined calls, 5s for `/live.json`).
- All user-controlled text (room names, entity friendly names, state labels) rendered into HTML
  must go through `html.EscapeString` — no raw string interpolation into HTML output.
- HTTP handlers always return `200 OK` with valid HTML/JSON in their normal and degraded paths —
  never let Glance fall back to its own generic widget-failed UI.
- SVG output must reference Glance's own theme CSS variables (`var(--color-primary)`,
  `var(--color-text-subdue)`, `var(--color-text-highlight)`, `var(--color-widget-background-highlight)`,
  `var(--color-widget-content-border)`) — never hardcoded hex colors.
- `go test ./...` must pass before every commit.
- Full design context: `docs/superpowers/specs/2026-07-10-glance-homeassistant-widget-design.md`.

---

## Task 1: Project scaffolding + config loading

**Files:**
- Create: `go.mod`, `go.sum` (generated), `LICENSE`, `.gitignore`, `.dockerignore`
- Create: `config.go`
- Create: `config.example.yml`
- Test: `config_test.go`

**Interfaces:**
- Produces: `Config` struct with fields `HomeAssistant HomeAssistantConfig`, `PublicURL string`,
  `Title string`, `Temperature TemperatureConfig`, `Live LiveConfig`, `Sensors SensorsConfig`.
  `HomeAssistantConfig{URL, Token string}`. `TemperatureConfig{Range string, MaxPoints int,
  ChartHeight int, ChartStyle string}`. `LiveConfig{PollInterval string, PauseWhenHidden *bool}`.
  `SensorsConfig{ContactDeviceClasses, MotionDeviceClasses []string}`.
- Produces: `func LoadConfig(path string) (*Config, error)`.

- [ ] **Step 1: Initialize the Go module**

Run: `cd /Users/sidun/GIT/glance-homeassistant && go mod init github.com/sidun-av/glance-homeassistant`
Expected: creates `go.mod` containing `module github.com/sidun-av/glance-homeassistant` and a `go 1.2x` line. Edit `go.mod` if needed so the `go` line reads exactly `go 1.23`.

- [ ] **Step 2: Add supporting repo files**

Create `LICENSE`:
```
MIT License

Copyright (c) 2026 sidun-av

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

Create `.gitignore`:
```
/glance-homeassistant
config.yml
```

Create `.dockerignore`:
```
.git
docs
.github
*_test.go
glance-homeassistant
```

- [ ] **Step 3: Write the failing config test**

Create `config_test.go`:
```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempConfig(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestLoadConfig_Defaults(t *testing.T) {
	path := writeTempConfig(t, `
home_assistant:
  url: http://homeassistant:8123
  token: test-token
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Title != "Home" {
		t.Errorf("Title = %q, want %q", cfg.Title, "Home")
	}
	if cfg.PublicURL != "" {
		t.Errorf("PublicURL = %q, want empty (no forced default)", cfg.PublicURL)
	}
	if cfg.Temperature.Range != "24h" {
		t.Errorf("Temperature.Range = %q, want %q", cfg.Temperature.Range, "24h")
	}
	if cfg.Temperature.MaxPoints != 60 {
		t.Errorf("Temperature.MaxPoints = %d, want 60", cfg.Temperature.MaxPoints)
	}
	if cfg.Temperature.ChartHeight != 34 {
		t.Errorf("Temperature.ChartHeight = %d, want 34", cfg.Temperature.ChartHeight)
	}
	if cfg.Temperature.ChartStyle != "sparkline" {
		t.Errorf("Temperature.ChartStyle = %q, want %q", cfg.Temperature.ChartStyle, "sparkline")
	}
	if cfg.Live.PollInterval != "10s" {
		t.Errorf("Live.PollInterval = %q, want %q", cfg.Live.PollInterval, "10s")
	}
	if cfg.Live.PauseWhenHidden == nil || *cfg.Live.PauseWhenHidden != true {
		t.Errorf("Live.PauseWhenHidden = %v, want true", cfg.Live.PauseWhenHidden)
	}
	wantContact := []string{"door", "window", "garage_door", "opening"}
	if len(cfg.Sensors.ContactDeviceClasses) != len(wantContact) {
		t.Errorf("ContactDeviceClasses = %v, want %v", cfg.Sensors.ContactDeviceClasses, wantContact)
	}
	wantMotion := []string{"motion", "occupancy"}
	if len(cfg.Sensors.MotionDeviceClasses) != len(wantMotion) {
		t.Errorf("MotionDeviceClasses = %v, want %v", cfg.Sensors.MotionDeviceClasses, wantMotion)
	}
}

func TestLoadConfig_EnvExpansion(t *testing.T) {
	os.Setenv("TEST_HA_URL", "http://ha.example:8123")
	os.Setenv("TEST_HA_TOKEN", "secret-token")
	defer os.Unsetenv("TEST_HA_URL")
	defer os.Unsetenv("TEST_HA_TOKEN")

	path := writeTempConfig(t, `
home_assistant:
  url: ${TEST_HA_URL}
  token: ${TEST_HA_TOKEN}
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.HomeAssistant.URL != "http://ha.example:8123" {
		t.Errorf("HomeAssistant.URL = %q, want expanded value", cfg.HomeAssistant.URL)
	}
	if cfg.HomeAssistant.Token != "secret-token" {
		t.Errorf("HomeAssistant.Token = %q, want expanded value", cfg.HomeAssistant.Token)
	}
}

func TestLoadConfig_MissingURL(t *testing.T) {
	path := writeTempConfig(t, `
home_assistant:
  token: test-token
`)
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for missing home_assistant.url, got nil")
	}
}

func TestLoadConfig_MissingToken(t *testing.T) {
	path := writeTempConfig(t, `
home_assistant:
  url: http://homeassistant:8123
`)
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for missing home_assistant.token, got nil")
	}
}

func TestLoadConfig_InvalidRange(t *testing.T) {
	path := writeTempConfig(t, `
home_assistant:
  url: http://homeassistant:8123
  token: test-token
temperature:
  range: 1d
`)
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for range \"1d\" (Go duration has no day unit), got nil")
	}
}

func TestLoadConfig_InvalidPollInterval(t *testing.T) {
	path := writeTempConfig(t, `
home_assistant:
  url: http://homeassistant:8123
  token: test-token
live:
  poll_interval: soon
`)
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for invalid live.poll_interval, got nil")
	}
}

func TestLoadConfig_InvalidChartStyle(t *testing.T) {
	path := writeTempConfig(t, `
home_assistant:
  url: http://homeassistant:8123
  token: test-token
temperature:
  chart_style: pie
`)
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for invalid temperature.chart_style, got nil")
	}
}

func TestLoadConfig_NegativeMaxPoints(t *testing.T) {
	path := writeTempConfig(t, `
home_assistant:
  url: http://homeassistant:8123
  token: test-token
temperature:
  max_points: -1
`)
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for negative max_points, got nil")
	}
}

func TestLoadConfig_CustomSensorClasses(t *testing.T) {
	path := writeTempConfig(t, `
home_assistant:
  url: http://homeassistant:8123
  token: test-token
sensors:
  contact_device_classes: [door]
  motion_device_classes: [motion]
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.Sensors.ContactDeviceClasses) != 1 || cfg.Sensors.ContactDeviceClasses[0] != "door" {
		t.Errorf("ContactDeviceClasses = %v, want [door] (override, not merged with defaults)", cfg.Sensors.ContactDeviceClasses)
	}
	if len(cfg.Sensors.MotionDeviceClasses) != 1 || cfg.Sensors.MotionDeviceClasses[0] != "motion" {
		t.Errorf("MotionDeviceClasses = %v, want [motion]", cfg.Sensors.MotionDeviceClasses)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	if _, err := LoadConfig("/nonexistent/config.yml"); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadConfig_ExplicitPauseWhenHiddenFalse(t *testing.T) {
	path := writeTempConfig(t, `
home_assistant:
  url: http://homeassistant:8123
  token: test-token
live:
  pause_when_hidden: false
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Live.PauseWhenHidden == nil || *cfg.Live.PauseWhenHidden != false {
		t.Errorf("Live.PauseWhenHidden = %v, want explicit false to be preserved", cfg.Live.PauseWhenHidden)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./... 2>&1 | head -30`
Expected: FAIL — compile error, `LoadConfig`/`Config` undefined (config.go doesn't exist yet).

- [ ] **Step 3: Write config.go**

Create `config.go`:
```go
package main

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	HomeAssistant HomeAssistantConfig `yaml:"home_assistant"`
	PublicURL     string              `yaml:"public_url"`
	Title         string              `yaml:"title"`
	Temperature   TemperatureConfig   `yaml:"temperature"`
	Live          LiveConfig          `yaml:"live"`
	Sensors       SensorsConfig       `yaml:"sensors"`
}

type HomeAssistantConfig struct {
	URL   string `yaml:"url"`
	Token string `yaml:"token"`
}

type TemperatureConfig struct {
	Range       string `yaml:"range"`
	MaxPoints   int    `yaml:"max_points"`
	ChartHeight int    `yaml:"chart_height"`
	ChartStyle  string `yaml:"chart_style"`
}

type LiveConfig struct {
	PollInterval    string `yaml:"poll_interval"`
	PauseWhenHidden *bool  `yaml:"pause_when_hidden"`
}

type SensorsConfig struct {
	ContactDeviceClasses []string `yaml:"contact_device_classes"`
	MotionDeviceClasses  []string `yaml:"motion_device_classes"`
}

func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.HomeAssistant.URL = os.Expand(cfg.HomeAssistant.URL, os.Getenv)
	cfg.HomeAssistant.Token = os.Expand(cfg.HomeAssistant.Token, os.Getenv)

	if cfg.Title == "" {
		cfg.Title = "Home"
	}
	if cfg.Temperature.Range == "" {
		cfg.Temperature.Range = "24h"
	}
	if cfg.Temperature.MaxPoints == 0 {
		cfg.Temperature.MaxPoints = 60
	}
	if cfg.Temperature.ChartHeight == 0 {
		cfg.Temperature.ChartHeight = 34
	}
	if cfg.Temperature.ChartStyle == "" {
		cfg.Temperature.ChartStyle = "sparkline"
	}
	if cfg.Live.PollInterval == "" {
		cfg.Live.PollInterval = "10s"
	}
	if cfg.Live.PauseWhenHidden == nil {
		t := true
		cfg.Live.PauseWhenHidden = &t
	}
	if len(cfg.Sensors.ContactDeviceClasses) == 0 {
		cfg.Sensors.ContactDeviceClasses = []string{"door", "window", "garage_door", "opening"}
	}
	if len(cfg.Sensors.MotionDeviceClasses) == 0 {
		cfg.Sensors.MotionDeviceClasses = []string{"motion", "occupancy"}
	}

	if cfg.HomeAssistant.URL == "" {
		return nil, fmt.Errorf("home_assistant.url is required")
	}
	if cfg.HomeAssistant.Token == "" {
		return nil, fmt.Errorf("home_assistant.token is required")
	}
	if _, err := time.ParseDuration(cfg.Temperature.Range); err != nil {
		return nil, fmt.Errorf("temperature.range %q must be a Go duration like \"24h\" or \"6h\": %w", cfg.Temperature.Range, err)
	}
	if _, err := time.ParseDuration(cfg.Live.PollInterval); err != nil {
		return nil, fmt.Errorf("live.poll_interval %q must be a Go duration like \"10s\": %w", cfg.Live.PollInterval, err)
	}
	if cfg.Temperature.ChartStyle != "sparkline" && cfg.Temperature.ChartStyle != "bars" {
		return nil, fmt.Errorf("temperature.chart_style must be \"sparkline\" or \"bars\", got %q", cfg.Temperature.ChartStyle)
	}
	if cfg.Temperature.MaxPoints < 0 {
		return nil, fmt.Errorf("temperature.max_points must not be negative, got %d", cfg.Temperature.MaxPoints)
	}
	if cfg.Temperature.ChartHeight < 0 {
		return nil, fmt.Errorf("temperature.chart_height must not be negative, got %d", cfg.Temperature.ChartHeight)
	}

	return &cfg, nil
}
```

- [ ] **Step 4: Add the yaml dependency and run tests**

Run: `go get gopkg.in/yaml.v3@v3.0.1 && go test ./... -v`
Expected: `go.sum` is created/updated; all tests in `config_test.go` PASS.

- [ ] **Step 5: Write config.example.yml**

Create `config.example.yml`:
```yaml
home_assistant:
  url: ${HA_URL}
  token: ${HA_TOKEN}

public_url: /ha-widget   # path (reverse-proxied, same origin as Glance) or full origin (direct LAN port)

title: Home

temperature:
  range: 24h
  max_points: 60
  chart_height: 34  # sparkline: chart height in px. bars: bar area height (label rows added on top)
  chart_style: sparkline  # or: bars

live:
  poll_interval: 10s
  pause_when_hidden: true

sensors:
  contact_device_classes: [door, window, garage_door, opening]
  motion_device_classes: [motion, occupancy]
```

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum LICENSE .gitignore .dockerignore config.go config.example.yml config_test.go
git commit -m "Add project scaffolding and config loading"
```

---

## Task 2: Home Assistant client — FetchAreas

**Files:**
- Create: `internal/hass/client.go`
- Test: `internal/hass/client_test.go`

**Interfaces:**
- Produces: `type Room struct { ID, Name string; EntityIDs []string }`.
- Produces: `type Client struct { HTTPClient *http.Client; BaseURL, Token string }`.
- Produces: `func New(baseURL, token string) *Client`.
- Produces: `func (c *Client) FetchAreas(ctx context.Context) ([]Room, error)`.

- [ ] **Step 1: Write the failing test**

Create `internal/hass/client_test.go`:
```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/hass/... -v`
Expected: FAIL — compile error, package `hass` / `New` / `FetchAreas` undefined.

- [ ] **Step 3: Write client.go**

Create `internal/hass/client.go`:
```go
package hass

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	HTTPClient *http.Client
	BaseURL    string
	Token      string
}

func New(baseURL, token string) *Client {
	return &Client{
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Token:      token,
	}
}

type Room struct {
	ID        string
	Name      string
	EntityIDs []string
}

// areasTemplate renders every HA area to a JSON array via the Jinja functions
// areas()/area_name()/area_entities() — area_entities() already resolves
// entities assigned to an area either directly or through a device, matching
// how HA's own Areas UI groups things.
const areasTemplate = `{% set ns = namespace(areas=[]) %}` +
	`{% for a in areas() %}` +
	`{% set ns.areas = ns.areas + [{'id': a, 'name': area_name(a), 'entities': area_entities(a)}] %}` +
	`{% endfor %}` +
	`{{ ns.areas | tojson }}`

type templateRequest struct {
	Template string `json:"template"`
}

func (c *Client) FetchAreas(ctx context.Context) ([]Room, error) {
	payload, err := json.Marshal(templateRequest{Template: areasTemplate})
	if err != nil {
		return nil, fmt.Errorf("marshal template request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/template", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request areas: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("areas template returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read areas response: %w", err)
	}

	type rawArea struct {
		ID       string   `json:"id"`
		Name     string   `json:"name"`
		Entities []string `json:"entities"`
	}
	var rawAreas []rawArea
	if err := json.Unmarshal(bytes.TrimSpace(body), &rawAreas); err != nil {
		return nil, fmt.Errorf("parse areas response: %w", err)
	}

	rooms := make([]Room, len(rawAreas))
	for i, a := range rawAreas {
		rooms[i] = Room{ID: a.ID, Name: a.Name, EntityIDs: a.Entities}
	}
	return rooms, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/hass/... -v`
Expected: PASS — `TestFetchAreas_ParsesRooms`, `TestFetchAreas_NonOKStatus`, `TestFetchAreas_MalformedResponse`.

- [ ] **Step 5: Commit**

```bash
git add internal/hass/client.go internal/hass/client_test.go
git commit -m "Add Home Assistant client: FetchAreas via /api/template"
```

---

## Task 3: Home Assistant client — FetchStates

**Files:**
- Modify: `internal/hass/client.go`
- Modify: `internal/hass/client_test.go`

**Interfaces:**
- Consumes: `Client` from Task 2.
- Produces: `type EntityState struct { EntityID, Domain, State, FriendlyName, DeviceClass string }`.
- Produces: `func (c *Client) FetchStates(ctx context.Context) (map[string]EntityState, error)`.

- [ ] **Step 1: Write the failing test**

Append to `internal/hass/client_test.go`:
```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/hass/... -v`
Expected: FAIL — compile error, `FetchStates`/`EntityState` undefined.

- [ ] **Step 3: Add FetchStates to client.go**

Append to `internal/hass/client.go` (add `strings` is already imported; the domain parsing needs it):
```go
type EntityState struct {
	EntityID     string
	Domain       string
	State        string
	FriendlyName string
	DeviceClass  string
}

func (c *Client) FetchStates(ctx context.Context) (map[string]EntityState, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/states", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request states: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("states returned status %d", resp.StatusCode)
	}

	type rawState struct {
		EntityID   string `json:"entity_id"`
		State      string `json:"state"`
		Attributes struct {
			FriendlyName string `json:"friendly_name"`
			DeviceClass  string `json:"device_class"`
		} `json:"attributes"`
	}
	var rawStates []rawState
	if err := json.NewDecoder(resp.Body).Decode(&rawStates); err != nil {
		return nil, fmt.Errorf("parse states response: %w", err)
	}

	states := make(map[string]EntityState, len(rawStates))
	for _, s := range rawStates {
		domain := s.EntityID
		if idx := strings.Index(s.EntityID, "."); idx != -1 {
			domain = s.EntityID[:idx]
		}
		name := s.Attributes.FriendlyName
		if name == "" {
			name = s.EntityID
		}
		states[s.EntityID] = EntityState{
			EntityID:     s.EntityID,
			Domain:       domain,
			State:        s.State,
			FriendlyName: name,
			DeviceClass:  s.Attributes.DeviceClass,
		}
	}
	return states, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/hass/... -v`
Expected: PASS — all `TestFetchAreas_*` and `TestFetchStates_*` tests.

- [ ] **Step 5: Commit**

```bash
git add internal/hass/client.go internal/hass/client_test.go
git commit -m "Add FetchStates to Home Assistant client"
```

---

## Task 4: Home Assistant client — FetchHistory

**Files:**
- Modify: `internal/hass/client.go`
- Modify: `internal/hass/client_test.go`

**Interfaces:**
- Consumes: `Client` from Task 2.
- Produces: `type HistoryPoint struct { Time time.Time; Value float64 }`.
- Produces: `func (c *Client) FetchHistory(ctx context.Context, entityIDs []string, start, end time.Time) (map[string][]HistoryPoint, error)`.

- [ ] **Step 1: Write the failing test**

Append to `internal/hass/client_test.go`:
```go
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
```

Add `"time"` to the import block at the top of `internal/hass/client_test.go` (alongside `context`, `fmt`, `net/http`, `net/http/httptest`, `strings`, `testing`).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/hass/... -v`
Expected: FAIL — compile error, `FetchHistory`/`HistoryPoint` undefined.

- [ ] **Step 3: Add FetchHistory to client.go**

Add `"strconv"` to the import block in `internal/hass/client.go`, then append:
```go
type HistoryPoint struct {
	Time  time.Time
	Value float64
}

func (c *Client) FetchHistory(ctx context.Context, entityIDs []string, start, end time.Time) (map[string][]HistoryPoint, error) {
	if len(entityIDs) == 0 {
		return map[string][]HistoryPoint{}, nil
	}

	u := fmt.Sprintf("%s/api/history/period/%s", c.BaseURL, start.UTC().Format(time.RFC3339))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	q := req.URL.Query()
	q.Set("filter_entity_id", strings.Join(entityIDs, ","))
	q.Set("end_time", end.UTC().Format(time.RFC3339))
	q.Set("minimal_response", "true")
	q.Set("no_attributes", "true")
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request history: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("history returned status %d", resp.StatusCode)
	}

	type rawPoint struct {
		EntityID    string `json:"entity_id"`
		State       string `json:"state"`
		LastChanged string `json:"last_changed"`
	}
	var series [][]rawPoint
	if err := json.NewDecoder(resp.Body).Decode(&series); err != nil {
		return nil, fmt.Errorf("parse history response: %w", err)
	}

	out := make(map[string][]HistoryPoint)
	for _, points := range series {
		if len(points) == 0 {
			continue
		}
		entityID := points[0].EntityID
		hist := make([]HistoryPoint, 0, len(points))
		for _, p := range points {
			value, err := strconv.ParseFloat(p.State, 64)
			if err != nil {
				continue
			}
			t, err := time.Parse(time.RFC3339, p.LastChanged)
			if err != nil {
				continue
			}
			hist = append(hist, HistoryPoint{Time: t, Value: value})
		}
		if len(hist) > 0 {
			out[entityID] = hist
		}
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/hass/... -v`
Expected: PASS — all client tests including the three new `TestFetchHistory_*` cases.

- [ ] **Step 5: Commit**

```bash
git add internal/hass/client.go internal/hass/client_test.go
git commit -m "Add FetchHistory to Home Assistant client"
```

---

## Task 5: Area cache

**Files:**
- Create: `internal/hass/cache.go`
- Test: `internal/hass/cache_test.go`

**Interfaces:**
- Consumes: `Client.FetchAreas` from Task 2.
- Produces: `type AreaCache struct{...}`, `func NewAreaCache(client *Client, ttl time.Duration) *AreaCache`, `func (c *AreaCache) Get(ctx context.Context) ([]Room, error)`.

- [ ] **Step 1: Write the failing test**

Create `internal/hass/cache_test.go`:
```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/hass/... -v`
Expected: FAIL — compile error, `AreaCache`/`NewAreaCache` undefined.

- [ ] **Step 3: Write cache.go**

Create `internal/hass/cache.go`:
```go
package hass

import (
	"context"
	"sync"
	"time"
)

// AreaCache wraps Client.FetchAreas with a TTL: room→entity assignment
// changes rarely (only when the user edits areas in HA), so both /widget and
// /live.json share one cached copy instead of each hitting /api/template on
// every request. On a refresh failure, it serves the last-known mapping
// rather than propagating the error, as long as it has one to serve.
type AreaCache struct {
	mu        sync.Mutex
	client    *Client
	ttl       time.Duration
	rooms     []Room
	fetchedAt time.Time
}

func NewAreaCache(client *Client, ttl time.Duration) *AreaCache {
	return &AreaCache{client: client, ttl: ttl}
}

func (c *AreaCache) Get(ctx context.Context) ([]Room, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.rooms != nil && time.Since(c.fetchedAt) < c.ttl {
		return c.rooms, nil
	}

	rooms, err := c.client.FetchAreas(ctx)
	if err != nil {
		if c.rooms != nil {
			return c.rooms, nil
		}
		return nil, err
	}

	c.rooms = rooms
	c.fetchedAt = time.Now()
	return c.rooms, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/hass/... -v`
Expected: PASS — all `TestAreaCache_*` tests plus everything from earlier tasks.

- [ ] **Step 5: Commit**

```bash
git add internal/hass/cache.go internal/hass/cache_test.go
git commit -m "Add TTL area cache with stale-on-error fallback"
```

---

## Task 6: Entity classification (BuildModel)

**Files:**
- Create: `internal/hass/discover.go`
- Test: `internal/hass/discover_test.go`

**Interfaces:**
- Consumes: `Room`, `EntityState` from Task 2/3.
- Produces: `type Model struct { TemperatureRooms []TemperatureRoom; LightRooms []LightRoom; Sensors []SensorEntity }`.
- Produces: `type TemperatureRoom struct { Room string; EntityIDs []string }`.
- Produces: `type LightRoom struct { Room string; On, Total int }`.
- Produces: `type SensorEntity struct { Name string; Attention bool; Label string }`.
- Produces: `type ClassificationConfig struct { ContactDeviceClasses, MotionDeviceClasses []string }`.
- Produces: `func BuildModel(rooms []Room, states map[string]EntityState, cfg ClassificationConfig) Model`.

- [ ] **Step 1: Write the failing test**

Create `internal/hass/discover_test.go`:
```go
package hass

import "testing"

func defaultClassificationConfig() ClassificationConfig {
	return ClassificationConfig{
		ContactDeviceClasses: []string{"door", "window", "garage_door", "opening"},
		MotionDeviceClasses:  []string{"motion", "occupancy"},
	}
}

func TestBuildModel_ClassifiesByDomainAndDeviceClass(t *testing.T) {
	rooms := []Room{
		{Name: "Living Room", EntityIDs: []string{"sensor.lr_temp", "light.lr_main", "binary_sensor.lr_window"}},
		{Name: "Bedroom", EntityIDs: []string{"light.bed_main", "binary_sensor.bed_motion"}},
	}
	states := map[string]EntityState{
		"sensor.lr_temp":         {EntityID: "sensor.lr_temp", Domain: "sensor", State: "21.4", DeviceClass: "temperature", FriendlyName: "LR Temp"},
		"light.lr_main":          {EntityID: "light.lr_main", Domain: "light", State: "on", FriendlyName: "LR Main"},
		"binary_sensor.lr_window": {EntityID: "binary_sensor.lr_window", Domain: "binary_sensor", State: "on", DeviceClass: "window", FriendlyName: "LR Window"},
		"light.bed_main":         {EntityID: "light.bed_main", Domain: "light", State: "off", FriendlyName: "Bed Main"},
		"binary_sensor.bed_motion": {EntityID: "binary_sensor.bed_motion", Domain: "binary_sensor", State: "off", DeviceClass: "motion", FriendlyName: "Bed Motion"},
	}

	model := BuildModel(rooms, states, defaultClassificationConfig())

	if len(model.TemperatureRooms) != 1 || model.TemperatureRooms[0].Room != "Living Room" {
		t.Fatalf("TemperatureRooms = %+v", model.TemperatureRooms)
	}
	if len(model.TemperatureRooms[0].EntityIDs) != 1 || model.TemperatureRooms[0].EntityIDs[0] != "sensor.lr_temp" {
		t.Errorf("TemperatureRooms[0].EntityIDs = %v", model.TemperatureRooms[0].EntityIDs)
	}

	if len(model.LightRooms) != 2 {
		t.Fatalf("LightRooms = %+v", model.LightRooms)
	}
	byRoom := map[string]LightRoom{}
	for _, lr := range model.LightRooms {
		byRoom[lr.Room] = lr
	}
	if byRoom["Living Room"].On != 1 || byRoom["Living Room"].Total != 1 {
		t.Errorf("Living Room lights = %+v, want On=1 Total=1", byRoom["Living Room"])
	}
	if byRoom["Bedroom"].On != 0 || byRoom["Bedroom"].Total != 1 {
		t.Errorf("Bedroom lights = %+v, want On=0 Total=1", byRoom["Bedroom"])
	}

	if len(model.Sensors) != 2 {
		t.Fatalf("Sensors = %+v", model.Sensors)
	}
	byName := map[string]SensorEntity{}
	for _, s := range model.Sensors {
		byName[s.Name] = s
	}
	if !byName["LR Window"].Attention || byName["LR Window"].Label != "Open" {
		t.Errorf("LR Window sensor = %+v, want Attention=true Label=Open", byName["LR Window"])
	}
	if byName["Bed Motion"].Attention || byName["Bed Motion"].Label != "Clear" {
		t.Errorf("Bed Motion sensor = %+v, want Attention=false Label=Clear", byName["Bed Motion"])
	}
}

func TestBuildModel_RoomWithoutMatchingEntitiesIsOmitted(t *testing.T) {
	rooms := []Room{
		{Name: "Garage", EntityIDs: []string{"switch.garage_opener"}},
	}
	states := map[string]EntityState{
		"switch.garage_opener": {EntityID: "switch.garage_opener", Domain: "switch", State: "off", FriendlyName: "Garage Opener"},
	}

	model := BuildModel(rooms, states, defaultClassificationConfig())

	if len(model.TemperatureRooms) != 0 || len(model.LightRooms) != 0 || len(model.Sensors) != 0 {
		t.Errorf("model = %+v, want all sections empty for a room with only an unclassified switch entity", model)
	}
}

func TestBuildModel_MultipleTemperatureSensorsInOneRoom(t *testing.T) {
	rooms := []Room{
		{Name: "Living Room", EntityIDs: []string{"sensor.lr_temp_1", "sensor.lr_temp_2"}},
	}
	states := map[string]EntityState{
		"sensor.lr_temp_1": {EntityID: "sensor.lr_temp_1", Domain: "sensor", State: "21.0", DeviceClass: "temperature", FriendlyName: "LR Temp 1"},
		"sensor.lr_temp_2": {EntityID: "sensor.lr_temp_2", Domain: "sensor", State: "22.0", DeviceClass: "temperature", FriendlyName: "LR Temp 2"},
	}

	model := BuildModel(rooms, states, defaultClassificationConfig())

	if len(model.TemperatureRooms) != 1 {
		t.Fatalf("TemperatureRooms = %+v", model.TemperatureRooms)
	}
	if len(model.TemperatureRooms[0].EntityIDs) != 2 {
		t.Errorf("EntityIDs = %v, want both sensors collected under one room", model.TemperatureRooms[0].EntityIDs)
	}
}

func TestBuildModel_SkipsUnavailableBinarySensor(t *testing.T) {
	rooms := []Room{
		{Name: "Hallway", EntityIDs: []string{"binary_sensor.hall_motion"}},
	}
	states := map[string]EntityState{
		"binary_sensor.hall_motion": {EntityID: "binary_sensor.hall_motion", Domain: "binary_sensor", State: "unavailable", DeviceClass: "motion", FriendlyName: "Hall Motion"},
	}

	model := BuildModel(rooms, states, defaultClassificationConfig())

	if len(model.Sensors) != 0 {
		t.Errorf("Sensors = %+v, want unavailable sensor excluded", model.Sensors)
	}
}

func TestBuildModel_MissingStateForEntityIsSkipped(t *testing.T) {
	rooms := []Room{
		{Name: "Office", EntityIDs: []string{"light.office_main"}},
	}
	states := map[string]EntityState{} // entity not present in states at all

	model := BuildModel(rooms, states, defaultClassificationConfig())

	if len(model.LightRooms) != 0 {
		t.Errorf("LightRooms = %+v, want none (entity missing from states map)", model.LightRooms)
	}
}

func TestBuildModel_SortsAlphabetically(t *testing.T) {
	rooms := []Room{
		{Name: "Zeta Room", EntityIDs: []string{"light.zeta"}},
		{Name: "Alpha Room", EntityIDs: []string{"light.alpha"}},
	}
	states := map[string]EntityState{
		"light.zeta":  {EntityID: "light.zeta", Domain: "light", State: "on", FriendlyName: "Zeta Light"},
		"light.alpha": {EntityID: "light.alpha", Domain: "light", State: "on", FriendlyName: "Alpha Light"},
	}

	model := BuildModel(rooms, states, defaultClassificationConfig())

	if len(model.LightRooms) != 2 || model.LightRooms[0].Room != "Alpha Room" || model.LightRooms[1].Room != "Zeta Room" {
		t.Errorf("LightRooms = %+v, want alphabetical order", model.LightRooms)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/hass/... -v`
Expected: FAIL — compile error, `BuildModel`/`Model`/`ClassificationConfig` undefined.

- [ ] **Step 3: Write discover.go**

Create `internal/hass/discover.go`:
```go
package hass

import "sort"

type Model struct {
	TemperatureRooms []TemperatureRoom
	LightRooms       []LightRoom
	Sensors          []SensorEntity
}

type TemperatureRoom struct {
	Room      string
	EntityIDs []string
}

type LightRoom struct {
	Room  string
	On    int
	Total int
}

type SensorEntity struct {
	Name      string
	Attention bool
	Label     string
}

type ClassificationConfig struct {
	ContactDeviceClasses []string
	MotionDeviceClasses  []string
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

func BuildModel(rooms []Room, states map[string]EntityState, cfg ClassificationConfig) Model {
	tempByRoom := make(map[string]*TemperatureRoom)
	lightByRoom := make(map[string]*LightRoom)
	var sensors []SensorEntity

	for _, room := range rooms {
		for _, entityID := range room.EntityIDs {
			state, ok := states[entityID]
			if !ok {
				continue
			}

			switch {
			case state.Domain == "sensor" && state.DeviceClass == "temperature":
				tr, exists := tempByRoom[room.Name]
				if !exists {
					tr = &TemperatureRoom{Room: room.Name}
					tempByRoom[room.Name] = tr
				}
				tr.EntityIDs = append(tr.EntityIDs, entityID)

			case state.Domain == "light":
				lr, exists := lightByRoom[room.Name]
				if !exists {
					lr = &LightRoom{Room: room.Name}
					lightByRoom[room.Name] = lr
				}
				lr.Total++
				if state.State == "on" {
					lr.On++
				}

			case state.Domain == "binary_sensor" && contains(cfg.ContactDeviceClasses, state.DeviceClass):
				if state.State != "on" && state.State != "off" {
					continue
				}
				attention := state.State == "on"
				label := "Closed"
				if attention {
					label = "Open"
				}
				sensors = append(sensors, SensorEntity{Name: state.FriendlyName, Attention: attention, Label: label})

			case state.Domain == "binary_sensor" && contains(cfg.MotionDeviceClasses, state.DeviceClass):
				if state.State != "on" && state.State != "off" {
					continue
				}
				attention := state.State == "on"
				label := "Clear"
				if attention {
					label = "Motion"
				}
				sensors = append(sensors, SensorEntity{Name: state.FriendlyName, Attention: attention, Label: label})
			}
		}
	}

	model := Model{}
	for _, tr := range tempByRoom {
		model.TemperatureRooms = append(model.TemperatureRooms, *tr)
	}
	for _, lr := range lightByRoom {
		model.LightRooms = append(model.LightRooms, *lr)
	}
	model.Sensors = sensors

	sort.Slice(model.TemperatureRooms, func(i, j int) bool { return model.TemperatureRooms[i].Room < model.TemperatureRooms[j].Room })
	sort.Slice(model.LightRooms, func(i, j int) bool { return model.LightRooms[i].Room < model.LightRooms[j].Room })
	sort.Slice(model.Sensors, func(i, j int) bool { return model.Sensors[i].Name < model.Sensors[j].Name })

	return model
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/hass/... -v`
Expected: PASS — all `TestBuildModel_*` tests plus everything from earlier tasks.

- [ ] **Step 5: Commit**

```bash
git add internal/hass/discover.go internal/hass/discover_test.go
git commit -m "Add entity classification into rooms (BuildModel)"
```

---

## Task 7: Temperature history resampling

**Files:**
- Create: `internal/hass/resample.go`
- Test: `internal/hass/resample_test.go`

**Interfaces:**
- Consumes: `HistoryPoint` from Task 4.
- Produces: `func BuildTimestamps(end time.Time, rangeDur time.Duration, maxPoints int) []time.Time`.
- Produces: `func StepForwardFill(points []HistoryPoint, timestamps []time.Time) []float64`.
- Produces: `func AverageSeries(series [][]float64) []float64`.

- [ ] **Step 1: Write the failing test**

Create `internal/hass/resample_test.go`:
```go
package hass

import (
	"math"
	"testing"
	"time"
)

func TestBuildTimestamps_EvenSpacing(t *testing.T) {
	end := time.Date(2026, 7, 10, 20, 0, 0, 0, time.UTC)
	timestamps := BuildTimestamps(end, 24*time.Hour, 5)

	if len(timestamps) != 5 {
		t.Fatalf("len(timestamps) = %d, want 5", len(timestamps))
	}
	wantFirst := end.Add(-24 * time.Hour)
	if !timestamps[0].Equal(wantFirst) {
		t.Errorf("timestamps[0] = %v, want %v", timestamps[0], wantFirst)
	}
	if !timestamps[4].Equal(end) {
		t.Errorf("timestamps[4] = %v, want %v", timestamps[4], end)
	}
	wantStep := 6 * time.Hour
	if gotStep := timestamps[1].Sub(timestamps[0]); gotStep != wantStep {
		t.Errorf("step = %v, want %v", gotStep, wantStep)
	}
}

func TestBuildTimestamps_SinglePoint(t *testing.T) {
	end := time.Date(2026, 7, 10, 20, 0, 0, 0, time.UTC)
	timestamps := BuildTimestamps(end, time.Hour, 1)
	if len(timestamps) != 1 || !timestamps[0].Equal(end) {
		t.Errorf("timestamps = %v, want [end]", timestamps)
	}
}

func TestStepForwardFill_CarriesLastKnownValue(t *testing.T) {
	base := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	points := []HistoryPoint{
		{Time: base, Value: 20.0},
		{Time: base.Add(2 * time.Hour), Value: 22.0},
	}
	timestamps := []time.Time{
		base.Add(-1 * time.Hour), // before first point -> falls back to first value
		base.Add(1 * time.Hour),  // between points -> carries 20.0 forward
		base.Add(3 * time.Hour),  // after second point -> carries 22.0 forward
	}

	values := StepForwardFill(points, timestamps)

	want := []float64{20.0, 20.0, 22.0}
	for i := range want {
		if values[i] != want[i] {
			t.Errorf("values[%d] = %v, want %v", i, values[i], want[i])
		}
	}
}

func TestStepForwardFill_EmptyPointsReturnsNaN(t *testing.T) {
	timestamps := []time.Time{time.Now(), time.Now().Add(time.Hour)}
	values := StepForwardFill(nil, timestamps)

	if len(values) != 2 {
		t.Fatalf("len(values) = %d, want 2", len(values))
	}
	for i, v := range values {
		if !math.IsNaN(v) {
			t.Errorf("values[%d] = %v, want NaN", i, v)
		}
	}
}

func TestAverageSeries_ElementwiseAverage(t *testing.T) {
	series := [][]float64{
		{10, 20, 30},
		{20, 30, 40},
	}
	avg := AverageSeries(series)

	want := []float64{15, 25, 35}
	for i := range want {
		if avg[i] != want[i] {
			t.Errorf("avg[%d] = %v, want %v", i, avg[i], want[i])
		}
	}
}

func TestAverageSeries_SkipsNaN(t *testing.T) {
	series := [][]float64{
		{10, math.NaN(), 30},
		{20, 25, 40},
	}
	avg := AverageSeries(series)

	if avg[0] != 15 {
		t.Errorf("avg[0] = %v, want 15", avg[0])
	}
	if avg[1] != 25 {
		t.Errorf("avg[1] = %v, want 25 (only non-NaN value)", avg[1])
	}
	if avg[2] != 35 {
		t.Errorf("avg[2] = %v, want 35", avg[2])
	}
}

func TestAverageSeries_AllNaNProducesNaN(t *testing.T) {
	series := [][]float64{
		{math.NaN()},
		{math.NaN()},
	}
	avg := AverageSeries(series)
	if !math.IsNaN(avg[0]) {
		t.Errorf("avg[0] = %v, want NaN", avg[0])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/hass/... -v`
Expected: FAIL — compile error, `BuildTimestamps`/`StepForwardFill`/`AverageSeries` undefined.

- [ ] **Step 3: Write resample.go**

Create `internal/hass/resample.go`:
```go
package hass

import (
	"math"
	"sort"
	"time"
)

func BuildTimestamps(end time.Time, rangeDur time.Duration, maxPoints int) []time.Time {
	if maxPoints < 2 {
		return []time.Time{end}
	}
	start := end.Add(-rangeDur)
	step := rangeDur / time.Duration(maxPoints-1)
	timestamps := make([]time.Time, maxPoints)
	for i := 0; i < maxPoints; i++ {
		timestamps[i] = start.Add(step * time.Duration(i))
	}
	return timestamps
}

// StepForwardFill resamples irregular history points onto evenly spaced
// timestamps: the value at each timestamp is the most recently known state
// at or before it. Timestamps before the first known point fall back to the
// first point's value, so a room's history never has a gap at the start of
// the window just because the entity's first ever state came slightly later.
func StepForwardFill(points []HistoryPoint, timestamps []time.Time) []float64 {
	values := make([]float64, len(timestamps))
	if len(points) == 0 {
		for i := range values {
			values[i] = math.NaN()
		}
		return values
	}

	sorted := make([]HistoryPoint, len(points))
	copy(sorted, points)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Time.Before(sorted[j].Time) })

	idx := 0
	last := sorted[0].Value
	for i, ts := range timestamps {
		for idx < len(sorted) && !sorted[idx].Time.After(ts) {
			last = sorted[idx].Value
			idx++
		}
		values[i] = last
	}
	return values
}

func AverageSeries(series [][]float64) []float64 {
	if len(series) == 0 {
		return nil
	}
	n := len(series[0])
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		sum := 0.0
		count := 0
		for _, s := range series {
			if i < len(s) && !math.IsNaN(s[i]) {
				sum += s[i]
				count++
			}
		}
		if count == 0 {
			out[i] = math.NaN()
		} else {
			out[i] = sum / float64(count)
		}
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/hass/... -v`
Expected: PASS — all `TestBuildTimestamps_*`, `TestStepForwardFill_*`, `TestAverageSeries_*` tests plus everything from earlier tasks.

- [ ] **Step 5: Commit**

```bash
git add internal/hass/resample.go internal/hass/resample_test.go
git commit -m "Add temperature history resampling (step-forward-fill + average)"
```

---

## Task 8: Sparkline SVG renderer

**Files:**
- Create: `internal/render/sparkline.go`
- Test: `internal/render/sparkline_test.go`

**Interfaces:**
- Produces: `type SparklineOptions struct { Width, Height float64 }`.
- Produces: `func DefaultSparklineOptions() SparklineOptions`.
- Produces: `func Sparkline(values []float64, opts SparklineOptions) string`.

- [ ] **Step 1: Write the failing test**

Create `internal/render/sparkline_test.go`:
```go
package render

import "testing"

func TestSparkline_EmptyValuesReturnsBareSVG(t *testing.T) {
	svg := Sparkline(nil, DefaultSparklineOptions())
	if !contains(svg, "<svg") {
		t.Errorf("svg = %q, want it to contain <svg", svg)
	}
	if contains(svg, "polyline") {
		t.Errorf("svg = %q, want no polyline for empty values", svg)
	}
}

func TestSparkline_RendersOnePointPerValue(t *testing.T) {
	svg := Sparkline([]float64{1, 2, 3, 2, 1}, SparklineOptions{Width: 100, Height: 20})
	if !contains(svg, "polyline") {
		t.Errorf("svg = %q, want a polyline", svg)
	}
	if !contains(svg, "var(--color-primary)") {
		t.Errorf("svg = %q, want it to reference the theme's primary color variable", svg)
	}
}

func TestSparkline_FlatSeriesDoesNotDivideByZero(t *testing.T) {
	svg := Sparkline([]float64{5, 5, 5, 5}, SparklineOptions{Width: 100, Height: 20})
	if contains(svg, "NaN") {
		t.Errorf("svg = %q, want no NaN for a flat series", svg)
	}
}

func TestSparkline_SinglePointDoesNotPanic(t *testing.T) {
	svg := Sparkline([]float64{42}, SparklineOptions{Width: 100, Height: 20})
	if !contains(svg, "<svg") {
		t.Errorf("svg = %q, want it to contain <svg", svg)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (func() bool {
		for i := 0; i+len(substr) <= len(s); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/render/... -v`
Expected: FAIL — compile error, package `render` / `Sparkline` undefined.

- [ ] **Step 3: Write sparkline.go**

Create `internal/render/sparkline.go`:
```go
package render

import (
	"fmt"
	"strings"
)

type SparklineOptions struct {
	Width  float64
	Height float64
}

func DefaultSparklineOptions() SparklineOptions {
	return SparklineOptions{Width: 220, Height: 34}
}

// Sparkline renders an inline SVG line+area chart auto-scaled to the
// series' own min/max (with 20% padding), themed via Glance's own CSS
// custom property so it matches the user's active accent color.
func Sparkline(values []float64, opts SparklineOptions) string {
	if len(values) == 0 {
		return fmt.Sprintf(`<svg viewBox="0 0 %g %g" height="%g" style="width:100%%;display:block" preserveAspectRatio="none"></svg>`, opts.Width, opts.Height, opts.Height)
	}

	min, max := values[0], values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	span := max - min
	if span < 1e-9 {
		min -= 0.5
		max += 0.5
		span = 1
	}
	pad := span * 0.2
	min -= pad
	max += pad
	span = max - min

	n := len(values)
	stepX := 0.0
	if n > 1 {
		stepX = opts.Width / float64(n-1)
	}

	points := make([]string, n)
	for i, v := range values {
		x := float64(i) * stepX
		y := opts.Height - ((v-min)/span)*opts.Height
		points[i] = fmt.Sprintf("%.2f,%.2f", x, y)
	}
	line := strings.Join(points, " ")
	area := fmt.Sprintf("0,%.2f %s %.2f,%.2f", opts.Height, line, opts.Width, opts.Height)

	return fmt.Sprintf(
		`<svg viewBox="0 0 %g %g" height="%g" style="width:100%%;display:block" preserveAspectRatio="none">`+
			`<polygon points="%s" fill="var(--color-primary)" fill-opacity="0.16"/>`+
			`<polyline points="%s" fill="none" stroke="var(--color-primary)" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"/>`+
			`</svg>`,
		opts.Width, opts.Height, opts.Height, area, line,
	)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/render/... -v`
Expected: PASS — all `TestSparkline_*` tests.

- [ ] **Step 5: Commit**

```bash
git add internal/render/sparkline.go internal/render/sparkline_test.go
git commit -m "Add sparkline SVG renderer"
```

---

## Task 9: Bar chart SVG renderer

**Files:**
- Create: `internal/render/barchart.go`
- Test: `internal/render/barchart_test.go`

**Interfaces:**
- Consumes: `contains` helper from Task 8's test file (same package `render`, test-only).
- Produces: `type BarChartOptions struct { Width, Height float64 }`.
- Produces: `func DefaultBarChartOptions() BarChartOptions`.
- Produces: `func BarChart(values []float64, axisLabels []string, currentValueLabel string, opts BarChartOptions) string`.

- [ ] **Step 1: Write the failing test**

Create `internal/render/barchart_test.go`:
```go
package render

import (
	"strings"
	"testing"
)

func TestBarChart_EmptyValuesReturnsBareSVG(t *testing.T) {
	svg := BarChart(nil, nil, "", DefaultBarChartOptions())
	if !contains(svg, "<svg") {
		t.Errorf("svg = %q, want it to contain <svg", svg)
	}
	if contains(svg, "<line") {
		t.Errorf("svg = %q, want no bars for empty values", svg)
	}
}

func TestBarChart_RendersOneBarPerValue(t *testing.T) {
	svg := BarChart([]float64{10, 15, 12, 20}, []string{"06:00", "", "", "18:00"}, "20.0°", BarChartOptions{Width: 220, Height: 60})
	if count := strings.Count(svg, "<line"); count != 4 {
		t.Errorf("bar (<line>) count = %d, want 4", count)
	}
	if !contains(svg, "var(--color-primary)") {
		t.Errorf("svg = %q, want it to reference the theme's primary color variable", svg)
	}
}

func TestBarChart_IncludesCurrentValueLabel(t *testing.T) {
	svg := BarChart([]float64{10, 20}, []string{"06:00", "18:00"}, "20.0°", BarChartOptions{Width: 220, Height: 60})
	if !contains(svg, "20.0") {
		t.Errorf("svg = %q, want it to contain the current value label", svg)
	}
}

func TestBarChart_IncludesAxisLabels(t *testing.T) {
	svg := BarChart([]float64{10, 15, 20}, []string{"06:00", "", "18:00"}, "20.0°", BarChartOptions{Width: 220, Height: 60})
	if !contains(svg, "06:00") || !contains(svg, "18:00") {
		t.Errorf("svg = %q, want both axis labels present", svg)
	}
}

func TestBarChart_FlatSeriesDoesNotDivideByZero(t *testing.T) {
	svg := BarChart([]float64{5, 5, 5}, []string{"", "", ""}, "5.0°", BarChartOptions{Width: 220, Height: 60})
	if contains(svg, "NaN") {
		t.Errorf("svg = %q, want no NaN for a flat series", svg)
	}
}

func TestBarChart_EscapesLabels(t *testing.T) {
	svg := BarChart([]float64{10}, []string{"<b>"}, "", BarChartOptions{Width: 220, Height: 60})
	if contains(svg, "<b>") {
		t.Errorf("svg = %q, want axis label HTML-escaped", svg)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/render/... -v`
Expected: FAIL — compile error, `BarChart`/`BarChartOptions` undefined.

- [ ] **Step 3: Write barchart.go**

Create `internal/render/barchart.go`:
```go
package render

import (
	"fmt"
	"html"
	"strings"
)

type BarChartOptions struct {
	Width  float64
	Height float64
}

func DefaultBarChartOptions() BarChartOptions {
	return BarChartOptions{Width: 220, Height: 61}
}

// BarChart renders a themed vertical bar chart mirroring Glance's own
// built-in WEATHER widget: one rounded-cap bar per value, auto min/max
// scaled (with a minimum bar height floor so the lowest point stays
// visible), opacity ramping from dim (oldest, index 0) to full brightness
// (most recent, last index), a value label above the most recent bar, and
// sparse x-axis labels wherever axisLabels[i] is non-empty.
func BarChart(values []float64, axisLabels []string, currentValueLabel string, opts BarChartOptions) string {
	if len(values) == 0 {
		return fmt.Sprintf(`<svg viewBox="0 0 %g %g" height="%g" style="width:100%%;display:block" preserveAspectRatio="none"></svg>`, opts.Width, opts.Height, opts.Height)
	}

	const topMargin = 14.0
	const bottomMargin = 13.0
	const minBarHeight = 3.0

	barAreaHeight := opts.Height - topMargin - bottomMargin
	if barAreaHeight < minBarHeight {
		barAreaHeight = minBarHeight
	}
	baseline := opts.Height - bottomMargin

	min, max := values[0], values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	span := max - min
	if span < 1e-9 {
		span = 1
	}

	n := len(values)
	step := opts.Width / float64(n)
	denom := n - 1
	if denom < 1 {
		denom = 1
	}

	var bars strings.Builder
	for i, v := range values {
		x := step*float64(i) + step/2
		normalized := (v - min) / span * barAreaHeight
		if normalized < minBarHeight {
			normalized = minBarHeight
		}
		y2 := baseline - normalized
		opacity := 0.32 + (0.68 * float64(i) / float64(denom))
		barWidth := step * 0.55
		fmt.Fprintf(&bars,
			`<line x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f" stroke="var(--color-primary)" stroke-opacity="%.2f" stroke-width="%.2f" stroke-linecap="round"/>`,
			x, baseline, x, y2, opacity, barWidth,
		)
	}

	var label string
	if currentValueLabel != "" {
		lastX := step*float64(n-1) + step/2
		label = fmt.Sprintf(`<text x="%.2f" y="%.2f" text-anchor="middle" font-size="9" fill="var(--color-text-highlight)">%s</text>`,
			lastX, topMargin-4, html.EscapeString(currentValueLabel))
	}

	var axis strings.Builder
	for i, lbl := range axisLabels {
		if lbl == "" || i >= n {
			continue
		}
		x := step*float64(i) + step/2
		fmt.Fprintf(&axis, `<text x="%.2f" y="%.2f" text-anchor="middle" font-size="9" fill="var(--color-text-subdue)">%s</text>`,
			x, opts.Height-2, html.EscapeString(lbl))
	}

	return fmt.Sprintf(`<svg viewBox="0 0 %g %g" height="%g" style="width:100%%;display:block" preserveAspectRatio="none">%s%s%s</svg>`,
		opts.Width, opts.Height, opts.Height, bars.String(), label, axis.String())
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/render/... -v`
Expected: PASS — all `TestBarChart_*` tests plus everything from Task 8.

- [ ] **Step 5: Commit**

```bash
git add internal/render/barchart.go internal/render/barchart_test.go
git commit -m "Add WEATHER-widget-style bar chart renderer"
```

---

## Task 10: Full widget HTML renderer

**Files:**
- Create: `internal/render/template.go`
- Test: `internal/render/template_test.go`

**Interfaces:**
- Consumes: `Sparkline`, `BarChart` from Tasks 8/9.
- Produces: `type WidgetData struct { Title string; ChartHeight int; ChartStyle string; TemperatureRooms []TemperatureRoomView; LightRooms []LightRoomView; Sensors []SensorView; LiveURL string; PollIntervalMS int; PauseWhenHidden bool }`.
- Produces: `type TemperatureRoomView struct { Room, Value, SVG string; NoData bool }`.
- Produces: `type LightRoomView struct { Room string; On, Total int }`.
- Produces: `type SensorView struct { Name string; Attention bool; Label string }`.
- Produces: `func RenderWidget(data WidgetData) string`.
- Produces: `func RenderUnavailable() string`.

- [ ] **Step 1: Write the failing test**

Create `internal/render/template_test.go`:
```go
package render

import "testing"

func sampleWidgetData() WidgetData {
	return WidgetData{
		Title:       "Home",
		ChartHeight: 34,
		ChartStyle:  "sparkline",
		TemperatureRooms: []TemperatureRoomView{
			{Room: "Living Room", Value: "21.4°", SVG: "<svg>lr</svg>"},
			{Room: "Bedroom", NoData: true},
		},
		LightRooms: []LightRoomView{
			{Room: "Living Room", On: 2, Total: 3},
			{Room: "Office", On: 0, Total: 2},
		},
		Sensors: []SensorView{
			{Name: "Front Door", Attention: false, Label: "Closed"},
			{Name: "LR Window", Attention: true, Label: "Open"},
		},
		LiveURL:         "/ha-widget/live.json",
		PollIntervalMS:  10000,
		PauseWhenHidden: true,
	}
}

func TestRenderWidget_TemperatureSection(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if !contains(html, "Living Room") || !contains(html, "21.4") {
		t.Errorf("html missing populated temperature room content")
	}
	if !contains(html, "no data") {
		t.Errorf("html missing NoData fallback for Bedroom")
	}
	if !contains(html, "<svg>lr</svg>") {
		t.Errorf("html missing rendered sparkline SVG")
	}
}

func TestRenderWidget_LightsSection(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if !contains(html, `data-room="Living Room"`) {
		t.Errorf("html missing data-room attribute for live updates")
	}
	if !contains(html, "2/3 on") || !contains(html, "0/2 on") {
		t.Errorf("html missing light counts")
	}
	if !contains(html, "ha-on") {
		t.Errorf("html missing ha-on class for a room with a light on")
	}
}

func TestRenderWidget_SensorsSection(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if !contains(html, `data-name="Front Door"`) || !contains(html, `data-name="LR Window"`) {
		t.Errorf("html missing data-name attributes for live updates")
	}
	if !contains(html, "Closed") || !contains(html, "Open") {
		t.Errorf("html missing sensor state labels")
	}
}

func TestRenderWidget_EmptySectionsShowFallbackMessages(t *testing.T) {
	data := WidgetData{Title: "Home", ChartStyle: "sparkline"}
	html := RenderWidget(data)
	if !contains(html, "no rooms with a temperature sensor") {
		t.Errorf("html missing empty-temperature fallback")
	}
	if !contains(html, "no rooms with lights") {
		t.Errorf("html missing empty-lights fallback")
	}
	if !contains(html, "no contact/motion sensors found") {
		t.Errorf("html missing empty-sensors fallback")
	}
}

func TestRenderWidget_EscapesRoomNames(t *testing.T) {
	data := sampleWidgetData()
	data.LightRooms = []LightRoomView{{Room: "<script>alert(1)</script>", On: 0, Total: 1}}
	html := RenderWidget(data)
	if contains(html, "<script>alert(1)</script>") {
		t.Errorf("html contains unescaped room name, want it HTML-escaped")
	}
}

func TestRenderWidget_BootstrapScriptCarriesLiveConfig(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if !contains(html, `data-live-url="/ha-widget/live.json"`) {
		t.Errorf("html missing data-live-url attribute")
	}
	if !contains(html, `data-poll-ms="10000"`) {
		t.Errorf("html missing data-poll-ms attribute")
	}
	if !contains(html, `data-pause-hidden="true"`) {
		t.Errorf("html missing data-pause-hidden attribute")
	}
	if !contains(html, "onerror=") {
		t.Errorf("html missing the onerror bootstrap trigger")
	}
}

func TestRenderWidget_BarsStyleOmitsSeparateHeaderValue(t *testing.T) {
	data := sampleWidgetData()
	data.ChartStyle = "bars"
	html := RenderWidget(data)
	if !contains(html, "ha-temp-room-label") {
		t.Errorf("html missing bars-style room label wrapper")
	}
}

func TestRenderUnavailable_ContainsMessage(t *testing.T) {
	html := RenderUnavailable()
	if !contains(html, "Home Assistant unavailable") {
		t.Errorf("html = %q, want unavailable message", html)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/render/... -v`
Expected: FAIL — compile error, `WidgetData`/`RenderWidget`/`RenderUnavailable` undefined.

- [ ] **Step 3: Write template.go**

Create `internal/render/template.go`:
```go
package render

import (
	"fmt"
	"html"
	"strings"
)

type WidgetData struct {
	Title            string
	ChartHeight      int
	ChartStyle       string // "sparkline" or "bars"
	TemperatureRooms []TemperatureRoomView
	LightRooms       []LightRoomView
	Sensors          []SensorView
	LiveURL          string
	PollIntervalMS   int
	PauseWhenHidden  bool
}

type TemperatureRoomView struct {
	Room   string
	Value  string
	SVG    string
	NoData bool
}

type LightRoomView struct {
	Room  string
	On    int
	Total int
}

type SensorView struct {
	Name      string
	Attention bool
	Label     string
}

func styleBlock() string {
	return `<style>
.ha-body{display:flex;flex-direction:column;gap:20px}
.ha-section-head{display:flex;align-items:center;gap:8px;margin-bottom:10px}
.ha-section-label{font-size:.85em;letter-spacing:.08em;text-transform:uppercase;color:var(--color-text-subdue)}
.ha-live-badge{display:inline-flex;align-items:center;gap:5px;font-size:.7em;letter-spacing:.06em;text-transform:uppercase;color:var(--color-primary)}
.ha-live-dot{width:6px;height:6px;border-radius:50%;background:var(--color-primary)}
.ha-temp-row{display:flex;gap:12px;flex-wrap:wrap}
.ha-temp-panel{flex:1;min-width:145px;background:var(--color-widget-background-highlight);border-radius:6px;padding:10px 12px}
.ha-temp-top{display:flex;justify-content:space-between;align-items:baseline;margin-bottom:6px}
.ha-temp-room-label{color:var(--color-text-subdue);font-size:.85em;margin-bottom:4px}
.ha-temp-nodata{color:var(--color-text-subdue);font-size:.85em;padding:8px 0}
.ha-lights-grid{display:grid;grid-template-columns:repeat(2,1fr);gap:10px}
.ha-light-chip{display:flex;align-items:center;justify-content:space-between;gap:10px;background:var(--color-widget-background-highlight);border-radius:6px;padding:9px 12px}
.ha-light-left{display:flex;align-items:center;gap:9px;min-width:0}
.ha-sensor-list{display:flex;flex-direction:column;gap:1px;background:var(--color-widget-content-border);border-radius:6px;overflow:hidden}
.ha-sensor-row{display:flex;align-items:center;justify-content:space-between;gap:10px;background:var(--color-widget-background-highlight);padding:9px 12px}
.ha-sensor-left{display:flex;align-items:center;gap:9px;min-width:0}
.ha-dot{flex:none;width:8px;height:8px;border-radius:50%;border:1.5px solid var(--color-text-subdue);background:transparent}
.ha-dot.ha-on{border-color:var(--color-primary);background:var(--color-primary)}
.ha-unavailable{color:var(--color-text-subdue);padding:12px 0}
</style>`
}

// bootstrapScript runs via an onerror attribute (see RenderWidget) because
// Glance mounts extension widget HTML with element.innerHTML, and <script>
// elements inserted that way are inert per the HTML spec — onerror/onload
// content attributes are not, so they're the standard way to run JS in
// HTML delivered through an innerHTML sink.
const bootstrapScript = `(function(img){var root=img.closest('.ha-widget');if(!root)return;var url=root.dataset.liveUrl;var interval=parseInt(root.dataset.pollMs,10)||10000;var pauseWhenHidden=root.dataset.pauseHidden==='true';var timer=null;function applyState(data){(data.lights||[]).forEach(function(l){var chip=root.querySelector('.ha-light-chip[data-room="'+CSS.escape(l.room)+'"]');if(!chip)return;var dot=chip.querySelector('.ha-dot');var count=chip.querySelector('.ha-light-count');if(dot)dot.classList.toggle('ha-on',l.on>0);if(count)count.textContent=l.on+'/'+l.total+' on';});(data.sensors||[]).forEach(function(s){var row=root.querySelector('.ha-sensor-row[data-name="'+CSS.escape(s.name)+'"]');if(!row)return;var dot=row.querySelector('.ha-dot');var state=row.querySelector('.ha-sensor-state');if(dot)dot.classList.toggle('ha-on',s.attention);if(state)state.textContent=s.label;});}function poll(){fetch(url,{cache:'no-store'}).then(function(r){return r.ok?r.json():null;}).then(function(data){if(data)applyState(data);}).catch(function(){});}function stop(){if(timer){clearInterval(timer);timer=null;}}function schedule(){stop();timer=setInterval(poll,interval);}if(pauseWhenHidden){document.addEventListener('visibilitychange',function(){if(document.hidden){stop();}else{poll();schedule();}});}if(!pauseWhenHidden||!document.hidden){poll();schedule();}})(this)`

func RenderWidget(data WidgetData) string {
	var b strings.Builder
	b.WriteString(styleBlock())

	pauseAttr := "false"
	if data.PauseWhenHidden {
		pauseAttr = "true"
	}
	fmt.Fprintf(&b, `<div class="ha-widget ha-body" data-live-url="%s" data-poll-ms="%d" data-pause-hidden="%s">`,
		html.EscapeString(data.LiveURL), data.PollIntervalMS, pauseAttr)

	// TEMPERATURE
	b.WriteString(`<div><div class="ha-section-head"><span class="ha-section-label">Temperature</span></div><div class="ha-temp-row">`)
	for _, r := range data.TemperatureRooms {
		b.WriteString(`<div class="ha-temp-panel">`)
		if data.ChartStyle == "bars" {
			fmt.Fprintf(&b, `<div class="ha-temp-room-label">%s</div>`, html.EscapeString(r.Room))
			if r.NoData {
				b.WriteString(`<div class="ha-temp-nodata">no data</div>`)
			} else {
				b.WriteString(r.SVG)
			}
		} else {
			fmt.Fprintf(&b, `<div class="ha-temp-top"><span class="color-text-subdue">%s</span>`, html.EscapeString(r.Room))
			if r.NoData {
				b.WriteString(`<span class="color-text-base">–</span></div><div class="ha-temp-nodata">no data</div>`)
			} else {
				fmt.Fprintf(&b, `<span class="color-text-base">%s</span></div>%s`, html.EscapeString(r.Value), r.SVG)
			}
		}
		b.WriteString(`</div>`)
	}
	if len(data.TemperatureRooms) == 0 {
		b.WriteString(`<div class="ha-temp-nodata">no rooms with a temperature sensor</div>`)
	}
	b.WriteString(`</div></div>`)

	// LIGHTS
	b.WriteString(`<div><div class="ha-section-head"><span class="ha-section-label">Lights</span><span class="ha-live-badge"><span class="ha-live-dot"></span>live</span></div><div class="ha-lights-grid">`)
	for _, l := range data.LightRooms {
		dotClass := "ha-dot"
		if l.On > 0 {
			dotClass = "ha-dot ha-on"
		}
		fmt.Fprintf(&b, `<div class="ha-light-chip" data-room="%s"><div class="ha-light-left"><span class="%s"></span><span>%s</span></div><span class="ha-light-count">%d/%d on</span></div>`,
			html.EscapeString(l.Room), dotClass, html.EscapeString(l.Room), l.On, l.Total)
	}
	if len(data.LightRooms) == 0 {
		b.WriteString(`<div class="ha-temp-nodata">no rooms with lights</div>`)
	}
	b.WriteString(`</div></div>`)

	// SENSORS
	b.WriteString(`<div><div class="ha-section-head"><span class="ha-section-label">Sensors</span><span class="ha-live-badge"><span class="ha-live-dot"></span>live</span></div><div class="ha-sensor-list">`)
	for _, s := range data.Sensors {
		dotClass := "ha-dot"
		if s.Attention {
			dotClass = "ha-dot ha-on"
		}
		fmt.Fprintf(&b, `<div class="ha-sensor-row" data-name="%s"><div class="ha-sensor-left"><span class="%s"></span><span>%s</span></div><span class="ha-sensor-state">%s</span></div>`,
			html.EscapeString(s.Name), dotClass, html.EscapeString(s.Name), html.EscapeString(s.Label))
	}
	if len(data.Sensors) == 0 {
		b.WriteString(`<div class="ha-temp-nodata">no contact/motion sensors found</div>`)
	}
	b.WriteString(`</div></div>`)

	fmt.Fprintf(&b, `<img src="x" alt="" style="display:none;width:0;height:0" onerror="%s">`, html.EscapeString(bootstrapScript))
	b.WriteString(`</div>`)

	return b.String()
}

func RenderUnavailable() string {
	return styleBlock() + `<div class="ha-unavailable">Home Assistant unavailable</div>`
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/render/... -v`
Expected: PASS — all `TestRenderWidget_*` and `TestRenderUnavailable_*` tests plus everything from Tasks 8/9.

- [ ] **Step 5: Commit**

```bash
git add internal/render/template.go internal/render/template_test.go
git commit -m "Add full widget HTML renderer with live-update bootstrap script"
```

---

## Task 11: Live JSON renderer

**Files:**
- Create: `internal/render/live.go`
- Test: `internal/render/live_test.go`

**Interfaces:**
- Consumes: `LightRoomView`, `SensorView` from Task 10.
- Produces: `func RenderLive(lightRooms []LightRoomView, sensors []SensorView) ([]byte, error)`.

- [ ] **Step 1: Write the failing test**

Create `internal/render/live_test.go`:
```go
package render

import (
	"encoding/json"
	"testing"
)

func TestRenderLive_MarshalsLightsAndSensors(t *testing.T) {
	body, err := RenderLive(
		[]LightRoomView{{Room: "Living Room", On: 2, Total: 3}},
		[]SensorView{{Name: "Front Door", Attention: false, Label: "Closed"}},
	)
	if err != nil {
		t.Fatalf("RenderLive: %v", err)
	}

	var parsed struct {
		Lights []struct {
			Room  string `json:"room"`
			On    int    `json:"on"`
			Total int    `json:"total"`
		} `json:"lights"`
		Sensors []struct {
			Name      string `json:"name"`
			Attention bool   `json:"attention"`
			Label     string `json:"label"`
		} `json:"sensors"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(parsed.Lights) != 1 || parsed.Lights[0].Room != "Living Room" || parsed.Lights[0].On != 2 || parsed.Lights[0].Total != 3 {
		t.Errorf("Lights = %+v", parsed.Lights)
	}
	if len(parsed.Sensors) != 1 || parsed.Sensors[0].Name != "Front Door" || parsed.Sensors[0].Attention != false || parsed.Sensors[0].Label != "Closed" {
		t.Errorf("Sensors = %+v", parsed.Sensors)
	}
}

func TestRenderLive_EmptyInputProducesEmptyArraysNotNull(t *testing.T) {
	body, err := RenderLive(nil, nil)
	if err != nil {
		t.Fatalf("RenderLive: %v", err)
	}
	if !contains(string(body), `"lights":[]`) {
		t.Errorf("body = %s, want \"lights\":[] not null", body)
	}
	if !contains(string(body), `"sensors":[]`) {
		t.Errorf("body = %s, want \"sensors\":[] not null", body)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/render/... -v`
Expected: FAIL — compile error, `RenderLive` undefined.

- [ ] **Step 3: Write live.go**

Create `internal/render/live.go`:
```go
package render

import "encoding/json"

type LiveLight struct {
	Room  string `json:"room"`
	On    int    `json:"on"`
	Total int    `json:"total"`
}

type LiveSensor struct {
	Name      string `json:"name"`
	Attention bool   `json:"attention"`
	Label     string `json:"label"`
}

type LivePayload struct {
	Lights  []LiveLight  `json:"lights"`
	Sensors []LiveSensor `json:"sensors"`
}

func RenderLive(lightRooms []LightRoomView, sensors []SensorView) ([]byte, error) {
	payload := LivePayload{
		Lights:  make([]LiveLight, len(lightRooms)),
		Sensors: make([]LiveSensor, len(sensors)),
	}
	for i, l := range lightRooms {
		payload.Lights[i] = LiveLight{Room: l.Room, On: l.On, Total: l.Total}
	}
	for i, s := range sensors {
		payload.Sensors[i] = LiveSensor{Name: s.Name, Attention: s.Attention, Label: s.Label}
	}
	return json.Marshal(payload)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/render/... -v`
Expected: PASS — all `TestRenderLive_*` tests plus everything from Tasks 8-10.

- [ ] **Step 5: Commit**

```bash
git add internal/render/live.go internal/render/live_test.go
git commit -m "Add /live.json payload renderer"
```

---

## Task 12: HTTP server wiring (main.go)

**Files:**
- Create: `main.go`
- Test: `main_test.go`

**Interfaces:**
- Consumes: `Config` (Task 1), `hass.Client`/`hass.AreaCache`/`hass.BuildModel`/`hass.BuildTimestamps`/`hass.StepForwardFill`/`hass.AverageSeries` (Tasks 2-7), `render.Sparkline`/`render.BarChart`/`render.RenderWidget`/`render.RenderUnavailable`/`render.RenderLive` (Tasks 8-11).
- Produces: `type app struct { cfg *Config; cache *hass.AreaCache; client *hass.Client }`, `func newApp(cfg *Config) *app`, `func liveURL(publicURL string) string`, `func sparseAxisLabels(timestamps []time.Time) []string`, `func newMux(cfg *Config, a *app) *http.ServeMux`.

- [ ] **Step 1: Write the failing test**

Create `main_test.go`:
```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -v`
Expected: FAIL — compile error, `main.go` doesn't exist yet (`newMux`, `newApp`, `liveURL`, `sparseAxisLabels` undefined).

- [ ] **Step 3: Write main.go**

Create `main.go`:
```go
package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sidun-av/glance-homeassistant/internal/hass"
	"github.com/sidun-av/glance-homeassistant/internal/render"
)

// app bundles the long-lived dependencies each handler needs.
type app struct {
	cfg    *Config
	cache  *hass.AreaCache
	client *hass.Client
}

func newApp(cfg *Config) *app {
	client := hass.New(cfg.HomeAssistant.URL, cfg.HomeAssistant.Token)
	return &app{cfg: cfg, cache: hass.NewAreaCache(client, 5*time.Minute), client: client}
}

func liveURL(publicURL string) string {
	return strings.TrimRight(publicURL, "/") + "/live.json"
}

func sparseAxisLabels(timestamps []time.Time) []string {
	labels := make([]string, len(timestamps))
	if len(timestamps) == 0 {
		return labels
	}
	last := len(timestamps) - 1
	labels[0] = timestamps[0].Format("15:04")
	labels[last] = timestamps[last].Format("15:04")
	if last > 1 {
		labels[last/2] = timestamps[last/2].Format("15:04")
	}
	return labels
}

func (a *app) buildModel(ctx context.Context) (hass.Model, error) {
	rooms, err := a.cache.Get(ctx)
	if err != nil {
		return hass.Model{}, fmt.Errorf("fetch areas: %w", err)
	}
	states, err := a.client.FetchStates(ctx)
	if err != nil {
		return hass.Model{}, fmt.Errorf("fetch states: %w", err)
	}
	return hass.BuildModel(rooms, states, hass.ClassificationConfig{
		ContactDeviceClasses: a.cfg.Sensors.ContactDeviceClasses,
		MotionDeviceClasses:  a.cfg.Sensors.MotionDeviceClasses,
	}), nil
}

func (a *app) widgetHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	w.Header().Set("Widget-Title", a.cfg.Title)
	w.Header().Set("Widget-Content-Type", "html")

	model, err := a.buildModel(ctx)
	if err != nil {
		log.Printf("home assistant unavailable: %v", err)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, render.RenderUnavailable())
		return
	}

	rangeDur, _ := time.ParseDuration(a.cfg.Temperature.Range)
	pollInterval, _ := time.ParseDuration(a.cfg.Live.PollInterval)
	now := time.Now()
	timestamps := hass.BuildTimestamps(now, rangeDur, a.cfg.Temperature.MaxPoints)
	axisLabels := sparseAxisLabels(timestamps)

	var allTempIDs []string
	for _, tr := range model.TemperatureRooms {
		allTempIDs = append(allTempIDs, tr.EntityIDs...)
	}

	history, err := a.client.FetchHistory(ctx, allTempIDs, now.Add(-rangeDur), now)
	if err != nil {
		log.Printf("fetch history: %v", err)
		history = map[string][]hass.HistoryPoint{}
	}

	tempViews := make([]render.TemperatureRoomView, len(model.TemperatureRooms))
	for i, tr := range model.TemperatureRooms {
		var series [][]float64
		for _, id := range tr.EntityIDs {
			points, ok := history[id]
			if !ok || len(points) == 0 {
				continue
			}
			series = append(series, hass.StepForwardFill(points, timestamps))
		}
		avg := hass.AverageSeries(series)
		if len(avg) == 0 || math.IsNaN(avg[len(avg)-1]) {
			tempViews[i] = render.TemperatureRoomView{Room: tr.Room, NoData: true}
			continue
		}

		value := fmt.Sprintf("%.1f°", avg[len(avg)-1])
		var svg string
		if a.cfg.Temperature.ChartStyle == "bars" {
			barOpts := render.BarChartOptions{Width: 220, Height: float64(a.cfg.Temperature.ChartHeight + 27)}
			svg = render.BarChart(avg, axisLabels, value, barOpts)
		} else {
			svg = render.Sparkline(avg, render.SparklineOptions{Width: 220, Height: float64(a.cfg.Temperature.ChartHeight)})
		}
		tempViews[i] = render.TemperatureRoomView{Room: tr.Room, Value: value, SVG: svg}
	}

	lightViews := make([]render.LightRoomView, len(model.LightRooms))
	for i, lr := range model.LightRooms {
		lightViews[i] = render.LightRoomView{Room: lr.Room, On: lr.On, Total: lr.Total}
	}
	sensorViews := make([]render.SensorView, len(model.Sensors))
	for i, s := range model.Sensors {
		sensorViews[i] = render.SensorView{Name: s.Name, Attention: s.Attention, Label: s.Label}
	}

	widgetData := render.WidgetData{
		Title:            a.cfg.Title,
		ChartHeight:      a.cfg.Temperature.ChartHeight,
		ChartStyle:       a.cfg.Temperature.ChartStyle,
		TemperatureRooms: tempViews,
		LightRooms:       lightViews,
		Sensors:          sensorViews,
		LiveURL:          liveURL(a.cfg.PublicURL),
		PollIntervalMS:   int(pollInterval.Milliseconds()),
		PauseWhenHidden:  a.cfg.Live.PauseWhenHidden != nil && *a.cfg.Live.PauseWhenHidden,
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, render.RenderWidget(widgetData))
}

func (a *app) liveHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	model, err := a.buildModel(ctx)
	if err != nil {
		log.Printf("home assistant unavailable: %v", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	lightViews := make([]render.LightRoomView, len(model.LightRooms))
	for i, lr := range model.LightRooms {
		lightViews[i] = render.LightRoomView{Room: lr.Room, On: lr.On, Total: lr.Total}
	}
	sensorViews := make([]render.SensorView, len(model.Sensors))
	for i, s := range model.Sensors {
		sensorViews[i] = render.SensorView{Name: s.Name, Attention: s.Attention, Label: s.Label}
	}

	body, err := render.RenderLive(lightViews, sensorViews)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

func newMux(cfg *Config, a *app) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})
	mux.HandleFunc("/widget", a.widgetHandler)
	mux.HandleFunc("/live.json", a.liveHandler)
	return mux
}

func main() {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "/config.yml"
	}
	cfg, err := LoadConfig(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	a := newApp(cfg)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, newMux(cfg, a)))
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go build ./... && go test ./... -v`
Expected: PASS — every test across `main_test.go`, `config_test.go`, `internal/hass/*_test.go`, `internal/render/*_test.go`. `go build ./...` must also succeed cleanly.

- [ ] **Step 5: Commit**

```bash
git add main.go main_test.go
git commit -m "Wire up HTTP server: /widget, /live.json, /healthz"
```

---

## Task 13: Dockerfile, compose example, CI

**Files:**
- Create: `Dockerfile`
- Create: `docker-compose.example.yml`
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Write the Dockerfile**

Create `Dockerfile`:
```dockerfile
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/glance-homeassistant .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/glance-homeassistant /glance-homeassistant
COPY config.example.yml /config.yml
EXPOSE 8080
ENTRYPOINT ["/glance-homeassistant"]
```

- [ ] **Step 2: Build the image locally to verify it works**

Run: `docker build -t glance-homeassistant:dev .`
Expected: image builds successfully (both stages complete, no errors).

- [ ] **Step 3: Write docker-compose.example.yml**

Create `docker-compose.example.yml`:
```yaml
services:
  glance-homeassistant:
    image: ghcr.io/sidun-av/glance-homeassistant:latest
    restart: unless-stopped
    environment:
      - HA_URL=http://homeassistant:8123
      - HA_TOKEN=${HA_TOKEN}
    volumes:
      - ./glance-homeassistant/config.yml:/config.yml:ro
```

- [ ] **Step 4: Write the CI workflow**

Create `.github/workflows/ci.yml`:
```yaml
name: CI

on:
  push:
    branches: [main]
    tags: ['v*']
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - run: go test ./...

  docker:
    needs: test
    if: github.event_name == 'push'
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - uses: docker/metadata-action@v5
        id: meta
        with:
          images: ghcr.io/${{ github.repository }}
          tags: |
            type=raw,value=latest,enable={{is_default_branch}}
            type=semver,pattern={{version}}
      - uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
```

- [ ] **Step 5: Commit**

```bash
git add Dockerfile docker-compose.example.yml .github/workflows/ci.yml
git commit -m "Add Dockerfile, compose example, and CI workflow"
```

---

## Task 14: README

**Files:**
- Create: `README.md`

- [ ] **Step 1: Write README.md**

Create `README.md`:
```markdown
# glance-homeassistant

A [Glance](https://github.com/glanceapp/glance) extension widget that shows Home Assistant data
in Glance's own visual language: room temperature (sparkline or bar-chart style), which lights
are on per room, and contact/motion sensor state — with lights and sensors updating live in the
browser while the dashboard tab stays open.

## How it works

A small Go HTTP service Glance calls on its own schedule (a Glance
[extension widget](https://github.com/glanceapp/glance/blob/main/docs/extensions.md)). Rooms and
their entities are read from Home Assistant's Areas via a server-rendered Jinja template
(`POST /api/template`), current values via `GET /api/states`, and temperature history via
`GET /api/history/period`. Lights and sensors additionally update live: the widget's HTML embeds
a small bootstrap script (triggered via the standard `<img onerror>` technique, since Glance
mounts extension HTML with `innerHTML`, which never executes plain `<script>` tags) that polls a
lightweight `/live.json` endpoint on this same service every ~10 seconds while the tab is open and
visible, patching just the lights/sensors DOM in place. Close the tab and the polling stops on its
own — nothing to configure.

## Setup

### 1. Create a Home Assistant long-lived access token

In Home Assistant: **Profile → Security → Long-Lived Access Tokens → Create Token**. Copy it —
you'll only see it once.

### 2. Expose this service to your browser, not just to Glance

Glance's own server calls `/widget` over your internal Docker network — that part just needs
`HA_URL`/`HA_TOKEN` below. But the live-update script runs in the *browser*, so it needs its own
route to this service's `/live.json`, reachable from wherever you actually open Glance (locally
and/or externally).

If you reverse-proxy Glance (e.g. NPMplus) on the same host/domain, add a location block that
proxies a path prefix to this container, and it'll work from both local and external URLs
automatically:

```
location /ha-widget/ {
    proxy_pass http://glance-homeassistant:8080/;
}
```

Then set `public_url: /ha-widget` in `config.yml` (see below). If you'd rather expose this
container on its own LAN port instead, set `public_url` to that full origin, e.g.
`http://192.168.1.50:8081`.

### 3. Configure

Copy [`config.example.yml`](config.example.yml) to `config.yml` and edit:

- `home_assistant.url` / `home_assistant.token` — reachable from *this container* (e.g. the HA
  container/host's address on your Docker/LAN network).
- `public_url` — reachable from *your browser* (see step 2).
- `temperature.chart_style` — `sparkline` (default, matches the SERVER STATS widget style) or
  `bars` (matches the built-in WEATHER widget's bar-chart style).
- `sensors.contact_device_classes` / `sensors.motion_device_classes` — HA `binary_sensor`
  device classes to treat as contact/motion sensors, if your setup uses ones not covered by the
  defaults.

### 4. Run it alongside Glance

```yaml
services:
  glance-homeassistant:
    image: ghcr.io/sidun-av/glance-homeassistant:latest
    restart: unless-stopped
    environment:
      - HA_URL=http://homeassistant:8123
      - HA_TOKEN=${HA_TOKEN}
    volumes:
      - ./glance-homeassistant/config.yml:/config.yml:ro
```

Add it to the same Docker network as Home Assistant.

### 5. Add the widget to Glance

```yaml
- type: extension
  url: http://glance-homeassistant:8080/widget
  cache: 15m
  allow-potentially-dangerous-html: true
```

`cache: 15m` is intentionally slow — temperature doesn't need to update often, and lights/sensors
get their freshness from the separate live-update mechanism instead, not from this cache.

## Configuration reference

| Field | Default | Description |
|---|---|---|
| `home_assistant.url` | — (required) | Home Assistant base URL, reachable from this container |
| `home_assistant.token` | — (required) | Home Assistant long-lived access token |
| `public_url` | `""` (site root) | Path or origin the *browser* uses to reach this service's `/live.json` |
| `title` | `Home` | Widget title shown in Glance |
| `temperature.range` | `24h` | Historical window for the temperature chart, a Go duration (`h`/`m`/`s` units only) |
| `temperature.max_points` | `60` | Points per room's temperature series (resolution) |
| `temperature.chart_height` | `34` | Chart height in px (bars add extra space above/below for labels automatically) |
| `temperature.chart_style` | `sparkline` | `sparkline` or `bars` |
| `live.poll_interval` | `10s` | How often the browser polls `/live.json` while the tab is open |
| `live.pause_when_hidden` | `true` | Pause polling while the browser tab is backgrounded |
| `sensors.contact_device_classes` | `[door, window, garage_door, opening]` | `binary_sensor` device classes shown as Open/Closed |
| `sensors.motion_device_classes` | `[motion, occupancy]` | `binary_sensor` device classes shown as Motion/Clear |

The service itself (not `config.yml`) is also configured via two environment variables:

| Env var | Default | Description |
|---|---|---|
| `PORT` | `8080` | Port the HTTP server listens on |
| `CONFIG_PATH` | `/config.yml` | Path to the config file read at startup |

## Error handling

If Home Assistant is unreachable, the whole widget shows a single "Home Assistant unavailable"
message instead of Glance's generic widget-failed state. If a specific room has no temperature
history, only that room's panel shows "no data" — the rest of the widget still renders normally.
`/live.json` failing at poll time leaves the last-known lights/sensors state on screen rather than
clearing it, and retries on the next interval.

## Out of scope (for now)

Light control (this widget is read-only), humidity and other HA domains, multiple Home Assistant
instances, history beyond HA's recorder retention, and pagination (aimed at homes small enough to
fit on one screen).

## Development

```bash
go test ./...
docker build -t glance-homeassistant:dev .
```
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "Add README"
```

---

## Final verification

- [ ] Run `go build ./... && go vet ./... && go test ./... -v` from the repo root — everything must build, vet cleanly, and pass.
- [ ] Run `docker build -t glance-homeassistant:dev .` — must succeed end-to-end.
- [ ] Confirm every file listed in the design spec's "Repository layout" section exists: `main.go`, `config.go`, `config.example.yml`, `internal/hass/{client,cache,discover,resample}.go`, `internal/render/{sparkline,barchart,template,live}.go`, `Dockerfile`, `docker-compose.example.yml`, `README.md`, `LICENSE`, `.github/workflows/ci.yml`.
- [ ] Push the branch and confirm the CI workflow's `test` job passes on GitHub Actions.
