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
	ChartHTML      string
	AxisRowHTML    string // "" for the "bars" chart style, which renders its own per-column time labels (see AxisLabelsRow)
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
	  container-type:inline-size;
	  flex:0 1 160px;min-height:%dpx;
	  background:var(--color-widget-background-highlight);
	  border:1px solid var(--color-widget-content-border);
	  border-radius:8px;padding:12px 14px 11px;
	  display:flex;flex-direction:column;gap:9px;
	  transition:background .2s,border-color .2s;
	}
	.ha-room.ha-size-md{flex:0 1 200px;min-height:%dpx}
	.ha-room.ha-size-lg{flex:0 1 340px;min-height:%dpx}
	.ha-room[data-chart="true"]{flex:1 1 160px;max-width:420px}
	.ha-room.ha-size-md[data-chart="true"]{flex:1 1 200px;max-width:420px}
	.ha-room.ha-size-lg[data-chart="true"]{flex:1 1 340px;max-width:420px}
	.ha-room[data-lit="true"]{background:rgba(240,196,121,.14);border-color:rgba(240,196,121,.35)}

	.ha-room-head{flex:none;display:flex;align-items:baseline;justify-content:space-between;gap:8px}
	.ha-room-name{font-size:13.5px;font-weight:600;color:var(--color-text-highlight)}
	.ha-room-temp{font-size:13px;color:var(--color-text-highlight);font-variant-numeric:tabular-nums;white-space:nowrap}
	.ha-temp-nodata{color:var(--color-text-subdue);font-size:.85em;padding:2px 0}
	.ha-room-chart{flex:2 1 auto;width:100%%;display:block;min-height:30px}

	.ha-bar-cols{
	  position:relative;
	  flex:2 1 auto;width:100%%;min-height:38px;
	  display:flex;align-items:flex-end;gap:2px;
	}
	.ha-bar-col{
	  position:relative;flex:1 1 0;min-width:0;height:100%%;
	  display:flex;flex-direction:column;align-items:center;justify-content:flex-end;
	  padding-top:13px;
	}
	.ha-bar-daylight{
	  position:absolute;bottom:0;height:54px;pointer-events:none;
	  background:linear-gradient(0deg,transparent 10px,color-mix(in srgb,var(--color-primary) 14%%,transparent));
	  border-radius:6px 6px 0 0;
	}
	.ha-room.ha-size-md .ha-bar-daylight{height:70px}
	.ha-room.ha-size-lg .ha-bar-daylight{height:150px}
	.ha-bar-value{
	  position:relative;margin-bottom:2px;font-size:7px;color:var(--color-text-subdue);
	  white-space:nowrap;font-variant-numeric:tabular-nums;line-height:1;
	}
	.ha-bar-value-current{font-size:9px;font-weight:600;color:var(--color-text-highlight)}
	@container (max-width:230px){.ha-bar-value:not(.ha-bar-value-current){display:none}}
	.ha-bar{
	  position:relative;width:6px;border-radius:4px 4px 0 0;
	  background:var(--color-progress-value);opacity:.55;
	  height:calc(10px + var(--ha-bar-height,0) * 34px);
	  mask-image:linear-gradient(0deg,transparent 0,#000 6px);
	  -webkit-mask-image:linear-gradient(0deg,transparent 0,#000 6px);
	}
	.ha-room.ha-size-md .ha-bar{width:7px;height:calc(10px + var(--ha-bar-height,0) * 50px)}
	.ha-room.ha-size-lg .ha-bar{width:9px;height:calc(10px + var(--ha-bar-height,0) * 130px)}
	.ha-bar-current{width:10px;opacity:1;background:var(--color-primary)}
	.ha-room.ha-size-md .ha-bar-current{width:11px}
	.ha-room.ha-size-lg .ha-bar-current{width:13px}
	.ha-bar-empty{opacity:0}
	.ha-bar-col-time{margin-top:3px;font-size:8px;color:var(--color-text-base);opacity:0;white-space:nowrap}
	.ha-bar-col-time-visible{opacity:1}

	.ha-chart-axis{display:flex;justify-content:space-between;flex:none;font-size:9px;letter-spacing:.02em;color:var(--color-text-base);padding:0 1px}
	.ha-chart-axis span{display:none}
	.ha-chart-axis span[data-tier="0"],.ha-chart-axis span[data-tier="1"]{display:inline}
	@container (min-width:380px){.ha-chart-axis span[data-tier="2"]{display:inline}}
	@container (min-width:520px){.ha-chart-axis span[data-tier="3"]{display:inline}}
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

	.ha-light path{transition:fill .2s,filter .2s}
	.ha-light[data-on="true"] path{fill:#f0c479;filter:drop-shadow(0 0 4px rgba(240,196,121,.65))}
	.ha-light[data-on="false"] path{fill:var(--color-text-subdue)}

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
	fmt.Fprintf(&b, `<div class="%s" data-room="%s" data-lit="%t" data-occupied="%t" data-chart="%t">`,
		classes, html.EscapeString(r.Room), r.Lit, r.Occupied, r.HasTemperature)

	if r.HasTemperature {
		fmt.Fprintf(&b, `<div class="ha-room-head"><span class="ha-room-name">%s</span>`, html.EscapeString(r.Room))
		if r.TempNoData {
			b.WriteString(`</div><div class="ha-temp-nodata">no data</div>`)
		} else {
			fmt.Fprintf(&b, `<span class="ha-room-temp">%s</span></div>%s%s`, html.EscapeString(r.TempValue), r.ChartHTML, r.AxisRowHTML)
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
