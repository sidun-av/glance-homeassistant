# glance-homeassistant

A [Glance](https://github.com/glanceapp/glance) extension widget that shows Home Assistant data
as one adaptive grid of per-room cards — each room's temperature (sparkline or bar-chart style),
lights (with their real HA icons), and occupancy/contact sensors together at a glance — with
lights and sensors updating live in the browser while the dashboard tab stays open. A room's card
grows automatically the more it has to show; a room with nothing classified gets no card at all.

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

Every setting can be set as an environment variable — no file to create or mount. Env vars always
take priority over `config.yml`, so the two approaches can be mixed if you want.

- `HA_URL` / `HA_TOKEN` — reachable from *this container* (e.g. the HA container/host's address
  on your Docker/LAN network).
- `PUBLIC_URL` — reachable from *your browser* (see step 2).
- `TEMPERATURE_CHART_STYLE` — `sparkline` (default, matches the SERVER STATS widget style) or
  `bars` (matches the built-in WEATHER widget's bar-chart style).
- `SENSORS_CONTACT_DEVICE_CLASSES` / `SENSORS_MOTION_DEVICE_CLASSES` — comma-separated HA
  `binary_sensor` device classes, if your setup uses ones not covered by the defaults.

See "Environment variable reference" below for the full list. If you'd rather hand-edit a file
instead, copy [`config.example.yml`](config.example.yml) to `config.yml`, mount it at `/config.yml`,
and skip the env vars it covers.

### 4. Run it alongside Glance

**Option A — Komodo (or any GUI stack manager that can pull a stack from a git repo):**

Point Komodo's Stack source at this repo (`sidun-av/glance-homeassistant`),
[`docker-compose.example.yml`](docker-compose.example.yml) as the compose file. Then set
`HA_URL`/`HA_TOKEN` (required) and any other overrides you want in the stack's Environment tab —
nothing to SSH in and edit. Add it to the same Docker network as Home Assistant.

**Option B — plain `docker compose`:**

```yaml
services:
  glance-homeassistant:
    image: ghcr.io/sidun-av/glance-homeassistant:latest
    restart: unless-stopped
    environment:
      - HA_URL=http://homeassistant:8123
      - HA_TOKEN=${HA_TOKEN}
      - PUBLIC_URL=/ha-widget
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

## Environment variable reference

Every one of these overrides the matching `config.yml` field when set to a non-empty value — set
just the ones you want to change (e.g. in Komodo's stack Environment tab) and leave the rest unset
to use the built-in default (or whatever `config.yml` has, if you're mounting one).

| Env var | `config.yml` field | Default | Description |
|---|---|---|---|
| `HA_URL` | `home_assistant.url` | — (required) | Home Assistant base URL, reachable from this container |
| `HA_TOKEN` | `home_assistant.token` | — (required) | Home Assistant long-lived access token |
| `PUBLIC_URL` | `public_url` | `""` (site root) | Path or origin the *browser* uses to reach this service's `/live.json` |
| `TITLE` | `title` | `Home` | Widget title shown in Glance |
| `TEMPERATURE_RANGE` | `temperature.range` | `24h` | Historical window for the temperature chart, a Go duration (`h`/`m`/`s` units only) |
| `TEMPERATURE_MAX_POINTS` | `temperature.max_points` | `60` | Points per room's temperature series (resolution) |
| `TEMPERATURE_CHART_HEIGHT` | `temperature.chart_height` | `130` | Base minimum room-card height in px — cards with more to show (lights, occupancy, contact) grow taller automatically |
| `TEMPERATURE_CHART_STYLE` | `temperature.chart_style` | `sparkline` | `sparkline` or `bars` |
| `LIVE_POLL_INTERVAL` | `live.poll_interval` | `10s` | How often the browser polls `/live.json` while the tab is open |
| `LIVE_PAUSE_WHEN_HIDDEN` | `live.pause_when_hidden` | `true` | Pause polling while the browser tab is backgrounded |
| `SENSORS_CONTACT_DEVICE_CLASSES` | `sensors.contact_device_classes` | `door,window,garage_door,opening` | Comma-separated `binary_sensor` device classes shown as Open/Closed |
| `SENSORS_MOTION_DEVICE_CLASSES` | `sensors.motion_device_classes` | `motion,occupancy` | Comma-separated `binary_sensor` device classes shown as Motion/Clear |

The service's own listen port and config-file path aren't `config.yml` fields — they're read from
the environment before any config is loaded, so they're always plain environment variables:

| Env var | Default | Description |
|---|---|---|
| `PORT` | `8080` | Port the HTTP server listens on |
| `CONFIG_PATH` | `/config.yml` | Path to the config file read at startup |

## Error handling

If Home Assistant is unreachable, the whole widget shows a single "Home Assistant unavailable"
message instead of Glance's generic widget-failed state. If a specific room has a temperature
sensor but no history data right now, only that room's card shows "no data" — the rest of the
widget still renders normally. `/live.json` failing at poll time leaves the last-known lights/
sensors state on screen rather than clearing it, and retries on the next interval.

## Out of scope (for now)

Light control (this widget is read-only), humidity and other HA domains, multiple Home Assistant
instances, history beyond HA's recorder retention, and pagination (aimed at homes small enough to
fit on one screen).

## Development

```bash
go test ./...
docker build -t glance-homeassistant:dev .
```
