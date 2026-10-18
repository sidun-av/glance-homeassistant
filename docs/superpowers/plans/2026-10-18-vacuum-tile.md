# Robot vacuum tile — implementation plan

Spec: `docs/superpowers/specs/2026-10-18-vacuum-tile-design.md`.
Branch: `feature/vacuum-tile`. TDD per task; `go vet ./... && go test ./...` green
after each; one commit per task. Examples use generic ids (`vacuum.robot`,
`camera.robot_map`, `sensor.robot_battery`).

## Task 1 — discovery (`internal/hass`)

- `client.go` `EntityState`: add `BatteryLevel *int` (`battery_level`),
  `FanSpeed string` (`fan_speed`, may be null), `FanSpeedList []string`
  (`fan_speed_list`). Parsed in `FetchStates`.
- `discover.go`:
  - `DeviceEffect` for `vacuum` returns effect `"vacuum"` (on still =
    cleaning/returning) so the tile carries `data-effect="vacuum"`.
  - `Device` gains `Activity string`, `Battery *int`, `FanSpeed string`,
    `FanSpeeds []string`, `SupportsLocate bool` (feature bit 512),
    `MapCamera string`.
  - `deviceGroupOf(entityID, room)` factored out of `deviceExtras`;
    `deviceBattery` (attribute first, else a numeric same-device
    `sensor` with `device_class: battery`) and `deviceMapCamera` (first available
    same-device `camera`, preferring one whose id contains `map`).
- Tests: `client_test.go` parses the three attributes; `discover_test.go`
  vacuum fields from attributes, battery fallback to sibling sensor, attribute
  wins over sensor, unavailable sensor/camera ignored, no device → nil/empty.

## Task 2 — vacuum actions (`main.go` `entityHandler`)

- `entityDomainActions["vacuum"] = {start, pause, return_to_base, locate,
  set_fan_speed}` (no more `toggle`: `homeassistant.toggle` is a no-op on vacuums
  without TURN_ON/TURN_OFF).
- Request gains `FanSpeed string json:"fan_speed"`. `set_fan_speed` fetches
  states and requires the value to be in the entity's own `fan_speed_list`.
- Tests (`main_test.go`): start/pause/return_to_base/locate reach
  `/api/services/vacuum/<action>`; `set_fan_speed` with a listed value passes,
  an unlisted/empty one → 400 and no service call; `toggle` on a vacuum → 400.

## Task 3 — map camera proxy (`main.go`)

- `recordControls` also records each vacuum's `MapCamera` in `a.mapCameras`;
  `isMapCamera(ctx, id)` mirrors `isControl` (lazy first build).
- `GET /camera?entity_id=camera.x` (and under the public_url prefix): not a map
  camera of a vacuum on the map → 404; else `client.FetchPicture(ctx,
  "/api/camera_proxy/"+id)`, upstream content type, `Cache-Control: no-store`.
- `cameraURL(publicURL)` → `WidgetData.CameraURL` → root `data-camera-url`.
- Tests: allowed camera streams bytes with type + no-store and the bearer token
  upstream; unrelated camera / non-camera id → 404 with no upstream camera call.

## Task 4 — tile (`internal/render`)

- `DeviceView` gains the Task 1 fields; `main.go roomCardView` copies them.
- `deviceTileAttrs` for vacuums: `data-activity`, `data-battery` (+
  `data-battery-low` under 20%), `data-fan-speed`, `data-fan-speeds` (JSON),
  `data-locate`, `data-map-camera`.
- `LiveDevice` gains `activity`, `battery`, `fan_speed` (omitempty);
  `bootstrapScript` copies them onto the tile.
- `floorplanCSS`: vacuum resets the beam pseudo-elements; cleaning = orbiting
  dot, returning = pulsing ring, docked = battery badge, paused = pause badge,
  error = red icon + `!` badge.
- Tests: tile attributes, live payload, CSS rules present.

## Task 5 — popover + tap (`entitycontrol.go`)

- Tap on a vacuum: `pause` while cleaning/returning, `start` otherwise
  (optimistic `data-activity`, reverted on error). Long-press always opens the
  vacuum popover.
- Panel: status · battery line, Start/Pause (by state), Dock, Locate (when
  supported), suction chips from `fan_speed_list`, map image from
  `<camera-url>?entity_id=…&t=…` in a fixed-aspect box (no reflow → no clipping),
  refreshed every 10 s while open and cleaning/returning, plus the device extras.
- Tests: rendered script contains the vacuum actions and camera wiring (a smoke
  check; behaviour verified in the Playwright harness).

## Task 6 — README

Vacuum bullets in "Controlling things from the map" and the `/camera` endpoint.

## Visual verification (not committed)

Scratch harness: Go program renders a fake floorplan with one vacuum per activity
and dumps HTML into a page loading Glance's `main.css`; served with
`python3 -m http.server`; Playwright screenshots of the map and the open popover
in the dark theme.
