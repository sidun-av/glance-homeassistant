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
