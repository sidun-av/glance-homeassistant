package render

// bulbGlyph, trackLightGlyph and ledStripGlyph are fixture-type glyphs
// only — they carry no on/off state in their own markup. On/off styling
// is driven entirely by the data-on attribute on the wrapper element the
// caller places the glyph in (see RenderWidget in template.go) via CSS
// attribute selectors, so a live update never needs to know which glyph a
// given light is to toggle it.
const bulbGlyph = `<svg class="bulb" viewBox="0 0 24 24" fill="none" stroke-width="1.6"><circle class="bulb-glass" cx="12" cy="10" r="6.2"/><path class="bulb-base" d="M9.3 17.5h5.4M9.8 20h4.4" stroke-linecap="round"/></svg>`

const trackLightGlyph = `<svg class="track-light" viewBox="0 0 24 24" fill="none" stroke-width="1.6"><path class="tl-rail" d="M7 4h10" stroke-linecap="round"/><path class="tl-rail" d="M12 4v3" stroke-linecap="round"/><path class="tl-head" d="M8.3 7h7.4l-2 6h-3.4z" stroke-linejoin="round"/><path class="tl-ray" d="M10 15.5v2.2M12 15.5v2.8M14 15.5v2.2" stroke-linecap="round"/></svg>`

const ledStripGlyph = `<svg class="led-strip" viewBox="0 0 24 24" fill="none" stroke-width="1.6"><rect class="ls-body" x="3" y="10" width="18" height="4.4" rx="2.2"/><circle class="ls-led" cx="6.6" cy="12.2" r=".9" stroke="none"/><circle class="ls-led" cx="10.6" cy="12.2" r=".9" stroke="none"/><circle class="ls-led" cx="14.6" cy="12.2" r=".9" stroke="none"/><circle class="ls-led" cx="18.6" cy="12.2" r=".9" stroke="none"/></svg>`

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
