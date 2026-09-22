# Floorplan layout — design

Date: 2026-09-22. Status: approved in chat.

## Goal

A second rendering mode for the widget: instead of a flex row of room
cards, draw a schematic map of the flat. Each room is a rectangle (or any
grid-aligned shape) positioned from config, with small badges in its corners
for the data that room actually has: current temperature, lights, occupancy,
contacts. No temperature chart. The map must be trivially re-shaped from
config alone, follow Glance's theme variables, and scale its height with its
width so the proportions stay right.

## Configuration

```yaml
layout: floorplan          # env: LAYOUT — "cards" (default, current behaviour) or "floorplan"
floorplan:
  grid:                    # env: FLOORPLAN_GRID — rows joined with ";" (e.g. "bedroom bedroom kitchen;bath hall kitchen")
    - "bedroom bedroom kitchen"
    - "bedroom bedroom kitchen"
    - "bath    hall     kitchen"
    - "bath    hall     kitchen"
  rooms:                   # env: FLOORPLAN_ROOMS — "key=Area Name,key2=Area 2"
    bedroom: Bedroom       # grid key → Home Assistant Area name (as discover already reports it)
    kitchen: Kitchen
    bath: Bathroom
    hall: Hallway
```

- Tokens are whitespace-separated; `.` is an empty cell.
- Every key used in the grid must appear in `rooms`; every key in `rooms`
  must appear in the grid; each key's cells must form one rectangle
  (CSS `grid-template-areas` requires it). Violations are a config error at
  startup with a message naming the key.
- Areas not in `rooms` are not drawn. A mapped area that HA does not report
  (no entities classified) is still drawn, empty, so the map keeps its shape.
- Rows may have different lengths only if padded with `.`; ragged rows are an
  error.

## Rendering

`RenderWidget` gains a branch on `WidgetData.Layout`:

- `cards` → unchanged.
- `floorplan` → `<div class="ha-floorplan" style="grid-template-areas:...;
  grid-template-columns:repeat(C,1fr);grid-template-rows:repeat(R,1fr);
  aspect-ratio:C/R">` followed by one `.ha-fp-room[data-room=…]` per mapped
  room with `style="grid-area:key"`. The container's `aspect-ratio` is what
  makes the height follow the width; rows are `1fr` so every cell is the same
  size and the map is proportional.

Each room:

```
<div class="ha-fp-room" data-room="Bedroom" data-lit data-occupied style="grid-area:bedroom">
  <span class="ha-fp-name">Bedroom</span>                       (top-left)
  <span class="ha-fp-temp">22°</span>                            (top-right, only with a current reading)
  <span class="ha-fp-lights"> …existing .ha-light spans… </span>   (bottom-left)
  <span class="ha-fp-status"> …existing .ha-occ-chip / .ha-badge… </span> (bottom-right)
</div>
```

Reusing the existing `.ha-light`, `.ha-occ-chip`, `.ha-badge` markup and
`data-*` attributes means the live poller (`bootstrapScript`) and
`/live.json` work unchanged — it looks rooms up by `.ha-room[data-room]`, so
`.ha-fp-room` also carries class `ha-room`.

Style: rooms are separated by 2px gaps showing the widget background; each
room uses `var(--color-widget-background-highlight)`-ish fill via
`color-mix(in srgb, var(--color-text-base) 6%, transparent)`, 1px border
`var(--color-widget-content-border)`, `border-radius` matching Glance cards.
A lit room keeps the same warm tint the card layout uses. All colours come
from Glance CSS variables or `color-mix` on them, so light/dark/custom themes
work without a theme switch of our own. Chips in the floorplan get a slightly
smaller font (`10px`) and the light icons `20px` to fit small rooms.

Current temperature: `RoomCardView` gains `CurrentTemp string` (already
computed as `avg[currentIdx]` in `widgetHandler`; formatted with `%.0f°`).
Empty when no data. Cards layout ignores it.

## Live updates

No change to `/live.json`. The bootstrap selector already targets
`.ha-room[data-room]`, and the badge elements keep their existing classes.

## Out of scope

Click-to-toggle lights, per-room links, temperature history, walls/doors
drawing, non-rectangular rooms beyond what grid areas allow.

## Testing

- `config_test.go`: valid grid parses; ragged rows, unknown key, non-rectangle
  key, key missing from grid → errors naming the key; env overrides.
- `template_test.go`: floorplan HTML has the `grid-template-areas` string,
  `aspect-ratio`, one `.ha-fp-room` per mapped room in config order, the
  temperature badge only when `CurrentTemp != ""`, and a mapped room absent
  from HA data still renders (empty).
- Existing tests untouched (cards is the default).
