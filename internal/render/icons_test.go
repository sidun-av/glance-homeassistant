package render

import "testing"

func TestLightIcon_KnownIconRendersMatchingGlyph(t *testing.T) {
	svg := LightIcon("mdi:track-light")
	if !contains(svg, `class="track-light"`) {
		t.Errorf("svg = %q, want a track-light glyph", svg)
	}
}

func TestLightIcon_LedStripVariant(t *testing.T) {
	svg := LightIcon("mdi:led-strip-variant")
	if !contains(svg, `class="led-strip"`) {
		t.Errorf("svg = %q, want a led-strip glyph", svg)
	}
}

func TestLightIcon_UnknownIconFallsBackToBulb(t *testing.T) {
	svg := LightIcon("mdi:alarm-off")
	if !contains(svg, `class="bulb"`) {
		t.Errorf("svg = %q, want fallback to bulb for an unrecognized icon", svg)
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
