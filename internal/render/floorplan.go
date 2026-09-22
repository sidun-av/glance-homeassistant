package render

import (
	"fmt"
	"html"
	"math"
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
	// MaxWidth caps the map's width in px (0 = fill the widget). With
	// aspect-ratio in play this is effectively the map's overall size.
	MaxWidth int
	// Placement, when present for a key, is the editor's per-room layout:
	// an inner grid and explicit cells for entities. Rooms without one
	// use the automatic wall slots.
	Placement map[string]RoomPlacement
	cells     [][]string
}

// RoomPlacement is one room's editor-defined interior.
type RoomPlacement struct {
	Rows, Columns int
	Cells         map[string][2]int // entity_id → [row, col] in the inner grid
	Hidden        map[string]bool   // entity_ids not drawn at all
	Name          string            // display name override ("" = Area name)
}

// CellsByKey returns every map cell ([row, col]) each key covers.
func (fp *Floorplan) CellsByKey() map[string][][2]int {
	out := map[string][][2]int{}
	for r, row := range fp.cells {
		for c, key := range row {
			if key != "." {
				out[key] = append(out[key], [2]int{r, c})
			}
		}
	}
	return out
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
	.ha-floorplan .ha-fp-room{position:relative;display:block;flex:none;min-width:0;min-height:0;padding:0;border:0;overflow:hidden;container-type:size;
	  border-radius:calc(var(--border-radius,8px) - 2px);background:var(--color-widget-background);transition:background .3s}
	.ha-floorplan .ha-fp-room[data-lit="true"]{background:color-mix(in srgb,#f0c479 6%,var(--color-widget-background))}
	.ha-floorplan .ha-fp-room[data-occupied="true"]{box-shadow:inset 0 0 0 1px color-mix(in srgb,var(--color-primary) 45%,transparent)}
	.ha-fp-name{position:absolute;top:6px;left:8px;font-size:12px;font-weight:600;color:var(--color-text-highlight);
	  white-space:nowrap;overflow:hidden;text-overflow:ellipsis;max-width:calc(100% - 16px)}
	.ha-fp-room:has(.ha-fp-temp) .ha-fp-name{max-width:calc(100% - 64px)}
	.ha-fp-temp{position:absolute;top:5px;right:7px;display:inline-flex;align-items:center;gap:2px;font-size:12px;font-weight:600;color:var(--color-text-base);
	  padding:1px 6px;border-radius:12px;background:color-mix(in srgb,var(--color-text-base) 8%,transparent)}
	.ha-fp-trend{font-size:14px;line-height:1;font-weight:700}
	.ha-fp-trend[data-trend="up"]{color:var(--color-negative)}
	.ha-fp-trend[data-trend="down"]{color:#7aa2f7}
	/* Everything the room has sits on a virtual 3x3 grid that covers the
	   whole room. Tiles take the eight wall slots first (top, bottom, left,
	   right, then corners), hugging the wall, and each one's flow — a light
	   cone, a stream of cool or warm air — is rotated to point at the room's
	   centre. Only when all eight walls are taken do tiles spill into the
	   centre cell. */
	.ha-fp-icons{position:absolute;inset:24px 4px 4px 4px;display:grid;grid-template-columns:1fr auto 1fr;grid-template-rows:1fr auto 1fr}
	/* --len is how far the flow travels: to the room's centre, in container
	   units, so a beam from the top wall stops at mid-height and one from
	   a side wall at mid-width, whatever the room's shape. */
	.ha-fp-icons>span{position:relative;display:flex;align-items:center;justify-content:center;width:36px;height:36px;--dir:0deg;--len:50cqh}
	.ha-fp-icons>span>svg{position:relative;z-index:1;width:28px;height:28px}
	.ha-fp-icons>[data-slot="t"]{grid-area:1/2;place-self:start center;--dir:0deg;--len:50cqh}
	.ha-fp-icons>[data-slot="b"]{grid-area:3/2;place-self:end center;--dir:180deg;--len:50cqh}
	.ha-fp-icons>[data-slot="l"]{grid-area:2/1;place-self:center start;--dir:-90deg;--len:50cqw}
	.ha-fp-icons>[data-slot="r"]{grid-area:2/3;place-self:center end;--dir:90deg;--len:50cqw}
	.ha-fp-icons>[data-slot="tl"]{grid-area:1/1;place-self:start start;--dir:-45deg;--len:70cqmin}
	.ha-fp-icons>[data-slot="tr"]{grid-area:1/3;place-self:start end;--dir:45deg;--len:70cqmin}
	.ha-fp-icons>[data-slot="bl"]{grid-area:3/1;place-self:end start;--dir:-135deg;--len:70cqmin}
	.ha-fp-icons>[data-slot="br"]{grid-area:3/3;place-self:end end;--dir:135deg;--len:70cqmin}
	.ha-fp-icons>[data-slot="c"]{grid-area:2/2;place-self:center}
	/* A source in the room's centre lights (or blows) all around: its
	   ::before becomes a soft disc sized to the room, its ::after an
	   expanding ripple of rings for air. */
	.ha-fp-icons>[data-center="true"]::before,.ha-fp-icons>[data-center="true"]::after{clip-path:none;mask:none;-webkit-mask:none;
	  width:95cqmin;height:95cqmin;border-radius:50%;transform-origin:50% 50%;transform:translate(-50%,-50%) scale(.2)}
	.ha-fp-icons>.ha-light[data-center="true"][data-on="true"]::before{transform:translate(-50%,-50%) scale(1);
	  background:radial-gradient(circle,rgba(255,225,170,.55) 0%,rgba(240,196,121,.28) 25%,rgba(240,196,121,.08) 55%,transparent 72%)}
	.ha-fp-icons>.ha-device[data-center="true"][data-effect="fan"]::before{background:radial-gradient(circle,rgba(170,200,255,.5) 0%,rgba(122,162,247,.2) 30%,transparent 72%)}
	.ha-fp-icons>.ha-device[data-center="true"][data-effect="heat"]::before{background:radial-gradient(circle,rgba(255,200,140,.55) 0%,rgba(255,150,70,.22) 30%,transparent 72%)}
	.ha-fp-icons>.ha-device[data-center="true"][data-on="true"]:is([data-effect="fan"],[data-effect="heat"])::before{transform:translate(-50%,-50%) scale(1)}
	.ha-fp-icons>.ha-device[data-center="true"][data-effect]::after{background:repeating-radial-gradient(circle,transparent 0 9px,var(--wave) 11px 13px,transparent 15px 24px)}
	.ha-fp-icons>.ha-device[data-center="true"][data-on="true"]:is([data-effect="fan"],[data-effect="heat"])::after{animation:ha-fp-ripple 2.2s linear infinite}
	/* Music: a playing media player sends notes drifting up and fading,
	   no beam. ::before and ::after are the two notes, staggered. */
	.ha-fp-icons>.ha-device[data-effect="music"]::before,.ha-fp-icons>.ha-device[data-effect="music"]::after{
	  clip-path:none;mask:none;-webkit-mask:none;background:none;width:auto;height:auto;border-radius:0;
	  left:50%;top:50%;transform:translate(-50%,-50%);font-size:13px;line-height:1;color:var(--accent,var(--color-primary));
	  transform-origin:50% 50%;transition:none}
	.ha-fp-icons>.ha-device[data-effect="music"]::before{content:"♪"}
	.ha-fp-icons>.ha-device[data-effect="music"]::after{content:"♫";font-size:11px}
	.ha-fp-icons>.ha-device[data-on="true"][data-effect="music"]::before{animation:ha-fp-note 2.4s ease-out infinite}
	.ha-fp-icons>.ha-device[data-on="true"][data-effect="music"]::after{animation:ha-fp-note 2.4s ease-out 1.2s infinite}
	@keyframes ha-fp-note{0%{opacity:0;transform:translate(-50%,-50%)}15%{opacity:1}100%{opacity:0;transform:translate(calc(-50% + 14px),calc(-50% - 44px)) rotate(12deg)}}
	.ha-fp-icons .ha-device[data-on="true"][data-effect="music"] svg path{fill:var(--accent,var(--color-primary))}
	@keyframes ha-fp-ripple{from{opacity:.9;transform:translate(-50%,-50%) scale(.35)}to{opacity:0;transform:translate(-50%,-50%) scale(1.05)}}
	.ha-fp-placed{grid-template-columns:none;grid-template-rows:none}
	.ha-fp-edit{position:absolute;top:0;right:0;width:22px;height:22px;display:flex;align-items:center;justify-content:center;
	  color:var(--color-text-subdue);text-decoration:none;opacity:0;transition:opacity .2s;font-size:15px;line-height:1}
	.ha-widget{position:relative}
	.ha-widget:hover .ha-fp-edit{opacity:.8}
	.ha-fp-edit:hover{opacity:1;color:var(--color-text-highlight)}
	/* The flow: a cone whose apex is the tile's centre, drawn pointing
	   "down" in its own frame and rotated by --dir toward the room centre.
	   ::before is the soft body, ::after the moving wave stripes (air only). */
	.ha-fp-icons>span::before,.ha-fp-icons>span::after{content:"";position:absolute;left:50%;top:50%;width:calc(var(--len) * .9);height:calc(var(--len) * var(--reach,.92));mix-blend-mode:screen;
	  transform-origin:50% 0;transform:translateX(-50%) rotate(var(--dir)) scaleY(.15);opacity:0;pointer-events:none;
	  clip-path:polygon(50% 0,100% 100%,0 100%);transition:opacity .5s ease,transform .6s ease;
	  /* soft edges + fade along the beam, like a spotlight cone */
	  mask:linear-gradient(90deg,transparent,#000 35%,#000 65%,transparent),linear-gradient(180deg,#000,#000 25%,transparent 95%);
	  -webkit-mask:linear-gradient(90deg,transparent,#000 35%,#000 65%,transparent),linear-gradient(180deg,#000,#000 25%,transparent 95%);
	  mask-composite:intersect;-webkit-mask-composite:source-in}
	.ha-fp-icons>span::after{--wave:transparent;background:repeating-linear-gradient(180deg,transparent 0 8px,var(--wave) 11px 13px,transparent 16px 24px);
	  background-size:100% 24px}
	@keyframes ha-fp-wave{from{background-position:0 0}to{background-position:0 24px}}
	.ha-fp-icons .ha-light::before{background:linear-gradient(180deg,rgba(255,225,170,.75) 0%,rgba(240,196,121,.4) 18%,rgba(240,196,121,.16) 50%,transparent 100%)}
	.ha-fp-icons .ha-light[data-on="true"]::before{opacity:1;transform:translateX(-50%) rotate(var(--dir)) scaleY(1)}
	.ha-fp-icons .ha-device[data-effect="fan"]::before{background:linear-gradient(180deg,rgba(170,200,255,.6) 0%,rgba(122,162,247,.3) 20%,rgba(122,162,247,.1) 55%,transparent 100%)}
	.ha-fp-icons .ha-device[data-effect="fan"]::after{--wave:rgba(170,200,255,.22)}
	.ha-fp-icons .ha-device[data-effect="heat"]::before{background:linear-gradient(180deg,rgba(255,200,140,.65) 0%,rgba(255,150,70,.32) 20%,rgba(255,150,70,.1) 55%,transparent 100%)}
	.ha-fp-icons .ha-device[data-effect="heat"]::after{--wave:rgba(255,200,140,.22)}
	.ha-fp-icons .ha-device[data-on="true"]:is([data-effect="fan"],[data-effect="heat"])::before{opacity:1;transform:translateX(-50%) rotate(var(--dir)) scaleY(1)}
	.ha-fp-icons .ha-device[data-on="true"]:is([data-effect="fan"],[data-effect="heat"])::after{opacity:1;transform:translateX(-50%) rotate(var(--dir)) scaleY(1);animation:ha-fp-wave 1.6s linear infinite}
	.ha-fp-icons .ha-device svg path{fill:var(--color-text-subdue);opacity:.35;transition:fill .2s,opacity .2s}
	.ha-fp-icons .ha-device[data-on="true"] svg path{fill:var(--color-text-highlight);opacity:1}
	.ha-fp-icons .ha-device[data-on="true"][data-effect="fan"] svg path{fill:#7aa2f7}
	.ha-fp-icons .ha-device[data-on="true"][data-effect="heat"] svg path{fill:#ff9646}
	.ha-fp-icons .ha-device[data-on="true"][data-effect="fan"] svg{animation:ha-fp-spin 2.4s linear infinite}
	@keyframes ha-fp-spin{to{transform:rotate(360deg)}}
	.ha-fp-icons .ha-badge{gap:0;font-size:0}
	.ha-fp-icons .ha-badge .ha-contact-label{display:none}
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
	maxWidth := ""
	if fp.MaxWidth > 0 {
		maxWidth = fmt.Sprintf(";max-width:%dpx", fp.MaxWidth)
	}
	fmt.Fprintf(&b, `<div class="ha-floorplan" style="grid-template-areas:%s;grid-template-columns:repeat(%d,1fr);grid-template-rows:repeat(%d,1fr);aspect-ratio:%s%s">`,
		fp.TemplateAreas(), fp.Columns, fp.Rows, html.EscapeString(aspect), maxWidth)
	for _, key := range fp.Keys {
		area := fp.Areas[key]
		r, _ := byRoom[area] // a mapped room with no HA data is still drawn, empty
		r.Room = area
		if p, ok := fp.Placement[key]; ok {
			b.WriteString(renderPlacedRoom(key, r, p))
		} else {
			b.WriteString(renderFloorplanRoom(key, r))
		}
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
	tiles := roomTiles(r)
	if len(tiles) > 0 {
		fmt.Fprintf(&b, `<span class="ha-fp-icons" style="--reach:%.2f">`, beamReach(r))
		for i, tile := range tiles {
			slot := wallSlots[min(i, len(wallSlots)-1)]
			attrs := fmt.Sprintf(`data-slot="%s"`, slot)
			if slot == "c" {
				attrs += ` data-center="true"`
			}
			b.WriteString(strings.Replace(tile, `<span class="`, `<span `+attrs+` class="`, 1))
		}
		b.WriteString(`</span>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// wallSlots is the order tiles take positions on the room's virtual 3x3
// grid: the four wall midpoints first, then corners, and everything past
// the eighth tile lands in the centre cell ("c").
var wallSlots = []string{"t", "b", "l", "r", "tl", "tr", "bl", "br", "c"}

// roomTiles renders every tile a room has, in a stable order (lights,
// devices, motion, contacts), each as a <span class="..."> ready to be
// given a wall slot. Classes and data-* attributes are the live poller's
// contract (see bootstrapScript) — don't rename them here.
func roomTiles(r RoomCardView) []string {
	var tiles []string
	for _, l := range r.Lights {
		tiles = append(tiles, fmt.Sprintf(`<span class="ha-light" data-entity-id="%s" data-on="%t"%s>%s</span>`,
			html.EscapeString(l.EntityID), l.On, lightTileAttrs(l), l.IconSVG))
	}
	for _, d := range r.Devices {
		// The accent rides in a data attribute (not a style attr) because
		// the slot/placement code prepends its own style attribute later.
		accent := ""
		if d.Accent != "" {
			accent = fmt.Sprintf(` data-accent="%s"`, html.EscapeString(d.Accent))
		}
		tiles = append(tiles, fmt.Sprintf(`<span class="ha-device" data-entity-id="%s" data-on="%t" data-effect="%s"%s%s title="%s">%s</span>`,
			html.EscapeString(d.EntityID), d.On, html.EscapeString(d.Effect), accent, deviceTileAttrs(d), html.EscapeString(d.Name), d.IconSVG))
	}
	// Occupancy is not a tile on the map: the room's outline (data-occupied
	// on the room, kept live by the poller) is the whole signal.
	for _, c := range r.Contacts {
		tiles = append(tiles, fmt.Sprintf(`<span class="ha-badge" data-sensor-name="%s" data-open="%t" title="%s">%s<span class="ha-contact-label">%s</span></span>`,
			html.EscapeString(c.Name), c.Attention, html.EscapeString(c.Label), ContactIcon(), html.EscapeString(c.Label)))
	}
	return tiles
}

// beamReach shortens every beam as more beam-casting sources share a room
// so they stop short of each other instead of crossing at the centre:
// 1 source → all the way to the centre (0.92 of the distance), 2 → 0.7,
// 3+ → 0.55. Only lights and air devices count; a speaker casts nothing.
func beamReach(r RoomCardView) float64 {
	n := len(r.Lights)
	for _, d := range r.Devices {
		if d.Effect == "fan" || d.Effect == "heat" {
			n++
		}
	}
	switch {
	case n <= 1:
		return 0.92
	case n == 2:
		return 0.7
	}
	return 0.55
}

// renderPlacedRoom draws a room whose interior the editor defined: an
// R×C inner grid, each placed tile at its cell (hugging the wall when the
// cell is on the grid's edge), its beam rotated to point at the grid's
// centre and long enough to almost reach it. Unplaced tiles get the
// automatic wall slots the un-edited layout uses; hidden ones are skipped.
func renderPlacedRoom(key string, r RoomCardView, p RoomPlacement) string {
	name := r.Room
	if p.Name != "" {
		name = p.Name
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<div class="ha-room ha-fp-room" data-room="%s" data-lit="%t" data-occupied="%t" style="grid-area:%s">`,
		html.EscapeString(r.Room), r.Lit, r.Occupied, html.EscapeString(key))
	fmt.Fprintf(&b, `<span class="ha-fp-name">%s</span>`, html.EscapeString(name))
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
	r = withoutHidden(r, p.Hidden)
	tiles, ids := roomTilesWithIDs(r)
	if len(tiles) == 0 {
		b.WriteString(`</div>`)
		return b.String()
	}
	rows, cols := max(p.Rows, 1), max(p.Columns, 1)
	fmt.Fprintf(&b, `<span class="ha-fp-icons ha-fp-placed" style="--reach:%.2f;grid-template-rows:repeat(%d,1fr);grid-template-columns:repeat(%d,1fr)">`, beamReach(r), rows, cols)
	// Unplaced tiles get free cells of THIS room's grid in wall order
	// (the old 3x3 slot names only make sense on a 3x3 grid).
	taken := map[[2]int]bool{}
	for _, id := range ids {
		if c, ok := p.Cells[id]; ok {
			taken[c] = true
		}
	}
	auto := autoCells(rows, cols, taken)
	next := 0
	for i, tile := range tiles {
		cell, placed := p.Cells[ids[i]]
		if !placed {
			if next < len(auto) {
				cell = auto[next]
				next++
			} else {
				cell = [2]int{rows / 2, cols / 2}
			}
		}
		attrs := `style="` + placedTileStyle(cell, rows, cols) + `"`
		if isCentre(cell, rows, cols) {
			attrs += ` data-center="true"`
		}
		b.WriteString(strings.Replace(tile, `<span class="`, `<span `+attrs+` class="`, 1))
	}
	b.WriteString(`</span></div>`)
	return b.String()
}

// autoCells lists free cells of a rows×cols grid in the order automatic
// tiles should take them: wall midpoints (top, bottom, left, right), then
// corners, then whatever is left row-major — mirroring the wall-slot
// order of the un-edited layout, but on the room's real grid.
func autoCells(rows, cols int, taken map[[2]int]bool) [][2]int {
	var out [][2]int
	seen := map[[2]int]bool{}
	add := func(c [2]int) {
		if c[0] < 0 || c[0] >= rows || c[1] < 0 || c[1] >= cols || taken[c] || seen[c] {
			return
		}
		seen[c] = true
		out = append(out, c)
	}
	mr, mc := (rows-1)/2, (cols-1)/2
	add([2]int{0, mc})
	add([2]int{rows - 1, mc})
	add([2]int{mr, 0})
	add([2]int{mr, cols - 1})
	add([2]int{0, 0})
	add([2]int{0, cols - 1})
	add([2]int{rows - 1, 0})
	add([2]int{rows - 1, cols - 1})
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			if !isCentre([2]int{r, c}, rows, cols) {
				add([2]int{r, c})
			}
		}
	}
	return out
}

// placedTileStyle positions a tile at cell [row,col] of a rows×cols inner
// grid and aims its beam at the grid centre. --dir: the beam is drawn
// pointing "down", so rotate by atan2(-dx, dy) where (dx,dy) is the vector
// to the centre in cell units. --len: distance to the centre in container
// units, approximated without sqrt (max + 0.41·min) — CSS calc has no
// hypot. A tile in the exact centre casts nothing (--len:0).
func placedTileStyle(cell [2]int, rows, cols int) string {
	dy := (float64(rows)-1)/2 - float64(cell[0])
	dx := (float64(cols)-1)/2 - float64(cell[1])
	just := "center"
	switch {
	case cell[1] == 0 && cols > 1:
		just = "start"
	case cell[1] == cols-1 && cols > 1:
		just = "end"
	}
	align := "center"
	switch {
	case cell[0] == 0 && rows > 1:
		align = "start"
	case cell[0] == rows-1 && rows > 1:
		align = "end"
	}
	dir := math.Round(math.Atan2(-dx, dy) * 180 / math.Pi)
	if dir == -180 || dir == 0 { // normalise -0 / -180 for a stable attribute
		dir = math.Abs(dir)
	}
	ax := math.Abs(dx) * 100 / float64(cols)
	ay := math.Abs(dy) * 100 / float64(rows)
	lenExpr := "0px"
	if ax > 0 || ay > 0 {
		lenExpr = fmt.Sprintf("calc(max(%.2fcqw,%.2fcqh) + 0.41 * min(%.2fcqw,%.2fcqh))", ax, ay, ax, ay)
	}
	return fmt.Sprintf("grid-area:%d/%d;place-self:%s %s;--dir:%.0fdeg;--len:%s", cell[0]+1, cell[1]+1, align, just, dir, lenExpr)
}

func withoutHidden(r RoomCardView, hidden map[string]bool) RoomCardView {
	if len(hidden) == 0 {
		return r
	}
	var lights []LightView
	for _, l := range r.Lights {
		if !hidden[l.EntityID] {
			lights = append(lights, l)
		}
	}
	var devices []DeviceView
	for _, d := range r.Devices {
		if !hidden[d.EntityID] {
			devices = append(devices, d)
		}
	}
	var contacts []SensorBadgeView
	for _, c := range r.Contacts {
		if !hidden[c.Name] {
			contacts = append(contacts, c)
		}
	}
	r.Lights, r.Devices, r.Contacts = lights, devices, contacts
	return r
}

// roomTilesWithIDs is roomTiles plus the id the editor places each tile
// by: entity_id for lights/devices, the sensor name for contacts.
func roomTilesWithIDs(r RoomCardView) (tiles []string, ids []string) {
	tiles = roomTiles(r)
	for _, l := range r.Lights {
		ids = append(ids, l.EntityID)
	}
	for _, d := range r.Devices {
		ids = append(ids, d.EntityID)
	}
	for _, c := range r.Contacts {
		ids = append(ids, c.Name)
	}
	return tiles, ids
}

// isCentre reports whether a cell is the exact middle of an odd×odd inner
// grid — the one place a source lights the whole room rather than one
// direction.
func isCentre(cell [2]int, rows, cols int) bool {
	return rows%2 == 1 && cols%2 == 1 && cell[0] == rows/2 && cell[1] == cols/2
}

// accentRulesCSS maps each palette colour's data-accent onto --accent, so
// a tile's notes take its Now-playing card's colour without an inline
// style attribute (which the slot/placement code owns).
func accentRulesCSS() string {
	var b strings.Builder
	for _, c := range accentPalette {
		fmt.Fprintf(&b, "\t.ha-fp-icons .ha-device[data-accent=\"%s\"]{--accent:%s}\n", c, c)
	}
	return b.String()
}
