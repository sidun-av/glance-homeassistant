package render

import (
	"fmt"
	"html"
	"strings"
)

// AxisLabel is one candidate time label for a room card's chart, computed
// by main.go's sparseAxisLabels. Tier 0 is always shown; higher tiers only
// appear once the room's card is wide enough to fit them without crowding
// (see the ha-chart-axis span[data-tier] rules and @container queries in
// template.go's styleBlock) — each tier is a superset of every lower tier's
// positions, so a card growing wider only reveals additional labels without
// repositioning ones already shown.
type AxisLabel struct {
	Text string
	Tier int
}

// AxisLabelsRow renders a room card's sparse x-axis time labels as a plain
// HTML flex row, not SVG text. The chart SVG it sits below uses
// preserveAspectRatio="none" so it can stretch to fill whatever width its
// flex-grown room card ends up with — but that scales X and Y
// non-uniformly, and SVG <text> glyphs scale (and visibly distort) right
// along with the geometry, unlike geometry the eye reads as "just a
// chart." Plain HTML text has no such distortion, which is also how
// Glance's own WEATHER widget renders its hour labels.
func AxisLabelsRow(labels []AxisLabel) string {
	if len(labels) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(`<div class="ha-chart-axis">`)
	for _, l := range labels {
		fmt.Fprintf(&b, `<span data-tier="%d">%s</span>`, l.Tier, html.EscapeString(l.Text))
	}
	b.WriteString(`</div>`)
	return b.String()
}
