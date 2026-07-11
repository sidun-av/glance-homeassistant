# HOME Widget Room-Card Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the widget's three stacked sections (Temperature / Lights / Sensors) with one adaptive flexbox grid of per-room cards, each showing whichever of temperature/lights/occupancy/contact that room actually has, with real HA-assigned icons for lights and live updates for everything.

**Architecture:** `internal/hass` merges its existing classification output into one `[]RoomCard` per room (dropping rooms with nothing to show), and `SensorEntity` gains a `Room` field. `internal/render` gains a curated icon-glyph lookup and a full rewrite of `RenderWidget`/`RenderLive` around a new `RoomCardView` type, using CSS flexbox (`flex-grow`/`flex-basis`, no `grid-row`/`grid-column` spans) for adaptive sizing and `data-*` attribute selectors (not class toggling) so the live-update bootstrap script never needs to know a light's fixture type or reconstruct any markup. `main.go` gains a small `hass.RoomCard` → `render.RoomCardView` mapping shared by both handlers.

**Tech Stack:** No new dependencies. Same Go stdlib + `gopkg.in/yaml.v3`, same inline-SVG-in-HTML approach, same `element.innerHTML` + `onerror` bootstrap trick.

Design spec: `docs/superpowers/specs/2026-07-11-home-widget-room-cards-design.md` — read it first for the *why* behind each decision below; this plan is the *how*.

## Global Constraints

- This **replaces** the three-section layout entirely. No config flag reverts to it.
- Every color must be a real Glance CSS custom property, verified against `github.com/glanceapp/glance`'s `main.css` (not guessed): `--color-progress-value` (temperature chart), `--color-primary` (LIVE badge, occupancy chip/glow), `--color-negative` (contact "open" state, confirmed present as `hsl(0, 70%, 70%)` in the default theme), `--color-text-subdue`/`--color-text-highlight`, `--color-widget-background-highlight`/`--color-widget-content-border`. The "light is on" amber and "room is lit" tint are deliberately hardcoded (`#f0c479` / `rgba(240,196,121,...)`), not theme tokens — a universal warm-light cue, not an accent-tracking one.
- Alpha-blended variants of a *theme* color (the occupied glow/chip) use CSS `color-mix(in srgb, var(--color-primary) N%, transparent)` — Glance's own CSS vars are plain `hsl(...)` values with no separate alpha-friendly channel exposed, and `color-mix()` is supported in all current major browsers. Hardcoded colors (amber) just use plain `rgba()`.
- All live-updatable state (a light's on/off, a room's lit/occupied state, a contact sensor's open/closed state) is carried via `data-*` attributes read/written by both the initial Go render and the bootstrap JS, never via class-name toggling — the JS never needs to know a light's fixture type (`bulb` vs `track-light` vs `led-strip`) to update it, only whether it's on.
- No new environment variables. The icon glyph lookup table is hardcoded, not user-configurable.
- `TEMPERATURE_CHART_HEIGHT` / `temperature.chart_height` changes meaning (Task 8) from "chart SVG pixel height" to "base card min-height in px"; its `LoadConfig` default changes from `34` to `130`.
- Every task ends with `go test ./...`, `gofmt -l .` (must print nothing), and `go vet ./...` all clean before committing.

---

### Task 1: `hass.EntityState` gains an `Icon` field

**Files:**
- Modify: `internal/hass/client.go`
- Test: `internal/hass/client_test.go`

**Interfaces:**
- Produces: `EntityState.Icon string` — HA's raw `attributes.icon` value (e.g. `"mdi:track-light"`), `""` if the entity has no custom icon. Consumed by Task 2's `BuildModel` (copied onto `Light.Icon`) and, indirectly, Task 3's `LightIcon`.

- [ ] **Step 1: Write the failing test**

Add to `internal/hass/client_test.go`, extending the existing `TestFetchStates_ParsesEntities` fixture (the light entity gains an `icon` attribute; the temperature entity has none, to confirm the empty-string default):

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
			{"entity_id":"light.living_room_main","state":"on","attributes":{"friendly_name":"Living Room Main Light","icon":"mdi:track-light"}},
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
	if temp.Icon != "" {
		t.Errorf("temp.Icon = %q, want empty (no icon attribute set)", temp.Icon)
	}

	light := states["light.living_room_main"]
	if light.Domain != "light" || light.State != "on" {
		t.Errorf("light entity = %+v", light)
	}
	if light.Icon != "mdi:track-light" {
		t.Errorf("light.Icon = %q, want mdi:track-light", light.Icon)
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/hass/... -run TestFetchStates_ParsesEntities -v`
Expected: FAIL — `light.Icon` undefined (field doesn't exist on `EntityState`) or compile error.

- [ ] **Step 3: Implement**

In `internal/hass/client.go`, add `Icon` to `EntityState`:

```go
type EntityState struct {
	EntityID     string
	Domain       string
	State        string
	FriendlyName string
	DeviceClass  string
	Icon         string
}
```

Add `Icon` to the raw decode struct and copy it through in `FetchStates`:

```go
	type rawState struct {
		EntityID   string `json:"entity_id"`
		State      string `json:"state"`
		Attributes struct {
			FriendlyName string `json:"friendly_name"`
			DeviceClass  string `json:"device_class"`
			Icon         string `json:"icon"`
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
			Icon:         s.Attributes.Icon,
		}
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/hass/... -v`
Expected: PASS (all `internal/hass` tests, not just the one edited).

- [ ] **Step 5: Commit**

```bash
git add internal/hass/client.go internal/hass/client_test.go
git commit -m "Decode each entity's icon attribute in FetchStates"
```

---

### Task 2: Merge classification into `[]RoomCard`

**Files:**
- Modify: `internal/hass/discover.go`
- Test: `internal/hass/discover_test.go` (full rewrite)

**Interfaces:**
- Consumes: `hass.Room` (`ID`, `Name`, `EntityIDs` — unchanged), `hass.EntityState` (now including `Icon`, from Task 1).
- Produces:
  ```go
  type Light struct {
      EntityID string
      Name     string
      On       bool
      Icon     string
  }

  type SensorEntity struct {
      Room      string
      Name      string
      Attention bool
      Label     string
  }

  type RoomCard struct {
      Room        string
      Temperature *TemperatureRoom // nil if no temperature sensor
      Lights      []Light
      Occupancy   []SensorEntity
      Contacts    []SensorEntity
      Weight      int
  }

  func BuildModel(rooms []Room, states map[string]EntityState, cfg ClassificationConfig) []RoomCard
  ```
  `TemperatureRoom` (`Room`, `EntityIDs`) is unchanged. The `Model` wrapper struct is deleted — `BuildModel` now returns `[]RoomCard` directly. Consumed by Task 7's `main.go`.

- [ ] **Step 1: Write the failing tests**

Replace the entire contents of `internal/hass/discover_test.go`:

```go
package hass

import "testing"

func defaultClassificationConfig() ClassificationConfig {
	return ClassificationConfig{
		ContactDeviceClasses: []string{"door", "window", "garage_door", "opening"},
		MotionDeviceClasses:  []string{"motion", "occupancy"},
	}
}

func findCard(cards []RoomCard, room string) (RoomCard, bool) {
	for _, c := range cards {
		if c.Room == room {
			return c, true
		}
	}
	return RoomCard{}, false
}

func TestBuildModel_ClassifiesByDomainAndDeviceClass(t *testing.T) {
	rooms := []Room{
		{Name: "Living Room", EntityIDs: []string{"sensor.lr_temp", "light.lr_main", "binary_sensor.lr_window"}},
		{Name: "Bedroom", EntityIDs: []string{"light.bed_main", "binary_sensor.bed_motion"}},
	}
	states := map[string]EntityState{
		"sensor.lr_temp":           {EntityID: "sensor.lr_temp", Domain: "sensor", State: "21.4", DeviceClass: "temperature", FriendlyName: "LR Temp"},
		"light.lr_main":            {EntityID: "light.lr_main", Domain: "light", State: "on", FriendlyName: "LR Main", Icon: "mdi:track-light"},
		"binary_sensor.lr_window":  {EntityID: "binary_sensor.lr_window", Domain: "binary_sensor", State: "on", DeviceClass: "window", FriendlyName: "LR Window"},
		"light.bed_main":           {EntityID: "light.bed_main", Domain: "light", State: "off", FriendlyName: "Bed Main"},
		"binary_sensor.bed_motion": {EntityID: "binary_sensor.bed_motion", Domain: "binary_sensor", State: "off", DeviceClass: "motion", FriendlyName: "Bed Motion"},
	}

	cards := BuildModel(rooms, states, defaultClassificationConfig())
	if len(cards) != 2 {
		t.Fatalf("len(cards) = %d, want 2", len(cards))
	}

	lr, ok := findCard(cards, "Living Room")
	if !ok {
		t.Fatalf("Living Room card missing")
	}
	if lr.Temperature == nil || len(lr.Temperature.EntityIDs) != 1 || lr.Temperature.EntityIDs[0] != "sensor.lr_temp" {
		t.Errorf("Living Room.Temperature = %+v", lr.Temperature)
	}
	if len(lr.Lights) != 1 || !lr.Lights[0].On || lr.Lights[0].Icon != "mdi:track-light" || lr.Lights[0].EntityID != "light.lr_main" {
		t.Errorf("Living Room.Lights = %+v", lr.Lights)
	}
	if len(lr.Contacts) != 1 || lr.Contacts[0].Room != "Living Room" || !lr.Contacts[0].Attention || lr.Contacts[0].Label != "Open" {
		t.Errorf("Living Room.Contacts = %+v", lr.Contacts)
	}
	if len(lr.Occupancy) != 0 {
		t.Errorf("Living Room.Occupancy = %+v, want none", lr.Occupancy)
	}
	if lr.Weight != 4 { // temp(2) + 1 light + 0 occupancy + 1 contact
		t.Errorf("Living Room.Weight = %d, want 4", lr.Weight)
	}

	bed, ok := findCard(cards, "Bedroom")
	if !ok {
		t.Fatalf("Bedroom card missing")
	}
	if bed.Temperature != nil {
		t.Errorf("Bedroom.Temperature = %+v, want nil", bed.Temperature)
	}
	if len(bed.Lights) != 1 || bed.Lights[0].On {
		t.Errorf("Bedroom.Lights = %+v", bed.Lights)
	}
	if len(bed.Occupancy) != 1 || bed.Occupancy[0].Attention || bed.Occupancy[0].Label != "Clear" {
		t.Errorf("Bedroom.Occupancy = %+v", bed.Occupancy)
	}
	if bed.Weight != 2 { // 0 temp + 1 light + 1 occupancy + 0 contact
		t.Errorf("Bedroom.Weight = %d, want 2", bed.Weight)
	}
}

func TestBuildModel_RoomWithoutMatchingEntitiesIsOmitted(t *testing.T) {
	rooms := []Room{
		{Name: "Garage", EntityIDs: []string{"switch.garage_opener"}},
	}
	states := map[string]EntityState{
		"switch.garage_opener": {EntityID: "switch.garage_opener", Domain: "switch", State: "off", FriendlyName: "Garage Opener"},
	}

	cards := BuildModel(rooms, states, defaultClassificationConfig())
	if len(cards) != 0 {
		t.Errorf("cards = %+v, want none for a room with only an unclassified switch entity", cards)
	}
}

func TestBuildModel_RoomWithNoEntitiesAtAllIsOmitted(t *testing.T) {
	rooms := []Room{
		{Name: "Bathroom", EntityIDs: nil},
	}
	cards := BuildModel(rooms, map[string]EntityState{}, defaultClassificationConfig())
	if len(cards) != 0 {
		t.Errorf("cards = %+v, want none for an area with zero entities", cards)
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

	cards := BuildModel(rooms, states, defaultClassificationConfig())
	lr, ok := findCard(cards, "Living Room")
	if !ok {
		t.Fatalf("Living Room card missing")
	}
	if lr.Temperature == nil || len(lr.Temperature.EntityIDs) != 2 {
		t.Errorf("Living Room.Temperature = %+v, want both sensors collected", lr.Temperature)
	}
}

func TestBuildModel_SkipsUnavailableBinarySensor(t *testing.T) {
	rooms := []Room{
		{Name: "Hallway", EntityIDs: []string{"binary_sensor.hall_motion"}},
	}
	states := map[string]EntityState{
		"binary_sensor.hall_motion": {EntityID: "binary_sensor.hall_motion", Domain: "binary_sensor", State: "unavailable", DeviceClass: "motion", FriendlyName: "Hall Motion"},
	}

	cards := BuildModel(rooms, states, defaultClassificationConfig())
	if len(cards) != 0 {
		t.Errorf("cards = %+v, want none (only entity is an unavailable motion sensor)", cards)
	}
}

func TestBuildModel_MissingStateForEntityIsSkipped(t *testing.T) {
	rooms := []Room{
		{Name: "Office", EntityIDs: []string{"light.office_main"}},
	}
	states := map[string]EntityState{}

	cards := BuildModel(rooms, states, defaultClassificationConfig())
	if len(cards) != 0 {
		t.Errorf("cards = %+v, want none (entity missing from states map)", cards)
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

	cards := BuildModel(rooms, states, defaultClassificationConfig())
	if len(cards) != 2 || cards[0].Room != "Alpha Room" || cards[1].Room != "Zeta Room" {
		t.Errorf("cards = %+v, want alphabetical order", cards)
	}
}

func TestBuildModel_WeightCombinesAllSignals(t *testing.T) {
	rooms := []Room{
		{Name: "Living Room", EntityIDs: []string{
			"sensor.lr_temp", "light.lr_1", "light.lr_2", "light.lr_3",
			"binary_sensor.lr_motion", "binary_sensor.lr_window",
		}},
	}
	states := map[string]EntityState{
		"sensor.lr_temp":          {EntityID: "sensor.lr_temp", Domain: "sensor", State: "21.0", DeviceClass: "temperature", FriendlyName: "LR Temp"},
		"light.lr_1":              {EntityID: "light.lr_1", Domain: "light", State: "on", FriendlyName: "LR 1"},
		"light.lr_2":              {EntityID: "light.lr_2", Domain: "light", State: "on", FriendlyName: "LR 2"},
		"light.lr_3":              {EntityID: "light.lr_3", Domain: "light", State: "off", FriendlyName: "LR 3"},
		"binary_sensor.lr_motion": {EntityID: "binary_sensor.lr_motion", Domain: "binary_sensor", State: "on", DeviceClass: "occupancy", FriendlyName: "LR Motion"},
		"binary_sensor.lr_window": {EntityID: "binary_sensor.lr_window", Domain: "binary_sensor", State: "off", DeviceClass: "window", FriendlyName: "LR Window"},
	}

	cards := BuildModel(rooms, states, defaultClassificationConfig())
	lr, ok := findCard(cards, "Living Room")
	if !ok {
		t.Fatalf("Living Room card missing")
	}
	if lr.Weight != 7 { // temp(2) + 3 lights + occupancy(1) + contact(1)
		t.Errorf("Weight = %d, want 7", lr.Weight)
	}
}

func TestBuildModel_OccupancyAttentionLabel(t *testing.T) {
	rooms := []Room{
		{Name: "Hallway", EntityIDs: []string{"binary_sensor.hall_occupancy"}},
	}
	states := map[string]EntityState{
		"binary_sensor.hall_occupancy": {EntityID: "binary_sensor.hall_occupancy", Domain: "binary_sensor", State: "on", DeviceClass: "occupancy", FriendlyName: "Hall Occupancy"},
	}

	cards := BuildModel(rooms, states, defaultClassificationConfig())
	hall, ok := findCard(cards, "Hallway")
	if !ok {
		t.Fatalf("Hallway card missing")
	}
	if len(hall.Occupancy) != 1 || !hall.Occupancy[0].Attention || hall.Occupancy[0].Label != "Occupied" {
		t.Errorf("Hallway.Occupancy = %+v, want Attention=true Label=Occupied", hall.Occupancy)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/hass/... -v`
Expected: FAIL to compile — `RoomCard`, `findCard` etc. undefined, or `BuildModel` return type mismatch.

- [ ] **Step 3: Implement**

Replace the entire contents of `internal/hass/discover.go`:

```go
package hass

import "sort"

type TemperatureRoom struct {
	Room      string
	EntityIDs []string
}

type Light struct {
	EntityID string
	Name     string
	On       bool
	Icon     string
}

type SensorEntity struct {
	Room      string
	Name      string
	Attention bool
	Label     string
}

type RoomCard struct {
	Room        string
	Temperature *TemperatureRoom
	Lights      []Light
	Occupancy   []SensorEntity
	Contacts    []SensorEntity
	Weight      int
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

type roomBuilder struct {
	temp      *TemperatureRoom
	lights    []Light
	occupancy []SensorEntity
	contacts  []SensorEntity
}

// BuildModel classifies each area's entities into a per-room card:
// temperature (sensor, device_class "temperature"), lights (domain
// "light"), occupancy and contact (binary_sensor, device_class from cfg).
// A room with none of these classified is dropped entirely — there is
// nothing for its card to show.
func BuildModel(rooms []Room, states map[string]EntityState, cfg ClassificationConfig) []RoomCard {
	byRoom := make(map[string]*roomBuilder)

	for _, room := range rooms {
		for _, entityID := range room.EntityIDs {
			state, ok := states[entityID]
			if !ok {
				continue
			}

			b, exists := byRoom[room.Name]
			if !exists {
				b = &roomBuilder{}
				byRoom[room.Name] = b
			}

			switch {
			case state.Domain == "sensor" && state.DeviceClass == "temperature":
				if b.temp == nil {
					b.temp = &TemperatureRoom{Room: room.Name}
				}
				b.temp.EntityIDs = append(b.temp.EntityIDs, entityID)

			case state.Domain == "light":
				b.lights = append(b.lights, Light{
					EntityID: entityID,
					Name:     state.FriendlyName,
					On:       state.State == "on",
					Icon:     state.Icon,
				})

			case state.Domain == "binary_sensor" && contains(cfg.ContactDeviceClasses, state.DeviceClass):
				if state.State != "on" && state.State != "off" {
					continue
				}
				attention := state.State == "on"
				label := "Closed"
				if attention {
					label = "Open"
				}
				b.contacts = append(b.contacts, SensorEntity{Room: room.Name, Name: state.FriendlyName, Attention: attention, Label: label})

			case state.Domain == "binary_sensor" && contains(cfg.MotionDeviceClasses, state.DeviceClass):
				if state.State != "on" && state.State != "off" {
					continue
				}
				attention := state.State == "on"
				label := "Clear"
				if attention {
					label = "Occupied"
				}
				b.occupancy = append(b.occupancy, SensorEntity{Room: room.Name, Name: state.FriendlyName, Attention: attention, Label: label})
			}
		}
	}

	cards := make([]RoomCard, 0, len(byRoom))
	for name, b := range byRoom {
		if b.temp == nil && len(b.lights) == 0 && len(b.occupancy) == 0 && len(b.contacts) == 0 {
			continue
		}
		weight := len(b.lights)
		if b.temp != nil {
			weight += 2
		}
		if len(b.occupancy) > 0 {
			weight++
		}
		if len(b.contacts) > 0 {
			weight++
		}
		cards = append(cards, RoomCard{
			Room:        name,
			Temperature: b.temp,
			Lights:      b.lights,
			Occupancy:   b.occupancy,
			Contacts:    b.contacts,
			Weight:      weight,
		})
	}

	sort.Slice(cards, func(i, j int) bool { return cards[i].Room < cards[j].Room })
	return cards
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/hass/... -v`
Expected: PASS, all tests in the package.

- [ ] **Step 5: Commit**

```bash
git add internal/hass/discover.go internal/hass/discover_test.go
git commit -m "Merge per-room classification into a single []RoomCard"
```

---

### Task 3: Curated light/contact icon glyphs

**Files:**
- Create: `internal/render/icons.go`
- Test: `internal/render/icons_test.go`

**Interfaces:**
- Produces: `LightIcon(icon string) string`, `ContactIcon() string`. Both return raw inline `<svg>` markup with a fixed `class` naming the fixture type only (never on/off or open/closed state — that's carried by the caller's wrapper element via `data-*` attributes, per Task 5). Consumed by Task 5's `RenderWidget` and Task 7's `main.go`.
- The `icon` string passed in is never HTML-escaped or echoed into output — it is only ever used as a Go map lookup key, so there's no injection surface here to worry about.

- [ ] **Step 1: Write the failing tests**

Create `internal/render/icons_test.go`:

```go
package render

import "testing"

func TestLightIcon_KnownIconRendersMatchingGlyph(t *testing.T) {
	svg := LightIcon("mdi:track-light")
	if !contains(svg, `class="track-light"`) {
		t.Errorf("svg = %q, want a track-light glyph", svg)
	}
}

func TestLightIcon_LedStripVariant(t *testing.T) {
	svg := LightIcon("mdi:led-strip-variant")
	if !contains(svg, `class="led-strip"`) {
		t.Errorf("svg = %q, want a led-strip glyph", svg)
	}
}

func TestLightIcon_UnknownIconFallsBackToBulb(t *testing.T) {
	svg := LightIcon("mdi:alarm-off")
	if !contains(svg, `class="bulb"`) {
		t.Errorf("svg = %q, want fallback to bulb for an unrecognized icon", svg)
	}
}

func TestLightIcon_EmptyIconFallsBackToBulb(t *testing.T) {
	svg := LightIcon("")
	if !contains(svg, `class="bulb"`) {
		t.Errorf("svg = %q, want fallback to bulb for an empty icon", svg)
	}
}

func TestContactIcon_RendersDoorGlyph(t *testing.T) {
	svg := ContactIcon()
	if !contains(svg, `class="ha-door"`) {
		t.Errorf("svg = %q, want an ha-door glyph", svg)
	}
	if !contains(svg, "ha-door-frame") || !contains(svg, "ha-door-leaf") {
		t.Errorf("svg = %q, want frame and leaf parts for CSS-driven open/closed rotation", svg)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/render/... -run 'TestLightIcon|TestContactIcon' -v`
Expected: FAIL — `LightIcon`/`ContactIcon` undefined.

- [ ] **Step 3: Implement**

Create `internal/render/icons.go`:

```go
package render

// bulbGlyph, trackLightGlyph and ledStripGlyph are fixture-type glyphs
// only — they carry no on/off state in their own markup. On/off styling
// is driven entirely by the data-on attribute on the wrapper element the
// caller places the glyph in (see RenderWidget in template.go) via CSS
// attribute selectors, so a live update never needs to know which glyph a
// given light is to toggle it.
const bulbGlyph = `<svg class="bulb" viewBox="0 0 24 24" fill="none" stroke-width="1.6"><circle class="bulb-glass" cx="12" cy="10" r="6.2"/><path class="bulb-base" d="M9.3 17.5h5.4M9.8 20h4.4" stroke-linecap="round"/></svg>`

const trackLightGlyph = `<svg class="track-light" viewBox="0 0 24 24" fill="none" stroke-width="1.6"><path class="tl-rail" d="M7 4h10" stroke-linecap="round"/><path class="tl-rail" d="M12 4v3" stroke-linecap="round"/><path class="tl-head" d="M8.3 7h7.4l-2 6h-3.4z" stroke-linejoin="round"/><path class="tl-ray" d="M10 15.5v2.2M12 15.5v2.8M14 15.5v2.2" stroke-linecap="round"/></svg>`

const ledStripGlyph = `<svg class="led-strip" viewBox="0 0 24 24" fill="none" stroke-width="1.6"><rect class="ls-body" x="3" y="10" width="18" height="4.4" rx="2.2"/><circle class="ls-led" cx="6.6" cy="12.2" r=".9" stroke="none"/><circle class="ls-led" cx="10.6" cy="12.2" r=".9" stroke="none"/><circle class="ls-led" cx="14.6" cy="12.2" r=".9" stroke="none"/><circle class="ls-led" cx="18.6" cy="12.2" r=".9" stroke="none"/></svg>`

// iconGlyphs maps HA's raw icon attribute to a curated fixture-type glyph.
// This is deliberately a small, hardcoded set, not a full Material Design
// Icons integration — extending it later (a new fixture type shows up in
// real data) is one new glyph constant plus one map entry.
var iconGlyphs = map[string]string{
	"mdi:track-light":       trackLightGlyph,
	"mdi:led-strip-variant": ledStripGlyph,
}

// LightIcon returns the fixture-type glyph for a light's HA icon
// attribute, falling back to a plain bulb for anything unrecognized
// (including empty) — including a clearly wrong/stale icon value like
// "mdi:alarm-off" assigned to an actual light: the lookup only trusts
// icons it explicitly recognizes.
func LightIcon(icon string) string {
	if glyph, ok := iconGlyphs[icon]; ok {
		return glyph
	}
	return bulbGlyph
}

const doorGlyph = `<svg class="ha-door" viewBox="0 0 24 24" fill="none" stroke-width="1.6"><path class="ha-door-frame" d="M6 3v18M6 3h9" stroke-linecap="round"/><path class="ha-door-leaf" d="M6 3v18l9-3V6z" stroke-linejoin="round"/></svg>`

// ContactIcon returns the door glyph; open/closed styling (including the
// leaf's rotation) is driven by the data-open attribute on the wrapper
// badge element, the same live-update-friendly pattern as LightIcon.
func ContactIcon() string {
	return doorGlyph
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/render/... -v`
Expected: PASS, all tests in the package (this file doesn't touch anything else, so the rest of the suite is unaffected).

- [ ] **Step 5: Commit**

```bash
git add internal/render/icons.go internal/render/icons_test.go
git commit -m "Add curated light and contact-sensor icon glyphs"
```

---

### Task 4: `ClassName` option on Sparkline and BarChart

**Files:**
- Modify: `internal/render/sparkline.go`
- Modify: `internal/render/barchart.go`
- Test: `internal/render/sparkline_test.go`, `internal/render/barchart_test.go`

**Interfaces:**
- Produces: `SparklineOptions.ClassName string` and `BarChartOptions.ClassName string` (both optional, default `""`) — when set, the returned `<svg>`'s root `class` attribute carries it, so the widget's CSS can `flex-grow` the chart to fill whatever space its room card allocates. Consumed by Task 7's `main.go` (passes `"ha-room-chart"`).

- [ ] **Step 1: Write the failing tests**

Add to `internal/render/sparkline_test.go`:

```go
func TestSparkline_AppliesClassName(t *testing.T) {
	svg := Sparkline([]float64{1, 2, 3}, nil, SparklineOptions{Width: 100, Height: 20, ClassName: "ha-room-chart"})
	if !contains(svg, `class="ha-room-chart"`) {
		t.Errorf("svg = %q, want class=\"ha-room-chart\"", svg)
	}
}

func TestSparkline_EmptyValuesStillAppliesClassName(t *testing.T) {
	svg := Sparkline(nil, nil, SparklineOptions{ClassName: "ha-room-chart"})
	if !contains(svg, `class="ha-room-chart"`) {
		t.Errorf("svg = %q, want class=\"ha-room-chart\" even for empty values", svg)
	}
}
```

Add to `internal/render/barchart_test.go`:

```go
func TestBarChart_AppliesClassName(t *testing.T) {
	svg := BarChart([]float64{10, 20}, nil, "", BarChartOptions{Width: 220, Height: 60, ClassName: "ha-room-chart"})
	if !contains(svg, `class="ha-room-chart"`) {
		t.Errorf("svg = %q, want class=\"ha-room-chart\"", svg)
	}
}

func TestBarChart_EmptyValuesStillAppliesClassName(t *testing.T) {
	svg := BarChart(nil, nil, "", BarChartOptions{ClassName: "ha-room-chart"})
	if !contains(svg, `class="ha-room-chart"`) {
		t.Errorf("svg = %q, want class=\"ha-room-chart\" even for empty values", svg)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/render/... -run 'ClassName' -v`
Expected: FAIL — `ClassName` field undefined on both options structs.

- [ ] **Step 3: Implement**

In `internal/render/sparkline.go`, add the field and thread it through both return paths:

```go
type SparklineOptions struct {
	Width     float64
	Height    float64
	ClassName string
}
```

```go
	if len(values) == 0 {
		return fmt.Sprintf(`<svg class="%s" viewBox="0 0 %g %g" height="%g" style="width:100%%;display:block" preserveAspectRatio="none"></svg>`, opts.ClassName, opts.Width, opts.Height, opts.Height)
	}
```

and, at the final return:

```go
	return fmt.Sprintf(
		`<svg class="%s" viewBox="0 0 %g %g" height="%g" style="width:100%%;display:block" preserveAspectRatio="none">`+
			`<polygon points="%s" fill="var(--color-progress-value)" fill-opacity="0.16"/>`+
			`<polyline points="%s" fill="none" stroke="var(--color-progress-value)" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"/>`+
			`%s`+
			`</svg>`,
		opts.ClassName, opts.Width, opts.Height, opts.Height, area, line, axis.String(),
	)
```

In `internal/render/barchart.go`, add the field and thread it through both return paths:

```go
type BarChartOptions struct {
	Width     float64
	Height    float64
	ClassName string
}
```

```go
	if len(values) == 0 {
		return fmt.Sprintf(`<svg class="%s" viewBox="0 0 %g %g" height="%g" style="width:100%%;display:block" preserveAspectRatio="none"></svg>`, opts.ClassName, opts.Width, opts.Height, opts.Height)
	}
```

and, at the final return:

```go
	return fmt.Sprintf(`<svg class="%s" viewBox="0 0 %g %g" height="%g" style="width:100%%;display:block" preserveAspectRatio="none">%s%s%s</svg>`,
		opts.ClassName, opts.Width, opts.Height, opts.Height, bars.String(), label, axis.String())
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/render/... -v`
Expected: PASS, all tests in the package.

- [ ] **Step 5: Commit**

```bash
git add internal/render/sparkline.go internal/render/sparkline_test.go internal/render/barchart.go internal/render/barchart_test.go
git commit -m "Add an optional ClassName to Sparkline/BarChart for flex-grow layout"
```

---

### Task 5: Rewrite `RenderWidget` around room cards

**Files:**
- Modify: `internal/render/template.go` (full rewrite)
- Test: `internal/render/template_test.go` (full rewrite)

**Interfaces:**
- Consumes: `LightIcon`/`ContactIcon` (Task 3).
- Produces:
  ```go
  type LightView struct {
      EntityID string
      IconSVG  string
      On       bool
  }

  type SensorBadgeView struct {
      Name      string
      Attention bool
      Label     string
  }

  type RoomCardView struct {
      Room           string
      SizeClass      string // "", "ha-size-md", "ha-size-lg"
      Lit            bool
      Occupied       bool
      HasTemperature bool
      TempNoData     bool
      TempValue      string
      ChartSVG       string
      Lights         []LightView
      Occupancy      []SensorBadgeView
      Contacts       []SensorBadgeView
  }

  type WidgetData struct {
      Rooms           []RoomCardView
      CardMinHeight   int
      LiveURL         string
      PollIntervalMS  int
      PauseWhenHidden bool
  }

  func RenderWidget(data WidgetData) string
  func RenderUnavailable() string
  ```
  Consumed by Task 6 (`RenderLive` takes `[]RoomCardView`) and Task 7 (`main.go` builds `WidgetData`/`RoomCardView`).

- [ ] **Step 1: Write the failing tests**

Replace the entire contents of `internal/render/template_test.go`:

```go
package render

import "testing"

func sampleRoomCard() RoomCardView {
	return RoomCardView{
		Room:           "Living Room",
		SizeClass:      "ha-size-md",
		Lit:            true,
		Occupied:       true,
		HasTemperature: true,
		TempValue:      "21.4°",
		ChartSVG:       "<svg>lr</svg>",
		Lights: []LightView{
			{EntityID: "light.lr_main", IconSVG: LightIcon("mdi:track-light"), On: true},
		},
		Occupancy: []SensorBadgeView{{Name: "LR Motion", Attention: true, Label: "Occupied"}},
		Contacts:  []SensorBadgeView{{Name: "LR Window", Attention: true, Label: "Open"}},
	}
}

func sampleWidgetData() WidgetData {
	return WidgetData{
		Rooms:           []RoomCardView{sampleRoomCard()},
		CardMinHeight:   130,
		LiveURL:         "/ha-widget/live.json",
		PollIntervalMS:  10000,
		PauseWhenHidden: true,
	}
}

func TestRenderWidget_RoomCardIncludesTemperature(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if !contains(html, "Living Room") || !contains(html, "21.4") {
		t.Errorf("html missing temperature content")
	}
	if !contains(html, "<svg>lr</svg>") {
		t.Errorf("html missing rendered chart SVG")
	}
}

func TestRenderWidget_TemperatureNoDataShowsFallback(t *testing.T) {
	data := WidgetData{Rooms: []RoomCardView{{Room: "Kitchen", HasTemperature: true, TempNoData: true}}, CardMinHeight: 130}
	html := RenderWidget(data)
	if !contains(html, "Kitchen") {
		t.Errorf("html missing Kitchen")
	}
	if !contains(html, "no data") {
		t.Errorf("html missing no-data fallback for a room with a sensor but no history")
	}
}

func TestRenderWidget_RoomCardIncludesLights(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if !contains(html, `data-entity-id="light.lr_main"`) {
		t.Errorf("html missing light entity id for live updates")
	}
	if !contains(html, `data-on="true"`) {
		t.Errorf("html missing on-state data attribute")
	}
	if !contains(html, `class="track-light"`) {
		t.Errorf("html missing the light's fixture-type glyph")
	}
}

func TestRenderWidget_RoomCardIncludesOccupancyAndContact(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if !contains(html, `data-sensor-name="LR Motion"`) || !contains(html, "Occupied") {
		t.Errorf("html missing occupancy chip")
	}
	if !contains(html, `data-sensor-name="LR Window"`) || !contains(html, "Open") {
		t.Errorf("html missing contact badge")
	}
}

func TestRenderWidget_RoomCardCarriesLitAndOccupiedState(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if !contains(html, `data-room="Living Room"`) {
		t.Errorf("html missing data-room attribute for live updates")
	}
	if !contains(html, `data-lit="true"`) {
		t.Errorf("html missing data-lit=\"true\"")
	}
	if !contains(html, `data-occupied="true"`) {
		t.Errorf("html missing data-occupied=\"true\"")
	}
}

func TestRenderWidget_SizeClassApplied(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if !contains(html, "ha-size-md") {
		t.Errorf("html missing size class")
	}
}

func TestRenderWidget_TemperatureOnlyRoomOmitsLightsAndStatus(t *testing.T) {
	data := WidgetData{
		Rooms:         []RoomCardView{{Room: "Kitchen", HasTemperature: true, TempValue: "25.0°", ChartSVG: "<svg>k</svg>"}},
		CardMinHeight: 130,
	}
	html := RenderWidget(data)
	if !contains(html, "Kitchen") || !contains(html, "25.0") {
		t.Errorf("html missing Kitchen's temperature")
	}
	if contains(html, "ha-room-lights") {
		t.Errorf("html has a lights row for a room with no lights")
	}
	if contains(html, "ha-room-status") {
		t.Errorf("html has a status row for a room with no occupancy/contact")
	}
}

func TestRenderWidget_NoRoomsShowsEmptyMessage(t *testing.T) {
	html := RenderWidget(WidgetData{CardMinHeight: 130})
	if !contains(html, "no rooms") {
		t.Errorf("html missing empty-state message")
	}
}

func TestRenderWidget_EscapesRoomAndSensorNames(t *testing.T) {
	data := WidgetData{
		Rooms: []RoomCardView{{
			Room:      `<script>alert(1)</script>`,
			Occupancy: []SensorBadgeView{{Name: `<b>x</b>`, Attention: false, Label: "Clear"}},
		}},
		CardMinHeight: 130,
	}
	html := RenderWidget(data)
	if contains(html, "<script>alert(1)</script>") || contains(html, "<b>x</b>") {
		t.Errorf("html contains unescaped content, want it HTML-escaped")
	}
}

func TestRenderWidget_AppliesConfiguredCardMinHeight(t *testing.T) {
	data := sampleWidgetData()
	data.CardMinHeight = 200
	html := RenderWidget(data)
	if !contains(html, "min-height:200px") {
		t.Errorf("html missing configured base card min-height in CSS")
	}
	if !contains(html, "min-height:220px") {
		t.Errorf("html missing size-md min-height (base+20)")
	}
	if !contains(html, "min-height:330px") {
		t.Errorf("html missing size-lg min-height (base+130)")
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

func TestRenderUnavailable_ContainsMessage(t *testing.T) {
	html := RenderUnavailable()
	if !contains(html, "Home Assistant unavailable") {
		t.Errorf("html = %q, want unavailable message", html)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/render/... -v`
Expected: FAIL to compile — `RoomCardView`, `LightView`, `SensorBadgeView`, `WidgetData.CardMinHeight` etc. don't match the current `template.go`.

- [ ] **Step 3: Implement**

Replace the entire contents of `internal/render/template.go`:

```go
package render

import (
	"fmt"
	"html"
	"strings"
)

type LightView struct {
	EntityID string
	IconSVG  string
	On       bool
}

type SensorBadgeView struct {
	Name      string
	Attention bool
	Label     string
}

type RoomCardView struct {
	Room           string
	SizeClass      string // "", "ha-size-md", "ha-size-lg"
	Lit            bool
	Occupied       bool
	HasTemperature bool
	TempNoData     bool
	TempValue      string
	ChartSVG       string
	Lights         []LightView
	Occupancy      []SensorBadgeView
	Contacts       []SensorBadgeView
}

type WidgetData struct {
	Rooms           []RoomCardView
	CardMinHeight   int
	LiveURL         string
	PollIntervalMS  int
	PauseWhenHidden bool
}

// styleBlock renders the widget's CSS. cardMinHeight is the base (small
// tier) room card's min-height in px, taken from temperature.chart_height
// — the "medium"/"large" tiers scale from it (+20, +130), matching the
// weight thresholds computed in main.go's sizeClassForWeight.
func styleBlock(cardMinHeight int) string {
	return fmt.Sprintf(`<style>
.ha-body{display:flex;flex-direction:column;gap:16px}
.ha-section-head{display:flex;align-items:center;gap:8px}
.ha-section-label{font-size:.85em;letter-spacing:.08em;text-transform:uppercase;color:var(--color-text-subdue)}
.ha-live-badge{display:inline-flex;align-items:center;gap:5px;font-size:.7em;letter-spacing:.06em;text-transform:uppercase;color:var(--color-primary)}
.ha-live-dot{width:6px;height:6px;border-radius:50%%;background:var(--color-primary)}
.ha-unavailable{color:var(--color-text-subdue);padding:12px 0}
.ha-empty{color:var(--color-text-subdue);font-size:.85em;padding:8px 0}

.ha-rooms{display:flex;flex-wrap:wrap;gap:10px;align-items:stretch}
.ha-room{
  flex:1 1 160px;min-height:%dpx;
  background:var(--color-widget-background-highlight);
  border:1px solid var(--color-widget-content-border);
  border-radius:8px;padding:12px 14px 11px;
  display:flex;flex-direction:column;gap:9px;
  transition:background .2s,border-color .2s,box-shadow .2s;
}
.ha-room.ha-size-md{flex:2 1 320px;min-height:%dpx}
.ha-room.ha-size-lg{flex:3 1 340px;min-height:%dpx}
.ha-room[data-lit="true"]{background:rgba(240,196,121,.14);border-color:rgba(240,196,121,.35)}
@keyframes ha-occ-glow{
  0%%,100%%{box-shadow:0 0 0 1.5px var(--color-primary),0 0 10px -2px color-mix(in srgb,var(--color-primary) 45%%,transparent)}
  50%%{box-shadow:0 0 0 1.5px var(--color-primary),0 0 20px 0 color-mix(in srgb,var(--color-primary) 80%%,transparent)}
}
.ha-room[data-occupied="true"]{animation:ha-occ-glow 2.6s ease-in-out infinite}

.ha-room-head{flex:none;display:flex;align-items:baseline;justify-content:space-between;gap:8px}
.ha-room-name{font-size:13.5px;font-weight:600;color:var(--color-text-highlight)}
.ha-room-temp{font-size:13px;color:var(--color-text-highlight);font-variant-numeric:tabular-nums;white-space:nowrap}
.ha-temp-nodata{color:var(--color-text-subdue);font-size:.85em;padding:2px 0}
.ha-room-chart{flex:2 1 auto;width:100%%;display:block;min-height:30px}
.ha-room-lights{flex:1 1 auto;display:flex;flex-wrap:wrap;align-content:center;align-items:center;gap:10px}
.ha-room-lights svg{width:26px;height:26px;flex:none}
.ha-room-status{flex:none;display:flex;flex-direction:column;gap:5px}

.ha-occ-chip{
  display:inline-flex;align-items:center;gap:6px;width:fit-content;
  font-size:11px;letter-spacing:.03em;padding:3px 9px 3px 7px;border-radius:20px;
  border:1px solid var(--color-text-subdue);color:var(--color-text-subdue);
}
.ha-occ-chip .ha-occ-dot{width:7px;height:7px;border-radius:50%%;background:var(--color-text-subdue)}
.ha-occ-chip[data-occupied="true"]{
  border-color:var(--color-primary);color:var(--color-primary);
  background:color-mix(in srgb,var(--color-primary) 16%%,transparent);
}
.ha-occ-chip[data-occupied="true"] .ha-occ-dot{background:var(--color-primary)}

.ha-badge{display:flex;align-items:center;gap:6px;font-size:11px;letter-spacing:.02em;color:var(--color-text-subdue)}
.ha-badge svg{width:14px;height:14px;flex:none}
.ha-badge[data-open="true"]{color:var(--color-negative)}

.ha-light[data-on="true"] .bulb-glass{stroke:#f0c479;fill:rgba(240,196,121,.16)}
.ha-light[data-on="false"] .bulb-glass{stroke:var(--color-text-subdue);fill:none}
.ha-light[data-on="true"] .bulb-base{stroke:#f0c479}
.ha-light[data-on="false"] .bulb-base{stroke:var(--color-text-subdue)}
.ha-light[data-on="true"] .tl-head{stroke:#f0c479;fill:rgba(240,196,121,.16)}
.ha-light[data-on="true"] .tl-rail{stroke:#f0c479}
.ha-light[data-on="true"] .tl-ray{stroke:#f0c479;opacity:1}
.ha-light[data-on="false"] .tl-head{stroke:var(--color-text-subdue);fill:none}
.ha-light[data-on="false"] .tl-rail{stroke:var(--color-text-subdue)}
.ha-light[data-on="false"] .tl-ray{opacity:0}
.ha-light[data-on="true"] .ls-body{stroke:#f0c479;fill:rgba(240,196,121,.16)}
.ha-light[data-on="true"] .ls-led{fill:#f0c479}
.ha-light[data-on="false"] .ls-body{stroke:var(--color-text-subdue);fill:none}
.ha-light[data-on="false"] .ls-led{fill:var(--color-text-subdue)}

.ha-badge[data-open="true"] .ha-door-leaf{stroke:var(--color-negative);transform:rotate(-38deg);transform-origin:2px 12.5px}
.ha-badge[data-open="true"] .ha-door-frame{stroke:var(--color-negative)}
.ha-badge[data-open="false"] .ha-door-leaf{stroke:var(--color-text-subdue);transform:rotate(0deg)}
.ha-badge[data-open="false"] .ha-door-frame{stroke:var(--color-text-subdue)}
.ha-door-leaf{transition:transform .2s}
</style>`, cardMinHeight, cardMinHeight+20, cardMinHeight+130)
}

// bootstrapScript runs via an onerror attribute (see RenderWidget) because
// Glance mounts extension widget HTML with element.innerHTML, and <script>
// elements inserted that way are inert per the HTML spec — onerror/onload
// content attributes are not, so they're the standard way to run JS in
// HTML delivered through an innerHTML sink. Everything it touches (a
// light's on state, a room's lit/occupied state, a contact's open state)
// is a data-* attribute, matching the initial render exactly — it never
// needs to know a light's fixture type or reconstruct any markup.
const bootstrapScript = `(function(img){var root=img.closest('.ha-widget');if(!root)return;var url=root.dataset.liveUrl;var interval=parseInt(root.dataset.pollMs,10)||10000;var pauseWhenHidden=root.dataset.pauseHidden==='true';var timer=null;function applyState(data){(data.rooms||[]).forEach(function(room){var card=root.querySelector('.ha-room[data-room="'+CSS.escape(room.room)+'"]');if(!card)return;var anyLit=false;(room.lights||[]).forEach(function(l){var el=card.querySelector('.ha-light[data-entity-id="'+CSS.escape(l.entity_id)+'"]');if(!el)return;el.dataset.on=l.on;if(l.on)anyLit=true;});var anyOccupied=false;(room.occupancy||[]).forEach(function(o){var chip=card.querySelector('.ha-occ-chip[data-sensor-name="'+CSS.escape(o.name)+'"]');if(!chip)return;chip.dataset.occupied=o.attention;var label=chip.querySelector('.ha-occ-label');if(label)label.textContent=o.label;if(o.attention)anyOccupied=true;});(room.contacts||[]).forEach(function(c){var badge=card.querySelector('.ha-badge[data-sensor-name="'+CSS.escape(c.name)+'"]');if(!badge)return;badge.dataset.open=c.attention;var label=badge.querySelector('.ha-contact-label');if(label)label.textContent=c.label;});card.dataset.lit=anyLit;card.dataset.occupied=anyOccupied;});}function poll(){fetch(url,{cache:'no-store'}).then(function(r){return r.ok?r.json():null;}).then(function(data){if(data)applyState(data);}).catch(function(){});}function stop(){if(timer){clearInterval(timer);timer=null;}}function schedule(){stop();timer=setInterval(poll,interval);}if(pauseWhenHidden){document.addEventListener('visibilitychange',function(){if(document.hidden){stop();}else{poll();schedule();}});}if(!pauseWhenHidden||!document.hidden){poll();schedule();}})(this)`

func RenderWidget(data WidgetData) string {
	var b strings.Builder
	b.WriteString(styleBlock(data.CardMinHeight))

	pauseAttr := "false"
	if data.PauseWhenHidden {
		pauseAttr = "true"
	}
	fmt.Fprintf(&b, `<div class="ha-widget ha-body" data-live-url="%s" data-poll-ms="%d" data-pause-hidden="%s">`,
		html.EscapeString(data.LiveURL), data.PollIntervalMS, pauseAttr)

	b.WriteString(`<div class="ha-section-head"><span class="ha-section-label">Home</span><span class="ha-live-badge"><span class="ha-live-dot"></span>live</span></div>`)

	if len(data.Rooms) == 0 {
		b.WriteString(`<div class="ha-empty">no rooms with a temperature sensor, light, or sensor found</div>`)
	} else {
		b.WriteString(`<div class="ha-rooms">`)
		for _, r := range data.Rooms {
			b.WriteString(renderRoomCard(r))
		}
		b.WriteString(`</div>`)
	}

	fmt.Fprintf(&b, `<img src="x" alt="" style="display:none;width:0;height:0" onerror="%s">`, html.EscapeString(bootstrapScript))
	b.WriteString(`</div>`)

	return b.String()
}

func renderRoomCard(r RoomCardView) string {
	var b strings.Builder

	classes := "ha-room"
	if r.SizeClass != "" {
		classes += " " + r.SizeClass
	}
	fmt.Fprintf(&b, `<div class="%s" data-room="%s" data-lit="%t" data-occupied="%t">`,
		classes, html.EscapeString(r.Room), r.Lit, r.Occupied)

	if r.HasTemperature {
		fmt.Fprintf(&b, `<div class="ha-room-head"><span class="ha-room-name">%s</span>`, html.EscapeString(r.Room))
		if r.TempNoData {
			b.WriteString(`</div><div class="ha-temp-nodata">no data</div>`)
		} else {
			fmt.Fprintf(&b, `<span class="ha-room-temp">%s</span></div>%s`, html.EscapeString(r.TempValue), r.ChartSVG)
		}
	} else {
		fmt.Fprintf(&b, `<div class="ha-room-head"><span class="ha-room-name">%s</span></div>`, html.EscapeString(r.Room))
	}

	if len(r.Lights) > 0 {
		b.WriteString(`<div class="ha-room-lights">`)
		for _, l := range r.Lights {
			fmt.Fprintf(&b, `<span class="ha-light" data-entity-id="%s" data-on="%t">%s</span>`,
				html.EscapeString(l.EntityID), l.On, l.IconSVG)
		}
		b.WriteString(`</div>`)
	}

	if len(r.Occupancy) > 0 || len(r.Contacts) > 0 {
		b.WriteString(`<div class="ha-room-status">`)
		for _, o := range r.Occupancy {
			fmt.Fprintf(&b, `<span class="ha-occ-chip" data-sensor-name="%s" data-occupied="%t"><span class="ha-occ-dot"></span><span class="ha-occ-label">%s</span></span>`,
				html.EscapeString(o.Name), o.Attention, html.EscapeString(o.Label))
		}
		for _, c := range r.Contacts {
			fmt.Fprintf(&b, `<span class="ha-badge" data-sensor-name="%s" data-open="%t">%s<span class="ha-contact-label">%s</span></span>`,
				html.EscapeString(c.Name), c.Attention, ContactIcon(), html.EscapeString(c.Label))
		}
		b.WriteString(`</div>`)
	}

	b.WriteString(`</div>`)
	return b.String()
}

func RenderUnavailable() string {
	return styleBlock(130) + `<div class="ha-unavailable">Home Assistant unavailable</div>`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/render/... -v`
Expected: PASS, all tests in the package.

- [ ] **Step 5: Commit**

```bash
git add internal/render/template.go internal/render/template_test.go
git commit -m "Rewrite RenderWidget around adaptive per-room cards"
```

---

### Task 6: Rewrite `RenderLive` for the per-room payload

**Files:**
- Modify: `internal/render/live.go` (full rewrite)
- Test: `internal/render/live_test.go` (full rewrite)

**Interfaces:**
- Consumes: `RoomCardView` (Task 5).
- Produces: `func RenderLive(rooms []RoomCardView) ([]byte, error)` — JSON shape: `{"rooms":[{"room":"...","lights":[{"entity_id":"...","on":bool}],"occupancy":[{"name":"...","attention":bool,"label":"..."}],"contacts":[...]}]}`. A room with no lights, occupancy, or contacts is omitted entirely — nothing on its card would ever need live-updating. Consumed by Task 7's `liveHandler`.

- [ ] **Step 1: Write the failing tests**

Replace the entire contents of `internal/render/live_test.go`:

```go
package render

import (
	"encoding/json"
	"testing"
)

func TestRenderLive_MarshalsRoomsWithLightsAndSensors(t *testing.T) {
	rooms := []RoomCardView{
		{
			Room:      "Bedroom",
			Lights:    []LightView{{EntityID: "light.bed_1", On: false}, {EntityID: "light.bed_2", On: true}},
			Occupancy: []SensorBadgeView{{Name: "Bed Motion", Attention: true, Label: "Occupied"}},
		},
	}
	body, err := RenderLive(rooms)
	if err != nil {
		t.Fatalf("RenderLive: %v", err)
	}

	var parsed struct {
		Rooms []struct {
			Room   string `json:"room"`
			Lights []struct {
				EntityID string `json:"entity_id"`
				On       bool   `json:"on"`
			} `json:"lights"`
			Occupancy []struct {
				Name      string `json:"name"`
				Attention bool   `json:"attention"`
				Label     string `json:"label"`
			} `json:"occupancy"`
		} `json:"rooms"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(parsed.Rooms) != 1 || parsed.Rooms[0].Room != "Bedroom" {
		t.Fatalf("Rooms = %+v", parsed.Rooms)
	}
	if len(parsed.Rooms[0].Lights) != 2 || parsed.Rooms[0].Lights[1].EntityID != "light.bed_2" || !parsed.Rooms[0].Lights[1].On {
		t.Errorf("Lights = %+v", parsed.Rooms[0].Lights)
	}
	if len(parsed.Rooms[0].Occupancy) != 1 || !parsed.Rooms[0].Occupancy[0].Attention || parsed.Rooms[0].Occupancy[0].Label != "Occupied" {
		t.Errorf("Occupancy = %+v", parsed.Rooms[0].Occupancy)
	}
}

func TestRenderLive_OmitsTemperatureOnlyRoom(t *testing.T) {
	rooms := []RoomCardView{
		{Room: "Kitchen", HasTemperature: true, TempValue: "25.0°", ChartSVG: "<svg></svg>"},
	}
	body, err := RenderLive(rooms)
	if err != nil {
		t.Fatalf("RenderLive: %v", err)
	}
	if contains(string(body), "Kitchen") {
		t.Errorf("body = %s, want temperature-only room omitted from live payload", body)
	}
}

func TestRenderLive_EmptyInputProducesEmptyRoomsArrayNotNull(t *testing.T) {
	body, err := RenderLive(nil)
	if err != nil {
		t.Fatalf("RenderLive: %v", err)
	}
	if !contains(string(body), `"rooms":[]`) {
		t.Errorf("body = %s, want \"rooms\":[] not null", body)
	}
}

func TestRenderLive_RoomWithOnlyLightsOmitsNullOccupancyAndContacts(t *testing.T) {
	rooms := []RoomCardView{
		{Room: "Hallway", Lights: []LightView{{EntityID: "light.hall", On: true}}},
	}
	body, err := RenderLive(rooms)
	if err != nil {
		t.Fatalf("RenderLive: %v", err)
	}
	if !contains(string(body), `"occupancy":[]`) || !contains(string(body), `"contacts":[]`) {
		t.Errorf("body = %s, want empty (not null) occupancy/contacts arrays", body)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/render/... -run TestRenderLive -v`
Expected: FAIL to compile — `RenderLive` signature doesn't match (`LightRoomView`/`SensorView` args no longer exist).

- [ ] **Step 3: Implement**

Replace the entire contents of `internal/render/live.go`:

```go
package render

import "encoding/json"

type LiveLight struct {
	EntityID string `json:"entity_id"`
	On       bool   `json:"on"`
}

type LiveSensor struct {
	Name      string `json:"name"`
	Attention bool   `json:"attention"`
	Label     string `json:"label"`
}

type LiveRoom struct {
	Room      string       `json:"room"`
	Lights    []LiveLight  `json:"lights"`
	Occupancy []LiveSensor `json:"occupancy"`
	Contacts  []LiveSensor `json:"contacts"`
}

type LivePayload struct {
	Rooms []LiveRoom `json:"rooms"`
}

// RenderLive builds the /live.json payload from the same RoomCardView data
// used to render the widget, so live updates always match one source of
// truth. A room with no lights, occupancy, or contacts is omitted from the
// payload entirely — its card never changes between polls, so there's
// nothing to send for it.
func RenderLive(rooms []RoomCardView) ([]byte, error) {
	payload := LivePayload{Rooms: []LiveRoom{}}
	for _, r := range rooms {
		if len(r.Lights) == 0 && len(r.Occupancy) == 0 && len(r.Contacts) == 0 {
			continue
		}
		lr := LiveRoom{
			Room:      r.Room,
			Lights:    make([]LiveLight, len(r.Lights)),
			Occupancy: make([]LiveSensor, len(r.Occupancy)),
			Contacts:  make([]LiveSensor, len(r.Contacts)),
		}
		for i, l := range r.Lights {
			lr.Lights[i] = LiveLight{EntityID: l.EntityID, On: l.On}
		}
		for i, o := range r.Occupancy {
			lr.Occupancy[i] = LiveSensor{Name: o.Name, Attention: o.Attention, Label: o.Label}
		}
		for i, c := range r.Contacts {
			lr.Contacts[i] = LiveSensor{Name: c.Name, Attention: c.Attention, Label: c.Label}
		}
		payload.Rooms = append(payload.Rooms, lr)
	}
	return json.Marshal(payload)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/render/... -v`
Expected: PASS, all tests in the package.

- [ ] **Step 5: Commit**

```bash
git add internal/render/live.go internal/render/live_test.go
git commit -m "Rewrite RenderLive for the per-room live-update payload"
```

---

### Task 7: Rewire `main.go` around `hass.RoomCard` → `render.RoomCardView`

**Files:**
- Modify: `main.go` (full rewrite)
- Test: `main_test.go` (full rewrite)

**Interfaces:**
- Consumes: `hass.RoomCard`/`hass.Light`/`hass.SensorEntity` (Task 2), `render.LightIcon`/`ContactIcon` (Task 3), `render.Sparkline`/`BarChart` with `ClassName` (Task 4), `render.RoomCardView`/`WidgetData`/`RenderWidget`/`RenderUnavailable` (Task 5), `render.RenderLive` (Task 6).
- Produces: `roomCardView(card hass.RoomCard) render.RoomCardView`, `sizeClassForWeight(weight int) string` — both package-private to `main`, mirroring how `sparseAxisLabels`/`liveURL` already work today.

- [ ] **Step 1: Write the failing tests**

Replace the entire contents of `main_test.go`:

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
	if !strings.Contains(body, "Front Door") {
		t.Errorf("body missing Front Door contact badge")
	}
	if !strings.Contains(body, `data-room="Hallway"`) || !strings.Contains(body, `data-occupied="true"`) {
		t.Errorf("body missing Hallway's occupied state")
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -v`
Expected: FAIL to compile — `roomCardView`/`sizeClassForWeight` undefined, `hass.RoomCard`/`hass.Light` fields don't match, `main.go` still calls the old `hass.Model`/`render.LightRoomView` APIs.

- [ ] **Step 3: Implement**

Replace the entire contents of `main.go`:

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

// sizeClassForWeight maps a RoomCard's Weight onto the CSS class driving
// its card's flex-grow/min-height tier (see internal/render/template.go's
// styleBlock). Thresholds are fixed, not configurable — see the design
// spec's "Card sizing" section for the rationale.
func sizeClassForWeight(weight int) string {
	switch {
	case weight >= 5:
		return "ha-size-lg"
	case weight >= 3:
		return "ha-size-md"
	default:
		return ""
	}
}

// roomCardView maps a classified hass.RoomCard onto the render package's
// view type, computing the derived Lit/Occupied flags used for the card's
// background tint and glow. Temperature (HasTemperature/TempValue/ChartSVG)
// is populated separately by widgetHandler, since only it fetches history
// — liveHandler never needs a chart.
func roomCardView(card hass.RoomCard) render.RoomCardView {
	view := render.RoomCardView{
		Room:      card.Room,
		SizeClass: sizeClassForWeight(card.Weight),
	}
	for _, l := range card.Lights {
		view.Lights = append(view.Lights, render.LightView{
			EntityID: l.EntityID,
			IconSVG:  render.LightIcon(l.Icon),
			On:       l.On,
		})
		if l.On {
			view.Lit = true
		}
	}
	for _, o := range card.Occupancy {
		view.Occupancy = append(view.Occupancy, render.SensorBadgeView{Name: o.Name, Attention: o.Attention, Label: o.Label})
		if o.Attention {
			view.Occupied = true
		}
	}
	for _, c := range card.Contacts {
		view.Contacts = append(view.Contacts, render.SensorBadgeView{Name: c.Name, Attention: c.Attention, Label: c.Label})
	}
	return view
}

func (a *app) buildModel(ctx context.Context) ([]hass.RoomCard, error) {
	rooms, err := a.cache.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch areas: %w", err)
	}
	states, err := a.client.FetchStates(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch states: %w", err)
	}
	return hass.BuildModel(rooms, states, hass.ClassificationConfig{
		ContactDeviceClasses: a.cfg.Sensors.ContactDeviceClasses,
		MotionDeviceClasses:  a.cfg.Sensors.MotionDeviceClasses,
	}), nil
}

// Nominal internal SVG coordinate-space heights for the temperature chart.
// These are unrelated to temperature.chart_height (which now sizes the
// room *card*, not the chart) — preserveAspectRatio="none" stretches the
// chart to fill whatever height its flex-grown .ha-room-chart box ends up
// with, so this only needs to give the chart's internal margin/plot-area
// proportions a sensible shape, not match any real pixel measurement.
const sparklineNominalHeight = 60
const barChartNominalHeight = 90

func (a *app) widgetHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	w.Header().Set("Widget-Title", a.cfg.Title)
	w.Header().Set("Widget-Content-Type", "html")

	cards, err := a.buildModel(ctx)
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
	for _, card := range cards {
		if card.Temperature != nil {
			allTempIDs = append(allTempIDs, card.Temperature.EntityIDs...)
		}
	}

	history, err := a.client.FetchHistory(ctx, allTempIDs, now.Add(-rangeDur), now)
	if err != nil {
		log.Printf("fetch history: %v", err)
		history = map[string][]hass.HistoryPoint{}
	}

	views := make([]render.RoomCardView, len(cards))
	for i, card := range cards {
		view := roomCardView(card)

		if card.Temperature != nil {
			view.HasTemperature = true

			var series [][]float64
			for _, id := range card.Temperature.EntityIDs {
				points, ok := history[id]
				if !ok || len(points) == 0 {
					continue
				}
				series = append(series, hass.StepForwardFill(points, timestamps))
			}
			avg := hass.AverageSeries(series)
			if len(avg) == 0 || math.IsNaN(avg[len(avg)-1]) {
				view.TempNoData = true
			} else {
				view.TempValue = fmt.Sprintf("%.1f°", avg[len(avg)-1])
				if a.cfg.Temperature.ChartStyle == "bars" {
					barOpts := render.BarChartOptions{Width: 220, Height: barChartNominalHeight, ClassName: "ha-room-chart"}
					view.ChartSVG = render.BarChart(avg, axisLabels, view.TempValue, barOpts)
				} else {
					view.ChartSVG = render.Sparkline(avg, axisLabels, render.SparklineOptions{Width: 220, Height: sparklineNominalHeight, ClassName: "ha-room-chart"})
				}
			}
		}

		views[i] = view
	}

	widgetData := render.WidgetData{
		Rooms:           views,
		CardMinHeight:   a.cfg.Temperature.ChartHeight,
		LiveURL:         liveURL(a.cfg.PublicURL),
		PollIntervalMS:  int(pollInterval.Milliseconds()),
		PauseWhenHidden: a.cfg.Live.PauseWhenHidden != nil && *a.cfg.Live.PauseWhenHidden,
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, render.RenderWidget(widgetData))
}

func (a *app) liveHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	w.Header().Set("Access-Control-Allow-Origin", "*")

	cards, err := a.buildModel(ctx)
	if err != nil {
		log.Printf("home assistant unavailable: %v", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	views := make([]render.RoomCardView, len(cards))
	for i, card := range cards {
		views[i] = roomCardView(card)
	}

	body, err := render.RenderLive(views)
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

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -v`
Expected: PASS, the entire module (every package touched by Tasks 1-7).

- [ ] **Step 5: Commit**

```bash
git add main.go main_test.go
git commit -m "Wire main.go's handlers around the merged room-card model"
```

---

### Task 8: `chart_height` default change, config docs, README

**Files:**
- Modify: `config.go`
- Modify: `config_test.go`
- Modify: `config.example.yml`
- Modify: `README.md`

**Interfaces:**
- No new types or functions — `TemperatureConfig.ChartHeight` keeps its `int` type; only `LoadConfig`'s default value and its documented meaning change.

- [ ] **Step 1: Write the failing test**

In `config_test.go`, change the existing assertion in `TestLoadConfig_Defaults`:

```go
	if cfg.Temperature.ChartHeight != 130 {
		t.Errorf("Temperature.ChartHeight = %d, want 130", cfg.Temperature.ChartHeight)
	}
```

(replacing the existing `!= 34` / `want 34` assertion — everywhere else in `config_test.go` that sets `TEMPERATURE_CHART_HEIGHT` explicitly, e.g. `TestLoadConfig_EnvOverridesAllFields`'s `setEnv(t, "TEMPERATURE_CHART_HEIGHT", "50")`, is unaffected since it doesn't exercise the default.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run TestLoadConfig_Defaults -v`
Expected: FAIL — `Temperature.ChartHeight = 34, want 130`.

- [ ] **Step 3: Implement**

In `config.go`, change the default:

```go
	if cfg.Temperature.ChartHeight == 0 {
		cfg.Temperature.ChartHeight = 130
	}
```

In `config.example.yml`, update the field's example value and comment:

```yaml
  chart_height: 130   # env: TEMPERATURE_CHART_HEIGHT — base minimum card height in px; a room's card grows taller automatically the more it has to show (lights, occupancy, contact), scaling from this base
```

In `README.md`'s "Environment variable reference" table, update the `TEMPERATURE_CHART_HEIGHT` row:

```markdown
| `TEMPERATURE_CHART_HEIGHT` | `temperature.chart_height` | `130` | Base minimum room-card height in px — cards with more to show (lights, occupancy, contact) grow taller automatically |
```

Also update the README's opening description and "Error handling" section to describe the merged card layout instead of three sections. Replace:

```markdown
A [Glance](https://github.com/glanceapp/glance) extension widget that shows Home Assistant data
in Glance's own visual language: room temperature (sparkline or bar-chart style), which lights
are on per room, and contact/motion sensor state — with lights and sensors updating live in the
browser while the dashboard tab stays open.
```

with:

```markdown
A [Glance](https://github.com/glanceapp/glance) extension widget that shows Home Assistant data
as one adaptive grid of per-room cards — each room's temperature (sparkline or bar-chart style),
lights (with their real HA icons), and occupancy/contact sensors together at a glance — with
lights and sensors updating live in the browser while the dashboard tab stays open. A room's card
grows automatically the more it has to show; a room with nothing classified gets no card at all.
```

And replace the "Error handling" section's second sentence:

```markdown
If Home Assistant is unreachable, the whole widget shows a single "Home Assistant unavailable"
message instead of Glance's generic widget-failed state. If a specific room has no temperature
history, only that room's panel shows "no data" — the rest of the widget still renders normally.
`/live.json` failing at poll time leaves the last-known lights/sensors state on screen rather than
clearing it, and retries on the next interval.
```

with:

```markdown
If Home Assistant is unreachable, the whole widget shows a single "Home Assistant unavailable"
message instead of Glance's generic widget-failed state. If a specific room has a temperature
sensor but no history data right now, only that room's card shows "no data" — the rest of the
widget still renders normally. `/live.json` failing at poll time leaves the last-known lights/
sensors state on screen rather than clearing it, and retries on the next interval.
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -v`
Expected: PASS, the entire module.

- [ ] **Step 5: Commit**

```bash
git add config.go config_test.go config.example.yml README.md
git commit -m "Change chart_height's default and meaning to a base card min-height"
```

---

## Final verification (after all tasks)

```bash
gofmt -l .
go vet ./...
go test ./...
```

All three must be clean/green before moving to the final whole-branch review and merge.
