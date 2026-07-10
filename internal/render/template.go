package render

import (
	"fmt"
	"html"
	"strings"
)

type WidgetData struct {
	Title            string
	ChartHeight      int
	ChartStyle       string // "sparkline" or "bars"
	TemperatureRooms []TemperatureRoomView
	LightRooms       []LightRoomView
	Sensors          []SensorView
	LiveURL          string
	PollIntervalMS   int
	PauseWhenHidden  bool
}

type TemperatureRoomView struct {
	Room   string
	Value  string
	SVG    string
	NoData bool
}

type LightRoomView struct {
	Room  string
	On    int
	Total int
}

type SensorView struct {
	Name      string
	Attention bool
	Label     string
}

func styleBlock() string {
	return `<style>
.ha-body{display:flex;flex-direction:column;gap:20px}
.ha-section-head{display:flex;align-items:center;gap:8px;margin-bottom:10px}
.ha-section-label{font-size:.85em;letter-spacing:.08em;text-transform:uppercase;color:var(--color-text-subdue)}
.ha-live-badge{display:inline-flex;align-items:center;gap:5px;font-size:.7em;letter-spacing:.06em;text-transform:uppercase;color:var(--color-primary)}
.ha-live-dot{width:6px;height:6px;border-radius:50%;background:var(--color-primary)}
.ha-temp-row{display:flex;gap:12px;flex-wrap:wrap}
.ha-temp-panel{flex:1;min-width:145px;background:var(--color-widget-background-highlight);border-radius:6px;padding:10px 12px}
.ha-temp-top{display:flex;justify-content:space-between;align-items:baseline;margin-bottom:6px}
.ha-temp-room-label{color:var(--color-text-subdue);font-size:.85em;margin-bottom:4px}
.ha-temp-nodata{color:var(--color-text-subdue);font-size:.85em;padding:8px 0}
.ha-lights-grid{display:grid;grid-template-columns:repeat(2,1fr);gap:10px}
.ha-light-chip{display:flex;align-items:center;justify-content:space-between;gap:10px;background:var(--color-widget-background-highlight);border-radius:6px;padding:9px 12px}
.ha-light-left{display:flex;align-items:center;gap:9px;min-width:0}
.ha-sensor-list{display:flex;flex-direction:column;gap:1px;background:var(--color-widget-content-border);border-radius:6px;overflow:hidden}
.ha-sensor-row{display:flex;align-items:center;justify-content:space-between;gap:10px;background:var(--color-widget-background-highlight);padding:9px 12px}
.ha-sensor-left{display:flex;align-items:center;gap:9px;min-width:0}
.ha-dot{flex:none;width:8px;height:8px;border-radius:50%;border:1.5px solid var(--color-text-subdue);background:transparent}
.ha-dot.ha-on{border-color:var(--color-primary);background:var(--color-primary)}
.ha-unavailable{color:var(--color-text-subdue);padding:12px 0}
</style>`
}

// bootstrapScript runs via an onerror attribute (see RenderWidget) because
// Glance mounts extension widget HTML with element.innerHTML, and <script>
// elements inserted that way are inert per the HTML spec — onerror/onload
// content attributes are not, so they're the standard way to run JS in
// HTML delivered through an innerHTML sink.
const bootstrapScript = `(function(img){var root=img.closest('.ha-widget');if(!root)return;var url=root.dataset.liveUrl;var interval=parseInt(root.dataset.pollMs,10)||10000;var pauseWhenHidden=root.dataset.pauseHidden==='true';var timer=null;function applyState(data){(data.lights||[]).forEach(function(l){var chip=root.querySelector('.ha-light-chip[data-room="'+CSS.escape(l.room)+'"]');if(!chip)return;var dot=chip.querySelector('.ha-dot');var count=chip.querySelector('.ha-light-count');if(dot)dot.classList.toggle('ha-on',l.on>0);if(count)count.textContent=l.on+'/'+l.total+' on';});(data.sensors||[]).forEach(function(s){var row=root.querySelector('.ha-sensor-row[data-name="'+CSS.escape(s.name)+'"]');if(!row)return;var dot=row.querySelector('.ha-dot');var state=row.querySelector('.ha-sensor-state');if(dot)dot.classList.toggle('ha-on',s.attention);if(state)state.textContent=s.label;});}function poll(){fetch(url,{cache:'no-store'}).then(function(r){return r.ok?r.json():null;}).then(function(data){if(data)applyState(data);}).catch(function(){});}function stop(){if(timer){clearInterval(timer);timer=null;}}function schedule(){stop();timer=setInterval(poll,interval);}if(pauseWhenHidden){document.addEventListener('visibilitychange',function(){if(document.hidden){stop();}else{poll();schedule();}});}if(!pauseWhenHidden||!document.hidden){poll();schedule();}})(this)`

func RenderWidget(data WidgetData) string {
	var b strings.Builder
	b.WriteString(styleBlock())

	pauseAttr := "false"
	if data.PauseWhenHidden {
		pauseAttr = "true"
	}
	fmt.Fprintf(&b, `<div class="ha-widget ha-body" data-live-url="%s" data-poll-ms="%d" data-pause-hidden="%s">`,
		html.EscapeString(data.LiveURL), data.PollIntervalMS, pauseAttr)

	// TEMPERATURE
	b.WriteString(`<div><div class="ha-section-head"><span class="ha-section-label">Temperature</span></div><div class="ha-temp-row">`)
	for _, r := range data.TemperatureRooms {
		b.WriteString(`<div class="ha-temp-panel">`)
		if data.ChartStyle == "bars" {
			fmt.Fprintf(&b, `<div class="ha-temp-room-label">%s</div>`, html.EscapeString(r.Room))
			if r.NoData {
				b.WriteString(`<div class="ha-temp-nodata">no data</div>`)
			} else {
				b.WriteString(r.SVG)
			}
		} else {
			fmt.Fprintf(&b, `<div class="ha-temp-top"><span class="color-text-subdue">%s</span>`, html.EscapeString(r.Room))
			if r.NoData {
				b.WriteString(`<span class="color-text-base">–</span></div><div class="ha-temp-nodata">no data</div>`)
			} else {
				fmt.Fprintf(&b, `<span class="color-text-base">%s</span></div>%s`, html.EscapeString(r.Value), r.SVG)
			}
		}
		b.WriteString(`</div>`)
	}
	if len(data.TemperatureRooms) == 0 {
		b.WriteString(`<div class="ha-temp-nodata">no rooms with a temperature sensor</div>`)
	}
	b.WriteString(`</div></div>`)

	// LIGHTS
	b.WriteString(`<div><div class="ha-section-head"><span class="ha-section-label">Lights</span><span class="ha-live-badge"><span class="ha-live-dot"></span>live</span></div><div class="ha-lights-grid">`)
	for _, l := range data.LightRooms {
		dotClass := "ha-dot"
		if l.On > 0 {
			dotClass = "ha-dot ha-on"
		}
		fmt.Fprintf(&b, `<div class="ha-light-chip" data-room="%s"><div class="ha-light-left"><span class="%s"></span><span>%s</span></div><span class="ha-light-count">%d/%d on</span></div>`,
			html.EscapeString(l.Room), dotClass, html.EscapeString(l.Room), l.On, l.Total)
	}
	if len(data.LightRooms) == 0 {
		b.WriteString(`<div class="ha-temp-nodata">no rooms with lights</div>`)
	}
	b.WriteString(`</div></div>`)

	// SENSORS
	b.WriteString(`<div><div class="ha-section-head"><span class="ha-section-label">Sensors</span><span class="ha-live-badge"><span class="ha-live-dot"></span>live</span></div><div class="ha-sensor-list">`)
	for _, s := range data.Sensors {
		dotClass := "ha-dot"
		if s.Attention {
			dotClass = "ha-dot ha-on"
		}
		fmt.Fprintf(&b, `<div class="ha-sensor-row" data-name="%s"><div class="ha-sensor-left"><span class="%s"></span><span>%s</span></div><span class="ha-sensor-state">%s</span></div>`,
			html.EscapeString(s.Name), dotClass, html.EscapeString(s.Name), html.EscapeString(s.Label))
	}
	if len(data.Sensors) == 0 {
		b.WriteString(`<div class="ha-temp-nodata">no contact/motion sensors found</div>`)
	}
	b.WriteString(`</div></div>`)

	fmt.Fprintf(&b, `<img src="x" alt="" style="display:none;width:0;height:0" onerror="%s">`, html.EscapeString(bootstrapScript))
	b.WriteString(`</div>`)

	return b.String()
}

func RenderUnavailable() string {
	return styleBlock() + `<div class="ha-unavailable">Home Assistant unavailable</div>`
}
