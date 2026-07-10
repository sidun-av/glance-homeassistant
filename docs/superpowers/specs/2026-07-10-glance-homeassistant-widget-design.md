# glance-homeassistant — design

Date: 2026-07-10
Status: approved (via conversational brainstorming + approved visual mockup, see history)

## Problem

The user's [Glance](https://github.com/glanceapp/glance) homelab dashboard (Proxmox host, Glance
running in a Komodo-managed LXC) has no visibility into Home Assistant, which runs on the same
server. They want to see room temperature trends, which lights are on, and contact/motion sensor
state, without leaving the dashboard.

## Goal

A new native-feeling Glance widget, `HOME`, backed by a small standalone service, following the
same pattern already proven by the user's own
[`glance-grafana-sparkline`](https://github.com/sidun-av/glance-grafana-sparkline) widget: a Go
extension-widget backend, inline SVG/HTML themed with Glance's own CSS variables, shipped as its
own public repo (`sidun-av/glance-homeassistant`) with a Docker image on GHCR.

MVP scope, in order of priority (confirmed with user):
1. Room temperature — historical graph, one per room.
2. Lights — on/off state, grouped by room.
3. Contact/motion sensors (door, window, garage door, motion) — flat list, current state.

Out of scope for v1: light control (read-only for now), humidity, other domains (climate,
media_player, switch, lock), multiple HA instances, pagination (home is 3-6 rooms, fits on one
screen), auth on the widget's own endpoints (same trust model as `glance-grafana-sparkline`:
reachable only within the user's own network/reverse proxy).

## Chosen approach: REST-only integration via `/api/template`

Two ways to talk to Home Assistant were considered:

- **REST + `/api/template`** (chosen): HA's REST API includes `POST /api/template`, which renders
  a Jinja2 template server-side. Templates have access to `areas()`, `area_name(area_id)`, and
  `area_entities(area_id)` — the latter already resolves entities assigned to an area either
  directly or through a device, which is exactly the "areas" org the user uses in HA. One POST
  request returns a JSON map of room → entity IDs. Combined with a bulk `GET /api/states` (current
  values + `device_class`/`unit_of_measurement` attributes) and one batched
  `GET /api/history/period` call for temperature history, this covers the whole widget with 3
  stateless HTTP calls per render, using a single long-lived access token. No persistent
  connection to manage.
- **WebSocket API** (rejected for v1): the "official" way to read area/entity/device registries,
  plus `state_changed` event subscription for real push updates. Would need connection lifecycle,
  auth handshake, and reconnect/backoff handling — real engineering cost for a 3-6 room home that
  Glance itself only re-renders on-demand anyway (see below). Revisit only if REST polling proves
  insufficient in practice.

This mirrors `glance-grafana-sparkline`'s own precedent: stateless service, no background workers,
container can restart anytime with no data loss.

## How Glance actually refreshes widgets (verified from source, not assumed)

Read directly from `glanceapp/glance` source during design:

- `internal/glance/static/js/page.js`: on page load, `setupPage()` calls `fetchPageContent()`
  **once** (`fetch(.../api/pages/{slug}/content/)`) and inserts the result via
  `pageContentElement.innerHTML = pageContent`. There is no client-side polling loop for widget
  content — the browser never re-fetches on its own.
- `internal/glance/glance.go`: `handlePageContentRequest` calls `page.updateOutdatedWidgets()`,
  which only calls `widget.update(ctx)` (i.e. re-fetches our `/widget` URL) for widgets whose
  `requiresUpdate()` says their `cache:` duration has elapsed — and this only runs when the
  `/api/pages/{slug}/content/` endpoint is actually hit, i.e. when a browser loads/reloads the
  page. There is no background ticker independent of page views.

Two consequences that directly shaped this design:
1. **No page view → zero requests to our backend → zero requests to HA.** This already satisfies
   "don't poll Home Assistant when nobody's looking" for the base case, with no extra work.
2. **An open, un-reloaded tab never gets fresher data on its own** — Glance has no built-in
   mechanism for that. Since the user wants lights/sensors to feel live while the dashboard is
   open, that has to be built ourselves (see "Live updates" below).

Also confirmed from source (`internal/glance/widget-extension.go`, `docs/extensions.md`): the
extension widget contract (`Widget-Title` / `Widget-Content-Type: html` response headers,
`allow-potentially-dangerous-html: true` required on the widget config) and the actual dark-theme
default CSS variables/tokens (`internal/glance/static/css/main.css`) used to build the approved
mockup — `--bgh/--bgs/--bgl` (240/8%/9%), `--color-widget-background-highlight`,
`--color-text-subdue`, `--color-progress-value`, etc. The user's real instance uses a customized
accent (lavender, not the shipped default gold `hsl(43,50%,70%)`), so production output must
reference `var(--color-primary)` etc. rather than hardcoded hex, exactly like
`glance-grafana-sparkline` already does — the mockup shown to the user approximated the color by
eye since the exact override wasn't readable from outside the running instance.

## Live updates for lights/sensors

Confirmed via source read of `page.js`: page content — including our widget's HTML — is inserted
with `innerHTML`, and `<script>` elements inserted this way are inert (standard HTML5 behavior,
not a bug). A plain `<script>` in our widget response will never run.

**Chosen mechanism:** bootstrap execution with the standard `<img src=x onerror="...">` pattern —
an `onerror` (or `onload`) attribute handler *does* fire even for elements inserted via
`innerHTML`, unlike a bare `<script>` tag. This is not exploiting anything about the user's own
site; it's the standard, deterministic technique for running JS in HTML that's mounted via
`innerHTML`, and the widget already requires `allow-potentially-dangerous-html: true`, i.e. the
user already opts the whole response into full trust.

The bootstrap script:
- Runs a `fetch()` loop against a second, lightweight endpoint on our own backend — `/live.json`
  — every `live.poll_interval` (default `10s`).
- Pauses while `document.hidden` is true (`visibilitychange` listener), resuming on foreground.
- Patches only the LIGHTS and SENSORS DOM nodes in place (dot class + count/state text) — leaves
  the TEMPERATURE section alone.
- Naturally stops the moment the tab is closed or navigated away — nothing to explicitly tear
  down, since the JS execution context simply ceases to exist. This is what gives us "live only
  while the page is open" with no separate on/off signaling needed.

`/live.json` does its own bulk `GET /api/states` call against HA (no template/history calls —
those stay on the slow `/widget` path) and applies a small (2-3s) in-memory response cache purely
as a safety net against multiple simultaneous tabs/devices hammering HA; not meant to add
perceptible staleness at a 10s poll interval.

**Network requirement:** the *browser* — not just our container — must be able to reach
`/live.json`. The user reaches Glance today through NPMplus, both on the local network and
externally (domain confirmed reachable both ways). Chosen solution: proxy a path prefix (e.g.
`/ha-widget/`) on the *same* NPMplus host/domain as Glance itself, to
`http://glance-homeassistant:8080/` on the docker network. The bootstrap script then fetches a
relative, absolute-path URL (`/ha-widget/live.json`), which the browser resolves against whatever
origin the page was actually loaded from — local IP or external domain — with no separate
configuration needed for each. This is why `public_url` is a distinct config value from
`home_assistant.url`: one is "how the browser reaches us", the other is "how we reach HA".

**Rejected alternative:** two Glance `extension` widgets with different `cache:` values (fast for
lights/sensors, slow for temperature), no custom JS at all. Simpler, but "freshness" would still
only ever update on a full browser page reload — not actually live while a tab sits open, unless
the user separately runs their own kiosk-style auto-reload. Explicitly rejected because the user
asked for genuinely live behavior.

## Data flow

```
Initial render (Glance-triggered, on its own `cache:` schedule — e.g. 15m):
Glance  --GET /widget-->  glance-homeassistant
                             │ POST {HA_URL}/api/template   (areas → entities, one call)
                             │ GET  {HA_URL}/api/states      (current values + device_class, bulk)
                             │ GET  {HA_URL}/api/history/period/<start>?filter_entity_id=<temp ids>&end_time=now&minimal_response
                             ▼
                          renders full HTML: TEMPERATURE (sparklines) + LIGHTS + SENSORS
                          (initial snapshot) + bootstrap <img onerror> script
                             ▼
Glance injects fragment into the page via innerHTML on next page load/reload.

Live tier (browser-triggered, only while tab open & visible):
Browser  --fetch /ha-widget/live.json (every ~10s)-->  glance-homeassistant
                             │ GET {HA_URL}/api/states   (bulk, small in-memory cache)
                             ▼
                          returns JSON: lights + sensors state only
                             ▼
Bootstrap script patches LIGHTS/SENSORS DOM nodes in place.
```

## Entity classification (per room)

For each area returned by `/api/template`, entities are classified by domain + `device_class`
from `/api/states`:

| Domain | device_class | Section |
|---|---|---|
| `sensor` | `temperature` | TEMPERATURE — averaged if a room has more than one |
| `light` | — | LIGHTS — "N of M on" per room |
| `binary_sensor` | `door`, `window`, `garage_door`, `opening` | SENSORS — Open / Closed |
| `binary_sensor` | `motion`, `occupancy` | SENSORS — Motion / Clear |

`device_class` lists for contact/motion are configurable (with these defaults) so new
`binary_sensor` classes can be added without a code change. A room with no entities in a given
section simply doesn't appear in that section — no placeholder/empty state per room.

## History resampling (temperature)

HA's `/api/history/period` returns irregular state-change events, not evenly spaced samples. To
produce a sparkline series of `max_points` (default 60) evenly spaced values over `range` (default
`24h` — a full day/night cycle is more legible for room temperature than the 6h default used for
server metrics):

1. For each temperature entity in a room, step-forward-fill onto `max_points` evenly spaced
   timestamps across `[now-range, now]` (value at time T = most recently known state at or before
   T; entities with no data before the window start use their first available value).
2. Average across all temperature entities in the room, per bucket, to produce the room's single
   series.
3. Feed that series into the same auto min/max-scaled SVG sparkline renderer used by
   `glance-grafana-sparkline` (ported into this repo, not imported as a dependency — small enough
   to duplicate, ~50 lines).

## Visual design

One widget, titled `Home`, three sections, matching the mockup approved by the user
([artifact link shown in conversation](https://claude.ai/code/artifact/62ddd792-2f62-41cd-95e1-c23639f86fe8)):

- **TEMPERATURE** — row of mini-panels (room label + current value + sparkline), same visual
  pattern as the existing SERVER STATS widget. No "live" badge (slow cadence).
- **LIGHTS** — 2-column grid of room chips: dot indicator (filled = at least one light on) + room
  name + "N/M on" count. Has a small pulsing "live" badge in the section header.
- **SENSORS** — flat list of contact/motion entities using their HA `friendly_name`: dot indicator
  (filled = "attention" state — open/motion) + name + state label (Closed/Open/Clear/Motion). Has
  the same "live" badge.

Dot semantics are uniform across LIGHTS and SENSORS: filled/accent-colored = noteworthy (light on,
door/window open, motion detected), hollow/muted = normal/idle. No separate red/green semantic
colors — matches the user's existing dashboard, which is accent-monochrome throughout (Glance's
own default theme also aliases `--color-positive` to `--color-primary` rather than using a
distinct green).

## Configuration

### Widget service config (`config.yml`)

```yaml
home_assistant:
  url: ${HA_URL}
  token: ${HA_TOKEN}

public_url: /ha-widget   # absolute path (reverse-proxied) or full origin (direct LAN port)

title: Home
range: 24h
max_points: 60
chart_height: 34

live:
  poll_interval: 10s
  pause_when_hidden: true

sensors:
  contact_device_classes: [door, window, garage_door, opening]
  motion_device_classes: [motion, occupancy]
```

### Glance config

```yaml
- type: extension
  url: http://glance-homeassistant:8080/widget
  cache: 15m
  allow-potentially-dangerous-html: true
```

### Reverse proxy (NPMplus, same host/domain as Glance)

```
location /ha-widget/ {
    proxy_pass http://glance-homeassistant:8080/;
}
```

### Deployment (docker-compose sidecar, alongside Glance in Komodo)

```yaml
glance-homeassistant:
  image: ghcr.io/sidun-av/glance-homeassistant:latest
  restart: unless-stopped
  environment:
    - HA_URL=http://homeassistant:8123
    - HA_TOKEN=${HA_TOKEN}
  volumes:
    - ./glance-homeassistant/config.yml:/config.yml:ro
```

`HA_URL` is reachable directly on the user's network (confirmed — HA and the Komodo/Glance LXC
share a flat Proxmox network, no reverse proxy needed for the container-to-HA leg).

## Error handling

- Every HA call uses a bounded context (5s timeout), matching `glance-grafana-sparkline`.
- HA fully unreachable → whole `/widget` response renders a single "Home Assistant unavailable"
  message (same pattern as "Grafana unavailable").
- A single room's temperature history fails/returns nothing → that room's sparkline panel shows
  "no data"; the rest of the widget renders normally.
- `/live.json` failing (HA unreachable at poll time) → bootstrap script leaves the last-known
  state in place rather than clearing it, and retries on the next interval.
- The HTTP handlers always return `200 OK` with valid HTML/JSON — the service owns its own
  degraded states rather than falling back to Glance's generic widget-failed UI.

## Repository layout

```
glance-homeassistant/
  main.go
  config.go                        # config.yml loading + validation
  config.example.yml
  internal/hass/client.go          # /api/template, /api/states, /api/history/period
  internal/hass/discover.go        # area/entity classification into rooms
  internal/hass/resample.go        # step-forward-fill + averaging for temperature history
  internal/render/sparkline.go     # ported from glance-grafana-sparkline
  internal/render/template.go      # full /widget HTML assembly (3 sections + bootstrap script)
  internal/render/live.go          # /live.json JSON assembly
  Dockerfile
  docker-compose.example.yml
  README.md
  LICENSE                          # MIT
  .github/workflows/ci.yml         # go test on push/PR, GHCR publish on main/tags
```

README follows the same structure as `glance-grafana-sparkline`'s: setup (HA long-lived token
creation), `config.yml` reference table, `glance.yml` snippet, reverse-proxy snippet, deployment
snippet, error-handling notes.

## Testing

Same culture as `glance-grafana-sparkline`: `go test ./...` in CI on every push/PR.

- `config_test.go` — defaults, validation, env var expansion.
- `internal/hass/client_test.go` — `httptest.Server` mocking `/api/template`, `/api/states`,
  `/api/history/period`.
- `internal/hass/discover_test.go` — domain/device_class classification into rooms, including a
  room with no matching entities being omitted from a section.
- `internal/hass/resample_test.go` — step-forward-fill against irregular timestamps, multi-entity
  averaging.
- `internal/render/*_test.go` — HTML/JSON output shape, "no data"/"unavailable" states.
- `main_test.go` — end-to-end `/widget` and `/live.json` handler tests against a mocked HA server.

## Out of scope (explicitly, per user answers)

- Light control (toggle from the dashboard) — read-only for v1; the user confirmed control can be
  a separate follow-up if wanted later.
- Humidity and any domain beyond temperature/light/binary_sensor (climate, media_player, switch,
  lock, etc.).
- Multiple Home Assistant instances.
- Temperature history beyond HA's recorder retention (default 10 days) — no long-term statistics
  API integration.
- Pagination/collapsing — confirmed 3-6 rooms fits on one screen without it.
- Auth on the widget's own HTTP endpoints — same trust model as `glance-grafana-sparkline`
  (reachable only within the user's own network/reverse proxy, not internet-exposed directly).
