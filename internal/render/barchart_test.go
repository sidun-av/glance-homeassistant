package render

import (
	"strconv"
	"strings"
	"testing"
)

func TestBarChart_EmptyValuesReturnsBareSVG(t *testing.T) {
	svg := BarChart(BarChartData{}, DefaultBarChartOptions())
	if !contains(svg, "<svg") {
		t.Errorf("svg = %q, want it to contain <svg", svg)
	}
	if contains(svg, "<line") {
		t.Errorf("svg = %q, want no bars for empty values", svg)
	}
}

func TestBarChart_RendersOneBarPerValue(t *testing.T) {
	svg := BarChart(BarChartData{Values: []float64{10, 15, 12, 20}, CurrentLabel: "20.0°"}, BarChartOptions{Width: 220, Height: 60})
	if count := strings.Count(svg, "<line"); count != 4 {
		t.Errorf("bar (<line>) count = %d, want 4", count)
	}
	if !contains(svg, "var(--color-progress-value)") {
		t.Errorf("svg = %q, want it to reference the theme's progress-value color variable", svg)
	}
}

func TestBarChart_IncludesCurrentValueLabel(t *testing.T) {
	svg := BarChart(BarChartData{Values: []float64{10, 20}, CurrentLabel: "20.0°"}, BarChartOptions{Width: 220, Height: 60})
	if !contains(svg, "20.0") {
		t.Errorf("svg = %q, want it to contain the current value label", svg)
	}
}

func TestBarChart_EscapesCurrentValueLabel(t *testing.T) {
	svg := BarChart(BarChartData{Values: []float64{10}, CurrentLabel: "<b>"}, BarChartOptions{Width: 220, Height: 60})
	if contains(svg, "<b>") {
		t.Errorf("svg = %q, want current value label HTML-escaped", svg)
	}
}

func TestBarChart_FlatSeriesDoesNotDivideByZero(t *testing.T) {
	svg := BarChart(BarChartData{Values: []float64{5, 5, 5}, CurrentLabel: "5.0°"}, BarChartOptions{Width: 220, Height: 60})
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

// --- New behavior: daytime band, min/max labels, wider current bar ---

func TestBarChart_DaytimeBand_TwoSeparateRunsProduceTwoRects(t *testing.T) {
	svg := BarChart(BarChartData{
		Values:    []float64{10, 11, 12, 13, 14},
		IsDaytime: []bool{true, true, false, false, true},
	}, BarChartOptions{Width: 220, Height: 60})

	count := strings.Count(svg, "<rect")
	if count != 2 {
		t.Errorf("<rect> count = %d, want 2 (two separate daytime runs must not merge into one rect spanning the night gap)", count)
	}
}

func TestBarChart_DaytimeBand_NoDaytimeProducesNoRect(t *testing.T) {
	svg := BarChart(BarChartData{
		Values:    []float64{10, 11, 12},
		IsDaytime: []bool{false, false, false},
	}, BarChartOptions{Width: 220, Height: 60})

	if contains(svg, "<rect") {
		t.Errorf("svg = %q, want no <rect> when IsDaytime is all false", svg)
	}
}

func TestBarChart_DaytimeBand_NilIsDaytimeProducesNoRect(t *testing.T) {
	svg := BarChart(BarChartData{
		Values: []float64{10, 11, 12},
	}, BarChartOptions{Width: 220, Height: 60})

	if contains(svg, "<rect") {
		t.Errorf("svg = %q, want no <rect> when IsDaytime is nil", svg)
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

func TestBarChart_MinMaxLabels_RenderBothWhenDistinctFromCurrent(t *testing.T) {
	// Current (last) index is 12.0; min is 8.0 at index 1, max is 20.0 at index 3.
	svg := BarChart(BarChartData{
		Values:       []float64{10, 8, 15, 20, 12},
		CurrentLabel: "12.0°",
	}, BarChartOptions{Width: 220, Height: 60})

	if !contains(svg, "8.0") {
		t.Errorf("svg = %q, want the min value (8.0) labeled", svg)
	}
	if !contains(svg, "20.0") {
		t.Errorf("svg = %q, want the max value (20.0) labeled", svg)
	}
}

func TestBarChart_MinMaxLabels_SuppressedWhenCoincidingWithCurrentIndex(t *testing.T) {
	// Current (last) index IS the max (20.0) — must not double-label that bar.
	svg := BarChart(BarChartData{
		Values:       []float64{10, 8, 15, 12, 20},
		CurrentLabel: "20.0°",
	}, BarChartOptions{Width: 220, Height: 60})

	if strings.Count(svg, "20.0") != 1 {
		t.Errorf("svg = %q, want the value 20.0 to appear exactly once (current label only, no duplicate max label on the same bar)", svg)
	}
}

func TestBarChart_MinMaxLabels_FlatSeriesRendersNeitherLabel(t *testing.T) {
	// min == max == every value — no meaningful min/max distinction to
	// call out, and min-index == max-index would otherwise double-render.
	svg := BarChart(BarChartData{
		Values:       []float64{15, 15, 15},
		CurrentLabel: "15.0°",
	}, BarChartOptions{Width: 220, Height: 60})

	if strings.Count(svg, "15.0") != 1 {
		t.Errorf("svg = %q, want 15.0 to appear exactly once (current label only) for a flat series", svg)
	}
}

// manyValues builds an n-point series ramping from start to start+n-1,
// matching the shape of a real room card's default 60-point window (see
// main.go's Temperature.MaxPoints default) — the bug this guards against
// only manifests with enough points to make each bar's step narrow
// relative to a label's text width; a handful of wide bars leaves too
// much margin to reproduce it.
func manyValues(n int, start float64) []float64 {
	values := make([]float64, n)
	for i := range values {
		values[i] = start + float64(i)
	}
	return values
}

func TestBarChart_CurrentLabelNearRightEdgeStaysWithinViewBox(t *testing.T) {
	// The current bar is always the LAST value — its label's x (with
	// text-anchor="middle") sits near the viewBox's right edge, close
	// enough with a realistic (60-point) bar count that an unclamped
	// position lets roughly half the text render outside 0..Width.
	// Confirmed visually against the live deployed widget: an unclamped
	// "23.8°" rendered as a clipped "23." at the card's right edge.
	svg := BarChart(BarChartData{
		Values:       manyValues(60, 10),
		CurrentLabel: "18.0°",
	}, BarChartOptions{Width: 220, Height: 60})

	x := extractLastTextX(t, svg, "18.0")
	const rightMargin = 14.0
	if x > 220-rightMargin {
		t.Errorf("current label x = %.2f, want <= %.2f (220 width minus a safe margin) so text-anchor=\"middle\" text doesn't overflow the viewBox's right edge", x, 220-rightMargin)
	}
}

func TestBarChart_MinLabelNearLeftEdgeStaysWithinViewBox(t *testing.T) {
	// Symmetric case: the minimum value at index 0 sits at the viewBox's
	// left edge, with the same 60-point realism requirement.
	values := manyValues(60, 10)
	values[0] = 1 // force index 0 to be the minimum
	svg := BarChart(BarChartData{
		Values:       values,
		CurrentLabel: "999.0°", // keep current (last) index far from 1.0 so it isn't suppressed as a min-index coincidence
	}, BarChartOptions{Width: 220, Height: 60})

	x := extractLastTextX(t, svg, "1.0")
	const leftMargin = 14.0
	if x < leftMargin {
		t.Errorf("min label x = %.2f, want >= %.2f so text-anchor=\"middle\" text doesn't overflow the viewBox's left edge", x, leftMargin)
	}
}

// extractLastTextX finds the <text ...>CONTENT</text> element whose
// CONTENT (not its attributes, and not any earlier/unrelated SVG markup)
// contains want, and returns that element's x attribute. Fails the test
// if no matching element is found.
func extractLastTextX(t *testing.T, svg, want string) float64 {
	t.Helper()
	// segments[0] is everything before the first <text — never a real
	// text element, and may itself contain "want" as a coincidental
	// substring of some unrelated coordinate/attribute value, so it's
	// always skipped.
	segments := strings.Split(svg, "<text")
	for _, seg := range segments[1:] {
		closeTag := strings.Index(seg, ">")
		closeElem := strings.Index(seg, "</text>")
		if closeTag == -1 || closeElem == -1 || closeElem < closeTag {
			continue
		}
		content := seg[closeTag+1 : closeElem]
		if !strings.Contains(content, want) {
			continue
		}
		attrs := seg[:closeTag]
		idx := strings.Index(attrs, `x="`)
		if idx == -1 {
			t.Fatalf("<text> element has no x attribute: %q", attrs)
		}
		rest := attrs[idx+len(`x="`):]
		end := strings.Index(rest, `"`)
		x, err := strconv.ParseFloat(rest[:end], 64)
		if err != nil {
			t.Fatalf("parse x %q: %v", rest[:end], err)
		}
		return x
	}
	t.Fatalf("no <text> element with content containing %q found in svg: %q", want, svg)
	return 0
}

func TestBarChart_CurrentBarIsWiderThanOthers(t *testing.T) {
	svg := BarChart(BarChartData{
		Values:       []float64{10, 12, 14, 16, 18},
		CurrentLabel: "18.0°",
	}, BarChartOptions{Width: 220, Height: 60})

	lines := strings.Split(svg, "<line")
	if len(lines) != 6 { // lines[0] is the pre-first-<line> prefix
		t.Fatalf("expected 5 <line> elements, got %d", len(lines)-1)
	}

	extractWidth := func(fragment string) float64 {
		idx := strings.Index(fragment, `stroke-width="`)
		if idx == -1 {
			t.Fatalf("no stroke-width found in fragment: %q", fragment)
		}
		rest := fragment[idx+len(`stroke-width="`):]
		end := strings.Index(rest, `"`)
		w, err := strconv.ParseFloat(rest[:end], 64)
		if err != nil {
			t.Fatalf("parse stroke-width %q: %v", rest[:end], err)
		}
		return w
	}

	firstWidth := extractWidth(lines[1])
	lastWidth := extractWidth(lines[5])
	if lastWidth <= firstWidth {
		t.Errorf("current (last) bar stroke-width = %v, want it wider than a non-current bar's %v", lastWidth, firstWidth)
	}
}
