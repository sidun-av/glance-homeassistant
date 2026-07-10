package render

import (
	"fmt"
	"html"
	"strings"
)

type BarChartOptions struct {
	Width  float64
	Height float64
}

func DefaultBarChartOptions() BarChartOptions {
	return BarChartOptions{Width: 220, Height: 61}
}

// BarChart renders a themed vertical bar chart mirroring Glance's own
// built-in WEATHER widget: one rounded-cap bar per value, auto min/max
// scaled (with a minimum bar height floor so the lowest point stays
// visible), opacity ramping from dim (oldest, index 0) to full brightness
// (most recent, last index), a value label above the most recent bar, and
// sparse x-axis labels wherever axisLabels[i] is non-empty.
func BarChart(values []float64, axisLabels []string, currentValueLabel string, opts BarChartOptions) string {
	if len(values) == 0 {
		return fmt.Sprintf(`<svg viewBox="0 0 %g %g" height="%g" style="width:100%%;display:block" preserveAspectRatio="none"></svg>`, opts.Width, opts.Height, opts.Height)
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
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	span := max - min
	if span < 1e-9 {
		span = 1
	}

	n := len(values)
	step := opts.Width / float64(n)
	denom := n - 1
	if denom < 1 {
		denom = 1
	}

	var bars strings.Builder
	for i, v := range values {
		x := step*float64(i) + step/2
		normalized := (v - min) / span * barAreaHeight
		if normalized < minBarHeight {
			normalized = minBarHeight
		}
		y2 := baseline - normalized
		opacity := 0.32 + (0.68 * float64(i) / float64(denom))
		barWidth := step * 0.55
		fmt.Fprintf(&bars,
			`<line x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f" stroke="var(--color-primary)" stroke-opacity="%.2f" stroke-width="%.2f" stroke-linecap="round"/>`,
			x, baseline, x, y2, opacity, barWidth,
		)
	}

	var label string
	if currentValueLabel != "" {
		lastX := step*float64(n-1) + step/2
		label = fmt.Sprintf(`<text x="%.2f" y="%.2f" text-anchor="middle" font-size="9" fill="var(--color-text-highlight)">%s</text>`,
			lastX, topMargin-4, html.EscapeString(currentValueLabel))
	}

	var axis strings.Builder
	for i, lbl := range axisLabels {
		if lbl == "" || i >= n {
			continue
		}
		x := step*float64(i) + step/2
		fmt.Fprintf(&axis, `<text x="%.2f" y="%.2f" text-anchor="middle" font-size="9" fill="var(--color-text-subdue)">%s</text>`,
			x, opts.Height-2, html.EscapeString(lbl))
	}

	return fmt.Sprintf(`<svg viewBox="0 0 %g %g" height="%g" style="width:100%%;display:block" preserveAspectRatio="none">%s%s%s</svg>`,
		opts.Width, opts.Height, opts.Height, bars.String(), label, axis.String())
}
