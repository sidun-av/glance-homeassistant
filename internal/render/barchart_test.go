package render

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

func TestBarChart_EmptyValuesReturnsBareSVG(t *testing.T) {
	svg := BarChart(BarChartData{}, DefaultBarChartOptions())
	if !contains(svg, "<svg") {
		t.Errorf("svg = %q, want it to contain <svg", svg)
	}
	if contains(svg, "<rect") {
		t.Errorf("svg = %q, want no bars for empty values", svg)
	}
}

func TestBarChart_RendersOneBarPerValue(t *testing.T) {
	svg := BarChart(BarChartData{Values: []float64{10, 15, 12, 20}, CurrentIndex: 3, CurrentLabel: "20.0°"}, BarChartOptions{Width: 220, Height: 60})
	// One <rect> per bar, plus the shared clip-path's own <rect> in <defs> — 5 total.
	if count := strings.Count(svg, "<rect"); count != 5 {
		t.Errorf("<rect> count = %d, want 5 (4 bars + 1 clipPath rect)", count)
	}
	if !contains(svg, "var(--color-progress-value)") {
		t.Errorf("svg = %q, want it to reference the theme's progress-value color variable", svg)
	}
}

func TestBarChart_SkipsBarForNaNValue(t *testing.T) {
	svg := BarChart(BarChartData{Values: []float64{10, math.NaN(), 12}}, BarChartOptions{Width: 220, Height: 60})
	if count := strings.Count(svg, "<rect"); count != 3 { // 2 real bars + 1 clipPath rect
		t.Errorf("<rect> count = %d, want 3 (2 bars, NaN produces no bar, + 1 clipPath rect)", count)
	}
}

func TestBarChart_IncludesCurrentValueLabel(t *testing.T) {
	svg := BarChart(BarChartData{Values: []float64{10, 20}, CurrentIndex: 1, CurrentLabel: "20.0°"}, BarChartOptions{Width: 220, Height: 60})
	if !contains(svg, "20.0") {
		t.Errorf("svg = %q, want it to contain the current value label", svg)
	}
	if !contains(svg, `class="ha-bar-label ha-bar-label-current"`) {
		t.Errorf("svg = %q, want the current label rendered as an ha-bar-label-current span, not SVG text", svg)
	}
	if contains(svg, "<text") {
		t.Errorf("svg = %q, want no SVG <text> elements (labels must be plain HTML to avoid non-uniform-scale distortion)", svg)
	}
}

func TestBarChart_NoCurrentLabelWhenCurrentIndexOutOfRange(t *testing.T) {
	svg := BarChart(BarChartData{Values: []float64{10, 20}, CurrentIndex: -1, CurrentLabel: "20.0°"}, BarChartOptions{Width: 220, Height: 60})
	if contains(svg, "ha-bar-label-current") {
		t.Errorf("svg = %q, want no current label when CurrentIndex is out of range", svg)
	}
}

func TestBarChart_EscapesCurrentValueLabel(t *testing.T) {
	svg := BarChart(BarChartData{Values: []float64{10}, CurrentIndex: 0, CurrentLabel: "<b>"}, BarChartOptions{Width: 220, Height: 60})
	if contains(svg, "<b>") {
		t.Errorf("svg = %q, want current value label HTML-escaped", svg)
	}
}

func TestBarChart_FlatSeriesDoesNotDivideByZero(t *testing.T) {
	svg := BarChart(BarChartData{Values: []float64{5, 5, 5}, CurrentIndex: 2, CurrentLabel: "5.0°"}, BarChartOptions{Width: 220, Height: 60})
	if contains(svg, "NaN") {
		t.Errorf("svg = %q, want no NaN for a flat series", svg)
	}
}

func TestBarChart_AppliesClassName(t *testing.T) {
	svg := BarChart(BarChartData{Values: []float64{10, 20}}, BarChartOptions{Width: 220, Height: 60, ClassName: "ha-room-chart"})
	if !contains(svg, `class="ha-room-chart"`) {
		t.Errorf("svg = %q, want class=\"ha-room-chart\"", svg)
	}
}

func TestBarChart_EmptyValuesStillAppliesClassName(t *testing.T) {
	svg := BarChart(BarChartData{}, BarChartOptions{ClassName: "ha-room-chart"})
	if !contains(svg, `class="ha-room-chart"`) {
		t.Errorf("svg = %q, want class=\"ha-room-chart\" even for empty values", svg)
	}
}

func TestBarChart_WrapsInPositionRelativeDiv(t *testing.T) {
	svg := BarChart(BarChartData{Values: []float64{10, 20}}, BarChartOptions{Width: 220, Height: 60})
	if !strings.HasPrefix(svg, `<div class="ha-bar-wrap">`) {
		t.Errorf("svg = %q, want it wrapped in a <div class=\"ha-bar-wrap\">", svg)
	}
}

// --- Daytime band ---

func TestBarChart_DaytimeBand_TwoSeparateRunsProduceTwoRects(t *testing.T) {
	svg := BarChart(BarChartData{
		Values:    []float64{10, 11, 12, 13, 14},
		IsDaytime: []bool{true, true, false, false, true},
	}, BarChartOptions{Width: 220, Height: 60})

	// 5 bars + 1 clipPath rect + 2 daytime rects = 8.
	count := strings.Count(svg, "<rect")
	if count != 8 {
		t.Errorf("<rect> count = %d, want 8 (5 bars + 1 clip + 2 separate daytime runs, must not merge into one spanning the night gap)", count)
	}
}

func TestBarChart_DaytimeBand_NoDaytimeProducesNoBandRect(t *testing.T) {
	svg := BarChart(BarChartData{
		Values:    []float64{10, 11, 12},
		IsDaytime: []bool{false, false, false},
	}, BarChartOptions{Width: 220, Height: 60})

	if contains(svg, `fill="var(--color-primary)"`) {
		t.Errorf("svg = %q, want no daytime-band rect when IsDaytime is all false", svg)
	}
}

func TestBarChart_DaytimeBand_NilIsDaytimeProducesNoBandRect(t *testing.T) {
	svg := BarChart(BarChartData{
		Values: []float64{10, 11, 12},
	}, BarChartOptions{Width: 220, Height: 60})

	if contains(svg, `fill="var(--color-primary)"`) {
		t.Errorf("svg = %q, want no daytime-band rect when IsDaytime is nil", svg)
	}
}

func TestBarChart_DaytimeBand_UsesThemeColorVariable(t *testing.T) {
	svg := BarChart(BarChartData{
		Values:    []float64{10, 11, 12},
		IsDaytime: []bool{true, true, true},
	}, BarChartOptions{Width: 220, Height: 60})

	if !contains(svg, `fill="var(--color-primary)"`) {
		t.Errorf("svg = %q, want the daytime band to fill with the theme's primary color variable, not a hardcoded hex", svg)
	}
}

// --- Min/max labels ---

func TestBarChart_MinMaxLabels_RenderBothWhenDistinctFromCurrent(t *testing.T) {
	// Current index 4 (12.0); min is 8.0 at index 1, max is 20.0 at index 3.
	svg := BarChart(BarChartData{
		Values:       []float64{10, 8, 15, 20, 12},
		CurrentIndex: 4,
		CurrentLabel: "12.0°",
	}, BarChartOptions{Width: 220, Height: 60})

	if !contains(svg, "8.0") {
		t.Errorf("svg = %q, want the min value (8.0) labeled", svg)
	}
	if !contains(svg, "20.0") {
		t.Errorf("svg = %q, want the max value (20.0) labeled", svg)
	}
	if strings.Count(svg, "ha-bar-label-secondary") != 2 {
		t.Errorf("svg = %q, want exactly 2 secondary (min+max) labels", svg)
	}
}

func TestBarChart_MinMaxLabels_SuppressedWhenCoincidingWithCurrentIndex(t *testing.T) {
	// Current index 4 IS the max (20.0) — must not double-label that bar.
	svg := BarChart(BarChartData{
		Values:       []float64{10, 8, 15, 12, 20},
		CurrentIndex: 4,
		CurrentLabel: "20.0°",
	}, BarChartOptions{Width: 220, Height: 60})

	if strings.Count(svg, "20.0") != 1 {
		t.Errorf("svg = %q, want the value 20.0 to appear exactly once (current label only, no duplicate max label on the same bar)", svg)
	}
}

func TestBarChart_MinMaxLabels_FlatSeriesRendersNeitherLabel(t *testing.T) {
	svg := BarChart(BarChartData{
		Values:       []float64{15, 15, 15},
		CurrentIndex: 2,
		CurrentLabel: "15.0°",
	}, BarChartOptions{Width: 220, Height: 60})

	if strings.Count(svg, "15.0") != 1 {
		t.Errorf("svg = %q, want 15.0 to appear exactly once (current label only) for a flat series", svg)
	}
}

func TestBarChart_MinMaxLabels_SkipNaNValues(t *testing.T) {
	// Index 3 (later today, NaN) must never be picked as min or max.
	svg := BarChart(BarChartData{
		Values:       []float64{10, 8, 20, math.NaN()},
		CurrentIndex: 2,
		CurrentLabel: "20.0°",
	}, BarChartOptions{Width: 220, Height: 60})

	if !contains(svg, "8.0") {
		t.Errorf("svg = %q, want the min value (8.0, ignoring the trailing NaN) labeled", svg)
	}
}

// --- Label positioning (percentage-based, not pixel/SVG-text) ---

func extractLabelLeftPercent(t *testing.T, svg, wantContent string) float64 {
	t.Helper()
	segments := strings.Split(svg, "<span")
	for _, seg := range segments[1:] {
		closeTag := strings.Index(seg, ">")
		closeElem := strings.Index(seg, "</span>")
		if closeTag == -1 || closeElem == -1 || closeElem < closeTag {
			continue
		}
		content := seg[closeTag+1 : closeElem]
		if !strings.Contains(content, wantContent) {
			continue
		}
		attrs := seg[:closeTag]
		idx := strings.Index(attrs, `left:`)
		if idx == -1 {
			t.Fatalf("<span> has no left: style: %q", attrs)
		}
		rest := attrs[idx+len("left:"):]
		end := strings.Index(rest, "%")
		v, err := strconv.ParseFloat(rest[:end], 64)
		if err != nil {
			t.Fatalf("parse left %% %q: %v", rest[:end], err)
		}
		return v
	}
	t.Fatalf("no <span> with content containing %q found in svg: %q", wantContent, svg)
	return 0
}

func TestBarChart_CurrentLabelNearRightEdgeIsNotClamped(t *testing.T) {
	// Percentage-based positioning never overflows a viewBox the way
	// unclamped SVG text x coordinates used to (the bug this chart used
	// to have) — a label at the last bar should sit close to 100%, not be
	// artificially pulled inward, since CSS (not this Go code) is
	// responsible for keeping span text on-screen via its own layout.
	values := make([]float64, 60)
	for i := range values {
		values[i] = 10 + float64(i)
	}
	svg := BarChart(BarChartData{Values: values, CurrentIndex: 59, CurrentLabel: "18.0°"}, BarChartOptions{Width: 220, Height: 60})

	pct := extractLabelLeftPercent(t, svg, "18.0")
	if pct < 90 {
		t.Errorf("current label left = %.2f%%, want it near the right edge (>=90%%) for the last of 60 bars", pct)
	}
}

func TestBarChart_CurrentBarIsWiderThanOthers(t *testing.T) {
	svg := BarChart(BarChartData{
		Values:       []float64{10, 12, 14, 16, 18},
		CurrentIndex: 4,
		CurrentLabel: "18.0°",
	}, BarChartOptions{Width: 220, Height: 60})

	rects := strings.Split(svg, "<rect")
	// rects[0] is pre-first-<rect>; rects[1] is the clipPath's own rect
	// (emitted before the bars); rects[2..6] are the 5 bars in order.
	if len(rects) != 7 {
		t.Fatalf("expected 1 clip rect + 5 bar rects, got %d <rect> fragments", len(rects)-1)
	}

	extractWidth := func(fragment string) float64 {
		idx := strings.Index(fragment, `width="`)
		if idx == -1 {
			t.Fatalf("no width found in fragment: %q", fragment)
		}
		rest := fragment[idx+len(`width="`):]
		end := strings.Index(rest, `"`)
		w, err := strconv.ParseFloat(rest[:end], 64)
		if err != nil {
			t.Fatalf("parse width %q: %v", rest[:end], err)
		}
		return w
	}

	firstBarWidth := extractWidth(rects[2])
	lastBarWidth := extractWidth(rects[6])
	if lastBarWidth <= firstBarWidth {
		t.Errorf("current (last) bar width = %v, want it wider than a non-current bar's %v", lastBarWidth, firstBarWidth)
	}
}

func TestBarChart_CurrentBarIsFullyOpaqueOthersAreFlatDimmer(t *testing.T) {
	svg := BarChart(BarChartData{
		Values:       []float64{10, 12, 14},
		CurrentIndex: 2,
		CurrentLabel: "14.0°",
	}, BarChartOptions{Width: 220, Height: 60})

	// Two-tier, not a ramp: exactly one bar at full opacity (the current
	// one), the rest all share the SAME dimmer value as each other.
	if strings.Count(svg, `fill-opacity="1"`) != 1 {
		t.Errorf("svg = %q, want exactly one fill-opacity=\"1\" (the current bar)", svg)
	}
	if strings.Count(svg, `fill-opacity="0.55"`) != 2 {
		t.Errorf("svg = %q, want the other 2 bars sharing one flat dimmer opacity (0.55), not a ramp", svg)
	}
}

func TestBarChart_ShortBarStillReadsAsARectNotACircle(t *testing.T) {
	// The bug this guards against: stroke-linecap="round" on a <line>
	// made a very short bar (near the minimum value in a series with a
	// wide range) render as a near-circular dot instead of a recognizable
	// short bar. A <rect> with height meaningfully greater than its
	// corner radius avoids that regardless of value.
	svg := BarChart(BarChartData{
		Values:       []float64{10, 1000}, // huge span forces index 0 to the height floor
		CurrentIndex: 1,
		CurrentLabel: "1000.0°",
	}, BarChartOptions{Width: 220, Height: 60})

	if !contains(svg, "<rect") {
		t.Fatalf("expected <rect>-based bars, svg: %q", svg)
	}
	// Just confirm no stroke-linecap remains anywhere (the old failure mode).
	if contains(svg, "stroke-linecap") {
		t.Errorf("svg = %q, want no stroke-linecap (bars are rects now, not round-capped lines)", svg)
	}
}
