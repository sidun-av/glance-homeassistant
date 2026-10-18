# Robot vacuum tile on the floorplan — design

Date: 2026-10-18.

## Intent

The floorplan already draws a vacuum as a plain device icon, and a tap sends
`homeassistant.toggle` — which does nothing on vacuums that don't advertise
`TURN_ON`/`TURN_OFF` (most modern ones only have start/pause/return). The owner
wants the robot to read as a robot vacuum on the map: what it is doing, its
battery, one-tap start/pause, dock, suction, and the last map with the route it
drove.

## Goals

1. A vacuum tile that shows activity at a glance: cleaning (animated), returning,
   docked (with battery), paused, error (red). Battery comes from the entity's
   `battery_level` attribute or, failing that, a `sensor` with
   `device_class: battery` on the same HA device.
2. Tap = start when docked/idle/paused, pause when cleaning or returning. Long-press
   opens the popover.
3. Popover: status + battery line, buttons Start/Pause (one, by state), Dock,
   Locate (when supported), a suction selector from `fan_speed_list`, and the
   map image from a `camera` entity on the same HA device (the integration's map
   camera already draws the route of the current or last clean). The map image
   refreshes every 10 s while the popover is open and the robot is cleaning or
   returning, otherwise once on open.
4. The backend only acts within allowlists, like the other domains: vacuum
   actions `start`, `pause`, `return_to_base`, `locate`, `set_fan_speed`
   (`fan_speed` must be one of the entity's own `fan_speed_list`); a camera image
   is served only for a camera that belongs to the same device as a vacuum tile on
   the map.

## Non-goals

Zone or room selection on the map (the HA dashboard card does that), schedules,
consumables, history.

## Anti-goals

- The HA token never reaches the browser: images are proxied by the widget
  server (`/api/camera_proxy/<entity_id>`), never linked directly.
- No new external requests from the browser; no new JS dependencies.
- No regression for other domains' tap/long-press behaviour.

## Design notes

- Discovery (`internal/hass/discover.go`) already classifies `vacuum` devices
  into the room of their area and sets `on` for cleaning/returning. It gains:
  `activity` (the state), `battery` (int or nil), `fan_speed`, `fan_speed_list`,
  `supports_locate`, `map_camera` (entity id or empty) — read from the entity's
  attributes and the same-device lookup the floorplan already does for a tile's
  device extras.
- Tile markup reuses `.ha-device` with `data-effect="vacuum"`, `data-activity`,
  `data-battery`. CSS: a slow orbiting dot while cleaning, a pulsing ring while
  returning, a small battery badge when docked, red icon on error.
- Popover follows the existing long-press popover component in
  `internal/render/entitycontrol.go`.
- New server endpoint `GET /camera?entity_id=camera.x` → allowlist check →
  stream HA's `camera_proxy` bytes with the upstream content type and
  `Cache-Control: no-store`.

## Verification

Go unit tests for discovery fields, action allowlist (incl. fan_speed validation),
camera allowlist. Visual: the local Playwright harness (Glance's real CSS + the
renderer's HTML) with the tile in each activity state and the popover open; then
live against the owner's HA after deploy.
