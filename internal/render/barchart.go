package render

import (
	"fmt"
	"html"
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
	Values []float64
	// IsDaytime is parallel to Values (same length expected, but a shorter
	// or nil slice is treated as "no daytime data" rather than an error —
	// this chart must still render correctly for callers that don't have
	// sun data, e.g. if the sun.sun history fetch failed).
	IsDaytime []bool
	// CurrentLabel is rendered above the most recent (last) bar. Also
	// used to decide whether the min/max labels should be suppressed for
	// that same bar (see BarChart's doc comment).
	CurrentLabel string
}

// BarChart renders a themed vertical bar chart mirroring Glance's own
// built-in WEATHER widget: one rounded-cap bar per value, auto min/max
// scaled (with a minimum bar height floor so the lowest point stays
// visible), opacity ramping from dim (oldest, index 0) to full brightness
// (most recent, last index), the most recent bar rendered wider than the
// rest (matching Weather's .weather-column-current width bump), a value
// label above the most recent bar, and — new — a rounded daytime band
// drawn behind the bars for each contiguous run of IsDaytime, plus small
// secondary labels marking the minimum and maximum values in the series
// (skipped for whichever one, if either, coincides with the current bar,
// to avoid rendering the same value twice on one bar).
//
// Axis labels are rendered separately as plain HTML (see AxisLabelsRow),
// not SVG text — see Sparkline's doc comment for why. The value labels
// here stay SVG text since each is tied to a specific bar's x-position
// rather than edge-anchored, though they're subject to the same
// non-uniform-scaling distortion in principle; not fixed here since bars
// wasn't the previous default chart_style and this hasn't been observed
// as a problem in practice.
func BarChart(data BarChartData, opts BarChartOptions) string {
	values := data.Values
	if len(values) == 0 {
		return fmt.Sprintf(`<svg class="%s" viewBox="0 0 %g %g" height="%g" style="width:100%%;display:block" preserveAspectRatio="none"></svg>`, opts.ClassName, opts.Width, opts.Height, opts.Height)
	}

	const topMargin = 14.0
	const bottomMargin = 13.0
	const minBarHeight = 3.0

	barAreaHeight := opts.Height - topMargin - bottomMargin
	if barAreaHeight < minBarHeight {
		barAreaHeight = minBarHeight
	}
	baseline := opts.Height - bottomMargin

	min, max := values[0], values[0]
	minIdx, maxIdx := 0, 0
	for i, v := range values {
		if v < min {
			min = v
			minIdx = i
		}
		if v > max {
			max = v
			maxIdx = i
		}
	}
	span := max - min
	flatSeries := span < 1e-9
	if flatSeries {
		span = 1
	}

	n := len(values)
	currentIdx := n - 1
	step := opts.Width / float64(n)
	denom := n - 1
	if denom < 1 {
		denom = 1
	}

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

	var bars strings.Builder
	// barTopY[i] is the y-coordinate of bar i's peak, kept for label
	// placement below — computed once here rather than recomputed per
	// label so the two passes can't drift out of sync.
	barTopY := make([]float64, n)
	for i, v := range values {
		x := step*float64(i) + step/2
		normalized := (v - min) / span * barAreaHeight
		if normalized < minBarHeight {
			normalized = minBarHeight
		}
		y2 := baseline - normalized
		barTopY[i] = y2
		opacity := 0.32 + (0.68 * float64(i) / float64(denom))
		barWidth := normalBarWidth
		if i == currentIdx {
			barWidth = currentBarWidth
		}
		fmt.Fprintf(&bars,
			`<line x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f" stroke="var(--color-progress-value)" stroke-opacity="%.2f" stroke-width="%.2f" stroke-linecap="round"/>`,
			x, baseline, x, y2, opacity, barWidth,
		)
	}

	var labels strings.Builder
	if data.CurrentLabel != "" {
		x := step*float64(currentIdx) + step/2
		fmt.Fprintf(&labels, `<text x="%.2f" y="%.2f" text-anchor="middle" font-size="9" fill="var(--color-text-highlight)">%s</text>`,
			x, topMargin-4, html.EscapeString(data.CurrentLabel))
	}
	if !flatSeries {
		if minIdx != currentIdx {
			x := step*float64(minIdx) + step/2
			fmt.Fprintf(&labels, `<text x="%.2f" y="%.2f" text-anchor="middle" font-size="7" fill="var(--color-text-subdue)">%s</text>`,
				x, barTopY[minIdx]-3, html.EscapeString(fmt.Sprintf("%.1f°", min)))
		}
		if maxIdx != currentIdx {
			x := step*float64(maxIdx) + step/2
			fmt.Fprintf(&labels, `<text x="%.2f" y="%.2f" text-anchor="middle" font-size="7" fill="var(--color-text-subdue)">%s</text>`,
				x, barTopY[maxIdx]-3, html.EscapeString(fmt.Sprintf("%.1f°", max)))
		}
	}

	return fmt.Sprintf(`<svg class="%s" viewBox="0 0 %g %g" height="%g" style="width:100%%;display:block" preserveAspectRatio="none">%s%s%s</svg>`,
		opts.ClassName, opts.Width, opts.Height, opts.Height, daytimeRects.String(), bars.String(), labels.String())
}
