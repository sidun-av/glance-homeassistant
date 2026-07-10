package render

import "testing"

func TestSparkline_EmptyValuesReturnsBareSVG(t *testing.T) {
	svg := Sparkline(nil, DefaultSparklineOptions())
	if !contains(svg, "<svg") {
		t.Errorf("svg = %q, want it to contain <svg", svg)
	}
	if contains(svg, "polyline") {
		t.Errorf("svg = %q, want no polyline for empty values", svg)
	}
}

func TestSparkline_RendersOnePointPerValue(t *testing.T) {
	svg := Sparkline([]float64{1, 2, 3, 2, 1}, SparklineOptions{Width: 100, Height: 20})
	if !contains(svg, "polyline") {
		t.Errorf("svg = %q, want a polyline", svg)
	}
	if !contains(svg, "var(--color-primary)") {
		t.Errorf("svg = %q, want it to reference the theme's primary color variable", svg)
	}
}

func TestSparkline_FlatSeriesDoesNotDivideByZero(t *testing.T) {
	svg := Sparkline([]float64{5, 5, 5, 5}, SparklineOptions{Width: 100, Height: 20})
	if contains(svg, "NaN") {
		t.Errorf("svg = %q, want no NaN for a flat series", svg)
	}
}

func TestSparkline_SinglePointDoesNotPanic(t *testing.T) {
	svg := Sparkline([]float64{42}, SparklineOptions{Width: 100, Height: 20})
	if !contains(svg, "<svg") {
		t.Errorf("svg = %q, want it to contain <svg", svg)
	}
}
