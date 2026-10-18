# 3D home view in the widget (House Control inside Glance) — design

Date: 2026-10-18. Status: approved in conversation (variant A, Dock invite, Russian, 30-day SSO);
remaining details settled by the builder as recorded under "Decided".

## Intent and why

The widget already shows the home as a flat, schematic floor plan. The owner also runs
[House Control](https://github.com/0xProbabillity/house-control) on a Mac: a live 3D model of the
same flat with lights, heaters/ACs and (from the owner's fork) a robot vacuum, all controllable.
The household should get that 3D home **on the dashboard itself**, so nobody has to install anything.

The people are different. Two are technical (the owner and a friend helping build the smart home)
and are happy with tools. One is a **non-technical household member** who mostly wants to switch
lights and set room temperature from her Mac. For her the bar is: it is obvious the first time,
it is pleasant, and she *wants* to use it. Everything below serves that.

## Goals

1. The widget gets a **two-way switch in its header: «План | 3D»**.
   - «План» is today's widget, unchanged.
   - «3D» shows the House Control web UI (3D home + its tiles and sheets) inside the widget,
     controlling real devices: lights on/off and brightness, heater/AC on/off and target
     temperature, robot vacuum start/pause/dock/suction.
   - The choice is remembered per browser; a first visit shows **3D**.
2. The 3D view is **in Russian** (all UI copy, status words, units, dates).
3. **Zero setup for viewers.** Being signed in to the dashboard (it is behind the dashboard's
   single sign-on) is all it takes. No Home Assistant login, no token, nothing to install.
   The HA token stays on the widget server, as today.
4. **The plan follows the Mac.** When the owner (or the friend) edits the home in House Control on
   the owner's Mac, the 3D view on the dashboard picks it up automatically within a minute or so.
   No manual copying.
5. **"Add to Dock" invite.** On a Mac in Safari, a small friendly pop-up offers to add the dashboard
   to the Dock (macOS web app), so the home opens like an app. «Добавить в Dock» explains the one
   step with a picture (Safari has no API to do it programmatically); «Не сейчас» hides it for good
   in that browser. Not shown in other browsers or on phones, or once running as a Dock web app.

## Non-goals

- Distributing the House Control native app to household members (unsigned build → macOS
  Gatekeeper warning; explicitly replaced by the Dock invite).
- Editing the plan from the dashboard (editing stays in the Mac app).
- Per-person permissions: everyone who can sign in to the dashboard may control the home
  (today that is exactly the household plus the helping friend).
- Smart Life/Hue/HomeKit devices in the web view (only Home Assistant–linked devices; the owner's
  plan links everything through Home Assistant).

## Anti-goals (would make it a failure even if it works)

- Anything a non-technical person has to configure, approve or understand to get going.
- English or raw entity states (`heat`, `docked`, `cleaning`) leaking into the 3D view.
- A slow or janky dashboard: the 3D must not load or run while «План» is selected, and must not
  keep the GPU busy when nothing moves (House Control already sleeps its render loop).
- The HA token or any HA URL reaching the browser.
- A command that silently does nothing: failures must show (the House Control UI already rolls
  back optimistic changes on a failed command).

## Constraints

- Glance mounts extension HTML via `innerHTML` (no `<script>`); the widget already boots its JS via
  the `<img onerror>` technique and serves assets under the dashboard's `/ha-widget/` path through
  the reverse proxy. The 3D view will be an **iframe** to a page served by this widget server
  (isolation from Glance's CSS/JS, its own WebGL context, easy to unload when switching to «План»).
- House Control's web UI is a Vite/Three.js app that today talks to a native host through a bridge
  (`window.house.applyRemote`, `call('command', …)`) and is fully simulated in a plain browser.
  It needs a **browser host mode**: a small adapter that, when served by this widget, reads state
  from and sends commands to an HTTP API on the same origin instead of a native host. This, plus a
  Russian localisation layer, is done in the owner's House Control fork in an upstream-friendly way
  (generic "remote host" + i18n), and the built `dist/` is vendored into / fetched by this widget.
- Device mapping (HA `light`/`switch`/`climate`/`vacuum` ↔ House Control state and commands) mirrors
  House Control's own Swift `HomeAssistantService` (including the vacuum battery from the device's
  battery sensor). Implemented in Go on the widget server, reusing its HA client.
- Plan transport from the Mac: the owner's Mac pushes `plan.json` to the widget server when it
  changes. The upload endpoint must not be reachable from the internet without a secret, and the
  plan contains entity ids only (no tokens). How the Mac side runs (e.g. a launchd agent watching
  the file) is the builder's call; it must survive reboots and need no attention.
- The widget repo is public: no owner names, IPs, entity ids or plan contents in code, tests or docs.

## Decided

- Variant **A** (switch inside the widget), chosen by the owner over a separate page/tab.
- First visit defaults to 3D; choice remembered in `localStorage`.
- Dock invite instead of native-app installation.
- Russian UI for the 3D view.
- Access = whoever can sign in to the dashboard.
- The SSO session now lasts 30 days (owner's decision), so a Dock web app does not show a login
  page every time it is reopened. The Dock app opens the whole dashboard (the widget remembers 3D).
- Plan transport: the owner's Mac is a desktop that never leaves home, so the push goes over the
  home network only, straight to the widget server — no proxy rule, no SSO bypass, no internet
  exposure. A shared secret still guards the upload endpoint. When the server is unreachable the
  Mac side simply retries on the next change/interval.
- Narrow screens (phones) default to «План»; 3D stays one tap away but gets no phone-specific work.
- House Control changes (browser host mode, Russian i18n) are built generic and upstream-friendly
  on top of the fork's combined branch (which already carries the vacuum work); proposing them
  upstream as separate PRs is a later, optional step.

## Left to the builder

- Exact HTTP API shape between the iframe and the widget server (polling interval vs SSE; the
  widget already polls `/live.json` every ~10 s — similar cadence is fine, faster right after a
  command).
- How the House Control build is pinned/vendored and kept updatable.
- iframe height/aspect inside the widget column and how the House Control panel layout adapts to it.
- Visual design of the switch and the Dock invite (should match the widget's existing look).
- Whether the Russian strings live in the fork as a generic i18n table (preferred, upstreamable)
  or as a widget-side override.
