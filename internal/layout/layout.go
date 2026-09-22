// Package layout is the editable floorplan: what the browser editor reads
// and writes, persisted as JSON on a volume, and converted into the
// render package's Floorplan for drawing. The YAML/env `floorplan:` config
// is only the seed used when no saved layout exists yet.
package layout

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/sidun-av/glance-homeassistant/internal/render"
)

type Cell [2]int // [row, col]

type Grid struct {
	Rows    int  `json:"rows"`
	Columns int  `json:"columns"`
	Auto    bool `json:"auto,omitempty"` // follow the room's footprint on the map (rows×cols of its cells)
}

type Room struct {
	Key      string          `json:"key"`
	Area     string          `json:"area"`
	Name     string          `json:"name,omitempty"`
	Cells    []Cell          `json:"cells"`
	Grid     Grid            `json:"grid"`
	Entities map[string]Cell `json:"entities,omitempty"`
	Hidden   []string        `json:"hidden,omitempty"`
}

type Layout struct {
	Version     int    `json:"version"`
	AspectRatio string `json:"aspect_ratio,omitempty"`
	MaxWidth    int    `json:"max_width,omitempty"`
	Columns     int    `json:"columns"`
	Rows        int    `json:"rows"`
	Rooms       []Room `json:"rooms"`
}

var keyRe = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// Validate normalises and checks a layout: keys well-formed and unique,
// every room one non-empty rectangle inside the map, no two rooms sharing
// a cell, inner grids at least 1x1, placed entities inside their room's
// inner grid. Errors name the room so the editor can show them as-is.
func (l *Layout) Validate() error {
	if l.Columns < 1 || l.Rows < 1 {
		return fmt.Errorf("map must be at least 1x1, got %dx%d", l.Columns, l.Rows)
	}
	if l.Columns > 24 || l.Rows > 24 {
		return fmt.Errorf("map is limited to 24x24 cells, got %dx%d", l.Columns, l.Rows)
	}
	if len(l.Rooms) == 0 {
		return fmt.Errorf("layout has no rooms")
	}
	if l.AspectRatio != "" && !aspectRe.MatchString(strings.TrimSpace(l.AspectRatio)) {
		return fmt.Errorf("aspect_ratio %q must look like \"4/3\" or \"1.15\"", l.AspectRatio)
	}
	if l.MaxWidth < 0 {
		return fmt.Errorf("max_width must not be negative")
	}
	seen := map[string]bool{}
	taken := map[Cell]string{}
	for i := range l.Rooms {
		r := &l.Rooms[i]
		r.Key = strings.TrimSpace(r.Key)
		r.Area = strings.TrimSpace(r.Area)
		r.Name = strings.TrimSpace(r.Name)
		label := r.Key
		if r.Name != "" {
			label = r.Name
		}
		if !keyRe.MatchString(r.Key) {
			return fmt.Errorf("room %q: key must be lowercase letters/digits/_/- and start with a letter", label)
		}
		if seen[r.Key] {
			return fmt.Errorf("room %q: duplicate key", r.Key)
		}
		seen[r.Key] = true
		if r.Area == "" {
			return fmt.Errorf("room %q: no Home Assistant area assigned", label)
		}
		if len(r.Cells) == 0 {
			return fmt.Errorf("room %q: has no cells on the map", label)
		}
		minR, minC, maxR, maxC := l.Rows, l.Columns, -1, -1
		for _, c := range r.Cells {
			if c[0] < 0 || c[0] >= l.Rows || c[1] < 0 || c[1] >= l.Columns {
				return fmt.Errorf("room %q: cell [%d,%d] is outside the %dx%d map", label, c[0], c[1], l.Columns, l.Rows)
			}
			if owner, ok := taken[c]; ok {
				return fmt.Errorf("room %q: cell [%d,%d] is already used by room %q", label, c[0], c[1], owner)
			}
			taken[c] = r.Key
			minR, minC = min(minR, c[0]), min(minC, c[1])
			maxR, maxC = max(maxR, c[0]), max(maxC, c[1])
		}
		if (maxR-minR+1)*(maxC-minC+1) != len(r.Cells) {
			return fmt.Errorf("room %q: must be one solid rectangle", label)
		}
		if r.Grid.Auto || (r.Grid.Rows < 1 && r.Grid.Columns < 1) {
			// An automatic grid mirrors the room's footprint, so a 2x4 room
			// on the map gets a 2x4 grid inside — and keeps following the
			// map until the user sets the grid by hand.
			r.Grid = Grid{Rows: maxR - minR + 1, Columns: maxC - minC + 1, Auto: true}
		}
		if r.Grid.Rows < 1 {
			r.Grid.Rows = 1
		}
		if r.Grid.Columns < 1 {
			r.Grid.Columns = 1
		}
		if r.Grid.Rows > 12 || r.Grid.Columns > 12 {
			return fmt.Errorf("room %q: inner grid is limited to 12x12", label)
		}
		for id, c := range r.Entities {
			if c[0] < 0 || c[0] >= r.Grid.Rows || c[1] < 0 || c[1] >= r.Grid.Columns {
				return fmt.Errorf("room %q: %s is placed at [%d,%d], outside its %dx%d grid", label, id, c[0], c[1], r.Grid.Columns, r.Grid.Rows)
			}
		}
		sort.Slice(r.Cells, func(a, b int) bool {
			if r.Cells[a][0] != r.Cells[b][0] {
				return r.Cells[a][0] < r.Cells[b][0]
			}
			return r.Cells[a][1] < r.Cells[b][1]
		})
	}
	if l.Version == 0 {
		l.Version = 1
	}
	return nil
}

var aspectRe = regexp.MustCompile(`^\d+(\.\d+)?(\s*/\s*\d+(\.\d+)?)?$`)

// ToFloorplan converts a validated layout into what the renderer draws.
func (l *Layout) ToFloorplan() (*render.Floorplan, error) {
	cells := make([][]string, l.Rows)
	for r := range cells {
		cells[r] = make([]string, l.Columns)
		for c := range cells[r] {
			cells[r][c] = "."
		}
	}
	rooms := map[string]string{}
	placement := map[string]render.RoomPlacement{}
	for _, room := range l.Rooms {
		for _, c := range room.Cells {
			cells[c[0]][c[1]] = room.Key
		}
		rooms[room.Key] = room.Area
		p := render.RoomPlacement{Rows: room.Grid.Rows, Columns: room.Grid.Columns, Cells: map[string][2]int{}, Hidden: map[string]bool{}, Name: room.Name}
		for id, c := range room.Entities {
			p.Cells[id] = [2]int{c[0], c[1]}
		}
		for _, id := range room.Hidden {
			p.Hidden[id] = true
		}
		placement[room.Key] = p
	}
	grid := make([]string, l.Rows)
	for r := range cells {
		grid[r] = strings.Join(cells[r], " ")
	}
	fp, err := render.ParseFloorplan(grid, rooms)
	if err != nil {
		return nil, err
	}
	fp.AspectRatio = strings.TrimSpace(l.AspectRatio)
	fp.MaxWidth = l.MaxWidth
	fp.Placement = placement
	return fp, nil
}

// FromFloorplan seeds a layout from the config-defined floorplan so the
// editor starts from what is on the dashboard today.
func FromFloorplan(fp *render.Floorplan) *Layout {
	l := &Layout{Version: 1, AspectRatio: fp.AspectRatio, MaxWidth: fp.MaxWidth, Columns: fp.Columns, Rows: fp.Rows}
	cells := fp.CellsByKey()
	for _, key := range fp.Keys {
		room := Room{Key: key, Area: fp.Areas[key], Grid: Grid{Auto: true}}
		for _, c := range cells[key] {
			room.Cells = append(room.Cells, Cell{c[0], c[1]})
		}
		l.Rooms = append(l.Rooms, room)
	}
	return l
}

// Load reads a saved layout; a missing file is (nil, nil).
func Load(path string) (*Layout, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var l Layout
	if err := json.Unmarshal(raw, &l); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := l.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &l, nil
}

// Save writes atomically (temp file + rename) so a crash mid-write never
// leaves a half layout behind.
func Save(path string, l *Layout) error {
	if err := l.Validate(); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".floorplan-*.json")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
