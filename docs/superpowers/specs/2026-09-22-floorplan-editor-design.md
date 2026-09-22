# Floorplan editor — design

Date: 2026-09-22. Status: approved in chat (scope confirmed by the user).

## Goal

A browser page, served by this same service, where the floorplan is edited
visually instead of in YAML/env: number of rooms, their size and position on
the map, each room's inner grid, and where each Home Assistant entity sits
inside its room (drag and drop). A gear icon on the widget (visible on
hover, top-right) opens it. Saved layouts persist in a file on a volume and
take precedence over the `floorplan:` config, which stays as the seed.

## Storage

`LAYOUT_FILE` (default `/data/floorplan.json`). Loaded at startup and after
every save; missing file → the config/env floorplan is used and shown in the
editor as the starting point. Written atomically (temp file + rename).

```json
{
  "version": 1,
  "aspect_ratio": "1.15",
  "max_width": 420,
  "columns": 4, "rows": 4,
  "rooms": [
    {"key": "bedroom", "area": "Bedroom", "name": "",
     "cells": [[0,0],[0,1],[1,0],[1,1]],
     "grid": {"columns": 3, "rows": 3},
     "entities": {"light.bed": [0,1], "media_player.x": [1,2]},
     "hidden": ["switch.usb_light_bedroom_l2"]}
  ]
}
```

- `cells` are `[row, col]` on the map grid; a room must be one rectangle
  (CSS grid-area) — the editor only lets you extend a rectangle.
- `name` overrides the Area name on the map when set.
- `grid` is the room's inner grid; `entities` maps entity_id → `[row, col]`
  in it. Unplaced entities fall back to the automatic wall slots; `hidden`
  ones are not drawn.
- The saved layout is converted to the existing `render.Floorplan` (grid
  strings + keys) plus a per-room placement table, so rendering reuses the
  current code path.

## Rendering changes

`render.Floorplan` gains `Placement map[key]RoomPlacement{Grid{Rows,Cols},
Cells map[entityID][2]int, Hidden set}`. A room with a placement renders its
inner grid `rows×cols` (instead of the fixed 3×3) and positions each placed
tile at its cell with `--dir` computed from the cell's position relative
to the grid centre (`atan2`) and `--len` from the distance to the centre in
container units. A tile in the centre cell casts no beam. Unplaced tiles
keep the wall-slot behaviour.

The widget gets `<a class="ha-fp-edit" href="{public_url}/edit" title="Edit
floorplan">⚙</a>` inside `.ha-widget`, `opacity:0` until `.ha-widget:hover`.

## Editor page (`GET /edit`, plus `{prefix}/edit`)

Single HTML page, inline CSS/JS, no framework, dark/light via
`prefers-color-scheme`. Data endpoints (also under the prefix):

- `GET /edit/data.json` → current layout + HA areas (id, name, entities with
  entity_id, friendly name, domain, icon, "classified" flag = would the
  widget draw it as light/device/contact).
- `PUT /edit/layout.json` (body = layout) → validates (rectangles, unique
  keys, cells inside grid, entities inside their inner grid), writes the
  file, reloads it into the running app, returns the normalised layout or a
  400 with a message.
- `GET /edit/preview` → the widget HTML rendered with the layout from the
  request body (POST) — used by the Preview button.

Layout of the page: left column = map (N×M cells, sizes as inputs, rooms
list with add/rename/delete/area select, click-drag on cells paints the
selected room, painting keeps it rectangular); right column = the selected
room: inner grid size inputs, the grid itself as drop targets, and the
entity list for the room's Area (classified first, "show all" toggle for the
rest), items draggable onto grid cells, drag back to the list to unplace, an
eye toggle to hide. Bottom: Preview, Save, Reset to config.

## Security

No auth of its own: externally the page sits behind the same forward-auth
as the rest of the domain; on the LAN it is open like the widget. The
service already trusts the LAN for `/live.json`.

## Testing

- `layout` package: JSON round-trip, validation errors (non-rectangle,
  overlapping rooms, cell outside grid, entity outside inner grid), and
  conversion to `render.Floorplan` (grid strings identical to what the YAML
  path produces for the same rooms).
- render: placed tile gets `--dir`/`--len` from its cell; centre cell → no
  beam; hidden entity not rendered; unplaced falls back to wall slots.
- handlers: PUT invalid → 400 with message; PUT valid → file written and
  `/widget` reflects it; GET /edit serves the page under both paths.

## Out of scope

Editing HA itself (areas/entity assignment stay in HA), multiple floors,
free-form (non-grid) shapes, undo history beyond "Reset to config".
