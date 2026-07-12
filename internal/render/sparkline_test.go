package render

import "testing"

func TestSparkline_EmptyValuesReturnsBareSVG(t *testing.T) {
	svg := Sparkline(nil, DefaultSparklineOptions())
	if !contains(svg, "<svg") {
		t.Errorf("svg = %q, want it to contain <svg", svg)
	}
	if contains(svg, "polyline") {
		t.Errorf("svg = %q, want no polyline for empty values", svg)
	}
}

func TestSparkline_RendersOnePointPerValue(t *testing.T) {
	svg := Sparkline([]float64{1, 2, 3, 2, 1}, SparklineOptions{Width: 100, Height: 20})
	if !contains(svg, "polyline") {
		t.Errorf("svg = %q, want a polyline", svg)
	}
	if !contains(svg, "var(--color-progress-value)") {
		t.Errorf("svg = %q, want it to reference the theme's progress-value color variable", svg)
	}
}

func TestSparkline_PolylineHasNonScalingStroke(t *testing.T) {
	// This SVG uses preserveAspectRatio="none" (see the doc comment on
	// Sparkline) so it can stretch to fill whatever box its flex-grown
	// room card gives it — but that scales X and Y by different factors,
	// which makes a plain stroke-width look uneven along the polyline:
	// each segment's apparent thickness after that non-uniform scaling
	// depends on its slope, most noticeable with small, jagged
	// temperature steps. vector-effect="non-scaling-stroke" draws the
	// stroke in device pixels instead, independent of that scaling.
	svg := Sparkline([]float64{1, 2, 3, 2, 1}, SparklineOptions{Width: 100, Height: 20})
	if !contains(svg, `vector-effect="non-scaling-stroke"`) {
		t.Errorf("svg = %q, want the polyline to have vector-effect=\"non-scaling-stroke\"", svg)
	}
}

func TestSparkline_FlatSeriesDoesNotDivideByZero(t *testing.T) {
	svg := Sparkline([]float64{5, 5, 5, 5}, SparklineOptions{Width: 100, Height: 20})
	if contains(svg, "NaN") {
		t.Errorf("svg = %q, want no NaN for a flat series", svg)
	}
}

func TestSparkline_SinglePointDoesNotPanic(t *testing.T) {
	svg := Sparkline([]float64{42}, SparklineOptions{Width: 100, Height: 20})
	if !contains(svg, "<svg") {
		t.Errorf("svg = %q, want it to contain <svg", svg)
	}
}

func TestSparkline_AppliesClassName(t *testing.T) {
	svg := Sparkline([]float64{1, 2, 3}, SparklineOptions{Width: 100, Height: 20, ClassName: "ha-room-chart"})
	if !contains(svg, `class="ha-room-chart"`) {
		t.Errorf("svg = %q, want class=\"ha-room-chart\"", svg)
	}
}

func TestSparkline_EmptyValuesStillAppliesClassName(t *testing.T) {
	svg := Sparkline(nil, SparklineOptions{ClassName: "ha-room-chart"})
	if !contains(svg, `class="ha-room-chart"`) {
		t.Errorf("svg = %q, want class=\"ha-room-chart\" even for empty values", svg)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (func() bool {
		for i := 0; i+len(substr) <= len(s); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()
}
