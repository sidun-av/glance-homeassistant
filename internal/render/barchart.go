package render

import (
	"fmt"
	"html"
	"math"
	"strings"
)

// BarChartData bundles everything BarColumns needs to know about the series
// itself.
type BarChartData struct {
	// Values may contain NaN for timestamps with no data yet (e.g. hours
	// later than "now" within a fixed calendar-day window) — those indices
	// render an empty column (no bar, no value label), are excluded from
	// min/max scaling, and are never treated as CurrentIndex.
	Values []float64
	// IsDaytime is parallel to Values. A shorter or nil slice is treated as
	// "no daytime data" rather than an error.
	IsDaytime []bool
	// CurrentIndex is which element of Values is "now" — that column gets
	// the ha-bar-col-current class, which is what makes its bar wide and
	// bright and its value label visible without a hover (see styleBlock).
	// A negative or out-of-range CurrentIndex disables the current-column
	// emphasis entirely.
	//
	// It also splits the chart in two: everything after it is a bucket that
	// has not happened yet, so any value there is a projection rather than a
	// measurement (the caller fills those from the same clock time 24 hours
	// earlier — see projectYesterday in main.go) and is drawn faded, via
	// ha-bar-projected. Weather has no equivalent because its future columns
	// come from a real forecast; a room thermometer has no forecast, and
	// yesterday at the same hour is the closest honest stand-in.
	CurrentIndex int
	// TimeLabels is parallel to Values. Unlike the pre-Weather-port version
	// of this chart, a label is expected for EVERY column, not a sparse
	// subset: which ones are visible at rest is a pure CSS concern
	// (:nth-child(3)/(7)/(11), mirroring Weather), and the rest are revealed
	// on hover — so they all have to actually be in the DOM. A shorter or
	// nil slice just leaves the remaining columns' labels empty.
	TimeLabels []string
}

// BarColumns renders the temperature chart as a flex row of plain HTML
// column divs — one per bucket — as a direct port of Glance's own built-in
// WEATHER widget (internal/glance/templates/weather.html and
// static/css/widget-weather.css in github.com/glanceapp/glance). Both the
// markup shape and the CSS in styleBlock deliberately mirror it
// declaration-for-declaration, because the ask was literally "make this
// look like the Weather widget" and every earlier attempt to approximate it
// from memory drifted (tiny 7px labels, a flat opacity-based bar, extra
// min/max labels Weather doesn't have, an off-by-two time-label position).
//
// Per-column structure, matching weather.html's range body exactly:
//
//	<div class="ha-bar-col[ ha-bar-col-current]">
//	  [<div class="ha-bar-daylight[ -sunrise][ -sunset]"></div>]
//	  <div class="ha-bar-value[ -negative][ -projected]">22</div>
//	  <div class="ha-bar[ ha-bar-empty][ ha-bar-projected]" style="--ha-bar-height:0.62"></div>
//	  <div class="ha-bar-col-time">8pm</div>
//	</div>
//
// The one addition to Weather's own column is ha-bar-projected, on every
// column after CurrentIndex — see that field's doc comment. Those values
// still take part in the min/max scaling, since they share the chart's
// single vertical axis and would otherwise be drawn at the wrong height.
//
// Three things about that are worth spelling out because they reverse
// earlier decisions in this file's history:
//
//   - The daylight highlight is back to being ONE DIV PER COLUMN, nested
//     inside the column (inset:0), which is what Weather does. The previous
//     "collapse contiguous daytime columns into one absolutely-positioned
//     run" machinery existed only to hide the seams that .ha-bar-cols'
//     flex `gap` left between adjacent highlighted columns. Weather has no
//     gap at all — its columns are width:calc(100%/12) and butt up against
//     each other — so with the gap gone the per-column divs are contiguous
//     by construction and the run machinery has nothing left to solve.
//
//   - EVERY column gets a value label, not just the current/min/max ones.
//     Weather has no notion of min/max labels; it renders all twelve values
//     and hides them with opacity, revealing the current one always and any
//     other on hover of its column. That's strictly more information than
//     the old three-fixed-labels scheme and it's why the chart no longer
//     has stray numbers floating over it.
//
//   - Values are rendered as absolute integers with no degree sign. The
//     sign and the "°" are CSS ::before/::after pseudo-elements (see
//     styleBlock), exactly as in Weather — that keeps the digits optically
//     centered over the bar instead of the whole "-12°" string being
//     centered, which visibly shifts the number off its own column.
//
// The wrapper carries --ha-bar-cols so the CSS can size each column as
// 100%/N. Weather hardcodes /12 because its forecast is always 12 columns;
// this widget's caller also uses 12 (see barColumnsCount in main.go) but
// BarColumns itself stays generic over len(Values).
func BarColumns(data BarChartData, className string) string {
	values := data.Values
	n := len(values)
	if n == 0 {
		return fmt.Sprintf(`<div class="%s"></div>`, className)
	}

	min, max := math.NaN(), math.NaN()
	for _, v := range values {
		if math.IsNaN(v) {
			continue
		}
		if math.IsNaN(min) || v < min {
			min = v
		}
		if math.IsNaN(max) || v > max {
			max = v
		}
	}
	span := max - min
	flatSeries := math.IsNaN(min) || span < 1e-9

	currentIdx := data.CurrentIndex
	hasCurrent := currentIdx >= 0 && currentIdx < n && !math.IsNaN(values[currentIdx])

	isDaytimeAt := func(i int) bool {
		if i < 0 || i >= len(data.IsDaytime) {
			return false
		}
		return data.IsDaytime[i]
	}
	timeLabelAt := func(i int) string {
		if i < 0 || i >= len(data.TimeLabels) {
			return ""
		}
		return data.TimeLabels[i]
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<div class="%s" style="--ha-bar-cols:%d">`, className, n)

	for i, v := range values {
		colClass := "ha-bar-col"
		if hasCurrent && i == currentIdx {
			colClass += " ha-bar-col-current"
		}
		fmt.Fprintf(&b, `<div class="%s">`, colClass)

		if isDaytimeAt(i) {
			dayClass := "ha-bar-daylight"
			if !isDaytimeAt(i - 1) {
				dayClass += " ha-bar-daylight-sunrise"
			}
			if !isDaytimeAt(i + 1) {
				dayClass += " ha-bar-daylight-sunset"
			}
			fmt.Fprintf(&b, `<div class="%s"></div>`, dayClass)
		}

		if math.IsNaN(v) {
			// Same three children as every other column so the flex
			// column's justify-content:end packs identically — just with
			// no number and an invisible bar.
			b.WriteString(`<div class="ha-bar-value"></div><div class="ha-bar ha-bar-empty" style="--ha-bar-height:0"></div>`)
		} else {
			valueClass := "ha-bar-value"
			if v < 0 {
				valueClass += " ha-bar-value-negative"
			}
			barClass := "ha-bar"
			if hasCurrent && i > currentIdx {
				valueClass += " ha-bar-value-projected"
				barClass += " ha-bar-projected"
			}
			fmt.Fprintf(&b, `<div class="%s">%.0f</div>`, valueClass, math.Abs(v))

			height := 0.5
			if !flatSeries {
				height = (v - min) / span
			}
			fmt.Fprintf(&b, `<div class="%s" style="--ha-bar-height:%.2f"></div>`, barClass, height)
		}

		fmt.Fprintf(&b, `<div class="ha-bar-col-time">%s</div>`, html.EscapeString(timeLabelAt(i)))
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}
