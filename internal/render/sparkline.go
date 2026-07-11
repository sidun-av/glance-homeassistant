package render

import (
	"fmt"
	"html"
	"strings"
)

type SparklineOptions struct {
	Width  float64
	Height float64
}

func DefaultSparklineOptions() SparklineOptions {
	return SparklineOptions{Width: 220, Height: 34}
}

// sparklineAxisMargin reserves room below the line/area for the x-axis time
// labels, mirroring the label row BarChart reserves via its own margins.
const sparklineAxisMargin = 12.0

// Sparkline renders an inline SVG line+area chart auto-scaled to the
// series' own min/max (with 20% padding), themed via Glance's own CSS
// custom property so it matches the user's active theme, with sparse x-axis
// time labels drawn wherever axisLabels[i] is non-empty (mirroring Glance's
// own WEATHER widget timeline).
func Sparkline(values []float64, axisLabels []string, opts SparklineOptions) string {
	if len(values) == 0 {
		return fmt.Sprintf(`<svg viewBox="0 0 %g %g" height="%g" style="width:100%%;display:block" preserveAspectRatio="none"></svg>`, opts.Width, opts.Height, opts.Height)
	}

	chartHeight := opts.Height - sparklineAxisMargin
	if chartHeight < 1 {
		chartHeight = opts.Height
	}

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
		min -= 0.5
		max += 0.5
		span = 1
	}
	pad := span * 0.2
	min -= pad
	max += pad
	span = max - min

	n := len(values)
	stepX := 0.0
	if n > 1 {
		stepX = opts.Width / float64(n-1)
	}

	points := make([]string, n)
	for i, v := range values {
		x := float64(i) * stepX
		y := chartHeight - ((v-min)/span)*chartHeight
		points[i] = fmt.Sprintf("%.2f,%.2f", x, y)
	}
	line := strings.Join(points, " ")
	area := fmt.Sprintf("0,%.2f %s %.2f,%.2f", chartHeight, line, opts.Width, chartHeight)

	var axis strings.Builder
	for i, lbl := range axisLabels {
		if lbl == "" || i >= n {
			continue
		}
		x := float64(i) * stepX
		anchor := "middle"
		switch i {
		case 0:
			anchor = "start"
		case n - 1:
			anchor = "end"
		}
		fmt.Fprintf(&axis, `<text x="%.2f" y="%.2f" text-anchor="%s" font-size="9" fill="var(--color-text-subdue)">%s</text>`,
			x, opts.Height-1, anchor, html.EscapeString(lbl))
	}

	return fmt.Sprintf(
		`<svg viewBox="0 0 %g %g" height="%g" style="width:100%%;display:block" preserveAspectRatio="none">`+
			`<polygon points="%s" fill="var(--color-progress-value)" fill-opacity="0.16"/>`+
			`<polyline points="%s" fill="none" stroke="var(--color-progress-value)" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"/>`+
			`%s`+
			`</svg>`,
		opts.Width, opts.Height, opts.Height, area, line, axis.String(),
	)
}
