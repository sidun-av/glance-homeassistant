package layout

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sidun-av/glance-homeassistant/internal/render"
)

func sample() *Layout {
	return &Layout{
		Columns: 4, Rows: 4, AspectRatio: "1.15", MaxWidth: 420,
		Rooms: []Room{
			{Key: "bedroom", Area: "Bedroom", Cells: []Cell{{0, 0}, {0, 1}, {1, 0}, {1, 1}}, Grid: Grid{Rows: 3, Columns: 3},
				Entities: map[string]Cell{"light.bed": {0, 1}}, Hidden: []string{"switch.x"}},
			{Key: "kitchen", Area: "Kitchen", Cells: []Cell{{0, 2}, {0, 3}, {1, 2}, {1, 3}, {2, 2}, {2, 3}, {3, 2}, {3, 3}}},
			{Key: "bath", Area: "Bathroom", Cells: []Cell{{2, 0}, {3, 0}}},
			{Key: "hall", Area: "Hallway", Name: "Прихожая", Cells: []Cell{{2, 1}, {3, 1}}},
		},
	}
}

func TestValidate_OK_AndDefaults(t *testing.T) {
	l := sample()
	if err := l.Validate(); err != nil {
		t.Fatal(err)
	}
	if l.Version != 1 || l.Rooms[1].Grid != (Grid{Rows: 4, Columns: 2, Auto: true}) {
		t.Errorf("defaults not applied: version=%d grid=%+v (kitchen is 2 wide x 4 tall on the map)", l.Version, l.Rooms[1].Grid)
	}
	if l.Rooms[0].Grid != (Grid{Rows: 3, Columns: 3}) {
		t.Errorf("explicit grid with placed entities must be kept: %+v", l.Rooms[0].Grid)
	}
	// Legacy: an untouched 3x3 (the old default) becomes automatic.
	l2 := sample()
	l2.Rooms[1].Grid = Grid{Rows: 3, Columns: 3}
	if err := l2.Validate(); err != nil {
		t.Fatal(err)
	}
	if l2.Rooms[1].Grid != (Grid{Rows: 4, Columns: 2, Auto: true}) {
		t.Errorf("legacy empty 3x3 must migrate to auto: %+v", l2.Rooms[1].Grid)
	}
}

func TestValidate_Errors(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Layout)
		want string
	}{
		{"bad key", func(l *Layout) { l.Rooms[0].Key = "Bed Room" }, "key must be lowercase"},
		{"dup key", func(l *Layout) { l.Rooms[1].Key = "bedroom" }, "duplicate key"},
		{"no area", func(l *Layout) { l.Rooms[0].Area = "" }, "no Home Assistant area"},
		{"outside map", func(l *Layout) { l.Rooms[2].Cells = []Cell{{9, 0}} }, "outside the 4x4 map"},
		{"overlap", func(l *Layout) { l.Rooms[2].Cells = []Cell{{0, 0}} }, `already used by room "bedroom"`},
		{"L-shape", func(l *Layout) { l.Rooms[0].Cells = []Cell{{0, 0}, {0, 1}, {1, 0}} }, "one solid rectangle"},
		{"entity outside grid", func(l *Layout) { l.Rooms[0].Entities["light.bed"] = Cell{3, 0} }, "outside its 3x3 grid"},
		{"no rooms", func(l *Layout) { l.Rooms = nil }, "no rooms"},
		{"bad aspect", func(l *Layout) { l.AspectRatio = "wide" }, "aspect_ratio"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := sample()
			c.mut(l)
			err := l.Validate()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error containing %q, got %v", c.want, err)
			}
		})
	}
}

func TestToFloorplan_MatchesYAMLPath(t *testing.T) {
	l := sample()
	if err := l.Validate(); err != nil {
		t.Fatal(err)
	}
	fp, err := l.ToFloorplan()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := render.ParseFloorplan([]string{
		"bedroom bedroom kitchen kitchen",
		"bedroom bedroom kitchen kitchen",
		"bath hall kitchen kitchen",
		"bath hall kitchen kitchen",
	}, map[string]string{"bedroom": "Bedroom", "kitchen": "Kitchen", "bath": "Bathroom", "hall": "Hallway"})
	if fp.TemplateAreas() != want.TemplateAreas() {
		t.Errorf("template areas differ:\n%s\n%s", fp.TemplateAreas(), want.TemplateAreas())
	}
	if fp.AspectRatio != "1.15" || fp.MaxWidth != 420 {
		t.Errorf("aspect/max_width not carried: %q %d", fp.AspectRatio, fp.MaxWidth)
	}
	p := fp.Placement["bedroom"]
	if p.Cells["light.bed"] != [2]int{0, 1} || !p.Hidden["switch.x"] || p.Rows != 3 {
		t.Errorf("placement not carried: %+v", p)
	}
	if fp.Placement["hall"].Name != "Прихожая" {
		t.Error("room display name not carried")
	}
}

func TestFromFloorplan_RoundTrip(t *testing.T) {
	fp, _ := render.ParseFloorplan([]string{"a a .", "b . ."}, map[string]string{"a": "A", "b": "B"})
	fp.AspectRatio = "2"
	l := FromFloorplan(fp)
	if err := l.Validate(); err != nil {
		t.Fatal(err)
	}
	if l.Columns != 3 || l.Rows != 2 || len(l.Rooms) != 2 || len(l.Rooms[0].Cells) != 2 || l.Rooms[0].Key != "a" || l.Rooms[0].Grid != (Grid{Rows: 1, Columns: 2, Auto: true}) {
		t.Errorf("seed wrong: %+v", l)
	}
	back, err := l.ToFloorplan()
	if err != nil || back.TemplateAreas() != fp.TemplateAreas() {
		t.Errorf("round trip: %v %q vs %q", err, back.TemplateAreas(), fp.TemplateAreas())
	}
}

func TestSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "floorplan.json")
	if got, err := Load(path); got != nil || err != nil {
		t.Fatalf("missing file must be (nil, nil), got %v %v", got, err)
	}
	if err := Save(path, sample()); err != nil {
		t.Fatal(err)
	}
	l, err := Load(path)
	if err != nil || l == nil || len(l.Rooms) != 4 || l.Rooms[0].Entities["light.bed"] != (Cell{0, 1}) {
		t.Fatalf("load: %v %+v", err, l)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("temp file left behind: %v", entries)
	}
	os.WriteFile(path, []byte("{not json"), 0o644)
	if _, err := Load(path); err == nil {
		t.Error("corrupt file must error, not silently fall back")
	}
}
