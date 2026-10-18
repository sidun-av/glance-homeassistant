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

## Floorplan layout

`layout: floorplan` swaps the room cards for a schematic map of the home. Rooms are laid out from
an ASCII grid in config (it is CSS `grid-template-areas`, verbatim), so reshaping the map is a
config edit, not a code change:

```yaml
layout: floorplan
floorplan:
  aspect_ratio: "1.15"                  # optional: width/height of the whole map
  max_width: 420                        # optional: px, keeps the map compact in a wide column
  grid:
    - "bedroom bedroom kitchen kitchen"
    - "bedroom bedroom kitchen kitchen"
    - "bath    hall    kitchen kitchen"
    - "bath    hall    kitchen kitchen"
  rooms:                                # grid key → Home Assistant Area name
    bedroom: Bedroom
    kitchen: Kitchen
    bath: Bathroom
    hall: Hallway
```

Each room shows its name, and — only where the room has that data — the current temperature
with a trend arrow (red ↑ rising, blue ↓ falling over the last hour, ≥0.2°; click it
for a chart of the last 12 hours), and its
entities as tiles on a virtual 3×3 grid: tiles take the wall slots first (top, bottom, left,
right, then the corners) and hug that wall; only a ninth tile lands in the centre. Every source
casts a beam toward the room's centre, sized to stop there whatever the room's shape:

- **Lights** — a lit lamp throws a warm spotlight cone (bright at the lamp, fading at the centre).
- **Devices** (`devices.domains`, default fans, climate, humidifiers, water heaters, media
  players, vacuums, covers, locks — real MDI icons per domain, or the entity's own `mdi:` icon
  when it is one we ship): a running `fan`, or a `climate` unit that is cooling/fanning, blows a
  blue stream with moving waves and the fan icon spins — a fan's stream is as long as its speed,
  gusts in a natural-wind/breeze preset, and while oscillating sweeps wall to wall (its edge comes
  to lie along the walls the fan stands against, never past them); a heating
  `climate`/`water_heater` blows an orange one; a playing media player sends notes drifting up; a
  robot vacuum shows what it is doing — a dot orbiting it while cleaning, a ring pulsing in with a
  dock badge while returning, its battery while docked (from `battery_level`, or a battery sensor of
  the same HA device), a pause badge while paused, red with a "!" on error, faded with a "?" when HA
  can't reach it. A source placed in a room's centre
  cell glows or ripples all around instead of casting a beam. Other devices just light up while
  active (cleaning, open, unlocked, on).
  `switch` is off by default because most switches are a light's second channel or a config
  toggle — add it to `devices.domains` and prune with `devices.exclude` if you want them.
- A running figure per motion/occupancy sensor (accent-coloured while motion is detected, faint
  otherwise), and a door icon per contact sensor. An occupied room also gets a thin accent outline.

`max_width` caps the map's size so it does not swallow a wide column.

### Controlling things from the map

With a `public_url` set, the map is interactive (mouse or touch):

- **Tap** a light or device tile to toggle it (`homeassistant.toggle`; the tile flips at once
  and reconciles with HA on the next poll).
  A vacuum never acts on a tap: a tap opens its controls (below), so a stray touch can't start a
  clean.
- **Long-press** (½ s) to open its controls (for a vacuum a plain tap does the same):
  - light — brightness, colour temperature and colour swatches, whichever `supported_color_modes`
    allows;
  - fan — speed, preset modes (e.g. straight / natural wind), oscillation;
  - climate — target temperature (−/+ and a slider) and HVAC mode; water heater — target;
  - vacuum — status and battery, Start/Pause, Dock, Locate (each as its `supported_features` allow), suction from its
    `fan_speed_list`, and the map with the route (see "Vacuum map" below; refreshed
    every 10 s while cleaning or returning and once more when the run ends; a refresh that fails
    dims the last map and marks it "Map not updated");
  - plus the device's other controls: every `select` (chips), `number` (slider) and `switch` of the
    same Home Assistant device — a fan's swing angle, level, off timer, beep, child lock… Stock
    Xiaomi names get readable labels; entities renamed in HA keep their names.
- **Keyboard**: interactive tiles are focusable — Enter/Space is a tap, the context-menu key (or
  Shift+F10) a long-press, Escape closes the controls.
- **Click a room's temperature** for a popover with its last 12 hours (now / min / max and a
  crosshair readout).

All of it goes through this service, never straight to HA: `POST <public_url>/entity`
`{entity_id, action, …}` checks the domain/action against an allowlist and validates values
(`select`/`number`/`switch` only for entities the map actually shows), and
`GET <public_url>/temperature?room=<Area>` serves the chart's 15-minute series. A vacuum's map comes
through `GET <public_url>/camera?entity_id=camera.…`, which serves only the map camera of a vacuum
on the map (HA's `camera_proxy`, or `image_proxy` for an `image.…` entity, with this service's
token, so the browser never needs it). Media players are controlled from the Now playing panel
instead.

**Vacuum map.** The map entity is `devices.map_cameras[<vacuum>]` when set (a `camera.…` or
`image.…` entity — use it when the guess below is wrong). Otherwise it is a `camera.…` or `image.…`
entity of the vacuum's own HA device: the one whose entity_id contains `map` if there are
several, else the first. That is a naming heuristic — most integrations call it
`camera.<robot>_map` or `image.<robot>_map`, but not all; a robot with several cameras and no
`map` in any id gets the first one. No entity on the device and nothing configured means no map
in the popover. The entity is picked whether or not it is available at the time; if HA can't
serve it, the popover says "Map unavailable".

### Now playing

Next to the map the floorplan layout lists every reachable `media_player` (anything not
off/unavailable): room · player name, track title and artist (or its state), and previous /
play-pause / next buttons that call the corresponding Home Assistant services through this
service's `<public_url>/media` endpoint (POST `{entity_id, action}`; only `media_player.*` and the
media_* transport services are accepted), plus album art (proxied through `<public_url>/art` so the
browser never needs the HA token) and a progress bar that keeps ticking between polls while a track
plays — click anywhere on the bar to seek — and a volume slider (hidden for players without one).
Cards share the panel's height so the panel matches the map. Each card has its own accent
colour, and the notes that drift up from that speaker's tile on the map use the same colour. Rows
update live with the rest of the widget; play/pause flips the card immediately and reconciles with
HA on the next poll.

### Editing the map in the browser

Hover the widget and click the gear in its top-right corner (or open `<public_url>/edit`, e.g.
`https://your-glance/ha-widget/edit`). The editor is one map that looks like the widget, styled with
Glance's theme variables, and works with mouse or touch:

- **Rooms** (left): select a room to set its Home Assistant Area and the label shown on the map,
  or delete it; **+ Add room** takes the largest free area. **Map**: size in cells (steppers),
  aspect ratio, max width.
- **Map** (centre): click a room to select it, drag it to move, pull the handles on its edges and
  corners to resize — a neighbour in the way gives way (trimmed to its largest remaining
  rectangle), a change that would erase a room is refused. Drag across empty floor to draw a new
  room. Inside the selected room, drag entity icons between cells: an edge cell hugs that wall and
  beams toward the centre, the centre cell glows all around.
- **Entities** (right): the selected room's tiles grouped as *On the map*, *Auto-placed* (the
  widget lines these up along the walls) and *Hidden* — drag between the lists and the map, or use
  the eye to hide/show. **Placement grid**: match the room's footprint, or a custom size.
- **Preview** (bottom): the real widget, refreshed as you edit (it is live — tapping a lamp there
  switches it).
- Undo/redo (⌘/Ctrl+Z, ⇧⌘Z / Ctrl+Y), Save (⌘/Ctrl+S), an unsaved-changes marker and a warning
  on leaving; the ⋯ menu discards changes or starts over from the config layout.

Saved layouts live in `LAYOUT_FILE` (default `/data/floorplan.json`) and take precedence over the
`floorplan:` config, which remains the seed. Mount a volume at `/data` to keep them across
container recreations:

```yaml
    volumes:
      - glance_ha_data:/data
```

The editor has no login of its own — it is exactly as exposed as the widget's `public_url`. If that
is reachable from the internet, put it behind your reverse proxy's auth (forward-auth, access list) — the same goes for the
map's controls (`/entity`), which switch real devices.

The floorplan layout has no per-room temperature chart on the map itself (the chip opens one).
Colours all come from Glance's theme variables, so it follows whatever theme the dashboard runs. The map's height
follows its width via `aspect_ratio` (default: square cells, i.e. `columns/rows`). Live updates
work exactly as in the cards layout.

Rules: every key in `grid` must be in `rooms` and vice versa, rows must have the same number of
cells (pad with `.` for empty space), and each key must cover one rectangle — L-shapes are not
possible with CSS grid areas; split such a room into two keys mapped to the same Area if you need
it. Violations are reported at startup naming the key.

## 3D home (House Control)

If you draw your home in [House Control](https://github.com/sidun-av/house-control) (a macOS app
with a 3D home model), the widget can show that model too, driven by the same Home Assistant:
lamps on/off, brightness and colour, climate mode and target temperature, a robot vacuum's
start/pause/dock and suction. House Control's web build is vendored into this image (see
"Updating the vendored build" below), so there is nothing extra to run on the server. The widget
server reads and controls Home Assistant on the 3D view's behalf, and the HA token never reaches
the browser.

**What you see.** Once a plan has been uploaded and `public_url` is set, the widget's header gets
a **Plan | 3D** switch. **3D** shows the House Control page in an iframe under
`<public_url>/house/`. **Plan** shows the usual map or cards and removes the iframe, so no 3D page
is loaded and no WebGL runs while it is selected. A first visit opens on **3D**, except on narrow
screens (600 px or less), which open on **Plan**. The choice is remembered per browser. In Safari
17 or later on a Mac, a small card offers to add the dashboard to the Dock (Safari's
*File → Add to Dock…*), so it opens in its own window like an app. **Not now** hides the card for
good in that browser. Without an uploaded plan the widget looks exactly as before: no switch, no
card.

Glance builds a page once, when it loads, and does not re-render widgets while the page stays open,
so the 3D view keeps running (and keeps its camera and selection) until you reload the page or
pick **Plan**. The widget's own live updates never touch it. One exception is Glance's
`split-column` widget: it re-arranges its children when its column count changes on resize, which
reloads an iframe inside it.

**What the 3D view controls.** Only devices that the plan links to Home Assistant, with a domain
that fits their kind: lights are `light.*` or `switch.*`, an AC is `climate.*`, a vacuum is
`vacuum.*`. Every other device is dropped from the served plan: unlinked ones, devices on another
service, unknown domains. So nothing in the view is simulated or silently inert. Room, device and
scene names are shown as they are in the plan. `HOUSE_LANG` sets the language of the view's own
text (`en` or `ru`).

**Getting the plan to the server.** House Control keeps its plan on the Mac at
`~/Library/Application Support/HouseControl/plan.json`. `tools/house-plan-push` pushes it to the
widget server whenever it changes. The push goes straight over the LAN to the container's own port,
not through the reverse proxy (see the upload rules below). On the Mac:

```bash
mkdir -p ~/.config/glance-house && chmod 700 ~/.config/glance-house
echo 'http://<lan-host>:<port>' > ~/.config/glance-house/url    # this container's LAN origin
( umask 077; openssl rand -hex 32 > ~/.config/glance-house/secret )
chmod 600 ~/.config/glance-house/secret
tools/house-plan-push/install.sh            # --dry-run first shows what it would do
```

Give the server the same secret as `HOUSE_PLAN_SECRET`. Both sides ignore leading and trailing
whitespace, such as the file's newline; `push.sh` refuses a secret with whitespace inside it. `install.sh` installs a user LaunchAgent.
It runs `push.sh` when the plan file changes, every 5 minutes, and at login. Each run first asks
the server (a plain `GET /house/plan` with `If-None-Match`) whether it already serves this exact
plan, and sends it only when it does not, so a server that lost its plan (a recreated data volume,
a restored backup) gets it back within 5 minutes. A failed push is retried on the next run. It logs to
`~/Library/Logs/glance-house-push.log`, where a successful run says `pushed <hash>`. To push once
by hand, run `sh tools/house-plan-push/push.sh`.

If the log says `push failed (curl 7, ...)` even though the server is up, macOS Local Network
privacy is blocking the background agent. Install the same job as a LaunchDaemon that runs as you
instead (needs sudo):

```bash
tools/house-plan-push/uninstall.sh
tools/house-plan-push/install.sh --daemon
```

`uninstall.sh [--daemon]` stops and removes the job. It keeps `~/.config/glance-house` and the log.

**Endpoints.** All of these are served under `<public_url>/house/`, and also at `/house/` on the
container's own port.

| Endpoint | What it does |
|---|---|
| `GET /house/` | The House Control page, with its boot config (API path, language) injected. Static files are under `/house/assets/`. |
| `GET /house/plan` | The stored plan, filtered to the HA-linked devices, with an `ETag`. Returns 404 `{"error":"no_plan"}` before the first upload. |
| `PUT /house/plan` | Upload a plan. Needs `Authorization: Bearer <HOUSE_PLAN_SECRET>`. The rules are listed below. |
| `GET /house/state` | Current state of the plan's devices, keyed by plan id, plus `planVersion` (the plan's ETag; open views reload when it changes). Returns 503 `{"error":"unavailable"}` when Home Assistant is down or slow. |
| `POST /house/command` | `{"kind","id","patch","state"}` with `Content-Type: application/json`. Accepted only for a device in the served plan, and only for patch keys and values that are allowed for its kind. Add `?dry_run=1` to get back the Home Assistant service calls it would make, without making them. The server as a whole takes about 10 commands a second (bursts of 30); past that it answers 429 `{"error":"rate_limited"}`. |

`PUT /house/plan` rules:

- Uploads are disabled (403) while `HOUSE_PLAN_SECRET` is empty.
- An upload is refused (403) unless the TCP peer has a private address: loopback, RFC 1918
  (`10/8`, `172.16/12`, `192.168/16`) or IPv6 unique-local (`fc00::/7`). Behind Docker's port
  publishing that is the LAN client or the Docker gateway.
- An upload is refused (403) when it carries `X-Forwarded-For`, `X-Real-IP` or `Forwarded`. That
  means it came through a reverse proxy rather than straight over the LAN.
- Both of these are defence in depth. The secret is the actual control, so keep it long and random.
- The body must be at most 1 MiB and a JSON object with a non-empty `rooms` array.
- No key at any depth may be named `token`, `accessToken`, `access_token`, `password` or `secret`.

Error bodies are only codes such as `{"error":"unknown_device"}`. Home Assistant's own error text
goes to the server log. `/house/*` sends no CORS headers: the page is meant to be used same-origin,
from inside the widget.

**Who can control the home.** Anyone who can open `<public_url>` can switch the devices in the 3D
view. This is the same rule as the map's controls and the editor. If `public_url` is reachable
from the internet, keep it behind your reverse proxy's auth. The container's own port is a second way in:
once it is published on the LAN for the plan pusher, every host on the LAN can read `/house/state`
and send `/house/command` with no login at all, bypassing the proxy's auth, and the upload secret
travels over plain HTTP there. If that matters, bind the published port to one interface or
firewall it so that only the Mac can reach it.

**Who can embed the 3D page.** `/house/` is sent with a `Content-Security-Policy: frame-ancestors`
header, so other sites cannot frame its controls (clickjacking). With a path `public_url` such as
`/ha-widget`, Glance and the iframe share one origin through the proxy, and the header is
`frame-ancestors 'self'`. With an origin `public_url` such as `http://192.168.1.50:8081`, Glance
(say `:3000`) is a different origin from the container's port, so the header also allows any port
on that host: `frame-ancestors 'self' http://192.168.1.50:* https://192.168.1.50:*`. If Glance runs
on another host name than the one in `public_url`, or `public_url` names an IPv6 address, set
`HOUSE_FRAME_ANCESTORS` to the CSP source list to use instead, e.g. `'self' http://glance.lan:3000`.
Otherwise the 3D view stays blank. A header your reverse proxy adds (`X-Frame-Options: DENY`,
`frame-ancestors 'none'`) blocks it the same way; `SAMEORIGIN` is fine with a path `public_url`.

**Updating the vendored build.** `internal/house/dist/` is House Control's `web/dist`, built at a
pinned commit and embedded into the binary. Its `VERSION` file records which commit, and
House Control's MIT `LICENSE` sits next to it. `THIRD_PARTY_LICENSES` carries the licences of the
libraries bundled into it (three.js, MIT; Lucide, ISC, with the Feather MIT notice it includes),
because the minified build drops their `@license` banners. To move to another commit, check out the House
Control source next to this repo (or point `HOUSE_CONTROL_SRC` at it) and run:

```bash
tools/house-dist/update.sh <commit>
go test ./...
```

The script exports that commit's `web/` with `git archive` into a temporary directory (your
checkout's working tree and `node_modules` are not used or changed), runs `npm ci` and
`npm run build` there, and replaces `internal/house/dist/` with the result minus the demo
`plans/`. `THIRD_PARTY_LICENSES` is regenerated from the `LICENSE` file of every non-dev package in
the commit's `web/package-lock.json`. The new build is copied next to the old one first and then
swapped in by renaming, so a failed or interrupted run leaves the previous build in place. It
refuses a build whose `index.html` loads anything other than `./assets/…`, and a runtime package
without a `LICENSE` file. That
needs git and Node; the Docker build does not.

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
- `TEMPERATURE_CHART_STYLE` — `bars` (default, a direct port of the built-in WEATHER widget's
  bar chart: same bar geometry, same daylight band, same hover behaviour — the current bucket's
  temperature is shown at rest, and hovering any other column reveals its temperature and time)
  or `sparkline` (matches the SERVER STATS widget style).
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
| `TEMPERATURE_CHART_STYLE` | `temperature.chart_style` | `bars` | `bars` (WEATHER-widget bar chart) or `sparkline` |
| `DEVICES_DOMAINS` | `devices.domains` | `fan,climate,humidifier,water_heater,media_player,vacuum,cover,lock` | HA domains that get a device tile (floorplan). Add `switch` to opt in. |
| `DEVICES_EXCLUDE` | `devices.exclude` | — | entity_ids to skip even if their domain is listed |
| `DEVICES_MAP_CAMERAS` | `devices.map_cameras` | — (auto) | `vacuum.x=camera.y,vacuum.z=image.w` — which entity a vacuum's popover map comes from |
| `LAYOUT` | `layout` | `cards` | `cards` or `floorplan` (see "Floorplan layout") |
| `LAYOUT_FILE` | `layout_file` | `/data/floorplan.json` | where the browser editor saves the floorplan |
| `FLOORPLAN_GRID` | `floorplan.grid` | — | rows joined with `;`, e.g. `bedroom bedroom kitchen;bath hall kitchen` |
| `FLOORPLAN_ROOMS` | `floorplan.rooms` | — | `key=Area Name,key2=Area 2` |
| `FLOORPLAN_ASPECT_RATIO` | `floorplan.aspect_ratio` | columns/rows | CSS aspect-ratio of the whole map, e.g. `4/3` or `1.15` |
| `FLOORPLAN_MAX_WIDTH` | `floorplan.max_width` | `0` (fill) | cap on the map's width in px — with aspect_ratio this is its overall size |
| `LIVE_POLL_INTERVAL` | `live.poll_interval` | `10s` | How often the browser polls `/live.json` while the tab is open |
| `LIVE_PAUSE_WHEN_HIDDEN` | `live.pause_when_hidden` | `true` | Pause polling while the browser tab is backgrounded |
| `SENSORS_CONTACT_DEVICE_CLASSES` | `sensors.contact_device_classes` | `door,window,garage_door,opening` | Comma-separated `binary_sensor` device classes shown as Open/Closed |
| `SENSORS_MOTION_DEVICE_CLASSES` | `sensors.motion_device_classes` | `motion,occupancy` | Comma-separated `binary_sensor` device classes shown as Motion/Clear |
| `HOUSE_PLAN_FILE` | `house.plan_file` | `/data/house-plan.json` | Where the uploaded House Control plan is kept (see "3D home") |
| `HOUSE_PLAN_SECRET` | `house.secret` | `""` (uploads off) | Bearer secret for `PUT /house/plan`; empty disables uploads |
| `HOUSE_LANG` | `house.lang` | `en` | Language of the Plan/3D switch, the Dock card and the 3D view: `en` or `ru` |
| `HOUSE_FRAME_ANCESTORS` | `house.frame_ancestors` | `""` (from `public_url`) | CSP `frame-ancestors` sources that may embed the 3D page, replacing the derived list (see "Who can embed the 3D page") |

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

Scenes/scripts/automations, humidity, multiple Home Assistant
instances, history beyond HA's recorder retention, and pagination (aimed at homes small enough to
fit on one screen).

## Development

```bash
go test ./...
docker build -t glance-homeassistant:dev .
```

`GLANCE_HA_DEVSERVER=:8099 go test -run TestDevServer -timeout 0 .` runs the server against a fake
Home Assistant with a demo home and a stored House Control plan (no real Home Assistant involved);
open `http://localhost:8099/harness` (`?theme=light` for the light palette) to see the widget
mounted the way Glance mounts it. Set `GLANCE_HA_DEVSERVER_CSS` to a directory holding Glance's
stylesheets for a closer look, and `GLANCE_HA_DEVSERVER_LANG=en` for English. Some tests run
`node` when it is installed and skip otherwise.
