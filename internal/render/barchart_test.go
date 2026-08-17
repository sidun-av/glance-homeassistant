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

// TestBarColumns_EveryColumnHasSameChildElementCount is the regression test
// for the "bars look misaligned when some columns have an hour label and
// others don't" bug: .ha-bar-col uses justify-content:flex-end, which packs
// each column's children as one contiguous group flush to the bottom. The
// time-label div used to be omitted entirely from the DOM for columns with
// no label text, so a labeled column had one more child than an unlabeled
// one and flex-end pushed its bar higher — an artifact of DOM structure,
// not real data. Every column must now emit the exact same number of child
// elements regardless of whether it's a "labeled" column, with visibility
// controlled purely via CSS.
func TestBarColumns_EveryColumnHasSameChildElementCount(t *testing.T) {
	// 12 columns, sparse labels every 4th (mirrors barColumnTimeLabels in
	// main.go), plus a NaN gap to also cover the empty-bar branch.
	timeLabels := []string{"12am", "", "", "", "8am", "", "", "", "5pm", "", "", ""}
	values := []float64{10, 11, 12, math.NaN(), 14, 15, 16, 17, 18, 19, 20, 21}
	html := BarColumns(BarChartData{
		Values: values, TimeLabels: timeLabels, CurrentIndex: 8, CurrentLabel: "18.0°",
	}, "ha-bar-cols")

	const colOpenPrefix = `<div class="ha-bar-col" data-current=`
	parts := strings.Split(html, colOpenPrefix)
	if len(parts) != len(values)+1 {
		t.Fatalf("got %d column parts, want %d (%d columns + preamble): %s", len(parts), len(values)+1, len(values), html)
	}

	counts := make([]int, len(values))
	for i, part := range parts[1:] {
		counts[i] = strings.Count(part, "<div")
	}
	for i, c := range counts {
		if c != counts[0] {
			t.Errorf("column %d has %d child <div> elements, want %d (same as column 0) — labeled and unlabeled columns must share identical DOM shape:\n%s", i, c, counts[0], html)
		}
	}
	if counts[0] != 3 {
		t.Errorf("child <div> count per column = %d, want 3 (value, bar, time-label div always present)", counts[0])
	}

	// The time-label div must be present, just empty, for unlabeled
	// columns — never omitted from the DOM.
	if got := strings.Count(html, `class="ha-bar-col-time"`); got != len(values)-3 {
		t.Errorf("plain (hidden) ha-bar-col-time count = %d, want %d", got, len(values)-3)
	}
	if got := strings.Count(html, `class="ha-bar-col-time ha-bar-col-time-visible"`); got != 3 {
		t.Errorf("visible ha-bar-col-time count = %d, want 3 (indices 0, 4, 8)", got)
	}
}

// --- Daytime band ---

// TestBarColumns_DaytimeBand_GapProducesTwoSeparateSeamlessRuns is the
// regression test for the "highlight band has gaps" bug: the daylight
// highlight used to be rendered per-column (nested inside each
// .ha-bar-col), so .ha-bar-cols' flex `gap` between adjacent columns left a
// visible unhighlighted seam at every boundary within what should read as
// one continuous run. It's now rendered as one container-level div per
// contiguous run of daytime columns (positioned with left/right spanning
// the whole run, gaps included), so a single gap in the pattern must
// produce exactly two of those divs, each with distinct left/right, and an
// unbroken run must produce exactly one seamless div — never one div per
// column and never one div spanning the gap.
func TestBarColumns_DaytimeBand_GapProducesTwoSeparateSeamlessRuns(t *testing.T) {
	// 9 columns; daytime at [2,3,4], a gap at 5, daytime again at [6,7,8].
	isDaytime := make([]bool, 9)
	for _, i := range []int{2, 3, 4, 6, 7, 8} {
		isDaytime[i] = true
	}
	values := make([]float64, 9)
	for i := range values {
		values[i] = float64(10 + i)
	}
	html := BarColumns(BarChartData{Values: values, IsDaytime: isDaytime}, "ha-bar-cols")

	if count := strings.Count(html, `class="ha-bar-daylight"`); count != 2 {
		t.Fatalf("daylight div count = %d, want exactly 2 (one per contiguous run, not one per column or one spanning the gap): %s", count, html)
	}

	// Run 1: columns 2..4 of 9 -> left 2/9*100=22.2222%, right (9-4-1)/9*100=44.4444%.
	if !strings.Contains(html, `style="left:22.2222%;right:44.4444%"`) {
		t.Errorf("html = %q, want the first run positioned left:22.2222%%;right:44.4444%%", html)
	}
	// Run 2: columns 6..8 of 9 -> left 6/9*100=66.6667%, right (9-8-1)/9*100=0.0000%.
	if !strings.Contains(html, `style="left:66.6667%;right:0.0000%"`) {
		t.Errorf("html = %q, want the second run positioned left:66.6667%%;right:0.0000%%", html)
	}

	// A single unbroken run must collapse into exactly one seamless div.
	unbroken := BarColumns(BarChartData{
		Values:    []float64{10, 11, 12, 13, 14},
		IsDaytime: []bool{false, true, true, true, false},
	}, "ha-bar-cols")
	if count := strings.Count(unbroken, `class="ha-bar-daylight"`); count != 1 {
		t.Errorf("daylight div count = %d, want exactly 1 for a single unbroken run: %s", count, unbroken)
	}

	// The old per-column rounding modifier classes must be gone — rounding
	// is now baked into the base .ha-bar-daylight rule (see styleBlock)
	// since each div now IS one whole run.
	if strings.Contains(html, "ha-bar-daylight-start") || strings.Contains(html, "ha-bar-daylight-end") {
		t.Errorf("html = %q, want no per-column daylight rounding modifier classes", html)
	}

	// The daylight divs must not be nested inside a .ha-bar-col (i.e. they
	// must come before any .ha-bar-col in the markup, as container-level
	// siblings, not per-column children).
	if idx := strings.Index(html, `class="ha-bar-daylight"`); idx == -1 || idx > strings.Index(html, `class="ha-bar-col"`) {
		t.Errorf("html = %q, want daylight run divs to appear before (i.e. not nested inside) any .ha-bar-col div", html)
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
