package render

import (
	"fmt"
	"strings"
)

type SparklineOptions struct {
	Width     float64
	Height    float64
	ClassName string
}

func DefaultSparklineOptions() SparklineOptions {
	return SparklineOptions{Width: 220, Height: 34}
}

// Sparkline renders an inline SVG line+area chart auto-scaled to the
// series' own min/max (with 20% padding), themed via Glance's own CSS
// custom property so it matches the user's active theme. Axis labels are
// rendered separately (see AxisLabelsRow) as plain HTML, not SVG text —
// this SVG uses preserveAspectRatio="none" to fill whatever box its
// flex-grown container gives it, and SVG text scales non-uniformly right
// along with the geometry under that setting, visibly stretching glyphs
// whenever the container's aspect ratio doesn't match the chart's own
// internal coordinate space (which is always true in a room card of
// varying width). Plain HTML text has no such distortion.
func Sparkline(values []float64, opts SparklineOptions) string {
	if len(values) == 0 {
		return fmt.Sprintf(`<svg class="%s" viewBox="0 0 %g %g" height="%g" style="width:100%%;display:block" preserveAspectRatio="none"></svg>`, opts.ClassName, opts.Width, opts.Height, opts.Height)
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
		y := opts.Height - ((v-min)/span)*opts.Height
		points[i] = fmt.Sprintf("%.2f,%.2f", x, y)
	}
	line := strings.Join(points, " ")
	area := fmt.Sprintf("0,%.2f %s %.2f,%.2f", opts.Height, line, opts.Width, opts.Height)

	return fmt.Sprintf(
		`<svg class="%s" viewBox="0 0 %g %g" height="%g" style="width:100%%;display:block" preserveAspectRatio="none">`+
			`<polygon points="%s" fill="var(--color-progress-value)" fill-opacity="0.16"/>`+
			`<polyline points="%s" fill="none" stroke="var(--color-progress-value)" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"/>`+
			`</svg>`,
		opts.ClassName, opts.Width, opts.Height, opts.Height, area, line,
	)
}
