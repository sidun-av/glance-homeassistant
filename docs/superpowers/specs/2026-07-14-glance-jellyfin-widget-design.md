# glance-jellyfin: Recently Added Library Widget

**Goal:** A new Glance extension widget that shows the most recently added movies/TV shows in a Jellyfin library as a grid of poster cards, replacing the built-in "Next Up", "TV/Movie Releases", and "Latest Movies/Latest Shows" widgets. The existing qBittorrent widget is untouched.

**Architecture:** A new standalone Go HTTP service, `glance-jellyfin`, built the same way as this user's existing `glance-homeassistant` and `glance-grafana-sparkline` widgets: one `/widget` endpoint Glance polls on its own schedule, no client-side live-update mechanism (a media library doesn't change on a 10-second cadence — Glance's own `cache: Xm` is sufficient). It calls Jellyfin's own "Latest Items" API, and proxies poster images through itself so the Jellyfin API key never reaches the browser.

**Tech Stack:** Go stdlib `net/http` only (no new dependencies beyond `gopkg.in/yaml.v3` for config, matching `glance-homeassistant`). Same inline-HTML-response widget pattern, same env-var-driven config with `config.yml` fallback.

## Global Constraints

- New, separate GitHub repo: `sidun-av/glance-jellyfin`. Not added to the existing `glance-homeassistant` repo — it's a distinct widget with no shared code, matching how `glance-grafana-sparkline` is already its own repo/image in this user's stack.
- No live-update JS / `/live.json` endpoint. Confirmed: recently-added media doesn't need sub-minute freshness; a periodic `cache:` refresh in `glance.yml` is the only freshness mechanism.
- Poster images are always proxied server-side through this service's own `/image/{itemId}` endpoint — the Jellyfin API key must never be embedded in HTML or an `<img src>` sent to the browser (same reasoning `glance-homeassistant` already applies to `HA_TOKEN`).
- Card content is poster + title only — no year, rating, or type badge (explicit choice: "ничего лишнего").
- Movies and TV shows render together in one sorted-by-date-added grid, not separate tabs.
- Default item count is 12, but configurable (matches this project's existing convention of making every such number a `config.yml`/env-var override, e.g. `TEMPERATURE_MAX_POINTS`).
- Replacing the old widgets in the user's `glance.yml` is a manual deployment step, out of scope for this repo's code — handled separately once the service is built and deployed.

---

## 1. Data source: Jellyfin's "Latest Items" API

Jellyfin has a purpose-built endpoint for exactly this: `GET /Users/{UserId}/Items/Latest`. It returns the most recently added items for a given user, already sorted newest-first, and supports:

- `IncludeItemTypes=Movie,Series` — restricts to movies and TV series (excludes episodes, music, etc.)
- `Limit=N` — caps the result count
- `Fields=...` — controls which metadata fields are returned (we only need `Id`, `Name`, and the image tag, so this can stay minimal)

This avoids hand-building a sort/filter query against the generic `/Items` endpoint, and matches what Jellyfin's own web UI uses for its "Latest" rows.

The exact request the client builds:

```
GET {JELLYFIN_URL}/Users/{JELLYFIN_USER_ID}/Items/Latest?IncludeItemTypes=Movie,Series&Limit={LIMIT}&GroupItems=false&Fields=PrimaryImageAspectRatio
Headers: X-Emby-Token: {JELLYFIN_TOKEN}
```

`GroupItems=false` is required for the "one combined grid" decision (clarifying question 2) — Jellyfin's default groups results per-library/view, which would return the latest items *per library* rather than one flat, date-sorted list mixing movies and series together.

Response is a flat JSON array of items (not the `{Items: [...], TotalRecordCount}` envelope some other Jellyfin endpoints use); each item has at least `Id`, `Name`, `Type`, and `ImageTags.Primary` (present when the item has a poster — some malformed library entries may lack one, handled by skipping the card rather than rendering a broken image). `ImageTags` is part of Jellyfin's base item DTO and doesn't need to be requested via `Fields`; the exact `Fields` value (if any turns out to be needed at all) gets verified against a real Jellyfin server during implementation rather than assumed here.

`internal/jellyfin` (mirroring `internal/hass` in the sibling project) owns this HTTP call and the response shape, exposing a single `FetchLatest(ctx, limit int) ([]Item, error)` to `main.go`.

---

## 2. Image proxy

Jellyfin's poster endpoint is `GET /Items/{Id}/Images/Primary`. Fetching it needs the same `X-Emby-Token` header as the API calls — so the browser can never hit it directly without the key being visible in the page.

This service exposes `GET /image/{itemId}`, which:

1. Calls `{JELLYFIN_URL}/Items/{itemId}/Images/Primary` server-side with the token header.
2. Streams the response body straight through to the client (no decoding/re-encoding — just an `io.Copy` after copying the `Content-Type` header).
3. Sets `Cache-Control: public, max-age=86400` on the response — posters don't change, so the browser only fetches each one once a day, independent of how often Glance re-polls `/widget`.
4. On a Jellyfin-side error (404, unreachable), responds with its own 404 rather than proxying Jellyfin's error body — the `<img>` tag's `alt` text (the title) becomes the fallback the user sees.

---

## 3. Widget rendering

`/widget` handler:

1. Calls `FetchLatest(ctx, cfg.Limit)`.
2. On failure (Jellyfin unreachable), renders a single "Jellyfin unavailable" message — same pattern as `glance-homeassistant`'s `RenderUnavailable()`, not Glance's generic widget-failed state.
3. On success, renders a CSS grid: one card per item, each `<a href="{JELLYFIN_PUBLIC_URL}/web/#/details?id={Id}" target="_blank">` wrapping an `<img src="/image/{Id}" alt="{Name}">` and a title `<div>`. Items with no `ImageTags.Primary` are skipped entirely (no card), since there is nothing useful to link/proxy for them.
4. Grid uses CSS Grid with `grid-template-columns: repeat(auto-fill, minmax(100px, 1fr))` (poster-card sizing, portrait `aspect-ratio: 2/3` on each `<img>`) so it reflows to however wide Glance's column/group layout gives it, the same "let flexible CSS handle layout, not hand-tuned breakpoints" approach already used throughout `glance-homeassistant`.

No `data-*` attributes or bootstrap script are needed — there is no live state to patch in after the initial render.

A `GET /healthz` endpoint (plain `200 ok`) is included for parity with `glance-homeassistant`, independent of Jellyfin's own reachability.

---

## 4. Configuration

Same pattern as `glance-homeassistant`: every field settable via env var, with `config.yml` as a fallback, env taking priority.

| Env var | `config.yml` field | Default | Description |
|---|---|---|---|
| `JELLYFIN_URL` | `jellyfin.url` | — (required) | Jellyfin base URL, reachable from *this container* (e.g. `http://jellyfin:8096` over the shared docker network) |
| `JELLYFIN_TOKEN` | `jellyfin.token` | — (required) | Jellyfin API key (Admin Dashboard → API Keys) |
| `JELLYFIN_USER_ID` | `jellyfin.user_id` | — (required) | The Jellyfin user whose library access/view the "Latest Items" call uses |
| `JELLYFIN_PUBLIC_URL` | `jellyfin.public_url` | — (required) | Jellyfin's browser-facing base URL, used only to build each card's click-through link |
| `TITLE` | `title` | `Library` | Widget title shown in Glance |
| `LIMIT` | `limit` | `12` | Number of most-recently-added items to show |

`PORT` / `CONFIG_PATH` remain plain environment variables outside `config.yml`, exactly as in `glance-homeassistant`.

---

## 5. Error handling

- Jellyfin unreachable or returns a non-2xx to `/Items/Latest`: whole widget shows "Jellyfin unavailable" (matches `glance-homeassistant`'s `RenderUnavailable()` pattern).
- An individual item with no poster image tag: silently skipped (not rendered), not treated as an overall failure.
- `/image/{itemId}` failing for one poster (mid-session, after `/widget` already rendered successfully): browser shows the broken-image icon plus the `alt` title text; does not affect any other card.

---

## 6. Testing

TDD throughout, mirroring `glance-homeassistant`'s existing structure:

- `internal/jellyfin`: unit tests for `FetchLatest` against an `httptest` fake Jellyfin server — happy path, empty library, non-2xx response, malformed JSON.
- `internal/render`: unit tests for the grid-rendering function — correct card count, items without a poster skipped, HTML-escaping of titles, correct `href`/`src` construction.
- `main.go`: end-to-end handler test with a fake Jellyfin server, covering the unavailable-Jellyfin path and the successful-render path (mirrors `TestWidgetHandler_EndToEnd` / `TestWidgetHandler_HomeAssistantUnavailable` in `glance-homeassistant`).
- Image proxy handler: unit test verifying the token header is sent upstream, the response is streamed through with the right `Content-Type`/`Cache-Control`, and a Jellyfin-side error becomes a clean 404.

---

## 7. Repo scaffolding (deployment parity with `glance-homeassistant`)

For this to be deployable the same way (Komodo pulling a compose file from git, image on `ghcr.io`), the new repo needs the same supporting files, adapted 1:1 from `glance-homeassistant`:

- **`Dockerfile`** — identical multi-stage shape: `golang:1.23-alpine` build stage, `gcr.io/distroless/static-debian12:nonroot` runtime stage, `EXPOSE 8080`.
- **`.github/workflows/ci.yml`** — `test` job (`go test ./...`, `gofmt -l .`, `go vet ./...`) then a `docker` job gated on push to `main`, building and pushing `ghcr.io/sidun-av/glance-jellyfin:latest`.
- **`config.example.yml`** — hand-editable config file with every field and example values/comments.
- **`config.docker-default.yml`** — the near-empty file baked into the image as `/config.yml`, so the image works from env vars alone with nothing mounted (same role as `glance-homeassistant`'s).
- **`docker-compose.example.yml`** — one `glance-jellyfin` service, env vars for every config field, comment pointing at Komodo's Environment tab as the intended way to set them.
- **`README.md`** — same structure as `glance-homeassistant`'s: how it works, setup steps (create a Jellyfin API key, find your user ID, configure, run alongside Glance, add the extension widget block), env var reference table, error handling, out-of-scope, development.
- **`LICENSE`** — copy of the existing one.
- **`.gitignore`** / **`.dockerignore`** — copied as-is.
- **`go.mod`** — new module path `github.com/sidun-av/glance-jellyfin`, same Go version.

---

## Out of scope (explicitly deferred, not silently dropped)

- Browsing the full library / pagination — rejected in favor of a fixed "latest N" list (see clarifying question 1).
- Separate Movies/TV Shows tabs — rejected in favor of one combined grid (see clarifying question 2).
- Year, rating, or type badge on cards — rejected, poster + title only (see clarifying question 4).
- Live/real-time updates — a media library doesn't need sub-minute freshness; Glance's own `cache:` interval is sufficient.
- Editing the user's `glance.yml` to remove the old widgets and add this one — a manual deployment step handled once this service exists, not part of this repo.
