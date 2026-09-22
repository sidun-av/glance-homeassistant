package render

import (
	"fmt"
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
		`data-slot="t" class="ha-light" data-entity-id="light.lr_main"`,
		`data-slot="b" class="ha-badge" data-sensor-name="LR Window"`,
		`class="ha-fp-icons" style="--reach:0.92"`,
		`class="ha-fp-icons"`,
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

func TestRenderFloorplan_MaxWidth(t *testing.T) {
	fp, _ := ParseFloorplan([]string{"a"}, map[string]string{"a": "A"})
	data := WidgetData{Layout: "floorplan", Floorplan: fp}
	if strings.Contains(RenderWidget(data), `aspect-ratio:1/1;max-width:`) {
		t.Error("no max-width by default")
	}
	fp.MaxWidth = 420
	if !strings.Contains(RenderWidget(data), `aspect-ratio:1/1;max-width:420px"`) {
		t.Error("max_width must land on the map container")
	}
}

func TestRenderFloorplanRoom_OccupancyIsOutlineOnly(t *testing.T) {
	html := renderFloorplanRoom("k", RoomCardView{Room: "R", Occupied: true, Occupancy: []SensorBadgeView{{Name: "M", Attention: true, Label: "Occupied"}}})
	if strings.Contains(html, "ha-occ-chip") || strings.Contains(html, "ha-fp-icons") {
		t.Errorf("occupancy must not render a tile: %s", html)
	}
	if !strings.Contains(html, `data-occupied="true"`) {
		t.Error("room must still carry data-occupied for the outline")
	}
}

func TestRenderFloorplanRoom_ReachShortensWithSources(t *testing.T) {
	r := RoomCardView{Room: "R", Lights: []LightView{{EntityID: "a"}, {EntityID: "b"}}, Devices: []DeviceView{{EntityID: "m", Effect: "music"}}}
	if !strings.Contains(renderFloorplanRoom("k", r), `style="--reach:0.70"`) {
		t.Error("two lights + a speaker → reach 0.7 (music casts no beam)")
	}
	r.Lights = append(r.Lights, LightView{EntityID: "c"})
	if !strings.Contains(renderFloorplanRoom("k", r), `style="--reach:0.55"`) {
		t.Error("three lights → reach 0.55")
	}
}

func TestFloorplanCSS_LightSpill(t *testing.T) {
	for _, want := range []string{`.ha-fp-icons .ha-light[data-on="true"]::before{opacity:1`, `overflow:hidden`} {
		if !strings.Contains(floorplanCSS, want) {
			t.Errorf("floorplan CSS missing %q", want)
		}
	}
}

func TestRenderFloorplanRoom_DevicesAndSlotOverflow(t *testing.T) {
	r := RoomCardView{Room: "R"}
	for i := 0; i < 9; i++ {
		r.Devices = append(r.Devices, DeviceView{EntityID: fmt.Sprintf("fan.%d", i), Name: "Fan", IconSVG: "<svg/>", On: i == 0, Effect: "fan"})
	}
	html := renderFloorplanRoom("k", r)
	for _, want := range []string{
		`data-slot="t" class="ha-device" data-entity-id="fan.0" data-on="true" data-effect="fan" title="Fan"`,
		`data-slot="br" class="ha-device" data-entity-id="fan.7"`,
		`data-slot="c" data-center="true" class="ha-device" data-entity-id="fan.8"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in %s", want, html)
		}
	}
}

func TestRenderPlacedRoom(t *testing.T) {
	r := RoomCardView{Room: "Bedroom",
		Lights:   []LightView{{EntityID: "light.a", IconSVG: "<svg/>", On: true}, {EntityID: "light.b", IconSVG: "<svg/>"}, {EntityID: "light.hidden", IconSVG: "<svg/>"}},
		Contacts: []SensorBadgeView{{Name: "Door", Label: "Closed"}}}
	p := RoomPlacement{Rows: 3, Columns: 3, Name: "Спальня",
		Cells:  map[string][2]int{"light.a": {0, 0}, "Door": {1, 1}},
		Hidden: map[string]bool{"light.hidden": true}}
	html := renderPlacedRoom("bedroom", r, p)
	for _, want := range []string{
		`<span class="ha-fp-name">Спальня</span>`,
		`class="ha-fp-icons ha-fp-placed" style="--reach:0.70;grid-template-rows:repeat(3,1fr);grid-template-columns:repeat(3,1fr)"`,
		`style="grid-area:1/1;place-self:start start;--dir:-45deg;--len:calc(max(33.33cqw,33.33cqh) + 0.41 * min(33.33cqw,33.33cqh))" class="ha-light" data-entity-id="light.a"`,
		`data-slot="t" class="ha-light" data-entity-id="light.b"`, // unplaced → first wall slot
		`style="grid-area:2/2;place-self:center center;--dir:0deg;--len:0px" data-center="true" class="ha-badge" data-sensor-name="Door"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in\n%s", want, html)
		}
	}
	if strings.Contains(html, "light.hidden") {
		t.Error("hidden entity must not render")
	}
}

func TestPlacedTileStyle_Directions(t *testing.T) {
	cases := map[[2]int]string{{0, 1}: "--dir:0deg", {2, 1}: "--dir:180deg", {1, 0}: "--dir:-90deg", {1, 2}: "--dir:90deg", {2, 2}: "--dir:135deg"}
	for cell, want := range cases {
		if got := placedTileStyle(cell, 3, 3); !strings.Contains(got, want) {
			t.Errorf("%v: %s (want %s)", cell, got, want)
		}
	}
}

func TestRenderWidget_FloorplanGear(t *testing.T) {
	fp, _ := ParseFloorplan([]string{"a"}, map[string]string{"a": "A"})
	data := WidgetData{Layout: "floorplan", Floorplan: fp, EditURL: "/ha-widget/edit"}
	if !strings.Contains(RenderWidget(data), `<a class="ha-fp-edit" href="/ha-widget/edit"`) {
		t.Error("gear link missing")
	}
	data.EditURL = ""
	if strings.Contains(RenderWidget(data), "ha-fp-edit\" href") {
		t.Error("no gear without EditURL")
	}
}

func TestRenderWidget_NowPlayingPanel(t *testing.T) {
	fp, _ := ParseFloorplan([]string{"a"}, map[string]string{"a": "A"})
	data := WidgetData{Layout: "floorplan", Floorplan: fp, MediaURL: "/ha-widget/media",
		Media: []MediaView{{EntityID: "media_player.x", Name: "Speaker", Room: "Kitchen", State: "playing", Title: "Song", Artist: "Band", Accent: "#f0a6c8", Position: 61, Duration: 200, PositionAt: 1700000000000, ArtURL: "/ha-widget/art?entity_id=media_player.x", Volume: 0.35}},
		Rooms: []RoomCardView{{Room: "A", Devices: []DeviceView{{EntityID: "media_player.x", Effect: "music", On: true, Accent: "#f0a6c8", IconSVG: "<svg/>"}}}}}
	html := RenderWidget(data)
	for _, want := range []string{
		`<div class="ha-fp-layout">`,
		`<div class="ha-np" data-media-url="/ha-widget/media">`,
		`class="ha-np-row" data-entity-id="media_player.x" data-state="playing" data-position="61" data-duration="200" data-position-at="1700000000000" style="--accent:#f0a6c8"`,
		`Kitchen · Speaker`, `<span class="ha-np-title">Song</span>`, `<span class="ha-np-artist">Band</span>`,
		`data-action="media_play_pause"`, `data-action="media_next_track"`,
		`data-accent="#f0a6c8"`, `.ha-device[data-accent="#f0a6c8"]{--accent:#f0a6c8}`,
		`data-position="61" data-duration="200" data-position-at="1700000000000"`,
		`<span class="ha-np-art" data-has-art="true"><img src="/ha-widget/art?entity_id=media_player.x"`,
		`<span class="ha-np-time">1:01</span>`, `style="width:30.5%"`, `<span class="ha-np-total">3:20</span>`,
		`<input type="range" min="0" max="100" step="1" value="35" style="--pct:35%"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q", want)
		}
	}
	data.Media = nil
	if !strings.Contains(RenderWidget(data), `nothing playing`) {
		t.Error("empty panel placeholder missing")
	}
	data.MediaURL = ""
	if strings.Contains(RenderWidget(data), `class="ha-np-btn"`) {
		t.Error("no controls without MediaURL")
	}
}

func TestClock(t *testing.T) {
	for in, want := range map[float64]string{0: "0:00", 61: "1:01", 3599: "59:59", 3661: "1:01:01", -5: "0:00"} {
		if got := Clock(in); got != want {
			t.Errorf("Clock(%v)=%q want %q", in, got, want)
		}
	}
}

func TestRenderNowPlayingRow_NoVolumeHidesSlider(t *testing.T) {
	html := renderNowPlayingRow(MediaView{EntityID: "media_player.x", State: "idle", Volume: -1}, true)
	if !strings.Contains(html, `<span class="ha-np-vol" hidden`) {
		t.Errorf("slider must be hidden without a volume: %s", html)
	}
}
