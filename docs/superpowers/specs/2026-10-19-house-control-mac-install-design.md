# Install House Control on a household Mac from the dashboard — design

Date: 2026-10-19. Status: decisions from the owner (Terminal install, one Home Assistant key per
person); the rest are the builder's calls, marked as such.

## Intent and why

The dashboard's 3D view (2026-10-18 design) runs House Control's web UI inside the widget. The owner
wants more for the household: the **real Mac app** — the menu-bar app with the "island" under the
notch and the 3D home — installed on another household member's Mac **with the home already set up**,
so it works exactly like on the owner's Mac. The current "Add to Dock" invite is replaced by this.

The main person is a non-technical household member. The bar stays: obvious the first time,
pleasant, nothing to configure.

## Goals

1. **Invite on the dashboard.** On a Mac (any browser, not phones/tablets), the widget shows a small
   card offering House Control. Buttons: **«Установить»**, **«Напомнить завтра»**, and a close ✕
   (= «Не показывать», for good in that browser). Gone for good after «Готово» following a copied command; a copy left without «Готово» hides it for a day (then shown again, so an unfinished install can copy a fresh command).
2. **Install = one Terminal line.** «Установить» opens a short picture guide: open Terminal, press
   «Скопировать команду», paste, Enter. The command downloads nothing quarantined, so macOS shows no
   Gatekeeper warning (the app is ad-hoc signed; there is no Developer ID). The script:
   - installs House Control into /Applications (replacing an older copy; quits and relaunches it);
   - writes the home: `plan.json`, `settings.json` (custom plan, Russian UI), `homeassistant.json`
     with the person's own key;
   - installs a small background job that keeps the plan up to date;
   - starts the app (and leaves Open at Login on).
   Re-running the same command later updates everything; an `uninstall` variant removes it all.
3. **One Home Assistant key per person.** The install creates (or reuses) a non-admin Home Assistant
   user for that person, named after the Mac account's full name plus "(Mac)", and a long-lived token
   for it named after the Mac. Each key can be revoked on its own; Home Assistant's logbook shows who
   switched what. The owner's token never leaves the widget server.
4. **The plan follows the owner's Mac.** The dashboard already receives the owner's plan; the
   household Mac pulls it from the widget server (on login and every 10 minutes, by ETag) and the app
   picks up a changed `plan.json` without being restarted.
5. **Russian.** The native panel and the island speak Russian on these Macs (the island is not
   localised today).

## Non-goals

- Working away from home (Home Assistant is reached on the home network, as on the owner's Mac).
- Editing the plan on a household Mac: edits there are overwritten by the next plan from the owner.
- iPhone, Smart Life/Hue/HomeKit on the household Mac.
- A notarised, double-click installer (needs a paid Apple Developer ID; the owner chose Terminal).

## Anti-goals

- A key or any home detail reachable from the internet, or served without a fresh one-time code.
- The owner's admin token on another machine.
- Anything to configure by hand after the command, or any English left in the household UI.
- The installer silently doing nothing: every step prints a plain Russian line, failures say what
  to do.

## Constraints and the how

- **One-time code.** «Скопировать команду» asks the widget server for a code (valid 30 min, single
  use, tied to nothing secret). The command is
  `curl -fsSL <dashboard>/ha-widget/house/install/<code> | sh`. The page is behind the dashboard's
  sign-on (or the home network), so only someone who can open the dashboard can get a code.
  Codes are kept in memory only; a restart invalidates them.
- **Home network only for keys.** The script reaches the widget server through the dashboard URL;
  from outside home that path asks for sign-on and `curl` fails with a clear message ("установка
  работает только дома"). The server additionally refuses to mint a key when the request did not
  come through the home network (builder: check the reverse proxy's client-IP header against the
  private ranges, same rule as the dashboard's LAN bypass).
- **Creating the person's HA user** uses the widget server's admin token over the websocket API
  (`config/auth/create`, `config/auth_provider/homeassistant/create` with a random password that is
  thrown away, then a login flow as that user and `auth/long_lived_access_token`). The same Mac
  account name reuses the existing user and adds a token. Builder's call: user is local-only,
  group `system-users`.
- **Vacuum battery without admin.** A non-admin token cannot call `/api/template`, which House
  Control uses to find a vacuum's battery sensor. The plan link gains an optional `battery` entity
  id that the app reads straight from `/api/states`; the owner's plan gets it filled in. Upstream-
  friendly change in the fork.
- **App binary.** Built from the owner's fork (combined branch with vacuum, airflow, idle, i18n) and
  published as a release asset on the fork; the widget server is configured with its URL and SHA-256
  and the script verifies the hash. No binaries in the widget repo.
- **Plan reload in the app.** The Mac app watches `plan.json` and reloads the home when it changes
  on disk (upstream-friendly; also useful for the owner's own edits from other tools).
- **Native language.** The Mac app reads `lang` from `settings.json` into the web boot config; the
  island gets the same i18n table as the panel.
- The widget repo is public: no names, IPs, entity ids or tokens in code, tests or docs.

## Left to the builder

- Exact look of the invite and the guide (match the widget; a screenshot of Terminal with the
  paste).
- Script structure, logging, and the LaunchAgent label/paths on the household Mac.
- How the server is configured with the release URL/hash (env, like the other HOUSE_* settings).
