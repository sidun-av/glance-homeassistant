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
	// render an empty column (no bar), are excluded from min/max, and are
	// never treated as CurrentIndex.
	Values []float64
	// IsDaytime is parallel to Values. A shorter or nil slice is treated as
	// "no daytime data" rather than an error.
	IsDaytime []bool
	// CurrentLabel is rendered on the CurrentIndex column. Also decides
	// whether the min/max labels are suppressed for that same column (see
	// BarColumns' doc comment). Ignored if CurrentIndex is out of range or
	// CurrentLabel is "".
	CurrentLabel string
	// CurrentIndex is which element of Values is "now". A negative or
	// out-of-range CurrentIndex disables the current-column emphasis
	// entirely.
	CurrentIndex int
	// TimeLabels is parallel to Values — a "" entry means that column shows
	// no time label (this chart has few enough columns, unlike the old
	// dense sparkline, that a caller can simply choose which indices get a
	// label directly, e.g. every 4th of 12 — mirroring Weather's own
	// hardcoded nth-child sparse pattern rather than reusing the denser
	// chart's dyadic-tier reveal system, which solved a different problem
	// this fixed 12-column layout doesn't have). A shorter or nil slice
	// means no column gets a time label.
	TimeLabels []string
}

// BarColumns renders the temperature chart as a flex row of plain HTML
// column divs — one per bucket — mirroring Glance's own built-in WEATHER
// widget's actual technique (see internal/glance/templates/weather.html and
// static/css/widget-weather.css in github.com/glanceapp/glance), not the
// SVG approximation this file used to contain. The SVG version (dozens of
// thin bars, non-uniform preserveAspectRatio scaling) read visibly worse
// than Weather on narrow/mobile widths — a handful of wide flex columns
// scales down cleanly where thin SVG strokes turn into noise. This repo's
// own var(--color-*) theme variables are used throughout instead of
// Glance's internal hsl(var(--ths)) ones, since this widget (being external
// to Glance's core) only has access to the former.
//
// Each column gets: an optional daylight-highlight div (only for daytime
// buckets, rounded on whichever edge starts/ends a contiguous daytime run),
// a value-label div (only current/min/max buckets get real text — content
// is simply "" for every other column, so no separate hover-reveal system
// is needed the way Weather's is, since Weather always populates every
// column's label and hides most by default), and a bar div whose height is
// driven by a CSS custom property (--ha-bar-height, 0..1) rather than an
// inline pixel height, so the same column scales correctly at any card
// width via plain CSS.
func BarColumns(data BarChartData, className string) string {
	values := data.Values
	n := len(values)
	if n == 0 {
		return fmt.Sprintf(`<div class="%s"></div>`, className)
	}

	min, max := math.NaN(), math.NaN()
	minIdx, maxIdx := -1, -1
	for i, v := range values {
		if math.IsNaN(v) {
			continue
		}
		if math.IsNaN(min) || v < min {
			min, minIdx = v, i
		}
		if math.IsNaN(max) || v > max {
			max, maxIdx = v, i
		}
	}
	haveData := minIdx != -1
	span := max - min
	flatSeries := !haveData || span < 1e-9
	if flatSeries {
		span = 1
	}

	currentIdx := data.CurrentIndex
	hasCurrent := currentIdx >= 0 && currentIdx < n && !math.IsNaN(values[currentIdx]) && data.CurrentLabel != ""

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
	fmt.Fprintf(&b, `<div class="%s">`, className)
	for i, v := range values {
		current := hasCurrent && i == currentIdx
		fmt.Fprintf(&b, `<div class="ha-bar-col" data-current="%t">`, current)

		if isDaytimeAt(i) {
			edge := ""
			if !isDaytimeAt(i - 1) {
				edge += " ha-bar-daylight-start"
			}
			if !isDaytimeAt(i + 1) {
				edge += " ha-bar-daylight-end"
			}
			fmt.Fprintf(&b, `<div class="ha-bar-daylight%s"></div>`, edge)
		}

		if math.IsNaN(v) {
			b.WriteString(`<div class="ha-bar-value"></div><div class="ha-bar ha-bar-empty"></div>`)
			if tl := timeLabelAt(i); tl != "" {
				fmt.Fprintf(&b, `<div class="ha-bar-col-time">%s</div>`, html.EscapeString(tl))
			}
			b.WriteString(`</div>`)
			continue
		}

		label := ""
		valueClass := "ha-bar-value"
		switch {
		case current:
			label = data.CurrentLabel
			valueClass += " ha-bar-value-current"
		case !flatSeries && i == minIdx:
			label = fmt.Sprintf("%.1f°", v)
		case !flatSeries && i == maxIdx:
			label = fmt.Sprintf("%.1f°", v)
		}
		fmt.Fprintf(&b, `<div class="%s">%s</div>`, valueClass, html.EscapeString(label))

		height := 0.5
		if !flatSeries {
			height = (v - min) / span
		}
		barClass := "ha-bar"
		if current {
			barClass += " ha-bar-current"
		}
		fmt.Fprintf(&b, `<div class="%s" style="--ha-bar-height:%.3f"></div>`, barClass, height)

		if tl := timeLabelAt(i); tl != "" {
			fmt.Fprintf(&b, `<div class="ha-bar-col-time">%s</div>`, html.EscapeString(tl))
		}

		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}
