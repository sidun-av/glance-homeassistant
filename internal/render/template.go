package render

import (
	"fmt"
	"html"
	"strings"
)

type LightView struct {
	EntityID string
	IconSVG  string
	On       bool
}

type SensorBadgeView struct {
	Name      string
	Attention bool
	Label     string
}

type RoomCardView struct {
	Room           string
	SizeClass      string // "", "ha-size-md", "ha-size-lg"
	Lit            bool
	Occupied       bool
	HasTemperature bool
	TempNoData     bool
	TempValue      string
	ChartSVG       string
	Lights         []LightView
	Occupancy      []SensorBadgeView
	Contacts       []SensorBadgeView
}

type WidgetData struct {
	Rooms           []RoomCardView
	CardMinHeight   int
	LiveURL         string
	PollIntervalMS  int
	PauseWhenHidden bool
}

// styleBlock renders the widget's CSS. cardMinHeight is the base (small
// tier) room card's min-height in px, taken from temperature.chart_height
// — the "medium"/"large" tiers scale from it (+20, +130), matching the
// weight thresholds computed in main.go's sizeClassForWeight.
func styleBlock(cardMinHeight int) string {
	return fmt.Sprintf(`<style>
	.ha-body{display:flex;flex-direction:column;gap:16px}
	.ha-section-head{display:flex;align-items:center;gap:8px}
	.ha-section-label{font-size:.85em;letter-spacing:.08em;text-transform:uppercase;color:var(--color-text-subdue)}
	.ha-live-badge{display:inline-flex;align-items:center;gap:5px;font-size:.7em;letter-spacing:.06em;text-transform:uppercase;color:var(--color-primary)}
	.ha-live-dot{width:6px;height:6px;border-radius:50%%;background:var(--color-primary)}
	.ha-unavailable{color:var(--color-text-subdue);padding:12px 0}
	.ha-empty{color:var(--color-text-subdue);font-size:.85em;padding:8px 0}

	.ha-rooms{display:flex;flex-wrap:wrap;gap:10px;align-items:stretch}
	.ha-room{
	  flex:1 1 160px;min-height:%dpx;
	  background:var(--color-widget-background-highlight);
	  border:1px solid var(--color-widget-content-border);
	  border-radius:8px;padding:12px 14px 11px;
	  display:flex;flex-direction:column;gap:9px;
	  transition:background .2s,border-color .2s,box-shadow .2s;
	}
	.ha-room.ha-size-md{flex:2 1 320px;min-height:%dpx}
	.ha-room.ha-size-lg{flex:3 1 340px;min-height:%dpx}
	.ha-room[data-lit="true"]{background:rgba(240,196,121,.14);border-color:rgba(240,196,121,.35)}
	@keyframes ha-occ-glow{
	  0%%,100%%{box-shadow:0 0 0 1.5px var(--color-primary),0 0 10px -2px color-mix(in srgb,var(--color-primary) 45%%,transparent)}
	  50%%{box-shadow:0 0 0 1.5px var(--color-primary),0 0 20px 0 color-mix(in srgb,var(--color-primary) 80%%,transparent)}
	}
	.ha-room[data-occupied="true"]{animation:ha-occ-glow 2.6s ease-in-out infinite}

	.ha-room-head{flex:none;display:flex;align-items:baseline;justify-content:space-between;gap:8px}
	.ha-room-name{font-size:13.5px;font-weight:600;color:var(--color-text-highlight)}
	.ha-room-temp{font-size:13px;color:var(--color-text-highlight);font-variant-numeric:tabular-nums;white-space:nowrap}
	.ha-temp-nodata{color:var(--color-text-subdue);font-size:.85em;padding:2px 0}
	.ha-room-chart{flex:2 1 auto;width:100%%;display:block;min-height:30px}
	.ha-room-lights{flex:1 1 auto;display:flex;flex-wrap:wrap;align-content:center;align-items:center;gap:10px}
	.ha-room-lights svg{width:26px;height:26px;flex:none}
	.ha-room-status{flex:none;display:flex;flex-direction:column;gap:5px}

	.ha-occ-chip{
	  display:inline-flex;align-items:center;gap:6px;width:fit-content;
	  font-size:11px;letter-spacing:.03em;padding:3px 9px 3px 7px;border-radius:20px;
	  border:1px solid var(--color-text-subdue);color:var(--color-text-subdue);
	}
	.ha-occ-chip .ha-occ-dot{width:7px;height:7px;border-radius:50%%;background:var(--color-text-subdue)}
	.ha-occ-chip[data-occupied="true"]{
	  border-color:var(--color-primary);color:var(--color-primary);
	  background:color-mix(in srgb,var(--color-primary) 16%%,transparent);
	}
	.ha-occ-chip[data-occupied="true"] .ha-occ-dot{background:var(--color-primary)}

	.ha-badge{display:flex;align-items:center;gap:6px;font-size:11px;letter-spacing:.02em;color:var(--color-text-subdue)}
	.ha-badge svg{width:14px;height:14px;flex:none}
	.ha-badge[data-open="true"]{color:var(--color-negative)}

	.ha-light[data-on="true"] .bulb-glass{stroke:#f0c479;fill:rgba(240,196,121,.16)}
	.ha-light[data-on="false"] .bulb-glass{stroke:var(--color-text-subdue);fill:none}
	.ha-light[data-on="true"] .bulb-base{stroke:#f0c479}
	.ha-light[data-on="false"] .bulb-base{stroke:var(--color-text-subdue)}
	.ha-light[data-on="true"] .tl-head{stroke:#f0c479;fill:rgba(240,196,121,.16)}
	.ha-light[data-on="true"] .tl-rail{stroke:#f0c479}
	.ha-light[data-on="true"] .tl-ray{stroke:#f0c479;opacity:1}
	.ha-light[data-on="false"] .tl-head{stroke:var(--color-text-subdue);fill:none}
	.ha-light[data-on="false"] .tl-rail{stroke:var(--color-text-subdue)}
	.ha-light[data-on="false"] .tl-ray{opacity:0}
	.ha-light[data-on="true"] .ls-body{stroke:#f0c479;fill:rgba(240,196,121,.16)}
	.ha-light[data-on="true"] .ls-led{fill:#f0c479}
	.ha-light[data-on="false"] .ls-body{stroke:var(--color-text-subdue);fill:none}
	.ha-light[data-on="false"] .ls-led{fill:var(--color-text-subdue)}

	.ha-badge[data-open="true"] .ha-door-leaf{stroke:var(--color-negative);transform:rotate(-38deg);transform-origin:2px 12.5px}
	.ha-badge[data-open="true"] .ha-door-frame{stroke:var(--color-negative)}
	.ha-badge[data-open="false"] .ha-door-leaf{stroke:var(--color-text-subdue);transform:rotate(0deg)}
	.ha-badge[data-open="false"] .ha-door-frame{stroke:var(--color-text-subdue)}
	.ha-door-leaf{transition:transform .2s}
</style>`, cardMinHeight, cardMinHeight+20, cardMinHeight+130)
}

// bootstrapScript runs via an onerror attribute (see RenderWidget) because
// Glance mounts extension widget HTML with element.innerHTML, and <script>
// elements inserted that way are inert per the HTML spec — onerror/onload
// content attributes are not, so they're the standard way to run JS in
// HTML delivered through an innerHTML sink. Everything it touches (a
// light's on state, a room's lit/occupied state, a contact's open state)
// is a data-* attribute, matching the initial render exactly — it never
// needs to know a light's fixture type or reconstruct any markup.
const bootstrapScript = `(function(img){var root=img.closest('.ha-widget');if(!root)return;var url=root.dataset.liveUrl;var interval=parseInt(root.dataset.pollMs,10)||10000;var pauseWhenHidden=root.dataset.pauseHidden==='true';var timer=null;function applyState(data){(data.rooms||[]).forEach(function(room){var card=root.querySelector('.ha-room[data-room="'+CSS.escape(room.room)+'"]');if(!card)return;var anyLit=false;(room.lights||[]).forEach(function(l){var el=card.querySelector('.ha-light[data-entity-id="'+CSS.escape(l.entity_id)+'"]');if(!el)return;el.dataset.on=l.on;if(l.on)anyLit=true;});var anyOccupied=false;(room.occupancy||[]).forEach(function(o){var chip=card.querySelector('.ha-occ-chip[data-sensor-name="'+CSS.escape(o.name)+'"]');if(!chip)return;chip.dataset.occupied=o.attention;var label=chip.querySelector('.ha-occ-label');if(label)label.textContent=o.label;if(o.attention)anyOccupied=true;});(room.contacts||[]).forEach(function(c){var badge=card.querySelector('.ha-badge[data-sensor-name="'+CSS.escape(c.name)+'"]');if(!badge)return;badge.dataset.open=c.attention;var label=badge.querySelector('.ha-contact-label');if(label)label.textContent=c.label;});card.dataset.lit=anyLit;card.dataset.occupied=anyOccupied;});}function poll(){fetch(url,{cache:'no-store'}).then(function(r){return r.ok?r.json():null;}).then(function(data){if(data)applyState(data);}).catch(function(){});}function stop(){if(timer){clearInterval(timer);timer=null;}}function schedule(){stop();timer=setInterval(poll,interval);}if(pauseWhenHidden){document.addEventListener('visibilitychange',function(){if(document.hidden){stop();}else{poll();schedule();}});}if(!pauseWhenHidden||!document.hidden){poll();schedule();}})(this)`

func RenderWidget(data WidgetData) string {
	var b strings.Builder
	b.WriteString(styleBlock(data.CardMinHeight))

	pauseAttr := "false"
	if data.PauseWhenHidden {
		pauseAttr = "true"
	}
	fmt.Fprintf(&b, `<div class="ha-widget ha-body" data-live-url="%s" data-poll-ms="%d" data-pause-hidden="%s">`,
		html.EscapeString(data.LiveURL), data.PollIntervalMS, pauseAttr)

	b.WriteString(`<div class="ha-section-head"><span class="ha-section-label">Home</span><span class="ha-live-badge"><span class="ha-live-dot"></span>live</span></div>`)

	if len(data.Rooms) == 0 {
		b.WriteString(`<div class="ha-empty">no rooms with a temperature sensor, light, or sensor found</div>`)
	} else {
		b.WriteString(`<div class="ha-rooms">`)
		for _, r := range data.Rooms {
			b.WriteString(renderRoomCard(r))
		}
		b.WriteString(`</div>`)
	}

	fmt.Fprintf(&b, `<img src="x" alt="" style="display:none;width:0;height:0" onerror="%s">`, html.EscapeString(bootstrapScript))
	b.WriteString(`</div>`)

	return b.String()
}

func renderRoomCard(r RoomCardView) string {
	var b strings.Builder

	classes := "ha-room"
	if r.SizeClass != "" {
		classes += " " + r.SizeClass
	}
	fmt.Fprintf(&b, `<div class="%s" data-room="%s" data-lit="%t" data-occupied="%t">`,
		classes, html.EscapeString(r.Room), r.Lit, r.Occupied)

	if r.HasTemperature {
		fmt.Fprintf(&b, `<div class="ha-room-head"><span class="ha-room-name">%s</span>`, html.EscapeString(r.Room))
		if r.TempNoData {
			b.WriteString(`</div><div class="ha-temp-nodata">no data</div>`)
		} else {
			fmt.Fprintf(&b, `<span class="ha-room-temp">%s</span></div>%s`, html.EscapeString(r.TempValue), r.ChartSVG)
		}
	} else {
		fmt.Fprintf(&b, `<div class="ha-room-head"><span class="ha-room-name">%s</span></div>`, html.EscapeString(r.Room))
	}

	if len(r.Lights) > 0 {
		b.WriteString(`<div class="ha-room-lights">`)
		for _, l := range r.Lights {
			fmt.Fprintf(&b, `<span class="ha-light" data-entity-id="%s" data-on="%t">%s</span>`,
				html.EscapeString(l.EntityID), l.On, l.IconSVG)
		}
		b.WriteString(`</div>`)
	}

	if len(r.Occupancy) > 0 || len(r.Contacts) > 0 {
		b.WriteString(`<div class="ha-room-status">`)
		for _, o := range r.Occupancy {
			fmt.Fprintf(&b, `<span class="ha-occ-chip" data-sensor-name="%s" data-occupied="%t"><span class="ha-occ-dot"></span><span class="ha-occ-label">%s</span></span>`,
				html.EscapeString(o.Name), o.Attention, html.EscapeString(o.Label))
		}
		for _, c := range r.Contacts {
			fmt.Fprintf(&b, `<span class="ha-badge" data-sensor-name="%s" data-open="%t">%s<span class="ha-contact-label">%s</span></span>`,
				html.EscapeString(c.Name), c.Attention, ContactIcon(), html.EscapeString(c.Label))
		}
		b.WriteString(`</div>`)
	}

	b.WriteString(`</div>`)
	return b.String()
}

func RenderUnavailable() string {
	return styleBlock(130) + `<div class="ha-unavailable">Home Assistant unavailable</div>`
}
