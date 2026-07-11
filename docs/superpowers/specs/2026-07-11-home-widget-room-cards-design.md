# HOME Widget Room-Card Redesign

**Goal:** Replace the widget's three stacked sections (Temperature / Lights / Sensors) with a single adaptive grid of per-room cards, so a room's temperature, lights, occupancy, and contact sensors are all visible together at a glance, with real HA-assigned icons where available.

**Architecture:** The `hass` package gains one new merge step that groups the existing classification output (temperature rooms, light rooms, occupancy/contact sensors) into one `[]RoomCard` per room, dropping rooms with nothing to show. `render` gains a per-fixture icon glyph mapper and a full rewrite of `RenderWidget` to lay out room cards in a CSS flexbox grid instead of three sections. `main.go`'s live-update payload and bootstrap script are reworked to match the new per-light, per-room shape.

**Tech Stack:** No new dependencies. Same Go stdlib + `gopkg.in/yaml.v3` as today; same inline-SVG-in-HTML rendering approach; same `element.innerHTML` + `onerror` bootstrap trick for the live-update script.

## Global Constraints

- This **replaces** the three-section layout entirely — there is no config flag to fall back to the old Temperature/Lights/Sensors sections. (Explicit instruction: "поменяем логику домашних виджетов... не будем выделять температуру отдельно".)
- Color tokens must be real Glance CSS custom properties, verified against Glance's own `main.css` (`github.com/glanceapp/glance`), never guessed:
  - `var(--color-progress-value)` — temperature graph fill/stroke (already fixed in `widget-polish`, unchanged here).
  - `var(--color-primary)` — the "LIVE" badge/dot and the occupancy chip/glow (small accent highlights, matching Glance's own convention of using this token for exactly this kind of thing, not large fills).
  - `var(--color-negative)` — confirmed present (`hsl(0, 70%, 70%)` in the default theme) — used for the contact-sensor "open" badge, replacing the placeholder hardcoded orange (`--attn`) used in the throwaway mockup.
  - `var(--color-text-subdue)` / `var(--color-text-highlight)` — secondary/primary text, as today.
  - `var(--color-widget-background-highlight)` / `var(--color-widget-content-border)` — card background/border, as today.
  - The "light is on" amber glow and the "room is lit" background tint stay **hardcoded** (not a Glance token) — this is a deliberate universal warm-light cue (like a real bulb), not meant to track the user's accent color, the same reasoning Glance itself uses for its weather widget's blue rain effect.
- No new environment variables for MVP. The icon glyph set is a fixed, hardcoded lookup table, not user-configurable.
- `TEMPERATURE_CHART_HEIGHT` / `temperature.chart_height` **changes meaning** (see "Card sizing" below) — this is a documented breaking behavior change to an existing config field, not a new field.

---

## 1. Merged per-room data model

### Problem with today's model

`hass.Model` has three independent, differently-shaped collections:

```go
type Model struct {
    TemperatureRooms []TemperatureRoom   // Room, EntityIDs
    LightRooms       []LightRoom         // Room, On (count), Total (count)
    Sensors          []SensorEntity      // Name, Attention, Label — NOT grouped by room at all
}
```

`SensorEntity` has no `Room` field, and `LightRoom` only carries an aggregate on/total count, not each light's own state. Neither supports "one card per room showing everything that room has."

### New model

```go
type Light struct {
    EntityID string
    Name     string
    On       bool
    Icon     string // raw HA `icon` attribute (e.g. "mdi:track-light"), "" if unset
}

type SensorEntity struct {
    Room      string // NEW
    Name      string
    Attention bool
    Label     string
}

type RoomCard struct {
    Room        string
    Temperature *TemperatureRoom // nil if this room has no temperature sensor
    Lights      []Light          // nil/empty if this room has no lights
    Occupancy   []SensorEntity   // motion/occupancy device_class entities in this room
    Contacts    []SensorEntity   // door/window/etc device_class entities in this room
    Weight      int              // see "Card sizing" below
}
```

`TemperatureRoom` (`Room`, `EntityIDs`) is unchanged — it's already correctly room-keyed.

`BuildModel` changes from producing three collections to producing `[]RoomCard`, sorted alphabetically by room name (same ordering convention as today). A room is **dropped entirely** (no card) when it has no temperature, no lights, no occupancy sensor, and no contact sensor — e.g. an HA area with zero assigned entities (confirmed real case: the user's "Bathroom" area exists but has no entities at all). This mirrors the existing "only show rooms with a temperature sensor" rule, just generalized to the whole card.

`Light.On` and each `SensorEntity.Attention` still come from `state == "on"`, exactly as today. The classification loop in `BuildModel` already has `room.Name` in scope when it processes each entity (it iterates `for _, room := range rooms { for _, entityID := range room.EntityIDs {...} }`), so plumbing `Room` through to `SensorEntity` and collecting individual `Light` entries instead of just counters is a local change, not a new data source.

### Fetching the icon attribute

`hass.Client.FetchStates` must additionally decode `attributes.icon` from HA's `/api/states` response into a new `EntityState.Icon string` field (empty string when the entity has no custom icon, which HA's response omits or sets to `null`). No new HTTP call — this rides along with data already being fetched.

---

## 2. Card sizing — adaptive, not hand-positioned

Confirmed by iteration in the browser mockup (three failed layout attempts are worth recording so this isn't re-litigated):

1. **Hand-placed `grid-template-areas` per room name** — rejected. Requires one-time manual config mapping room names to positions, and doesn't scale to rooms that don't exist yet.
2. **CSS Grid with `grid-auto-flow: dense` and `grid-column`/`grid-row` spans computed from a "weight"** — rejected. `auto-fill` pre-declares a fixed column count sized to the container; whenever a row's spans don't sum to exactly that count (near-guaranteed with a handful of rooms of varying size), the leftover columns render as a dead, un-fillable rectangle — this happened concretely in testing (a 2-row-tall empty gap next to a big card with nothing to its right).
3. **Flexbox with `flex-wrap: wrap`, per-card `flex-grow`/`flex-basis`, `align-items: stretch`** — adopted. Flexbox redistributes whatever space is left on *every* line (including a partial last line) among the items that actually exist on it, so there is structurally no way for empty space to appear — this is the correct tool for "cards should occupy all the space, distributed efficiently" with an unpredictable, changing number of rooms.

### Weight formula

```
weight = (2 if room has a temperature sensor else 0)
       + len(room.Lights)
       + (1 if room has any occupancy sensor else 0)
       + (1 if room has any contact sensor else 0)
```

Temperature counts double because its sparkline/bar chart needs width to be legible; each light and each sensor type contributes 1.

### Size tiers (CSS, verified in the browser mockup)

```css
.b-room       { flex: 1 1 160px; min-height: 130px; }  /* weight <= 2 */
.b-room.size-md { flex: 2 1 320px; min-height: 150px; } /* weight 3-4 */
.b-room.size-lg { flex: 3 1 340px; min-height: 260px; } /* weight >= 5 */
```

`Weight` is computed once, in `hass.BuildModel`, as part of building each `RoomCard` (it's a property of the room's data, not of how it's drawn). Which CSS class a given weight maps to (`""`/`"size-md"`/`"size-lg"`) is a rendering concern and lives in the `render` package as a small pure function consuming `Weight`, applied server-side when building each `RoomCardView` — no client-side JS involved in sizing. Verified against the user's real data: Kitchen (temp only, weight 2) and Hallway (1 light + occupancy, weight 2) stay base-sized; Bedroom (2 lights + occupancy, weight 3) gets `.size-md`. No real room currently reaches `.size-lg` (weight ≥ 5) — verified in the mockup with a hypothetical "fully equipped" example card (temp + 3 lights + occupancy + contact = weight 7), dashed-outlined and clearly marked as illustrative, not shipped as real markup.

### Card sizing config (breaking change to an existing field)

`temperature.chart_height` / `TEMPERATURE_CHART_HEIGHT` no longer sets a literal pixel height for the temperature SVG (that no longer makes sense once the graph flex-grows to fill whatever space its card's tier gives it). It becomes the **base `min-height` for a `.b-room` card** (replacing the `130` literal above); `.size-md`/`.size-lg` scale from it proportionally (`+20`, `+130`). **`LoadConfig`'s default must change from `34` to `130`** to match the value verified in the mockup — `34` was sized for a bare sparkline SVG and would make every base-tier card absurdly short under its new meaning. This default change plus the semantic change both belong in the README's env var reference table as a behavior change, not a new variable.

`temperature.chart_style` (`sparkline` | `bars`) is unchanged in meaning — both styles are still supported, now rendered inside a room's card instead of a standalone section, using `flex: 1` on the chart SVG so it fills whatever vertical space its card allocates (same technique already shipped in `widget-polish` for the standalone sparkline's timeline).

---

## 3. Per-light bulb tiles with curated HA icons

Each light in a room's card renders as its own icon glyph (not an aggregate "2/3" count), reflecting that light's own on/off state.

### Icon glyph mapping

A small lookup table maps HA's raw `icon` attribute string to one of a fixed set of hand-drawn SVG glyphs; anything not in the table (including an empty/missing icon) falls back to a generic light bulb glyph:

| HA `icon` attribute | Glyph |
|---|---|
| `mdi:track-light` | track-light (spotlight on a rail) |
| `mdi:led-strip-variant` | led-strip (rounded bar with LED dots) |
| *(anything else, or empty)* | generic bulb (fallback) |

This is deliberately **not** a full Material Design Icons integration — MDI has thousands of icons, and bundling/rendering arbitrary ones was explicitly rejected in favor of a small curated set (user's choice: "Небольшой набор иконок из HA"). The two glyphs above are the ones confirmed present in the user's real data (`bedroom_photo_light` → `mdi:track-light`, `LEDlight_bed_bedroom` → `mdi:led-strip-variant`); `light_switch_hallway` has no icon attribute and correctly falls back to the bulb, and this fallback is also what would catch a clearly wrong/stale icon value like `light.one_way_color_light`'s actual `mdi:alarm-off` — the lookup only trusts icons it explicitly recognizes, so a mis-set attribute never gets drawn literally.

Extending the table later (e.g. adding `mdi:ceiling-light` if the user assigns one) is a one-line addition: one new SVG constant plus one lookup entry, no architectural change — not built speculatively now per YAGNI, since only two glyph types exist in the user's real data today.

### On/off rendering

Each glyph has an "on" and "off" CSS variant (glass/body stroke+fill switch between `var(--amber)`/`var(--amber-dim)` when on and `var(--color-text-subdue)`/none when off), matching the already-approved bulb glyph's on/off treatment.

---

## 4. Occupancy visualization

A room with an occupancy sensor (device_class in `sensors.motion_device_classes`, same config as today) shows two things when occupied, verified together after animation-only feedback proved insufficient:

1. **A static, always-visible pill** inside the card: `● Occupied` (filled with `var(--color-primary)`) or, when present but idle, `● Clear` (outlined, dim `var(--color-text-subdue)`). This is the source of truth — legible in a single still frame (a screenshot, a paused browser, `prefers-reduced-motion`), not just live viewing.
2. **A soft breathing glow** around the whole card border (`box-shadow` animation cycling opacity/spread, `var(--color-primary)`) — an ambient extra for when the page is actually being watched live, not required to understand the state.

Reasoning for needing both: the first iteration used *only* the animated glow plus a small pulsing icon, and neither survived being screenshotted — animation is invisible in a single frame by definition, so a genuinely static indicator is required alongside it, not instead of it.

A room can have more than one occupancy sensor (uncommon, not present in the user's real data, but the model is a slice, not a single value) — MVP renders one pill per occupancy sensor; the card-level glow triggers if *any* of them is occupied.

---

## 5. Contact sensors

Unchanged in spirit from the original design (`sensors.contact_device_classes` config, same open/closed classification), just relocated: rendered as a small badge (door icon rotating open/closed + text label "Window open"/"Door closed") inside the room's card instead of a separate flat list, using `var(--color-negative)` for the "open" (attention) state instead of the earlier placeholder color.

The user currently has **zero** contact sensors configured, so this badge won't render for any real room today — the capability is built and tested against synthetic data, ready for whenever a door/window sensor gets added to an area.

---

## 6. Live updates

`/live.json`'s payload and the bootstrap script both change shape to match the merged, per-light model:

```json
{
  "rooms": [
    {
      "room": "Bedroom",
      "lights": [
        {"entity_id": "light.usb_light_bedroom_l1", "on": false},
        {"entity_id": "light.ledlight_bed_bedroom", "on": false}
      ],
      "occupancy": [{"name": "presence_sens_bedroom Occupancy", "attention": true, "label": "Occupied"}],
      "contacts": [{"name": "LR Window", "attention": true, "label": "Open"}]
    }
  ]
}
```

`lights`/`occupancy`/`contacts` are each omitted (or empty) when the room has none of that kind — a room with no lights, no occupancy sensor, and no contact sensor (temperature-only, e.g. Kitchen) is **omitted from the `rooms` array entirely**, since there's nothing on that card to live-update, exactly like today's `/live.json` never mentioned temperature-only rooms.

The bootstrap script's `applyState` is rewritten to: find each light icon element by `data-entity-id` and toggle its on/off glyph class; find each room card by `data-room` and toggle its occupancy pill/glow (still using the existing `find-by-name` pattern for the pill's label text, since a pill's text also needs to flip between "Occupied"/"Clear"); find each contact badge by name, same lookup pattern used today, just re-scoped to look inside the room's card instead of a flat top-level list. The 10s poll / pause-when-hidden / visibility-change logic is unchanged — only the DOM shape being updated changes.

---

## 7. Rendering / CSS

`internal/render`'s `styleBlock()` is rewritten (not incrementally patched) to match the browser-verified mockup: `.b-plan` (flex-wrap container), `.b-room` (+ `.size-md`/`.size-lg`), `.b-room-head` (room name + temp value), `.b-spark` (flex-grow chart), `.b-lights-row` (flex-grow, wraps), `.occ-chip` (+ `.occupied`), `.status-badge` (+ `.attn` using `--color-negative`), the light glyph classes (`bulb-on`/`bulb-off`, `track-on`/`track-off`, `strip-on`/`strip-off`), and the `occ-glow` keyframes. The `.ha-*` class names from the old three-section layout (`.ha-temp-row`, `.ha-lights-grid`, `.ha-sensor-list`, etc.) are removed along with the sections themselves — nothing references them once `RenderWidget` no longer emits them.

`RenderWidget(data WidgetData)` is rewritten to loop over `[]RoomCardView` (new render-package view type mirroring `hass.RoomCard`, with the chart SVG and icon glyphs already rendered to strings, same separation of concerns as today's `TemperatureRoomView`/`LightRoomView`/`SensorView`) instead of three separate slices.

---

## Out of scope (explicitly deferred, not silently dropped)

- Full Material Design Icons support (arbitrary `mdi:*` passthrough) — rejected in favor of the curated set above.
- Hand-positioned "real floor plan" layout — rejected in favor of the adaptive weight-based grid.
- Illuminance sensors (`sensor.*_illumination`, lux-value sensors) present in the user's real data — not part of temperature/lights/occupancy/contact scope, not added here.
- Multiple occupancy/contact sensors per room getting a combined/deduplicated display (e.g. two door sensors becoming one "any open" badge) — each renders its own pill/badge; no combining logic, since the user's real data never has more than one of either per room today.
