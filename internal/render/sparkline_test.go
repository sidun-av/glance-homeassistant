package render

import "testing"

func TestSparkline_EmptyValuesReturnsBareSVG(t *testing.T) {
	svg := Sparkline(nil, nil, DefaultSparklineOptions())
	if !contains(svg, "<svg") {
		t.Errorf("svg = %q, want it to contain <svg", svg)
	}
	if contains(svg, "polyline") {
		t.Errorf("svg = %q, want no polyline for empty values", svg)
	}
}

func TestSparkline_RendersOnePointPerValue(t *testing.T) {
	svg := Sparkline([]float64{1, 2, 3, 2, 1}, nil, SparklineOptions{Width: 100, Height: 20})
	if !contains(svg, "polyline") {
		t.Errorf("svg = %q, want a polyline", svg)
	}
	if !contains(svg, "var(--color-progress-value)") {
		t.Errorf("svg = %q, want it to reference the theme's progress-value color variable", svg)
	}
}

func TestSparkline_FlatSeriesDoesNotDivideByZero(t *testing.T) {
	svg := Sparkline([]float64{5, 5, 5, 5}, nil, SparklineOptions{Width: 100, Height: 20})
	if contains(svg, "NaN") {
		t.Errorf("svg = %q, want no NaN for a flat series", svg)
	}
}

func TestSparkline_SinglePointDoesNotPanic(t *testing.T) {
	svg := Sparkline([]float64{42}, nil, SparklineOptions{Width: 100, Height: 20})
	if !contains(svg, "<svg") {
		t.Errorf("svg = %q, want it to contain <svg", svg)
	}
}

func TestSparkline_IncludesAxisLabels(t *testing.T) {
	svg := Sparkline([]float64{10, 15, 20}, []string{"06:00", "", "18:00"}, SparklineOptions{Width: 220, Height: 34})
	if !contains(svg, "06:00") || !contains(svg, "18:00") {
		t.Errorf("svg = %q, want both axis labels present", svg)
	}
}

func TestSparkline_EscapesAxisLabels(t *testing.T) {
	svg := Sparkline([]float64{10, 20}, []string{"<b>", ""}, SparklineOptions{Width: 220, Height: 34})
	if contains(svg, "<b>") {
		t.Errorf("svg = %q, want axis label HTML-escaped", svg)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (func() bool {
		for i := 0; i+len(substr) <= len(s); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()
}
