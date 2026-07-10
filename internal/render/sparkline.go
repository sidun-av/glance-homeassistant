package render

import (
	"fmt"
	"strings"
)

type SparklineOptions struct {
	Width  float64
	Height float64
}

func DefaultSparklineOptions() SparklineOptions {
	return SparklineOptions{Width: 220, Height: 34}
}

// Sparkline renders an inline SVG line+area chart auto-scaled to the
// series' own min/max (with 20% padding), themed via Glance's own CSS
// custom property so it matches the user's active accent color.
func Sparkline(values []float64, opts SparklineOptions) string {
	if len(values) == 0 {
		return fmt.Sprintf(`<svg viewBox="0 0 %g %g" height="%g" style="width:100%%;display:block" preserveAspectRatio="none"></svg>`, opts.Width, opts.Height, opts.Height)
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
		`<svg viewBox="0 0 %g %g" height="%g" style="width:100%%;display:block" preserveAspectRatio="none">`+
			`<polygon points="%s" fill="var(--color-primary)" fill-opacity="0.16"/>`+
			`<polyline points="%s" fill="none" stroke="var(--color-primary)" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"/>`+
			`</svg>`,
		opts.Width, opts.Height, opts.Height, area, line,
	)
}
