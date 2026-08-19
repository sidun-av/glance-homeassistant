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
	if countColumns(html) != 0 {
		t.Errorf("html = %q, want no columns for empty values", html)
	}
}

func TestBarColumns_RendersOneColumnPerValue(t *testing.T) {
	html := BarColumns(BarChartData{Values: []float64{10, 15, 12, 20}, CurrentIndex: 3}, "ha-bar-cols")
	if count := countColumns(html); count != 4 {
		t.Errorf("column count = %d, want 4", count)
	}
	if count := strings.Count(html, `class="ha-bar-col-current"`); count != 0 {
		t.Errorf("html = %q, want ha-bar-col-current only as a second class on .ha-bar-col", html)
	}
	if count := strings.Count(html, `class="ha-bar-col ha-bar-col-current"`); count != 1 {
		t.Errorf("html = %q, want exactly 1 current column", html)
	}
}

// TestBarColumns_WrapperCarriesColumnCount pins the custom property the CSS
// needs to size each column as 100%/N (Weather hardcodes /12; this chart
// stays generic over len(Values)).
func TestBarColumns_WrapperCarriesColumnCount(t *testing.T) {
	html := BarColumns(BarChartData{Values: []float64{10, 15, 12}}, "ha-bar-cols")
	if !strings.HasPrefix(html, `<div class="ha-bar-cols" style="--ha-bar-cols:3">`) {
		t.Errorf("html = %q, want the wrapper to carry --ha-bar-cols:3", html)
	}
}

func TestBarColumns_SkipsBarForNaNValue(t *testing.T) {
	html := BarColumns(BarChartData{Values: []float64{10, math.NaN(), 12}}, "ha-bar-cols")
	if countColumns(html) != 3 {
		t.Errorf("want 3 columns total even with a NaN gap")
	}
	if strings.Count(html, "ha-bar-empty") != 1 {
		t.Errorf("html = %q, want exactly 1 empty-bar column for the NaN value", html)
	}
}

// TestBarColumns_EveryColumnGetsAValueLabel is the port's headline change:
// Weather renders all twelve values and hides them with opacity, revealing
// the current one always and any other on hover. The old scheme labelled
// only current/min/max, which is what left stray numbers floating over the
// chart and made it read unlike the widget it's modelled on.
func TestBarColumns_EveryColumnGetsAValueLabel(t *testing.T) {
	html := BarColumns(BarChartData{Values: []float64{10, 15, 12, 20}, CurrentIndex: 3}, "ha-bar-cols")
	if got := strings.Count(html, `class="ha-bar-value"`); got != 4 {
		t.Errorf("value-label count = %d, want 4 (one per column): %s", got, html)
	}
	for _, want := range []string{">10<", ">15<", ">12<", ">20<"} {
		if !strings.Contains(html, want) {
			t.Errorf("html = %q, want it to contain the value label %q", html, want)
		}
	}
	// No per-column "current"/"min"/"max" label variants survive — emphasis
	// is a column-level class now, exactly like Weather's.
	if strings.Contains(html, "ha-bar-value-current") {
		t.Errorf("html = %q, want no ha-bar-value-current class (superseded by .ha-bar-col-current)", html)
	}
}

// TestBarColumns_ValuesAreAbsoluteIntegersWithNoDegreeSign pins Weather's
// own formatting: the digits alone go in the div (absInt in weather.html),
// with the sign and the "°" supplied by CSS ::before/::after so the digits
// stay optically centered over their bar.
func TestBarColumns_ValuesAreAbsoluteIntegersWithNoDegreeSign(t *testing.T) {
	html := BarColumns(BarChartData{Values: []float64{-3.4, 21.6}, CurrentIndex: 1}, "ha-bar-cols")
	if strings.Contains(html, "°") {
		t.Errorf("html = %q, want no degree sign in the markup (CSS ::after supplies it)", html)
	}
	if strings.Contains(html, ">-3<") || strings.Contains(html, "-3.4") {
		t.Errorf("html = %q, want the negative value rendered as its absolute integer", html)
	}
	if !strings.Contains(html, `class="ha-bar-value ha-bar-value-negative">3<`) {
		t.Errorf("html = %q, want the negative column flagged for the CSS '-' pseudo-element", html)
	}
	if !strings.Contains(html, `class="ha-bar-value">22<`) {
		t.Errorf("html = %q, want 21.6 rounded to 22", html)
	}
}

func TestBarColumns_NoCurrentColumnWhenCurrentIndexOutOfRange(t *testing.T) {
	html := BarColumns(BarChartData{Values: []float64{10, 20}, CurrentIndex: -1}, "ha-bar-cols")
	if strings.Contains(html, "ha-bar-col-current") {
		t.Errorf("html = %q, want no current column when CurrentIndex is out of range", html)
	}
}

func TestBarColumns_NoCurrentColumnWhenCurrentValueIsNaN(t *testing.T) {
	html := BarColumns(BarChartData{Values: []float64{10, math.NaN()}, CurrentIndex: 1}, "ha-bar-cols")
	if strings.Contains(html, "ha-bar-col-current") {
		t.Errorf("html = %q, want no current column when that bucket has no data", html)
	}
}

func TestBarColumns_EscapesTimeLabel(t *testing.T) {
	html := BarColumns(BarChartData{Values: []float64{10}, TimeLabels: []string{"<b>"}}, "ha-bar-cols")
	if strings.Contains(html, "<b>") {
		t.Errorf("html = %q, want the time label HTML-escaped", html)
	}
}

func TestBarColumns_FlatSeriesDoesNotDivideByZero(t *testing.T) {
	html := BarColumns(BarChartData{Values: []float64{5, 5, 5}, CurrentIndex: 2}, "ha-bar-cols")
	if strings.Contains(html, "NaN") {
		t.Errorf("html = %q, want no NaN for a flat series", html)
	}
	if got := strings.Count(html, "--ha-bar-height:0.50"); got != 3 {
		t.Errorf("html = %q, want all three flat-series bars at the mid height", html)
	}
}

func TestBarColumns_AppliesClassName(t *testing.T) {
	html := BarColumns(BarChartData{Values: []float64{10, 20}}, "custom-class")
	if !strings.HasPrefix(html, `<div class="custom-class" style="--ha-bar-cols:2">`) {
		t.Errorf("html = %q, want it to start with the given class name", html)
	}
}

// TestBarColumns_EveryColumnHasSameChildElementCount is the regression test
// for the "bars look misaligned when some columns have an hour label and
// others don't" bug: .ha-bar-col uses justify-content:end, which packs each
// column's children as one contiguous group flush to the bottom. The
// time-label div used to be omitted entirely from the DOM for columns with
// no label text, so a labeled column had one more child than an unlabeled
// one and flex-end pushed its bar higher — an artifact of DOM structure,
// not real data. Every column must emit the same number of child elements
// (daylight columns excepted — that div is absolutely positioned and so is
// out of flow), with visibility controlled purely via CSS.
func TestBarColumns_EveryColumnHasSameChildElementCount(t *testing.T) {
	timeLabels := []string{"12am", "2am", "4am", "6am", "8am", "10am", "12pm", "2pm", "4pm", "6pm", "8pm", "10pm"}
	values := []float64{10, 11, 12, math.NaN(), 14, 15, 16, 17, 18, 19, 20, 21}
	html := BarColumns(BarChartData{Values: values, TimeLabels: timeLabels, CurrentIndex: 8}, "ha-bar-cols")

	parts := splitColumns(html)
	if len(parts) != len(values)+1 {
		t.Fatalf("got %d column parts, want %d (%d columns + preamble): %s", len(parts), len(values)+1, len(values), html)
	}
	counts := make([]int, len(values))
	for i, part := range parts[1:] {
		counts[i] = strings.Count(part, "<div")
	}
	for i, c := range counts {
		if c != counts[0] {
			t.Errorf("column %d has %d child <div> elements, want %d (same as column 0):\n%s", i, c, counts[0], html)
		}
	}
	if counts[0] != 3 {
		t.Errorf("child <div> count per column = %d, want 3 (value, bar, time-label div always present)", counts[0])
	}

	// Every column's time label is in the DOM — the sparse look is CSS-only.
	if got := strings.Count(html, `class="ha-bar-col-time"`); got != len(values) {
		t.Errorf("time-label div count = %d, want %d (one per column)", got, len(values))
	}
	if strings.Contains(html, "ha-bar-col-time-visible") {
		t.Errorf("html = %q, want no per-column visibility modifier class — :nth-child does that now", html)
	}
	for _, want := range timeLabels {
		if !strings.Contains(html, ">"+want+"<") {
			t.Errorf("html = %q, want it to contain the time label %q", html, want)
		}
	}
}

// --- Daytime band ---

// TestBarColumns_DaytimeBand_IsOneNestedDivPerDaytimeColumn pins the revert
// back to Weather's own technique. The band used to be rendered as one
// absolutely-positioned div per contiguous RUN of daytime columns, spanning
// the flex `gap` between them, purely because that gap otherwise showed
// through as an unhighlighted seam. Weather has no gap (columns are
// width:100%/12 and butt up against each other), so the highlight is just a
// plain inset:0 div nested in each daytime column, with the run's first and
// last column carrying the rounded-corner modifiers.
func TestBarColumns_DaytimeBand_IsOneNestedDivPerDaytimeColumn(t *testing.T) {
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

	if count := strings.Count(html, "ha-bar-daylight"); count != 6+4 {
		// 6 base classes + 2 sunrise + 2 sunset modifiers, all sharing the
		// "ha-bar-daylight" prefix.
		t.Errorf("daylight class occurrences = %d, want 10 (6 columns + 2 sunrise + 2 sunset): %s", count, html)
	}
	if count := strings.Count(html, `<div class="ha-bar-daylight`); count != 6 {
		t.Errorf("daylight div count = %d, want one per daytime column (6): %s", count, html)
	}
	if count := strings.Count(html, "ha-bar-daylight-sunrise"); count != 2 {
		t.Errorf("sunrise-rounded count = %d, want 2 (one per run's first column): %s", count, html)
	}
	if count := strings.Count(html, "ha-bar-daylight-sunset"); count != 2 {
		t.Errorf("sunset-rounded count = %d, want 2 (one per run's last column): %s", count, html)
	}
	// No positioned run divs left over from the old approach.
	if strings.Contains(html, "left:") || strings.Contains(html, "right:") {
		t.Errorf("html = %q, want no inline left/right run positioning — the divs are inset:0 inside their column now", html)
	}
	// Each daylight div must be nested INSIDE its column, i.e. appear after
	// that column's opening tag and before the next one.
	firstCol := strings.Index(html, `<div class="ha-bar-col"`)
	firstDay := strings.Index(html, `<div class="ha-bar-daylight`)
	if firstDay < firstCol {
		t.Errorf("html = %q, want daylight divs nested inside their columns", html)
	}
}

// A single-column daytime run carries both rounding modifiers.
func TestBarColumns_DaytimeBand_SingleColumnRunIsRoundedOnBothSides(t *testing.T) {
	html := BarColumns(BarChartData{
		Values:    []float64{10, 11, 12},
		IsDaytime: []bool{false, true, false},
	}, "ha-bar-cols")
	if !strings.Contains(html, `class="ha-bar-daylight ha-bar-daylight-sunrise ha-bar-daylight-sunset"`) {
		t.Errorf("html = %q, want a single-column run rounded on both top corners", html)
	}
}

// A run that reaches the chart's edge must not be treated as continuing
// past it — index 0 has no left neighbour, index n-1 no right neighbour.
func TestBarColumns_DaytimeBand_EdgeColumnsGetRoundedCorners(t *testing.T) {
	html := BarColumns(BarChartData{
		Values:    []float64{10, 11, 12},
		IsDaytime: []bool{true, true, true},
	}, "ha-bar-cols")
	if got := strings.Count(html, "ha-bar-daylight-sunrise"); got != 1 {
		t.Errorf("sunrise count = %d, want 1 (column 0): %s", got, html)
	}
	if got := strings.Count(html, "ha-bar-daylight-sunset"); got != 1 {
		t.Errorf("sunset count = %d, want 1 (last column): %s", got, html)
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

func TestBarColumns_BarHeightUsesCSSCustomProperty(t *testing.T) {
	html := BarColumns(BarChartData{Values: []float64{10, 12, 14, 16, 18}, CurrentIndex: 4}, "ha-bar-cols")

	if !strings.Contains(html, "--ha-bar-height:1.00") {
		t.Errorf("html = %q, want the max-value bar's height custom property at 1.00", html)
	}
	if !strings.Contains(html, "--ha-bar-height:0.00") {
		t.Errorf("html = %q, want the min-value bar's height custom property at 0.00", html)
	}
	if strings.Contains(html, "<svg") || strings.Contains(html, "<text") {
		t.Errorf("html = %q, want no SVG at all — this is a plain-HTML chart", html)
	}
}

// NaN buckets must not drag the min/max scaling.
func TestBarColumns_NaNIsExcludedFromScaling(t *testing.T) {
	html := BarColumns(BarChartData{Values: []float64{10, 20, math.NaN()}, CurrentIndex: 1}, "ha-bar-cols")
	if !strings.Contains(html, "--ha-bar-height:0.00") || !strings.Contains(html, "--ha-bar-height:1.00") {
		t.Errorf("html = %q, want the two real values to span the full 0..1 range", html)
	}
}

// countColumns counts .ha-bar-col opening tags without also matching the
// .ha-bar-col-time / .ha-bar-cols prefixes.
func countColumns(html string) int {
	return strings.Count(html, `<div class="ha-bar-col"`) + strings.Count(html, `<div class="ha-bar-col `)
}

// splitColumns splits the markup on column boundaries, returning the
// preamble followed by one chunk per column.
func splitColumns(html string) []string {
	marker := "\x00"
	normalized := strings.ReplaceAll(html, `<div class="ha-bar-col"`, marker)
	normalized = strings.ReplaceAll(normalized, `<div class="ha-bar-col `, marker)
	return strings.Split(normalized, marker)
}

// --- Projected (not-yet-happened) columns ---

// TestBarColumns_MarksColumnsAfterCurrentAsProjected pins the one addition
// to Weather's own column markup. Weather's future columns are a real
// forecast and need no distinguishing; a room thermometer has none, so the
// caller fills those buckets from the same clock time a day earlier (see
// projectYesterday in main.go) and they must be drawn faded rather than
// passed off as measurements.
func TestBarColumns_MarksColumnsAfterCurrentAsProjected(t *testing.T) {
	html := BarColumns(BarChartData{
		Values:       []float64{10, 11, 12, 13, 14},
		CurrentIndex: 2,
	}, "ha-bar-cols")

	if got := strings.Count(html, "ha-bar-projected"); got != 2 {
		t.Errorf("projected bar count = %d, want 2 (indices 3 and 4): %s", got, html)
	}
	if got := strings.Count(html, "ha-bar-value-projected"); got != 2 {
		t.Errorf("projected value-label count = %d, want 2: %s", got, html)
	}
	// The measured half, current column included, must carry neither class.
	measured, _, _ := strings.Cut(html, `class="ha-bar-value ha-bar-value-projected"`)
	if strings.Contains(measured, "projected") {
		t.Errorf("a bucket at or before CurrentIndex was marked projected: %s", html)
	}
}

func TestBarColumns_NoProjectedColumnsWhenThereIsNoCurrentColumn(t *testing.T) {
	// Without a current column there is no "now" to split measured from
	// projected, so nothing may be faded.
	html := BarColumns(BarChartData{Values: []float64{10, 11, 12}, CurrentIndex: -1}, "ha-bar-cols")
	if strings.Contains(html, "projected") {
		t.Errorf("html = %q, want no projected columns when CurrentIndex is out of range", html)
	}
}

// A projected bucket with no reading a day earlier stays an empty column
// rather than being faded-but-present.
func TestBarColumns_ProjectedGapRendersAsEmptyColumn(t *testing.T) {
	html := BarColumns(BarChartData{
		Values:       []float64{10, 11, math.NaN(), 13},
		CurrentIndex: 1,
	}, "ha-bar-cols")

	if got := strings.Count(html, "ha-bar-empty"); got != 1 {
		t.Errorf("empty-bar count = %d, want 1: %s", got, html)
	}
	if got := strings.Count(html, "ha-bar-projected"); got != 1 {
		t.Errorf("projected bar count = %d, want 1 (only index 3 has a value): %s", got, html)
	}
}

// Projected values share the chart's single vertical axis, so they have to
// take part in the min/max scaling — otherwise a warm afternoon yesterday
// would be drawn at the same height as a cool morning today.
func TestBarColumns_ProjectedValuesParticipateInScaling(t *testing.T) {
	html := BarColumns(BarChartData{
		Values:       []float64{20, 21, 30}, // the max lives in a projected bucket
		CurrentIndex: 1,
	}, "ha-bar-cols")

	if !strings.Contains(html, "--ha-bar-height:0.00") || !strings.Contains(html, "--ha-bar-height:1.00") {
		t.Errorf("html = %q, want the projected maximum to define the top of the scale", html)
	}
	if !strings.Contains(html, "--ha-bar-height:0.10") {
		t.Errorf("html = %q, want the measured values scaled against the projected maximum", html)
	}
}
