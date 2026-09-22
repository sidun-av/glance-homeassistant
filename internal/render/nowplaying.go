package render

import (
	"fmt"
	"html"
	"strings"
)

// MediaView is one row of the "Now playing" panel next to the map.
type MediaView struct {
	EntityID   string
	Name       string
	Room       string
	State      string // playing, paused, idle, on ...
	Title      string
	Artist     string
	Accent     string  // CSS colour shared with the player's tile on the map
	Position   float64 // seconds at PositionAt
	Duration   float64 // seconds, 0 = no progress bar
	PositionAt int64   // unix ms when Position was reported (0 = unknown)
	ArtURL     string  // album art through this service's /art proxy, "" = none
	Volume     float64 // 0..1; <0 = player has no volume → no slider
}

// accentPalette gives every media player its own colour, stable while the
// set of players is stable (assigned by sorted entity_id — see
// AssignAccents), so the notes drifting up from a speaker on the map match
// its Now-playing card.
var accentPalette = []string{"#a9c2f7", "#f0a6c8", "#8fd3a0", "#f5c26b", "#c9a7f5", "#7fd8d0", "#f2a07b", "#b8d47a"}

// AccentFor returns the palette colour for a player given its index in the
// entity_id-sorted list of players.
func AccentFor(i int) string { return accentPalette[i%len(accentPalette)] }

const nowPlayingCSS = `
	.ha-fp-layout{display:flex;flex-wrap:wrap;gap:14px;align-items:stretch}
	.ha-fp-layout>.ha-floorplan{flex:0 1 auto}
	.ha-np{flex:1 1 260px;min-width:0;display:flex;flex-direction:column;gap:8px}
	.ha-np-empty{color:var(--color-text-subdue);font-size:.85em;padding:8px 0}
	/* Rows share the panel's height equally, so the panel matches the map. */
	.ha-np-row{--accent:var(--color-primary);flex:1 1 0;min-height:64px;display:grid;grid-template-columns:auto 1fr;grid-template-rows:1fr auto auto;gap:2px 12px;align-items:center;
	  padding:8px 10px 8px 8px;border-radius:var(--border-radius,8px);border:1px solid var(--color-widget-content-border);
	  border-left:3px solid var(--accent);background:var(--color-widget-background);overflow:hidden}
	.ha-np-row[data-state="playing"]{background:color-mix(in srgb,var(--accent) 8%,var(--color-widget-background))}
	.ha-np-art{grid-row:1/4;align-self:center;height:100%;max-height:110px;min-height:40px;aspect-ratio:1;border-radius:6px;overflow:hidden;
	  background:color-mix(in srgb,var(--accent) 14%,var(--color-widget-background));display:flex;align-items:center;justify-content:center}
	.ha-np-art img{width:100%;height:100%;object-fit:cover;display:block}
	.ha-np-art img[src=""],.ha-np-art img:not([src]){display:none}
	.ha-np-art svg{width:45%;height:45%}
	.ha-np-art svg path{fill:var(--accent)}
	.ha-np-art[data-has-art="true"] svg{display:none}
	.ha-np-row[data-state="idle"] .ha-np-art svg path,.ha-np-row[data-state="on"] .ha-np-art svg path{fill:var(--color-text-subdue)}
	.ha-np-text{min-width:0;display:flex;flex-direction:column;gap:2px;align-self:end}
	.ha-np-name{font-size:11px;letter-spacing:.03em;color:var(--color-text-subdue);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
	.ha-np-title{font-size:13px;font-weight:600;color:var(--color-text-highlight);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
	.ha-np-artist{font-size:11px;color:var(--color-text-base);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
	.ha-np-row[data-state="playing"] .ha-np-title{color:var(--accent)}
	.ha-np-ctl{grid-column:2;display:flex;gap:4px;align-items:center;justify-self:stretch;margin-left:-6px;min-width:0}
	.ha-np-vol{flex:1 1 60px;max-width:150px;margin-left:10px;display:flex;align-items:center;gap:6px;min-width:0}
	.ha-np-vol svg{width:14px;height:14px;flex:none}
	.ha-np-vol svg path{fill:var(--color-text-subdue)}
	.ha-np-vol input{flex:1;min-width:0;height:14px;margin:0;-webkit-appearance:none;appearance:none;background:transparent;cursor:pointer}
	.ha-np-vol input::-webkit-slider-runnable-track{height:4px;border-radius:2px;background:linear-gradient(90deg,var(--accent) var(--pct,0%),color-mix(in srgb,var(--color-text-base) 16%,transparent) var(--pct,0%))}
	.ha-np-vol input::-moz-range-track{height:4px;border-radius:2px;background:color-mix(in srgb,var(--color-text-base) 16%,transparent)}
	.ha-np-vol input::-moz-range-progress{height:4px;border-radius:2px;background:var(--accent)}
	.ha-np-vol input::-webkit-slider-thumb{-webkit-appearance:none;width:12px;height:12px;border-radius:50%;background:var(--accent);margin-top:-4px;border:0}
	.ha-np-vol input::-moz-range-thumb{width:12px;height:12px;border-radius:50%;background:var(--accent);border:0}
	.ha-np-vol[hidden]{display:none}
	.ha-np-btn{width:30px;height:30px;border:0;border-radius:6px;background:transparent;color:var(--color-text-base);cursor:pointer;
	  display:flex;align-items:center;justify-content:center;font:inherit;padding:0}
	.ha-np-btn:hover{background:color-mix(in srgb,var(--accent) 18%,transparent);color:var(--color-text-highlight)}
	.ha-np-btn svg{width:17px;height:17px}
	.ha-np-btn svg path{fill:currentColor}
	.ha-np-btn[data-busy="true"]{opacity:.4;pointer-events:none}
	.ha-np-row[data-state="playing"] .ha-np-play .ha-np-ico-play,.ha-np-row:not([data-state="playing"]) .ha-np-play .ha-np-ico-pause{display:none}
	/* Progress: the bar spans the text+controls columns under them. */
	.ha-np-prog{grid-column:2;align-self:end;display:flex;align-items:center;gap:8px;font-size:10px;color:var(--color-text-subdue);font-variant-numeric:tabular-nums}
	.ha-np-row[data-duration="0"] .ha-np-prog{display:none}
	.ha-np-bar{flex:1;display:block;height:4px;border-radius:2px;background:color-mix(in srgb,var(--color-text-base) 16%,transparent);overflow:hidden;cursor:pointer;position:relative}
	.ha-np-bar::before{content:"";position:absolute;inset:-6px 0}
	.ha-np-fill{display:block;height:100%;width:0;background:var(--accent);border-radius:2px;transition:width .5s linear}
`

// mdi:skip-previous, mdi:play, mdi:pause, mdi:skip-next
const (
	glyphPrev  = `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6,18V6H8V18H6M9.5,12L18,6V18L9.5,12Z"/></svg>`
	glyphPlay  = `<svg class="ha-np-ico-play" viewBox="0 0 24 24" aria-hidden="true"><path d="M8,5.14V19.14L19,12.14L8,5.14Z"/></svg>`
	glyphPause = `<svg class="ha-np-ico-pause" viewBox="0 0 24 24" aria-hidden="true"><path d="M14,19H18V5H14M6,19H10V5H6V19Z"/></svg>`
	glyphNext  = `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M16,18H18V6H16M6,18L14.5,12L6,6V18Z"/></svg>`
	// mdi:volume-medium
	glyphVol = `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M5,9V15H9L14,20V4L9,9M18.5,12C18.5,10.23 17.5,8.71 16,7.97V16C17.5,15.29 18.5,13.76 18.5,12Z"/></svg>`
)

// renderNowPlaying draws the panel. mediaURL is the endpoint the buttons
// POST to ("" hides the controls). Rows keep data-entity-id so the live
// poller can patch state/title in place.
func renderNowPlaying(media []MediaView, mediaURL string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<div class="ha-np" data-media-url="%s">`, html.EscapeString(mediaURL))
	if len(media) == 0 {
		b.WriteString(`<div class="ha-np-empty">nothing playing</div>`)
	}
	for _, m := range media {
		b.WriteString(renderNowPlayingRow(m, mediaURL != ""))
	}
	b.WriteString(`</div>`)
	return b.String()
}

func renderNowPlayingRow(m MediaView, controls bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<div class="ha-np-row" data-entity-id="%s" data-state="%s" data-position="%.0f" data-duration="%.0f" data-position-at="%d" style="--accent:%s">`,
		html.EscapeString(m.EntityID), html.EscapeString(m.State), m.Position, m.Duration, m.PositionAt, html.EscapeString(m.Accent))
	hasArt := m.ArtURL != ""
	fmt.Fprintf(&b, `<span class="ha-np-art" data-has-art="%t"><img src="%s" alt="" loading="lazy">%s</span>`, hasArt, html.EscapeString(m.ArtURL), DeviceIcon("media_player", ""))
	name := m.Name
	if m.Room != "" {
		name = m.Room + " · " + m.Name
	}
	title, artist := m.Title, m.Artist
	if title == "" {
		title = stateLabel(m.State)
	}
	fmt.Fprintf(&b, `<span class="ha-np-text"><span class="ha-np-name">%s</span><span class="ha-np-title">%s</span><span class="ha-np-artist">%s</span></span>`,
		html.EscapeString(name), html.EscapeString(title), html.EscapeString(artist))
	if controls {
		fmt.Fprintf(&b, `<span class="ha-np-ctl"><button type="button" class="ha-np-btn" data-action="media_previous_track" title="Previous">%s</button><button type="button" class="ha-np-btn ha-np-play" data-action="media_play_pause" title="Play / pause">%s%s</button><button type="button" class="ha-np-btn" data-action="media_next_track" title="Next">%s</button>`,
			glyphPrev, glyphPlay, glyphPause, glyphNext)
		hidden := ""
		vol := 0
		if m.Volume < 0 {
			hidden = " hidden"
		} else {
			vol = int(m.Volume*100 + 0.5)
		}
		fmt.Fprintf(&b, `<span class="ha-np-vol"%s title="Volume">%s<input type="range" min="0" max="100" step="1" value="%d" style="--pct:%d%%" aria-label="Volume"></span></span>`, hidden, glyphVol, vol, vol)
	} else {
		b.WriteString(`<span></span>`)
	}
	pct := 0.0
	if m.Duration > 0 {
		pct = 100 * m.Position / m.Duration
	}
	fmt.Fprintf(&b, `<span class="ha-np-prog"><span class="ha-np-time">%s</span><span class="ha-np-bar"><span class="ha-np-fill" style="width:%.1f%%"></span></span><span class="ha-np-total">%s</span></span>`,
		Clock(m.Position), pct, Clock(m.Duration))
	b.WriteString(`</div>`)
	return b.String()
}

// Clock formats seconds as m:ss (or h:mm:ss).
func Clock(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	t := int(sec + 0.5)
	if t >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", t/3600, t%3600/60, t%60)
	}
	return fmt.Sprintf("%d:%02d", t/60, t%60)
}

func stateLabel(state string) string {
	switch state {
	case "playing":
		return "Playing"
	case "paused":
		return "Paused"
	case "idle":
		return "Idle"
	case "on":
		return "On"
	}
	return state
}
