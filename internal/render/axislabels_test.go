package render

import (
	"strings"
	"testing"
)

func TestAxisLabelsRow_RendersOnlyNonEmptyLabels(t *testing.T) {
	row := AxisLabelsRow([]string{"8am", "", "7pm", "", "8am"})
	if got := strings.Count(row, "<span"); got != 3 {
		t.Errorf("span count = %d, want 3 (only non-empty labels)", got)
	}
	if !contains(row, "8am") || !contains(row, "7pm") {
		t.Errorf("row = %q, missing label text", row)
	}
}

func TestAxisLabelsRow_EmptyInputRendersNothing(t *testing.T) {
	if row := AxisLabelsRow(nil); row != "" {
		t.Errorf("row = %q, want empty string for no labels", row)
	}
	if row := AxisLabelsRow([]string{"", "", ""}); row != "" {
		t.Errorf("row = %q, want empty string when every entry is empty", row)
	}
}

func TestAxisLabelsRow_EscapesLabels(t *testing.T) {
	row := AxisLabelsRow([]string{"<b>"})
	if contains(row, "<b>") {
		t.Errorf("row = %q, want label HTML-escaped", row)
	}
}
