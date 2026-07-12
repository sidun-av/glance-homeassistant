package render

import "testing"

func TestLightIcon_KnownIconRendersMatchingGlyph(t *testing.T) {
	svg := LightIcon("mdi:track-light")
	if !contains(svg, `class="track-light"`) {
		t.Errorf("svg = %q, want a track-light glyph", svg)
	}
	// Fragment unique to MDI's real track-light path — guards against ever
	// silently reverting to a hand-drawn approximation.
	if !contains(svg, "M6,1V3H9V6.4") {
		t.Errorf("svg = %q, want the real MDI track-light path", svg)
	}
}

func TestLightIcon_LedStripVariant(t *testing.T) {
	svg := LightIcon("mdi:led-strip-variant")
	if !contains(svg, `class="led-strip"`) {
		t.Errorf("svg = %q, want a led-strip glyph", svg)
	}
	if !contains(svg, "M2.95 3L2 6.91") {
		t.Errorf("svg = %q, want the real MDI led-strip-variant path", svg)
	}
}

func TestLightIcon_UnknownIconFallsBackToBulb(t *testing.T) {
	svg := LightIcon("mdi:alarm-off")
	if !contains(svg, `class="bulb"`) {
		t.Errorf("svg = %q, want fallback to bulb for an unrecognized icon", svg)
	}
	if !contains(svg, "M12,2A7,7") {
		t.Errorf("svg = %q, want the real MDI lightbulb path", svg)
	}
}

func TestLightIcon_EmptyIconFallsBackToBulb(t *testing.T) {
	svg := LightIcon("")
	if !contains(svg, `class="bulb"`) {
		t.Errorf("svg = %q, want fallback to bulb for an empty icon", svg)
	}
}

func TestContactIcon_RendersDoorGlyph(t *testing.T) {
	svg := ContactIcon()
	if !contains(svg, `class="ha-door"`) {
		t.Errorf("svg = %q, want an ha-door glyph", svg)
	}
	if !contains(svg, "ha-door-frame") || !contains(svg, "ha-door-leaf") {
		t.Errorf("svg = %q, want frame and leaf parts for CSS-driven open/closed rotation", svg)
	}
}
