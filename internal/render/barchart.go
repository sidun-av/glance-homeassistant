package render

import (
	"fmt"
	"html"
	"math"
	"strings"
)

type BarChartOptions struct {
	Width     float64
	Height    float64
	ClassName string
}

func DefaultBarChartOptions() BarChartOptions {
	return BarChartOptions{Width: 220, Height: 61}
}

// BarChartData bundles everything BarChart needs to know about the series
// itself, separate from purely visual sizing (BarChartOptions) — Values is
// required, IsDaytime and CurrentLabel are both optional (nil/"" disables
// the daytime band / current-value label respectively).
type BarChartData struct {
	// Values may contain NaN for timestamps with no data yet (e.g. a
	// fixed-calendar-day window's hours later than "now") — those indices
	// render no bar at all (a gap), are excluded from min/max, and are
	// never treated as CurrentIndex.
	Values []float64
	// IsDaytime is parallel to Values (same length expected, but a shorter
	// or nil slice is treated as "no daytime data" rather than an error —
	// this chart must still render correctly for callers that don't have
	// sun data, e.g. if the sun.sun history fetch failed).
	IsDaytime []bool
	// CurrentLabel is rendered above the CurrentIndex bar. Also used to
	// decide whether the min/max labels should be suppressed for that same
	// bar (see BarChart's doc comment). Ignored (no label rendered) if
	// CurrentIndex is out of range or CurrentLabel is "".
	CurrentLabel string
	// CurrentIndex is which element of Values is "now" — the bar that gets
	// the brightness/width emphasis and CurrentLabel. Unlike the previous
	// version of this chart, this is NOT always len(Values)-1: a
	// fixed-calendar-day window's current reading sits wherever "now"
	// falls within the day, with later (not-yet-happened) hours as NaN
	// gaps after it. A negative or out-of-range CurrentIndex disables the
	// current-bar emphasis entirely (falls back to every bar rendering at
	// the same "normal" tier).
	CurrentIndex int
}

// BarChart renders a themed vertical bar chart mirroring Glance's own
// built-in WEATHER widget: rounded-top/flat-bottom bars (clipped, not
// stroke-linecap="round" — a round cap on both ends made short bars look
// like dots instead of bars), a two-tier brightness scheme (every bar at
// one flat "normal" shade except CurrentIndex, which is both wider and
// fully bright — matching Weather's .weather-column-current treatment,
// not a gradual opacity ramp), a rounded daytime band drawn behind the
// bars for each contiguous run of IsDaytime, and small value labels for
// the current/min/max points.
//
// Axis labels are rendered separately as plain HTML (see AxisLabelsRow),
// not SVG text, because this chart's SVG uses preserveAspectRatio="none"
// to fill whatever box its flex-grown container gives it — that scales X
// and Y non-uniformly, and SVG <text> glyphs visibly distort under that
// scaling the same way a photo looks distorted when stretched to the
// wrong aspect ratio, unlike bars/rects which just read as "a stretched
// chart." The current/min/max value labels used to be SVG <text> too
// (this was observed live once "bars" became the default chart_style:
// stretched, sometimes-clipped digits) — they're now plain HTML <span>s,
// absolutely positioned by percentage (not pixels, since the card's real
// rendered width isn't known server-side) over a position:relative
// wrapper around the SVG, exactly mirroring AxisLabelsRow's reasoning.
// BarChart therefore returns a wrapper <div>, not a bare <svg>, unlike
// Sparkline.
func BarChart(data BarChartData, opts BarChartOptions) string {
	values := data.Values
	if len(values) == 0 {
		return fmt.Sprintf(`<div class="ha-bar-wrap"><svg class="%s" viewBox="0 0 %g %g" height="%g" style="width:100%%;display:block" preserveAspectRatio="none"></svg></div>`, opts.ClassName, opts.Width, opts.Height, opts.Height)
	}

	const topMargin = 14.0
	const bottomMargin = 13.0
	const minBarHeight = 4.0

	barAreaHeight := opts.Height - topMargin - bottomMargin
	if barAreaHeight < minBarHeight {
		barAreaHeight = minBarHeight
	}
	baseline := opts.Height - bottomMargin

	min, max := math.NaN(), math.NaN()
	minIdx, maxIdx := -1, -1
	for i, v := range values {
		if math.IsNaN(v) {
			continue
		}
		if math.IsNaN(min) || v < min {
			min = v
			minIdx = i
		}
		if math.IsNaN(max) || v > max {
			max = v
			maxIdx = i
		}
	}
	haveData := minIdx != -1
	span := max - min
	flatSeries := !haveData || span < 1e-9
	if flatSeries {
		span = 1
	}

	n := len(values)
	currentIdx := data.CurrentIndex
	hasCurrent := currentIdx >= 0 && currentIdx < n && !math.IsNaN(values[currentIdx]) && data.CurrentLabel != ""
	step := opts.Width / float64(n)

	isDaytimeAt := func(i int) bool {
		if i < len(data.IsDaytime) {
			return data.IsDaytime[i]
		}
		return false
	}

	// Daytime band: one rounded rect per maximal contiguous run of
	// isDaytimeAt(i)==true, drawn first so bars paint on top of it.
	var daytimeRects strings.Builder
	runStart := -1
	for i := 0; i <= n; i++ {
		day := i < n && isDaytimeAt(i)
		if day {
			if runStart == -1 {
				runStart = i
			}
			continue
		}
		if runStart != -1 {
			x1 := step * float64(runStart)
			x2 := step * float64(i)
			fmt.Fprintf(&daytimeRects,
				`<rect x="%.2f" y="0" width="%.2f" height="%g" rx="6" ry="6" fill="var(--color-primary)" fill-opacity="0.10"/>`,
				x1, x2-x1, opts.Height,
			)
			runStart = -1
		}
	}

	normalBarWidth := step * 0.55
	currentBarWidth := normalBarWidth * (10.0 / 6.0)
	if currentBarWidth > step {
		currentBarWidth = step
	}
	// barRadius scales with bar width rather than a fixed pixel value, so
	// a bar's rounded top stays proportionate whether it's a normal or
	// current-width bar — capped at a third of the width so it can never
	// exceed what SVG rx auto-clamps to (half-width, which would round
	// into a full pill again, the exact shape this rect-based approach
	// exists to avoid for short bars).
	barRadius := func(width float64) float64 {
		r := width * 0.4
		if r > width/2 {
			r = width / 2
		}
		return r
	}

	// A single shared clipPath, used by every bar's <rect>, cuts off
	// whatever a bar's rounded corners would otherwise draw below
	// baseline — every bar in this chart shares the same baseline (fixed
	// per BarChartOptions.Height), so one shared clip region is exactly
	// as correct as a per-bar one would be, without needing a
	// request-scoped unique ID for multiple room cards' charts coexisting
	// in one dashboard page.
	const clipID = "ha-bar-baseline-clip"
	fmt.Fprintf(&daytimeRects, `<defs><clipPath id="%s"><rect x="0" y="0" width="%g" height="%.2f"/></clipPath></defs>`, clipID, opts.Width, baseline)

	var bars strings.Builder
	// barTopY[i] is the y-coordinate of bar i's peak, kept for label
	// placement below — computed once here rather than recomputed per
	// label so the two passes can't drift out of sync. NaN for indices
	// with no bar drawn.
	barTopY := make([]float64, n)
	for i := range barTopY {
		barTopY[i] = math.NaN()
	}
	for i, v := range values {
		if math.IsNaN(v) {
			continue // no data yet for this timestamp (e.g. later today) — leave a gap, not a zero-height bar
		}
		x := step*float64(i) + step/2
		normalized := (v - min) / span * barAreaHeight
		if normalized < minBarHeight {
			normalized = minBarHeight
		}
		y2 := baseline - normalized
		barTopY[i] = y2

		barWidth := normalBarWidth
		opacity := "0.55"
		if hasCurrent && i == currentIdx {
			barWidth = currentBarWidth
			opacity = "1"
		}
		radius := barRadius(barWidth)
		// Drawn taller than the visible bar (extending barRadius past
		// baseline) so its bottom-corner rounding falls entirely below
		// the clip boundary, leaving a flat bottom at baseline — only
		// the top corners' rounding survives the clip.
		fmt.Fprintf(&bars,
			`<rect x="%.2f" y="%.2f" width="%.2f" height="%.2f" rx="%.2f" ry="%.2f" fill="var(--color-progress-value)" fill-opacity="%s" clip-path="url(#%s)"/>`,
			x-barWidth/2, y2, barWidth, (baseline-y2)+radius, radius, radius, opacity, clipID,
		)
	}

	var labels strings.Builder
	labelPct := func(x, y float64) (float64, float64) {
		return x / opts.Width * 100, y / opts.Height * 100
	}
	if hasCurrent {
		lx, ly := labelPct(step*float64(currentIdx)+step/2, topMargin-4)
		fmt.Fprintf(&labels, `<span class="ha-bar-label ha-bar-label-current" style="left:%.2f%%;top:%.2f%%">%s</span>`,
			lx, ly, html.EscapeString(data.CurrentLabel))
	}
	if !flatSeries {
		if minIdx != -1 && minIdx != currentIdx {
			lx, ly := labelPct(step*float64(minIdx)+step/2, barTopY[minIdx]-3)
			fmt.Fprintf(&labels, `<span class="ha-bar-label ha-bar-label-secondary" style="left:%.2f%%;top:%.2f%%">%s</span>`,
				lx, ly, html.EscapeString(fmt.Sprintf("%.1f°", min)))
		}
		if maxIdx != -1 && maxIdx != currentIdx {
			lx, ly := labelPct(step*float64(maxIdx)+step/2, barTopY[maxIdx]-3)
			fmt.Fprintf(&labels, `<span class="ha-bar-label ha-bar-label-secondary" style="left:%.2f%%;top:%.2f%%">%s</span>`,
				lx, ly, html.EscapeString(fmt.Sprintf("%.1f°", max)))
		}
	}

	svg := fmt.Sprintf(`<svg class="%s" viewBox="0 0 %g %g" height="%g" style="width:100%%;display:block" preserveAspectRatio="none">%s%s</svg>`,
		opts.ClassName, opts.Width, opts.Height, opts.Height, daytimeRects.String(), bars.String())
	return fmt.Sprintf(`<div class="ha-bar-wrap">%s%s</div>`, svg, labels.String())
}
