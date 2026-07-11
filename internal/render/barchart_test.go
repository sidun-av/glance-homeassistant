package render

import (
	"strings"
	"testing"
)

func TestBarChart_EmptyValuesReturnsBareSVG(t *testing.T) {
	svg := BarChart(nil, nil, "", DefaultBarChartOptions())
	if !contains(svg, "<svg") {
		t.Errorf("svg = %q, want it to contain <svg", svg)
	}
	if contains(svg, "<line") {
		t.Errorf("svg = %q, want no bars for empty values", svg)
	}
}

func TestBarChart_RendersOneBarPerValue(t *testing.T) {
	svg := BarChart([]float64{10, 15, 12, 20}, []string{"06:00", "", "", "18:00"}, "20.0°", BarChartOptions{Width: 220, Height: 60})
	if count := strings.Count(svg, "<line"); count != 4 {
		t.Errorf("bar (<line>) count = %d, want 4", count)
	}
	if !contains(svg, "var(--color-progress-value)") {
		t.Errorf("svg = %q, want it to reference the theme's progress-value color variable", svg)
	}
}

func TestBarChart_IncludesCurrentValueLabel(t *testing.T) {
	svg := BarChart([]float64{10, 20}, []string{"06:00", "18:00"}, "20.0°", BarChartOptions{Width: 220, Height: 60})
	if !contains(svg, "20.0") {
		t.Errorf("svg = %q, want it to contain the current value label", svg)
	}
}

func TestBarChart_IncludesAxisLabels(t *testing.T) {
	svg := BarChart([]float64{10, 15, 20}, []string{"06:00", "", "18:00"}, "20.0°", BarChartOptions{Width: 220, Height: 60})
	if !contains(svg, "06:00") || !contains(svg, "18:00") {
		t.Errorf("svg = %q, want both axis labels present", svg)
	}
}

func TestBarChart_FlatSeriesDoesNotDivideByZero(t *testing.T) {
	svg := BarChart([]float64{5, 5, 5}, []string{"", "", ""}, "5.0°", BarChartOptions{Width: 220, Height: 60})
	if contains(svg, "NaN") {
		t.Errorf("svg = %q, want no NaN for a flat series", svg)
	}
}

func TestBarChart_EscapesLabels(t *testing.T) {
	svg := BarChart([]float64{10}, []string{"<b>"}, "", BarChartOptions{Width: 220, Height: 60})
	if contains(svg, "<b>") {
		t.Errorf("svg = %q, want axis label HTML-escaped", svg)
	}
}

func TestBarChart_AppliesClassName(t *testing.T) {
	svg := BarChart([]float64{10, 20}, nil, "", BarChartOptions{Width: 220, Height: 60, ClassName: "ha-room-chart"})
	if !contains(svg, `class="ha-room-chart"`) {
		t.Errorf("svg = %q, want class=\"ha-room-chart\"", svg)
	}
}

func TestBarChart_EmptyValuesStillAppliesClassName(t *testing.T) {
	svg := BarChart(nil, nil, "", BarChartOptions{ClassName: "ha-room-chart"})
	if !contains(svg, `class="ha-room-chart"`) {
		t.Errorf("svg = %q, want class=\"ha-room-chart\" even for empty values", svg)
	}
}
