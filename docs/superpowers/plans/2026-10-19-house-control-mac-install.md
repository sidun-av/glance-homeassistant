# House Control Mac install — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** From the dashboard, a household member installs the House Control Mac app with one Terminal line; it arrives configured (plan, Russian UI, their own Home Assistant key) and keeps the plan in sync.

**Architecture:** Two repos. The House Control fork (`~/GIT/house-control`, new branch `local/household`) gains: a `lang` setting for the native app, a localised island, live reload of `plan.json`, and a per-vacuum `battery` sensor hint in plan links (so a non-admin token needs no `/api/template`). The widget server (`~/GIT/glance-homeassistant`, `main`) gains: one-time install codes, an install script endpoint, a key endpoint that mints a per-person Home Assistant user + long-lived token over HA's websocket API, battery hints in `GET /house/plan`, and the reworked invite card in the widget.

**Tech Stack:** Swift (HouseCore, macOS AppKit shell), Vite/Three.js web UI with node tests; Go 1.x widget server with `net/http`, `embed`, and `github.com/coder/websocket` (new dependency, HA websocket); POSIX `sh` install script.

**Spec:** `docs/superpowers/specs/2026-10-19-house-control-mac-install-design.md` (this repo).

## Global Constraints

- glance-homeassistant and the House Control fork are PUBLIC: no person names, IPs, entity ids, tokens or plan contents in code, tests, docs or commit messages. Use `example`/`demo` values.
- One-time code: valid **30 minutes**, **single use** (consumed when a key is minted), memory only.
- Keys are minted only for a request from the home network: TCP peer private AND (no proxy headers OR `X-Real-IP` private).
- HA user per person: name `"<Mac account full name> (Mac)"`, `local_only: true`, group `system-users`, never admin; token `client_name` = `"House Control — <computer name>"`, lifespan 3650 days.
- Household Mac files live in `~/Library/Application Support/HouseControl/`: `plan.json`, `settings.json` (`{"plan":"custom","lang":"ru"}` merged into what is there), `homeassistant.json` (`{"url","token","name","version"}`, mode 600).
- Script output is Russian, one plain line per step; every failure says what to do next and exits non-zero.
- The app download is verified by SHA-256 against the server's `HOUSE_APP_SHA256`; mismatch aborts before anything is touched.
- The plan puller never writes anything that is not a JSON object with `rooms` (an Authentik login page away from home must not replace the plan).
- Swift tests cannot run here (Command Line Tools, no XCTest); Swift work is verified by `swift build` in `core/` and a live run. Web: `npm test` and `npx vite build` in `web/`. Go: `go test ./...`, `go vet ./...`.
- Commit trailers: `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` and `Claude-Session: https://claude.ai/code/session_01SqHA6DZ5oWGicx6zw5ZnXH`.

## Review Focus

1. The same command run twice (code already used, or expired) → the script stops with «Ссылка устарела — откройте дашборд и скопируйте команду заново» (the script names the dashboard's host), installs nothing half-way. (Task 8 test `TestInstall_CodeSingleUse`, Task 9 script test.)
2. A Mac account without write access to `/Applications` → app goes to `~/Applications`, everything else identical. (Task 9 script test with `HC_APPLICATIONS_DIR` unwritable.)
3. House Control already running during install/update → quit, replace, relaunch; never two copies. (Task 9.)
4. Key request arriving through the proxy from the internet (`X-Real-IP` public) → 403, code NOT consumed. (Task 8 `TestKey_RefusesPublicClient`.)
5. Plan pull away from home returns HTML/302/401 → `plan.json` untouched. (Task 9 puller test.)

---

## Part A — House Control fork (`~/GIT/house-control`)

### Task 1: Branch `local/household`

**Files:** merge only.

- [ ] Step 1: `git switch -c local/household fix/climate-idle && git merge upstream/main` (brings Open at Login and upstream fixes). Resolve conflicts keeping both sides' intent: the fork's i18n `t()` calls stay; upstream's new behaviour is kept.
- [ ] Step 2: `cd core && swift build` → `Build complete!`; `cd ../web && npm test && npx vite build` → 0 failures, built.
- [ ] Step 3: Commit the merge.

### Task 2: Native language from `settings.json`

**Files:** Modify `core/Sources/HouseCore/PlanStore.swift`; Test `core/Tests/HouseCoreTests/` (PlanStore tests file that exists, else new `PlanStoreLangTests.swift`).

**Interfaces:** Produces `PlanStore.lang: String?` (from `settings.json` key `"lang"`, only `"en"`/`"ru"`, else nil) and `bootPayload()["lang"]` when non-nil. The web's `pickLang` already prefers `boot.lang`.

- [ ] Step 1: XCTest: settings `{"plan":"home","lang":"ru"}` → `bootPayload()["lang"] as? String == "ru"`; `{"lang":"xx"}` → key absent; `select("home")` keeps `lang` in the rewritten settings.json.
- [ ] Step 2: Implement; `select()` must preserve unknown settings keys (read-modify-write, not overwrite).
- [ ] Step 3: `swift build` clean; commit.

### Task 3: Island in the reader's language

**Files:** Modify `web/src/island/island.js`, `web/src/i18n/en.js`, `web/src/i18n/ru.js`; Test `web/test/literals.test.js` (extend its file list to `island/island.js`).

- [ ] Step 1: Extend the literals test to cover `web/src/island/island.js`; run `npm test` → FAIL listing island literals ("Off", "inside", "Idle", "lights on", aria labels…).
- [ ] Step 2: Replace each with `t('island.<key>')` / `num()` (temperatures with `num(x, 0)`), add keys to en and ru (ru: «Выкл.», «Ожидание», «{n}° в комнате», plural «{n} из {total} ламп горит» via the table's plural rules, etc.). English output must stay byte-identical to today.
- [ ] Step 3: `npm test` → PASS; `npx vite build`; commit.

### Task 4: Reload the home when `plan.json` changes on disk

**Files:** Modify `core/Sources/HouseCore/PlanStore.swift` (watcher), `macos/Sources/HouseControl/AppDelegate.swift` (react).

**Interfaces:** Produces `PlanStore.onCustomPlanChange: (() -> Void)?`, fired on the main actor when `customPlanRevision` changes (watch the directory with a `DispatchSource` file-system object source, because an atomic replace swaps the inode; debounce 300 ms; compare `PlanStore.revision(of:)` so the app's own saves do not loop).

- [ ] Step 1: Implement the watcher; AppDelegate handles it exactly as after "Load Floor Plan…" (hub links via `hub.setLinks(from:)`, web views get the new boot/plan) — only when `activePlan == "custom"`.
- [ ] Step 2: `swift build`; live check: run the built app with `HOUSE_DATA_DIR=<tmp>` holding a custom plan, rewrite `plan.json` with a renamed room via `mv` of a temp file, and confirm (HOUSE_EVAL_DIR snapshot or log) the name changed without restart.
- [ ] Step 3: Commit.

### Task 5: Vacuum battery hint in the plan link

**Files:** Modify `core/Sources/HouseCore/DeviceService.swift` (protocol), `DeviceHub.swift`, `HomeAssistantService.swift`; Test `ServiceTests.swift`.

**Interfaces:** `DeviceService.hintBatteries(_ sensors: [String: String])` (vacuum entity → battery sensor entity) with a no-op default in a protocol extension. `DeviceHub.setLinks` reads `link["battery"]` (string, non-empty) for vacuums and calls it per service in `watchLinks()`. `HomeAssistantService` seeds `batterySensors[id]` from the hints (so `findBatteries` / `/api/template` is skipped for them) and adds them to `watched`.

- [ ] Step 1: XCTest: with hint `vacuum.demo → sensor.demo_battery`, `read()` against a stub transport returns battery from that sensor and issues no `/api/template` request.
- [ ] Step 2: Implement; `swift build`; commit.

### Task 6 (controller): Build and publish the app

- [ ] Build: `SKIP_WIDGETS=1 APPEX=<HouseWidgets.appex from the installed app> APP_PATH=<scratch>/House Control.app scripts/build-app.sh`; zip with `ditto -c -k --keepParent`; `shasum -a 256`.
- [ ] `git push -u origin local/household`; `gh release create household-2026.10.19 -R sidun-av/house-control --target local/household --prerelease` with the zip and its `.sha256`. Notes say what branch and what it is for, no personal data.
- [ ] Install the same build on the owner's Mac (backup first) and confirm it starts.

---

## Part B — Widget server (`~/GIT/glance-homeassistant`)

### Task 7: Home Assistant person keys over websocket

**Files:** Create `internal/hass/person.go`, `internal/hass/person_test.go`; Modify `go.mod`/`go.sum` (`github.com/coder/websocket`).

**Interfaces:** Produces
```go
// MintPersonKey returns a new long-lived token for the household user named person+" (Mac)",
// creating that user (local-only, system-users, not admin) on first use.
func (c *Client) MintPersonKey(ctx context.Context, person, computer string) (token string, err error)
```
Flow (all as the admin token unless noted): ws `config/auth/list` → find by name; else `config/auth/create {name, group_ids:["system-users"], local_only:true}` and `config/auth_provider/homeassistant/create {user_id, username, password}`; existing user → `config/auth_provider/homeassistant/admin_change_password {user_id, password}`. Username `hc-` + first 10 hex of sha256(person). Password: 32 random bytes, base64url, never stored. Then REST login as that user: `POST /auth/login_flow {client_id, handler:["homeassistant",null], redirect_uri}` → `POST /auth/login_flow/<flow_id> {client_id, username, password}` → `POST /auth/token grant_type=authorization_code` → access token → ws auth with it → `auth/long_lived_access_token {client_name:"House Control — "+computer, lifespan:3650}`. `client_id`/`redirect_uri` = the HA base URL + `/` (same host, as HA requires).

- [ ] Step 1: Tests against an `httptest` fake HA (REST + ws): `TestMintPersonKey_CreatesUser` (asserts create payload: `local_only` true, group `system-users`, no admin, name `"Demo (Mac)"`), `TestMintPersonKey_ReusesUser` (no create, one password change), `TestMintPersonKey_TokenName` (`"House Control — demo-mac"`, 3650), `TestMintPersonKey_HAErrorNamesStep` (a ws error is returned with the step name, never with the token or password in it).
- [ ] Step 2: Implement; `go test ./internal/hass/` PASS; `go vet`; commit.

### Task 8: Install codes, script and key endpoints

**Files:** Create `internal/house/install.go`, `internal/house/install_test.go`, `internal/house/install.sh.tmpl` (embedded); Modify `handler.go` (routes, Handler fields), `config.go` (+ `config_test.go`), `main.go` (wiring), `config.example.yml`, `README.md`.

**Interfaces:** Handler gains `Persons PersonKeys` (interface `MintPersonKey(ctx, person, computer string) (string, error)`), `HAURL string` (what the Mac uses), `AppURL, AppSHA256 string`. Config: `house.app_url` / `HOUSE_APP_URL`, `house.app_sha256` / `HOUSE_APP_SHA256` (64 hex, validated), `house.ha_url_for_macs` / `HOUSE_HA_URL` (default: the hass url). Install disabled (404) unless app URL+hash and plan exist.

Routes:
- `POST /house/install/code` → `{"code","command"}`; code = 16 random bytes base32 lowercase; command = `curl -fsSL '<public base>/house/install/<code>' | sh`. Rate: 10/min process-wide.
- `GET /house/install/<code>` → `text/plain` script rendered from the template with base URL, code, app URL, hash; 404 for unknown/expired (script fetch does not consume).
- `POST /house/install/<code>/key` `{"person","computer"}` (each 1–80 runes, trimmed) → LAN check → `{"homeassistant":{"url","token","name":"Home Assistant"}}`, then the code is consumed. Unknown/used/expired → 410 `{"error":"expired"}`.
- `GET /house/plan` vacuum links gain `"battery":"<sensor>"` when the server knows it; ETag covers the hints.

- [ ] Step 1: Tests: `TestInstall_CodeExpires` (clock +30 min → 404/410), `TestInstall_CodeSingleUse`, `TestKey_RefusesPublicClient` (peer private, `X-Real-IP: 203.0.113.9` → 403, code still valid), `TestKey_AllowsLANThroughProxy` (`X-Real-IP: 192.168.0.20`), `TestInstall_ScriptHasNoSecrets` (rendered script contains code, URL, hash; no token), `TestInstall_DisabledWithoutApp` (404), `TestPlan_BatteryHint` (link gains battery; ETag differs from plain plan's).
- [ ] Step 2: Implement; `go test ./...` PASS; commit.

### Task 9: The install script and plan puller

**Files:** `internal/house/install.sh.tmpl` (from Task 8, filled here), `internal/house/install_script_test.go`.

Behaviour (POSIX sh, `set -eu`, Russian lines):
1. Check macOS (`uname`), arm64/x86_64 fine; `uninstall` argument → remove app, LaunchAgent `local.house-control.plan-pull`, the puller script; keep nothing with the token (delete `homeassistant.json`); done.
2. Download zip to `mktemp -d`; `shasum -a 256` must equal the hash, else stop.
3. Ask the server for the key (`curl -fsS -X POST …/key` with `id -F` and `scutil --get ComputerName`); 410 → the "ссылка устарела" line; 403 → «Установка работает только из дома (Wi-Fi дома)».
4. `osascript -e 'quit app "House Control"'`, wait ≤10 s for exit; install to `${HC_APPLICATIONS_DIR:-/Applications}`, fallback `~/Applications` when not writable.
5. Write `homeassistant.json` (umask 077), merge settings.json (`plutil`-free: python3 not guaranteed → use `/usr/bin/osascript -l JavaScript` or plain sed on the known small file; builder's call, must keep unknown keys), fetch plan via the puller once.
6. Puller `~/Library/Application Support/HouseControl/plan-pull.sh` + LaunchAgent (`RunAtLoad`, `StartInterval` 600): `curl` with `If-None-Match` from a stored ETag; write to a temp file, accept only a JSON object containing `"rooms"` (check with `osascript -l JavaScript` JSON.parse), then `mv` over `plan.json`.
7. Second LaunchAgent `local.house-control.open` (`RunAtLoad` only: `open -g -a` the installed app) so it starts at login; `open -a` the app now; final line «Готово: House Control в строке меню. Наведите на чёлку или нажмите на домик.»

- [ ] Step 1: Test runs the rendered script under `sh` with `HOME=<tmp>`, `HC_APPLICATIONS_DIR=<tmp>/Apps`, stubbed `curl`, `osascript`, `open`, `id`, `scutil`, `launchctl` on `PATH` (shell stubs writing to a log) against an `httptest` server: happy path writes the three files (homeassistant.json mode 600), installs the app, loads the agent; hash mismatch leaves `HOME` untouched; 410 path prints the expired line; puller given HTML keeps the old plan; unwritable apps dir → `~/Applications`. Skip the test when not on darwin.
- [ ] Step 2: Implement; `shellcheck` if available; tests PASS; commit.

### Task 10: The invite card in the widget

**Files:** Modify `internal/render/house.go`, `internal/render/house_test.go`.

- Show on a Mac in any browser (`Macintosh` UA, `maxTouchPoints === 0`), not when dismissed, not before `remindAt`, not after `started`. localStorage keys: `ha-house-invite` = `dismissed` | `started`, `ha-house-invite-remind` = epoch ms.
- Ask card: icon, title «House Control на Mac», body «3D-дом в строке меню и в «чёлке», уже настроенный на ваш дом.»; buttons «Установить» (primary), «Напомнить завтра» (sets remind = now + 24 h), close ✕ (aria «Не показывать», sets dismissed).
- Steps card: a small drawn Terminal window picture (SVG, like today's Safari menu picture) with the command line highlighted; text: «1. Откройте Терминал (⌘ Пробел → «Терминал»). 2. Нажмите «Скопировать команду». 3. Вставьте её в Терминал (⌘V) и нажмите Enter.» Button «Скопировать команду» → `POST code` → `navigator.clipboard.writeText(command)` → button text «Скопировано ✓», sets `started`; failure → «Не получилось — обновите страницу». Secondary «Готово» closes.
- Remove the Dock copy and `haHouseShouldInvite`'s Safari-only rule; English strings mirror the Russian.

- [ ] Step 1: Tests: rendered HTML has the three ask buttons and no "Dock"; JS predicate `haHouseShouldInvite(ua, touch, state, remindAt, now)` cases (Chrome on Mac → true; iPad UA with touch → false; dismissed → false; remind in future → false, past → true; started → false), run via the existing JS-in-Go test approach in `house_test.go`.
- [ ] Step 2: Implement; `go test ./...`; commit.

### Task 11 (controller): Deploy and live check

- [ ] Push `main`, wait for CI, set `HOUSE_APP_URL`, `HOUSE_APP_SHA256` in the glance stack compose (backup first), pull + up.
- [ ] Live: get a code from the real page, run the command on the owner's Mac with `HOME` pointed at a scratch dir and `HC_APPLICATIONS_DIR` scratch (so the owner's own app and config stay untouched), check a new HA user "<name> (Mac)" + token exist and the token reads `/api/states` but gets 401/403 on `/api/template`; then delete that test user (owner-authorised test artefact).
- [ ] Update homelab-infra docs and memory.
