package render

// bulbGlyph, trackLightGlyph and ledStripGlyph are the real Material Design
// Icons paths (mdi:lightbulb, mdi:track-light, mdi:led-strip-variant) — a
// hand-drawn first attempt at these didn't read as recognizable shapes at
// the small size they render at, so these are the actual vector paths
// instead. Each is a single fill path with no on/off state of its own —
// that's driven entirely by the data-on attribute on the wrapper element
// the caller places the glyph in (see RenderWidget in template.go) via a
// CSS rule targeting "path" generically, so a live update never needs to
// know which glyph a given light is to toggle it.
const bulbGlyph = `<svg class="bulb" viewBox="0 0 24 24"><path d="M12,2A7,7 0 0,0 5,9C5,11.38 6.19,13.47 8,14.74V17A1,1 0 0,0 9,18H15A1,1 0 0,0 16,17V14.74C17.81,13.47 19,11.38 19,9A7,7 0 0,0 12,2M9,21A1,1 0 0,0 10,22H14A1,1 0 0,0 15,21V20H9V21Z"/></svg>`

const trackLightGlyph = `<svg class="track-light" viewBox="0 0 24 24"><path d="M6,1V3H9V6.4L4.11,4.38L1.43,10.84L6.97,13.14L11.94,16.82L13.79,17.59L17.62,8.35L15.77,7.58L11,6.87V3H14V1H6M21.81,6.29L19.5,7.25L20.26,9.1L22.57,8.14L21.81,6.29M19.78,13.57L19,15.42L21.79,16.57L22.55,14.72L19.78,13.57M16.19,18.93L14.34,19.69L15.3,22L17.15,21.23L16.19,18.93Z"/></svg>`

const ledStripGlyph = `<svg class="led-strip" viewBox="0 0 24 24"><path d="M2.95 3L2 6.91L19.34 11.25L20.29 7.34L2.95 3M6.09 6.89L4.16 6.41L4.64 4.46L6.57 4.94L6.09 6.89M9.94 7.86L8 7.38L8.5 5.42L10.42 5.91L9.94 7.86M13.8 8.82L11.87 8.34L12.35 6.39L14.27 6.87L13.8 8.82M17.65 9.79L15.72 9.31L16.2 7.35L18.13 7.84L17.65 9.79M4.66 12.75L3.71 16.66L21.05 21L22 17.1L4.66 12.75M7.8 16.65L5.88 16.16L6.35 14.21L8.28 14.69L7.8 16.65M11.65 17.61L9.73 17.13L10.2 15.18L12.13 15.66L11.65 17.61M15.5 18.58L13.58 18.09L14.06 16.14L16 16.62L15.5 18.58M19.36 19.54L17.43 19.06L17.91 17.11L19.84 17.59L19.36 19.54M6.25 12.11L11 10.2L17.75 11.89L13 13.8L6.25 12.11Z"/></svg>`

// iconGlyphs maps HA's raw icon attribute to a curated fixture-type glyph.
// This is deliberately a small, hardcoded set, not a full Material Design
// Icons integration — extending it later (a new fixture type shows up in
// real data) is one new glyph constant plus one map entry.
var iconGlyphs = map[string]string{
	"mdi:track-light":       trackLightGlyph,
	"mdi:led-strip-variant": ledStripGlyph,
}

// LightIcon returns the fixture-type glyph for a light's HA icon
// attribute, falling back to a plain bulb for anything unrecognized
// (including empty) — including a clearly wrong/stale icon value like
// "mdi:alarm-off" assigned to an actual light: the lookup only trusts
// icons it explicitly recognizes.
func LightIcon(icon string) string {
	if glyph, ok := iconGlyphs[icon]; ok {
		return glyph
	}
	return bulbGlyph
}

const doorGlyph = `<svg class="ha-door" viewBox="0 0 24 24" fill="none" stroke-width="1.6"><path class="ha-door-frame" d="M6 3v18M6 3h9" stroke-linecap="round"/><path class="ha-door-leaf" d="M6 3v18l9-3V6z" stroke-linejoin="round"/></svg>`

// ContactIcon returns the door glyph; open/closed styling (including the
// leaf's rotation) is driven by the data-open attribute on the wrapper
// badge element, the same live-update-friendly pattern as LightIcon.
func ContactIcon() string {
	return doorGlyph
}
