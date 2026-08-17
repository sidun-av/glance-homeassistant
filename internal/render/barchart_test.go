package render

import (
	"math"
	"strings"
	"testing"
)

func TestBarColumns_EmptyValuesReturnsEmptyWrapper(t *testing.T) {
	html := BarColumns(BarChartData{}, "ha-bar-cols")
	if !strings.HasPrefix(html, `<div class="ha-bar-cols">`) {
		t.Errorf("html = %q, want it to start with the wrapper div", html)
	}
	if strings.Count(html, `class="ha-bar-col"`)+strings.Count(html, `data-current`) > 0 {
		t.Errorf("html = %q, want no columns for empty values", html)
	}
}

func TestBarColumns_RendersOneColumnPerValue(t *testing.T) {
	html := BarColumns(BarChartData{Values: []float64{10, 15, 12, 20}, CurrentIndex: 3, CurrentLabel: "20.0°"}, "ha-bar-cols")
	if count := strings.Count(html, `class="ha-bar-col"`); count != 4 {
		t.Errorf("column count = %d, want 4", count)
	}
	if strings.Count(html, `<div class="ha-bar `) != 1 || strings.Count(html, `class="ha-bar"`) != 3 {
		t.Errorf("html = %q, want 3 plain bars + 1 current bar (ha-bar ha-bar-current)", html)
	}
}

func TestBarColumns_SkipsBarForNaNValue(t *testing.T) {
	html := BarColumns(BarChartData{Values: []float64{10, math.NaN(), 12}}, "ha-bar-cols")
	if strings.Count(html, `class="ha-bar-col"`) != 3 {
		t.Errorf("want 3 columns total even with a NaN gap")
	}
	if strings.Count(html, "ha-bar-empty") != 1 {
		t.Errorf("html = %q, want exactly 1 empty-bar column for the NaN value", html)
	}
}

func TestBarColumns_IncludesCurrentValueLabel(t *testing.T) {
	html := BarColumns(BarChartData{Values: []float64{10, 20}, CurrentIndex: 1, CurrentLabel: "20.0°"}, "ha-bar-cols")
	if !strings.Contains(html, "20.0") {
		t.Errorf("html = %q, want it to contain the current value label", html)
	}
	if !strings.Contains(html, "ha-bar-value-current") {
		t.Errorf("html = %q, want the current label to carry the ha-bar-value-current class", html)
	}
	if strings.Contains(html, "<svg") || strings.Contains(html, "<text") {
		t.Errorf("html = %q, want no SVG at all — this is a plain-HTML chart now", html)
	}
}

func TestBarColumns_NoCurrentLabelWhenCurrentIndexOutOfRange(t *testing.T) {
	html := BarColumns(BarChartData{Values: []float64{10, 20}, CurrentIndex: -1, CurrentLabel: "20.0°"}, "ha-bar-cols")
	if strings.Contains(html, "ha-bar-value-current") {
		t.Errorf("html = %q, want no current label when CurrentIndex is out of range", html)
	}
	if strings.Contains(html, `data-current="true"`) {
		t.Errorf("html = %q, want no column marked data-current=\"true\"", html)
	}
}

func TestBarColumns_EscapesCurrentValueLabel(t *testing.T) {
	html := BarColumns(BarChartData{Values: []float64{10}, CurrentIndex: 0, CurrentLabel: "<b>"}, "ha-bar-cols")
	if strings.Contains(html, "<b>") {
		t.Errorf("html = %q, want current value label HTML-escaped", html)
	}
}

func TestBarColumns_FlatSeriesDoesNotDivideByZero(t *testing.T) {
	html := BarColumns(BarChartData{Values: []float64{5, 5, 5}, CurrentIndex: 2, CurrentLabel: "5.0°"}, "ha-bar-cols")
	if strings.Contains(html, "NaN") {
		t.Errorf("html = %q, want no NaN for a flat series", html)
	}
}

func TestBarColumns_AppliesClassName(t *testing.T) {
	html := BarColumns(BarChartData{Values: []float64{10, 20}}, "custom-class")
	if !strings.HasPrefix(html, `<div class="custom-class">`) {
		t.Errorf("html = %q, want it to start with the given class name", html)
	}
}

// --- Daytime band ---

func TestBarColumns_DaytimeBand_TwoSeparateRunsHaveDistinctEdges(t *testing.T) {
	// Daytime columns: 0, 1 (one run of 2) and 4 (its own isolated run).
	html := BarColumns(BarChartData{
		Values:    []float64{10, 11, 12, 13, 14},
		IsDaytime: []bool{true, true, false, false, true},
	}, "ha-bar-cols")

	// One daylight div per daytime COLUMN (3 total: indices 0, 1, 4), not
	// one per run — matching Weather's own per-column structure.
	if count := strings.Count(html, `<div class="ha-bar-daylight`); count != 3 {
		t.Errorf("daylight div count = %d, want 3 (one per daytime column)", count)
	}
	// Column 0 (run start) and column 4 (isolated, both edges) get -start;
	// column 1 (run end) and column 4 get -end.
	if strings.Count(html, "ha-bar-daylight-start") != 2 {
		t.Errorf("html = %q, want 2 columns carrying -start (run-start column 0, isolated column 4)", html)
	}
	if strings.Count(html, "ha-bar-daylight-end") != 2 {
		t.Errorf("html = %q, want 2 columns carrying -end (run-end column 1, isolated column 4)", html)
	}
}

func TestBarColumns_DaytimeBand_NoDaytimeProducesNoDaylightDiv(t *testing.T) {
	html := BarColumns(BarChartData{
		Values:    []float64{10, 11, 12},
		IsDaytime: []bool{false, false, false},
	}, "ha-bar-cols")

	if strings.Contains(html, "ha-bar-daylight") {
		t.Errorf("html = %q, want no daylight div when IsDaytime is all false", html)
	}
}

func TestBarColumns_DaytimeBand_NilIsDaytimeProducesNoDaylightDiv(t *testing.T) {
	html := BarColumns(BarChartData{
		Values: []float64{10, 11, 12},
	}, "ha-bar-cols")

	if strings.Contains(html, "ha-bar-daylight") {
		t.Errorf("html = %q, want no daylight div when IsDaytime is nil", html)
	}
}

// --- Min/max labels ---

func TestBarColumns_MinMaxLabels_RenderBothWhenDistinctFromCurrent(t *testing.T) {
	html := BarColumns(BarChartData{
		Values:       []float64{10, 8, 15, 20, 12},
		CurrentIndex: 4,
		CurrentLabel: "12.0°",
	}, "ha-bar-cols")

	if !strings.Contains(html, "8.0") {
		t.Errorf("html = %q, want the min value (8.0) labeled", html)
	}
	if !strings.Contains(html, "20.0") {
		t.Errorf("html = %q, want the max value (20.0) labeled", html)
	}
}

func TestBarColumns_MinMaxLabels_SuppressedWhenCoincidingWithCurrentIndex(t *testing.T) {
	// Current index 4 IS the max (20.0) — must not double-label that column.
	html := BarColumns(BarChartData{
		Values:       []float64{10, 8, 15, 12, 20},
		CurrentIndex: 4,
		CurrentLabel: "20.0°",
	}, "ha-bar-cols")

	if strings.Count(html, "20.0") != 1 {
		t.Errorf("html = %q, want the value 20.0 to appear exactly once (current label only)", html)
	}
}

func TestBarColumns_MinMaxLabels_FlatSeriesRendersNeitherLabel(t *testing.T) {
	html := BarColumns(BarChartData{
		Values:       []float64{15, 15, 15},
		CurrentIndex: 2,
		CurrentLabel: "15.0°",
	}, "ha-bar-cols")

	if strings.Count(html, "15.0") != 1 {
		t.Errorf("html = %q, want 15.0 to appear exactly once (current label only) for a flat series", html)
	}
}

func TestBarColumns_MinMaxLabels_SkipNaNValues(t *testing.T) {
	// Index 3 (later today, NaN) must never be picked as min or max.
	html := BarColumns(BarChartData{
		Values:       []float64{10, 8, 20, math.NaN()},
		CurrentIndex: 2,
		CurrentLabel: "20.0°",
	}, "ha-bar-cols")

	if !strings.Contains(html, "8.0") {
		t.Errorf("html = %q, want the min value (8.0, ignoring the trailing NaN) labeled", html)
	}
}

func TestBarColumns_CurrentBarHeightUsesCSSCustomProperty(t *testing.T) {
	html := BarColumns(BarChartData{
		Values:       []float64{10, 12, 14, 16, 18},
		CurrentIndex: 4,
		CurrentLabel: "18.0°",
	}, "ha-bar-cols")

	if !strings.Contains(html, "--ha-bar-height:1.000") {
		t.Errorf("html = %q, want the max-value (current) bar's height custom property at 1.000", html)
	}
	if !strings.Contains(html, "--ha-bar-height:0.000") {
		t.Errorf("html = %q, want the min-value bar's height custom property at 0.000", html)
	}
}
