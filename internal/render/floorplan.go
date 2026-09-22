package render

import (
	"fmt"
	"html"
	"sort"
	"strings"
)

// Floorplan is a validated room map: an ASCII grid (one string per row,
// whitespace-separated keys, "." for an empty cell) plus key → Home
// Assistant Area name. It maps 1:1 onto CSS grid-template-areas, so the
// same rules apply: every key must cover exactly one rectangle.
type Floorplan struct {
	Rows    int
	Columns int
	Keys    []string          // grid keys in row-major first-appearance order
	Areas   map[string]string // key → Area name
	// AspectRatio is the CSS aspect-ratio of the whole map ("4/3", "1.2").
	// Empty means Columns/Rows, i.e. square cells. Set it when the grid's
	// cell count does not match the flat's real proportions.
	AspectRatio string
	cells       [][]string
}

// ParseFloorplan validates grid+rooms from config. Errors name the
// offending key so a config typo is obvious at startup.
func ParseFloorplan(grid []string, rooms map[string]string) (*Floorplan, error) {
	if len(grid) == 0 {
		return nil, fmt.Errorf("floorplan.grid is empty")
	}
	fp := &Floorplan{Rows: len(grid), Areas: rooms}
	type box struct{ r0, c0, r1, c1, n int }
	boxes := map[string]*box{}
	for r, row := range grid {
		cells := strings.Fields(row)
		if r == 0 {
			fp.Columns = len(cells)
			if fp.Columns == 0 {
				return nil, fmt.Errorf("floorplan.grid row 1 is empty")
			}
		} else if len(cells) != fp.Columns {
			return nil, fmt.Errorf("floorplan.grid row %d has %d cells, expected %d", r+1, len(cells), fp.Columns)
		}
		fp.cells = append(fp.cells, cells)
		for c, key := range cells {
			if key == "." {
				continue
			}
			if _, ok := rooms[key]; !ok {
				return nil, fmt.Errorf("%q is used in floorplan.grid but missing from floorplan.rooms", key)
			}
			b, seen := boxes[key]
			if !seen {
				boxes[key] = &box{r, c, r, c, 1}
				fp.Keys = append(fp.Keys, key)
				continue
			}
			b.n++
			if r < b.r0 {
				b.r0 = r
			}
			if r > b.r1 {
				b.r1 = r
			}
			if c < b.c0 {
				b.c0 = c
			}
			if c > b.c1 {
				b.c1 = c
			}
		}
	}
	for key, b := range boxes {
		if (b.r1-b.r0+1)*(b.c1-b.c0+1) != b.n {
			return nil, fmt.Errorf("floorplan.grid: %q must cover one rectangle (CSS grid areas cannot be L-shaped or split)", key)
		}
	}
	var unused []string
	for key := range rooms {
		if _, ok := boxes[key]; !ok {
			unused = append(unused, key)
		}
	}
	if len(unused) > 0 {
		sort.Strings(unused)
		return nil, fmt.Errorf("%q is in floorplan.rooms but never used in floorplan.grid", unused[0])
	}
	return fp, nil
}

// TemplateAreas renders the grid as a CSS grid-template-areas value.
func (fp *Floorplan) TemplateAreas() string {
	rows := make([]string, len(fp.cells))
	for i, cells := range fp.cells {
		rows[i] = "'" + strings.Join(cells, " ") + "'" // single quotes: the value lands inside a double-quoted style attribute
	}
	return strings.Join(rows, " ")
}

// floorplanCSS: every colour is a Glance theme variable (or color-mix on
// one) so the map follows whichever theme the dashboard runs. Rooms are
// separated by a 2px gap that shows the widget background — that gap IS
// the wall. aspect-ratio on the container (set inline from the grid
// dimensions) is what makes the height follow the width.
const floorplanCSS = `
	.ha-floorplan{display:grid;gap:3px;width:100%;padding:3px;border-radius:var(--border-radius,8px);
	  background:var(--color-widget-content-border);border:1px solid var(--color-widget-content-border)}
	/* .ha-floorplan .ha-fp-room out-specifies the card layout's .ha-room base
	   rule (min-width, min-height, flex, border, padding), which the room
	   still carries so the live poller finds it by .ha-room[data-room]. */
	.ha-floorplan .ha-fp-room{position:relative;display:block;flex:none;min-width:0;min-height:0;padding:8px 9px;border:0;
	  border-radius:calc(var(--border-radius,8px) - 2px);background:var(--color-widget-background);transition:background .3s}
	.ha-floorplan .ha-fp-room[data-lit="true"]{background:color-mix(in srgb,#f0c479 14%,var(--color-widget-background))}
	.ha-floorplan .ha-fp-room[data-occupied="true"]{box-shadow:inset 0 0 0 1px color-mix(in srgb,var(--color-primary) 45%,transparent)}
	.ha-fp-name{position:absolute;top:7px;left:9px;font-size:12.5px;font-weight:600;color:var(--color-text-highlight);
	  white-space:nowrap;overflow:hidden;text-overflow:ellipsis;max-width:calc(100% - 60px)}
	.ha-fp-temp{position:absolute;top:6px;right:8px;display:inline-flex;align-items:center;gap:2px;font-size:12px;font-weight:600;color:var(--color-text-base);
	  padding:1px 6px;border-radius:12px;background:color-mix(in srgb,var(--color-text-base) 8%,transparent)}
	.ha-fp-trend{font-size:11px;line-height:1}
	.ha-fp-trend[data-trend="up"]{color:var(--color-negative)}
	.ha-fp-trend[data-trend="down"]{color:#7aa2f7}
	.ha-fp-lights{position:absolute;left:8px;bottom:7px;display:flex;flex-wrap:wrap;gap:6px;max-width:calc(100% - 16px)}
	.ha-fp-lights svg{width:20px;height:20px;flex:none}
	.ha-fp-status{position:absolute;right:8px;bottom:7px;display:flex;flex-direction:column;align-items:flex-end;gap:4px}
	.ha-fp-status .ha-occ-chip{font-size:10px;padding:2px 7px 2px 6px}
	.ha-fp-status .ha-badge{font-size:10px}
`

func renderFloorplan(data WidgetData) string {
	fp := data.Floorplan
	aspect := fp.AspectRatio
	if aspect == "" {
		aspect = fmt.Sprintf("%d/%d", fp.Columns, fp.Rows)
	}
	byRoom := map[string]RoomCardView{}
	for _, r := range data.Rooms {
		byRoom[r.Room] = r
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<div class="ha-floorplan" style="grid-template-areas:%s;grid-template-columns:repeat(%d,1fr);grid-template-rows:repeat(%d,1fr);aspect-ratio:%s">`,
		fp.TemplateAreas(), fp.Columns, fp.Rows, html.EscapeString(aspect))
	for _, key := range fp.Keys {
		area := fp.Areas[key]
		r, _ := byRoom[area] // a mapped room with no HA data is still drawn, empty
		r.Room = area
		b.WriteString(renderFloorplanRoom(key, r))
	}
	b.WriteString(`</div>`)
	return b.String()
}

func renderFloorplanRoom(key string, r RoomCardView) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<div class="ha-room ha-fp-room" data-room="%s" data-lit="%t" data-occupied="%t" style="grid-area:%s">`,
		html.EscapeString(r.Room), r.Lit, r.Occupied, html.EscapeString(key))
	fmt.Fprintf(&b, `<span class="ha-fp-name">%s</span>`, html.EscapeString(r.Room))
	if r.CurrentTemp != "" {
		b.WriteString(`<span class="ha-fp-temp">` + html.EscapeString(r.CurrentTemp))
		switch {
		case r.TempTrend > 0:
			b.WriteString(`<span class="ha-fp-trend" data-trend="up">↑</span>`)
		case r.TempTrend < 0:
			b.WriteString(`<span class="ha-fp-trend" data-trend="down">↓</span>`)
		}
		b.WriteString(`</span>`)
	}
	if len(r.Lights) > 0 {
		b.WriteString(`<span class="ha-fp-lights">`)
		for _, l := range r.Lights {
			fmt.Fprintf(&b, `<span class="ha-light" data-entity-id="%s" data-on="%t">%s</span>`,
				html.EscapeString(l.EntityID), l.On, l.IconSVG)
		}
		b.WriteString(`</span>`)
	}
	if len(r.Occupancy) > 0 || len(r.Contacts) > 0 {
		b.WriteString(`<span class="ha-fp-status">`)
		for _, o := range r.Occupancy {
			fmt.Fprintf(&b, `<span class="ha-occ-chip" data-sensor-name="%s" data-occupied="%t"><span class="ha-occ-dot"></span><span class="ha-occ-label">%s</span></span>`,
				html.EscapeString(o.Name), o.Attention, html.EscapeString(o.Label))
		}
		for _, c := range r.Contacts {
			fmt.Fprintf(&b, `<span class="ha-badge" data-sensor-name="%s" data-open="%t">%s<span class="ha-contact-label">%s</span></span>`,
				html.EscapeString(c.Name), c.Attention, ContactIcon(), html.EscapeString(c.Label))
		}
		b.WriteString(`</span>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}
