package render

import (
	"strings"
	"testing"
)

func sampleGrid() []string {
	return []string{
		"bedroom bedroom kitchen",
		"bedroom bedroom kitchen",
		"bath    hall    kitchen",
	}
}

func sampleRooms() map[string]string {
	return map[string]string{"bedroom": "Bedroom", "kitchen": "Kitchen", "bath": "Bathroom", "hall": "Hallway"}
}

func TestParseFloorplan_Valid(t *testing.T) {
	fp, err := ParseFloorplan(sampleGrid(), sampleRooms())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fp.Columns != 3 || fp.Rows != 3 {
		t.Fatalf("want 3x3, got %dx%d", fp.Columns, fp.Rows)
	}
	if got := fp.TemplateAreas(); got != `'bedroom bedroom kitchen' 'bedroom bedroom kitchen' 'bath hall kitchen'` {
		t.Fatalf("template areas: %q", got)
	}
	// Order follows first appearance in the grid (row-major), so the
	// rendered DOM is deterministic regardless of map iteration order.
	want := []string{"bedroom", "kitchen", "bath", "hall"}
	if len(fp.Keys) != len(want) {
		t.Fatalf("keys: %v", fp.Keys)
	}
	for i, k := range want {
		if fp.Keys[i] != k {
			t.Fatalf("keys order: %v", fp.Keys)
		}
	}
}

func TestParseFloorplan_DotIsEmptyCell(t *testing.T) {
	fp, err := ParseFloorplan([]string{"a .", ". b"}, map[string]string{"a": "A", "b": "B"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := fp.TemplateAreas(); got != `'a .' '. b'` {
		t.Fatalf("template areas: %q", got)
	}
}

func TestParseFloorplan_Errors(t *testing.T) {
	cases := []struct {
		name  string
		grid  []string
		rooms map[string]string
		want  string
	}{
		{"empty grid", nil, sampleRooms(), "floorplan.grid is empty"},
		{"ragged rows", []string{"a a", "a"}, map[string]string{"a": "A"}, "row 2 has 1 cells, expected 2"},
		{"key not in rooms", sampleGrid(), map[string]string{"bedroom": "Bedroom"}, `"kitchen" is used in floorplan.grid but missing from floorplan.rooms`},
		{"room not in grid", []string{"a"}, map[string]string{"a": "A", "b": "B"}, `"b" is in floorplan.rooms but never used in floorplan.grid`},
		{"non-rectangular", []string{"a a", "a ."}, map[string]string{"a": "A"}, `"a" must cover one rectangle`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseFloorplan(c.grid, c.rooms)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error containing %q, got %v", c.want, err)
			}
		})
	}
}

func TestRenderWidget_FloorplanLayout(t *testing.T) {
	fp, err := ParseFloorplan(sampleGrid(), sampleRooms())
	if err != nil {
		t.Fatal(err)
	}
	data := sampleWidgetData()
	data.Layout = "floorplan"
	data.Floorplan = fp
	bedroom := sampleRoomCard()
	bedroom.Room = "Bedroom"
	bedroom.CurrentTemp = "22°"
	bedroom.TempTrend = 1
	data.Rooms = []RoomCardView{bedroom}

	html := RenderWidget(data)

	for _, want := range []string{
		`class="ha-floorplan"`,
		`grid-template-areas:'bedroom bedroom kitchen' 'bedroom bedroom kitchen' 'bath hall kitchen'`,
		`grid-template-columns:repeat(3,1fr)`,
		`grid-template-rows:repeat(3,1fr)`,
		`aspect-ratio:3/3`,
		`class="ha-room ha-fp-room" data-room="Bedroom" data-lit="true" data-occupied="true" style="grid-area:bedroom"`,
		`<span class="ha-fp-name">Bedroom</span>`,
		`<span class="ha-fp-temp">22°<span class="ha-fp-trend" data-trend="up">↑</span></span>`,
		`data-entity-id="light.lr_main" data-on="true"`,
		`data-sensor-name="LR Motion" data-occupied="true"`,
		`data-sensor-name="LR Window" data-open="true"`,
		// Mapped rooms with no HA data still get drawn, empty, so the map keeps its shape.
		`class="ha-room ha-fp-room" data-room="Kitchen" data-lit="false" data-occupied="false" style="grid-area:kitchen"`,
		`<span class="ha-fp-name">Hallway</span>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("floorplan HTML missing %q", want)
		}
	}
	if strings.Contains(html, `class="ha-rooms"`) {
		t.Error("floorplan layout must not render the cards container")
	}
	if strings.Contains(html, `<svg>lr</svg>`) {
		t.Error("floorplan layout must not render the temperature chart")
	}
	// Kitchen has no data → no temp badge for it.
	if strings.Count(html, `class="ha-fp-temp"`) != 1 {
		t.Errorf("want exactly one temp badge, html: %s", html)
	}
	// Room order follows the grid, not the HA data order.
	if strings.Index(html, `data-room="Bedroom"`) > strings.Index(html, `data-room="Kitchen"`) ||
		strings.Index(html, `data-room="Kitchen"`) > strings.Index(html, `data-room="Bathroom"`) {
		t.Error("rooms must render in grid order")
	}
}

func TestRenderWidget_CardsLayoutIsDefault(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if !strings.Contains(html, `class="ha-rooms"`) || strings.Contains(html, `class="ha-floorplan"`) {
		t.Error("empty Layout must render the cards layout")
	}
}

func TestRenderFloorplanRoom_TrendArrow(t *testing.T) {
	for trend, want := range map[int]string{0: "", -1: `data-trend="down">↓`, 1: `data-trend="up">↑`} {
		html := renderFloorplanRoom("k", RoomCardView{Room: "R", CurrentTemp: "20°", TempTrend: trend})
		if want == "" {
			if strings.Contains(html, "ha-fp-trend") {
				t.Errorf("trend %d: unexpected arrow in %s", trend, html)
			}
		} else if !strings.Contains(html, want) {
			t.Errorf("trend %d: missing %q in %s", trend, want, html)
		}
	}
}

func TestRenderFloorplan_AspectRatioOverride(t *testing.T) {
	fp, _ := ParseFloorplan([]string{"a b"}, map[string]string{"a": "A", "b": "B"})
	data := WidgetData{Layout: "floorplan", Floorplan: fp}
	if !strings.Contains(RenderWidget(data), "aspect-ratio:2/1") {
		t.Error("default aspect must be columns/rows")
	}
	fp.AspectRatio = "1.15"
	if !strings.Contains(RenderWidget(data), "aspect-ratio:1.15\"") {
		t.Error("explicit aspect_ratio must be used verbatim")
	}
}
