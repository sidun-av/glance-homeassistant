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

// chartCSS is a near-literal port of Glance's own static/css/widget-weather.css
// (github.com/glanceapp/glance), renamed onto this widget's ha-* classes.
// Keep it that way: the ask is for this chart to be visually identical to the
// built-in Weather widget's, and every past attempt to "improve" a rule here
// from memory is what made it drift.
//
// Two details are load-bearing and easy to destroy by reordering or tidying:
//
//   - The duplicated selectors ".ha-bar-value, .ha-bar-cols:hover
//     .ha-bar-value" (and the same shape for .ha-bar and .ha-bar-col-time)
//     are Weather's hover mechanic, not redundancy. The row-hover copy has
//     one more class' worth of specificity than ".ha-bar-col-current
//     .ha-bar-value" / ".ha-bar-col:nth-child(3) .ha-bar-col-time", so
//     hovering anywhere in the chart suppresses the at-rest labels and the
//     current column's emphasis, and the later ".ha-bar-col:hover ..." rules
//     then light up only the column actually under the cursor. Merge or
//     reorder these and either the hover does nothing or two columns light
//     up at once.
//
//   - hsl(var(--ths), calc(var(--scheme) ((var(--scheme) var(--bgl)) + 18%)))
//     is verbatim Glance's own bar color expression. --ths/--bgl/--scheme are
//     :root custom properties defined by Glance's main.css and inherited into
//     any extension widget's markup, so this tracks the user's theme (and its
//     light/dark inversion, which is what the --scheme token substitution
//     does) exactly the way the real Weather widget does — which
//     var(--color-*) alone cannot reproduce, since Glance exposes no
//     ready-made variable for these particular tints.
//
// Columns are width:100%/N with no gap between them, again per Weather. That
// is also why the daylight highlight can go back to one plain inset:0 div per
// daytime column (see BarColumns): with no gap there are no seams to hide, so
// the previous "collapse contiguous runs into one positioned div" workaround
// is gone.
const chartCSS = `
	.ha-bar-cols{flex:1 0 auto;width:100%;display:flex;justify-content:center}
	.ha-bar-col{
	  position:relative;
	  display:flex;align-items:center;justify-content:end;flex-direction:column;
	  width:calc(100% / var(--ha-bar-cols,12));
	  padding-top:3px;
	}

	.ha-bar-value, .ha-bar-cols:hover .ha-bar-value{
	  font-size:var(--font-size-base);color:var(--color-text-highlight);
	  letter-spacing:-0.1rem;margin-right:0.1rem;
	  position:relative;margin-bottom:0.3rem;
	  opacity:0;transform:translateY(0.5rem);
	  transition:opacity .2s,transform .2s;
	  user-select:none;white-space:nowrap;
	}
	.ha-bar-col-current .ha-bar-value, .ha-bar-col:hover .ha-bar-value{opacity:1;transform:translateY(0)}
	.ha-bar-value::after{position:absolute;content:'\00b0';left:100%;color:var(--color-text-subdue)}
	.ha-bar-value.ha-bar-value-negative::before{position:absolute;content:'-';right:100%}

	.ha-bar, .ha-bar-cols:hover .ha-bar{
	  height:calc(20px + var(--ha-bar-height,0) * 40px);
	  width:6px;flex-shrink:0;
	  background-color:hsl(var(--ths), calc(var(--scheme) ((var(--scheme) var(--bgl)) + 18%)));
	  border:1px solid hsl(var(--ths), calc(var(--scheme) ((var(--scheme) var(--bgl)) + 24%)));
	  border-bottom:0;
	  border-radius:6px 6px 0 0;
	  mask-image:linear-gradient(0deg,transparent 0,#000 10px);
	  -webkit-mask-image:linear-gradient(0deg,transparent 0,#000 10px);
	  transition:background-color .2s,border-color .2s,width .2s;
	}
	.ha-bar-col-current .ha-bar, .ha-bar-col:hover .ha-bar{
	  width:10px;
	  background-color:hsl(var(--ths), calc(var(--scheme) ((var(--scheme) var(--bgl)) + 40%)));
	  border:1px solid hsl(var(--ths), calc(var(--scheme) ((var(--scheme) var(--bgl)) + 50%)));
	}
	.ha-bar-empty, .ha-bar-cols:hover .ha-bar-empty, .ha-bar-col:hover .ha-bar-empty{visibility:hidden}

	/* Projected columns: same bar, faded. Nothing in Weather corresponds to
	   these (its future columns are a real forecast) — see BarChartData's
	   CurrentIndex. Only opacity and the label colour change, so hovering
	   one still widens and lifts it exactly like a measured column; it just
	   stays visibly dimmer while it does. The row-hover copy of the label
	   rule is needed for the same specificity reason as everywhere else in
	   this stylesheet. */
	.ha-bar-projected{opacity:.4}
	.ha-bar-value-projected, .ha-bar-cols:hover .ha-bar-value-projected{color:var(--color-text-base)}

	.ha-bar-col:nth-child(3) .ha-bar-col-time,
	.ha-bar-col:nth-child(7) .ha-bar-col-time,
	.ha-bar-col:nth-child(11) .ha-bar-col-time{opacity:1;transform:translateY(0)}

	.ha-bar-col-time, .ha-bar-cols:hover .ha-bar-col-time{
	  margin-top:0.3rem;font-size:var(--font-size-h6);
	  opacity:0;transform:translateY(-0.5rem);
	  transition:opacity .2s,transform .2s;
	  user-select:none;white-space:nowrap;
	}
	.ha-bar-col:hover .ha-bar-col-time{opacity:1;transform:translateY(0)}

	.ha-bar-daylight{
	  position:absolute;inset:0;pointer-events:none;
	  background:linear-gradient(0deg,transparent 30px,hsl(50,50%,30%,0.2));
	}
	.ha-bar-daylight-sunrise{border-radius:20px 0 0 0}
	.ha-bar-daylight-sunset{border-radius:0 20px 0 0}
`

// widgetCSS is everything outside the chart itself — the room-card grid and
// the light/occupancy/contact chrome. Kept as a raw literal (no Sprintf) so
// its many percentage values never have to be %%-escaped; only the three
// card min-heights, which contain no other percent signs, go through
// Sprintf in styleBlock.
const widgetCSS = `
	.ha-body{display:flex;flex-direction:column;gap:16px}
	.ha-section-head{display:flex;align-items:center;gap:8px}
	.ha-section-label{font-size:.85em;letter-spacing:.08em;text-transform:uppercase;color:var(--color-text-subdue)}
	.ha-live-badge{display:inline-flex;align-items:center;gap:5px;font-size:.7em;letter-spacing:.06em;text-transform:uppercase;color:var(--color-primary)}
	.ha-live-dot{width:6px;height:6px;border-radius:50%;background:var(--color-primary)}
	.ha-unavailable{color:var(--color-text-subdue);padding:12px 0}
	.ha-empty{color:var(--color-text-subdue);font-size:.85em;padding:8px 0}

	.ha-rooms{display:flex;flex-wrap:wrap;gap:10px;align-items:stretch}
	/* container-type:inline-size lives ONLY on chart cards, which are the
	   only ones with @container rules (.ha-chart-axis) and which get an
	   explicit flex-basis anyway. It used to be on every .ha-room, and
	   inline-size containment makes a box's intrinsic width zero — so a
	   non-chart card could never size to its own content and always
	   collapsed to min-width, leaving the unused strip on its right that
	   read as lopsided padding. */
	.ha-room[data-chart="true"]{container-type:inline-size;flex:1 1 160px;max-width:420px}
	.ha-room.ha-size-md[data-chart="true"]{flex:1 1 200px;max-width:420px}
	.ha-room.ha-size-lg[data-chart="true"]{flex:1 1 340px;max-width:420px}
	.ha-room[data-lit="true"]{background:rgba(240,196,121,.14);border-color:rgba(240,196,121,.35)}

	.ha-room-head{flex:none;display:flex;align-items:baseline;gap:8px}
	.ha-room-name{font-size:13.5px;font-weight:600;color:var(--color-text-highlight)}
	.ha-temp-nodata{color:var(--color-text-subdue);font-size:.85em;padding:2px 0}
	.ha-room-chart{flex:2 1 auto;width:100%;display:block;min-height:30px}

	.ha-chart-axis{display:flex;justify-content:space-between;flex:none;font-size:9px;letter-spacing:.02em;color:var(--color-text-base);padding:0 1px}
	.ha-chart-axis span{display:none}
	.ha-chart-axis span[data-tier="0"],.ha-chart-axis span[data-tier="1"]{display:inline}
	@container (min-width:380px){.ha-chart-axis span[data-tier="2"]{display:inline}}
	@container (min-width:520px){.ha-chart-axis span[data-tier="3"]{display:inline}}

	.ha-room-lights{flex:1 1 auto;display:flex;flex-wrap:wrap;align-content:center;align-items:center;justify-content:center;gap:10px}
	.ha-room-lights svg{width:26px;height:26px;flex:none}
	.ha-room-status{flex:none;display:flex;flex-direction:column;align-items:center;gap:5px}

	.ha-occ-chip{
	  display:inline-flex;align-items:center;gap:6px;width:fit-content;
	  font-size:11px;letter-spacing:.03em;padding:3px 9px 3px 7px;border-radius:20px;
	  border:1px solid var(--color-text-subdue);color:var(--color-text-subdue);
	}
	.ha-occ-chip .ha-occ-dot{width:7px;height:7px;border-radius:50%;background:var(--color-text-subdue)}
	.ha-occ-chip[data-occupied="true"]{
	  border-color:var(--color-primary);color:var(--color-primary);
	  background:color-mix(in srgb,var(--color-primary) 16%,transparent);
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
`

// roomSizeCSS carries the only size-dependent rules. Room cards are
// flex:0 0 auto — they size to their own content and are never squeezed
// narrower by a greedy chart card sharing the row (which is what used to
// leave a wide unused strip on a card's right-hand side while its left
// padding stayed at 14px, reading as asymmetric padding). min-width is a
// floor for near-empty cards only; where it does exceed the content, the
// light row's justify-content:center (see widgetCSS) splits the leftover
// space evenly instead of pushing it all to the right.
const roomSizeCSS = `
	.ha-room{
	  flex:0 0 auto;min-width:120px;min-height:%dpx;
	  background:var(--color-widget-background-highlight);
	  border:1px solid var(--color-widget-content-border);
	  border-radius:8px;padding:12px 14px 11px;
	  display:flex;flex-direction:column;gap:9px;
	  transition:background .2s,border-color .2s;
	}
	.ha-room.ha-size-md{flex:0 0 auto;min-width:160px;min-height:%dpx}
	.ha-room.ha-size-lg{flex:0 0 auto;min-width:200px;min-height:%dpx}
`

// styleBlock renders the widget's CSS. cardMinHeight is the base (small
// tier) room card's min-height in px, taken from temperature.chart_height
// — the "medium"/"large" tiers scale from it (+20, +130), matching the
// weight thresholds computed in main.go's sizeClassForWeight.
func styleBlock(cardMinHeight int) string {
	return "<style>" +
		fmt.Sprintf(roomSizeCSS, cardMinHeight, cardMinHeight+20, cardMinHeight+130) +
		widgetCSS + chartCSS +
		"</style>"
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

	// Every card's head is just the room name. A chart card used to also
	// carry the current temperature in its top-right corner, back when the
	// chart itself only ever labelled three of its buckets; now that every
	// column is labelled and the current one's label is always on show (see
	// BarColumns), that corner was printing the same number twice.
	fmt.Fprintf(&b, `<div class="ha-room-head"><span class="ha-room-name">%s</span></div>`, html.EscapeString(r.Room))
	if r.HasTemperature {
		if r.TempNoData {
			b.WriteString(`<div class="ha-temp-nodata">no data</div>`)
		} else {
			b.WriteString(r.ChartHTML + r.AxisRowHTML)
		}
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
