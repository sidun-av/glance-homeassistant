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
	cells    [][]string
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
	.ha-fp-icons>[data-slot="c"]::before,.ha-fp-icons>[data-slot="c"]::after{display:none}
	/* The flow: a cone whose apex is the tile's centre, drawn pointing
	   "down" in its own frame and rotated by --dir toward the room centre.
	   ::before is the soft body, ::after the moving wave stripes (air only). */
	.ha-fp-icons>span::before,.ha-fp-icons>span::after{content:"";position:absolute;left:50%;top:50%;width:calc(var(--len) * .9 * var(--spread,1));height:calc(var(--len) * .92);mix-blend-mode:screen;
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
	.ha-fp-icons .ha-device[data-on="true"][data-effect]:not([data-effect=""])::before{opacity:1;transform:translateX(-50%) rotate(var(--dir)) scaleY(1)}
	.ha-fp-icons .ha-device[data-on="true"][data-effect]:not([data-effect=""])::after{opacity:1;transform:translateX(-50%) rotate(var(--dir)) scaleY(1);animation:ha-fp-wave 1.6s linear infinite}
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
	tiles := roomTiles(r)
	if len(tiles) > 0 {
		fmt.Fprintf(&b, `<span class="ha-fp-icons" style="--spread:%.2f">`, beamSpread(r))
		for i, tile := range tiles {
			b.WriteString(strings.Replace(tile, `<span class="`, fmt.Sprintf(`<span data-slot="%s" class="`, wallSlots[min(i, len(wallSlots)-1)]), 1))
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
		tiles = append(tiles, fmt.Sprintf(`<span class="ha-light" data-entity-id="%s" data-on="%t">%s</span>`,
			html.EscapeString(l.EntityID), l.On, l.IconSVG))
	}
	for _, d := range r.Devices {
		tiles = append(tiles, fmt.Sprintf(`<span class="ha-device" data-entity-id="%s" data-on="%t" data-effect="%s" title="%s">%s</span>`,
			html.EscapeString(d.EntityID), d.On, html.EscapeString(d.Effect), html.EscapeString(d.Name), d.IconSVG))
	}
	// Occupancy is not a tile on the map: the room's outline (data-occupied
	// on the room, kept live by the poller) is the whole signal.
	for _, c := range r.Contacts {
		tiles = append(tiles, fmt.Sprintf(`<span class="ha-badge" data-sensor-name="%s" data-open="%t" title="%s">%s<span class="ha-contact-label">%s</span></span>`,
			html.EscapeString(c.Name), c.Attention, html.EscapeString(c.Label), ContactIcon(), html.EscapeString(c.Label)))
	}
	return tiles
}

// beamSpread narrows every beam as more beam-casting sources share a room
// so they meet at the centre instead of piling on top of each other:
// 1 source → full width, 2 → ~0.7, 3+ → 0.6 (floor, so a beam still reads
// as a beam). Only lights and air devices count; a speaker casts nothing.
func beamSpread(r RoomCardView) float64 {
	n := len(r.Lights)
	for _, d := range r.Devices {
		if d.Effect != "" {
			n++
		}
	}
	if n <= 1 {
		return 1
	}
	return math.Max(0.6, 1/math.Sqrt(float64(n)))
}
