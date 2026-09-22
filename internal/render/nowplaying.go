package render

import (
	"fmt"
	"html"
	"strings"
)

// MediaView is one row of the "Now playing" panel next to the map.
type MediaView struct {
	EntityID string
	Name     string
	Room     string
	State    string // playing, paused, idle, on ...
	Title    string
	Artist   string
	Accent   string // CSS colour shared with the player's tile on the map
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
	.ha-fp-layout{display:flex;flex-wrap:wrap;gap:14px;align-items:flex-start}
	.ha-fp-layout>.ha-floorplan{flex:0 1 auto}
	.ha-np{flex:1 1 240px;min-width:0;display:flex;flex-direction:column;gap:8px}
	.ha-np-empty{color:var(--color-text-subdue);font-size:.85em;padding:8px 0}
	.ha-np-row{--accent:var(--color-primary);display:grid;grid-template-columns:auto 1fr auto;gap:10px;align-items:center;
	  padding:8px 10px;border-radius:var(--border-radius,8px);border:1px solid var(--color-widget-content-border);
	  border-left:3px solid var(--accent);background:var(--color-widget-background)}
	.ha-np-row[data-state="playing"]{background:color-mix(in srgb,var(--accent) 8%,var(--color-widget-background))}
	.ha-np-icon svg{width:22px;height:22px;display:block}
	.ha-np-icon svg path{fill:var(--accent)}
	.ha-np-row[data-state="idle"] .ha-np-icon svg path,.ha-np-row[data-state="on"] .ha-np-icon svg path{fill:var(--color-text-subdue)}
	.ha-np-text{min-width:0;display:flex;flex-direction:column;gap:1px}
	.ha-np-name{font-size:11px;letter-spacing:.03em;color:var(--color-text-subdue);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
	.ha-np-title{font-size:12.5px;font-weight:600;color:var(--color-text-highlight);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
	.ha-np-artist{font-size:11px;color:var(--color-text-base);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
	.ha-np-row[data-state="playing"] .ha-np-title{color:var(--accent)}
	.ha-np-ctl{display:flex;gap:2px}
	.ha-np-btn{width:28px;height:28px;border:0;border-radius:6px;background:transparent;color:var(--color-text-base);cursor:pointer;
	  display:flex;align-items:center;justify-content:center;font:inherit;padding:0}
	.ha-np-btn:hover{background:color-mix(in srgb,var(--accent) 18%,transparent);color:var(--color-text-highlight)}
	.ha-np-btn svg{width:16px;height:16px}
	.ha-np-btn svg path{fill:currentColor}
	.ha-np-btn[data-busy="true"]{opacity:.4;pointer-events:none}
	.ha-np-row[data-state="playing"] .ha-np-play .ha-np-ico-play,.ha-np-row:not([data-state="playing"]) .ha-np-play .ha-np-ico-pause{display:none}
`

// mdi:skip-previous, mdi:play, mdi:pause, mdi:skip-next
const (
	glyphPrev  = `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6,18V6H8V18H6M9.5,12L18,6V18L9.5,12Z"/></svg>`
	glyphPlay  = `<svg class="ha-np-ico-play" viewBox="0 0 24 24" aria-hidden="true"><path d="M8,5.14V19.14L19,12.14L8,5.14Z"/></svg>`
	glyphPause = `<svg class="ha-np-ico-pause" viewBox="0 0 24 24" aria-hidden="true"><path d="M14,19H18V5H14M6,19H10V5H6V19Z"/></svg>`
	glyphNext  = `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M16,18H18V6H16M6,18L14.5,12L6,6V18Z"/></svg>`
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
	fmt.Fprintf(&b, `<div class="ha-np-row" data-entity-id="%s" data-state="%s" style="--accent:%s">`,
		html.EscapeString(m.EntityID), html.EscapeString(m.State), html.EscapeString(m.Accent))
	fmt.Fprintf(&b, `<span class="ha-np-icon">%s</span>`, DeviceIcon("media_player", ""))
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
		fmt.Fprintf(&b, `<span class="ha-np-ctl"><button type="button" class="ha-np-btn" data-action="media_previous_track" title="Previous">%s</button><button type="button" class="ha-np-btn ha-np-play" data-action="media_play_pause" title="Play / pause">%s%s</button><button type="button" class="ha-np-btn" data-action="media_next_track" title="Next">%s</button></span>`,
			glyphPrev, glyphPlay, glyphPause, glyphNext)
	}
	b.WriteString(`</div>`)
	return b.String()
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
