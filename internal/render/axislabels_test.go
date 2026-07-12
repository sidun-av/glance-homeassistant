package render

import (
	"strings"
	"testing"
)

func TestAxisLabelsRow_RendersASpanPerLabelWithItsTier(t *testing.T) {
	row := AxisLabelsRow([]AxisLabel{{Text: "8am", Tier: 0}, {Text: "2pm", Tier: 3}, {Text: "7pm", Tier: 0}})
	if got := strings.Count(row, "<span"); got != 3 {
		t.Errorf("span count = %d, want 3", got)
	}
	if !contains(row, `<span data-tier="0">8am</span>`) {
		t.Errorf("row = %q, missing tier-0 8am span", row)
	}
	if !contains(row, `<span data-tier="3">2pm</span>`) {
		t.Errorf("row = %q, missing tier-3 2pm span", row)
	}
}

func TestAxisLabelsRow_EmptyInputRendersNothing(t *testing.T) {
	if row := AxisLabelsRow(nil); row != "" {
		t.Errorf("row = %q, want empty string for no labels", row)
	}
}

func TestAxisLabelsRow_EscapesLabels(t *testing.T) {
	row := AxisLabelsRow([]AxisLabel{{Text: "<b>", Tier: 0}})
	if contains(row, "<b>") {
		t.Errorf("row = %q, want label HTML-escaped", row)
	}
}
