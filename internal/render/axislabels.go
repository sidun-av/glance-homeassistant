package render

import (
	"fmt"
	"html"
	"strings"
)

// AxisLabelsRow renders a room card's sparse x-axis time labels (the
// first/middle/last populated entries computed by main.go's
// sparseAxisLabels) as a plain HTML flex row, not SVG text. The chart SVG
// it sits below uses preserveAspectRatio="none" so it can stretch to fill
// whatever width its flex-grown room card ends up with — but that scales
// X and Y non-uniformly, and SVG <text> glyphs scale (and visibly
// distort) right along with the geometry, unlike geometry the eye reads
// as "just a chart." Plain HTML text has no such distortion, which is
// also how Glance's own WEATHER widget renders its hour labels. Empty
// entries in labels are skipped entirely; if none are non-empty, this
// returns "".
func AxisLabelsRow(labels []string) string {
	var visible []string
	for _, l := range labels {
		if l != "" {
			visible = append(visible, l)
		}
	}
	if len(visible) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(`<div class="ha-chart-axis">`)
	for _, l := range visible {
		fmt.Fprintf(&b, `<span>%s</span>`, html.EscapeString(l))
	}
	b.WriteString(`</div>`)
	return b.String()
}
