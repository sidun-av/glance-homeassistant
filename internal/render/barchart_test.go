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
