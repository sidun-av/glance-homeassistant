package render

import (
	"strings"
	"testing"
)

func sampleRoomCard() RoomCardView {
	return RoomCardView{
		Room:           "Living Room",
		SizeClass:      "ha-size-md",
		Lit:            true,
		Occupied:       true,
		HasTemperature: true,
		ChartHTML:      "<svg>lr</svg>",
		AxisRowHTML:    `<div class="ha-chart-axis"><span>8am</span><span>7pm</span></div>`,
		Lights: []LightView{
			{EntityID: "light.lr_main", IconSVG: LightIcon("mdi:track-light"), On: true},
		},
		Occupancy: []SensorBadgeView{{Name: "LR Motion", Attention: true, Label: "Occupied"}},
		Contacts:  []SensorBadgeView{{Name: "LR Window", Attention: true, Label: "Open"}},
	}
}

func sampleWidgetData() WidgetData {
	return WidgetData{
		Rooms:           []RoomCardView{sampleRoomCard()},
		CardMinHeight:   130,
		LiveURL:         "/ha-widget/live.json",
		PollIntervalMS:  10000,
		PauseWhenHidden: true,
	}
}

// The card head carries the room name only. The current temperature used
// to sit in its top-right corner, but the chart's current column now always
// shows that same number (see BarColumns), so the corner was a duplicate.
func TestRenderWidget_RoomCardIncludesChartButNotADuplicateTemperature(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if !contains(html, "Living Room") {
		t.Errorf("html missing the room name")
	}
	if contains(html, "ha-room-temp") {
		t.Errorf("html still renders the card head's separate temperature readout")
	}
	if !contains(html, "<svg>lr</svg>") {
		t.Errorf("html missing rendered chart SVG")
	}
	if !contains(html, `<div class="ha-chart-axis">`) {
		t.Errorf("html missing the axis labels row")
	}
}

func TestRenderWidget_TemperatureNoDataShowsFallback(t *testing.T) {
	data := WidgetData{Rooms: []RoomCardView{{Room: "Kitchen", HasTemperature: true, TempNoData: true}}, CardMinHeight: 130}
	html := RenderWidget(data)
	if !contains(html, "Kitchen") {
		t.Errorf("html missing Kitchen")
	}
	if !contains(html, "no data") {
		t.Errorf("html missing no-data fallback for a room with a sensor but no history")
	}
}

func TestRenderWidget_RoomCardIncludesLights(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	// Combined into one substring (not just "data-on=\"true\"" alone) because
	// the static CSS block also contains that exact text as part of its
	// [data-on="true"] attribute selectors — a bare check would pass even if
	// the actual <span> never got the attribute.
	if !contains(html, `data-entity-id="light.lr_main" data-on="true"`) {
		t.Errorf("html missing the light's entity id with its on-state data attribute")
	}
	if !contains(html, `class="track-light"`) {
		t.Errorf("html missing the light's fixture-type glyph")
	}
}

func TestRenderWidget_RoomCardIncludesOccupancyAndContact(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	// ">Occupied<", not a bare "Occupied" — the bootstrap script declares a
	// JS variable literally named anyOccupied, which contains "Occupied" as
	// a substring and is always present on the page regardless of this
	// room's data, so a bare check would never actually verify the chip's
	// label text was rendered.
	if !contains(html, `data-sensor-name="LR Motion"`) || !contains(html, ">Occupied<") {
		t.Errorf("html missing occupancy chip")
	}
	// ">Open<", not a bare "Open" — same durability reasoning as the
	// ">Occupied<" check above: not currently vacuous, but hardened against
	// a future edit introducing a capital "Open" anywhere in the static
	// styleBlock/bootstrapScript text.
	if !contains(html, `data-sensor-name="LR Window"`) || !contains(html, ">Open<") {
		t.Errorf("html missing contact badge")
	}
}

func TestRenderWidget_RoomCardCarriesLitAndOccupiedState(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	// One combined substring, not separate data-room/data-lit/data-occupied
	// checks — the static CSS block contains data-lit="true" and
	// data-occupied="true" on their own (as part of its attribute
	// selectors), so a bare check for either would pass regardless of what
	// the room's own <div> actually carries. This exact four-attribute
	// sequence (including data-chart, added for Fix 1) only ever appears on
	// the rendered room element.
	if !contains(html, `data-room="Living Room" data-lit="true" data-occupied="true" data-chart="true">`) {
		t.Errorf("html missing the room's data-room/data-lit/data-occupied/data-chart attributes")
	}
}

func TestRenderWidget_NoOccupiedGlowAnimation(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if contains(html, "ha-occ-glow") || contains(html, `[data-occupied="true"]{animation`) {
		t.Errorf("html = %q, want no pulsing glow animation tied to data-occupied on the room card", html)
	}
	// The lit-state background/border tint is a different, non-animated
	// visual cue (see .ha-room[data-lit="true"] in styleBlock) and must
	// stay — only the occupied glow is being removed.
	if !contains(html, `[data-lit="true"]{background`) {
		t.Errorf("html missing the lit-state room background/border tint")
	}
}

func TestRenderWidget_AxisTiersRevealedByContainerWidth(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	// Tiers 0/1 (first/middle/last) show unconditionally, matching the
	// widget's original fixed behavior on a narrow card.
	if !contains(html, `.ha-chart-axis span{display:none}`) {
		t.Errorf("html missing the default-hidden rule for axis label spans")
	}
	if !contains(html, `.ha-chart-axis span[data-tier="0"]`) || !contains(html, `[data-tier="1"]{display:inline}`) {
		t.Errorf("html missing the always-shown tier 0/1 rule")
	}
	// Higher tiers only reveal via a width-based @container query on the
	// room card, so a wider card can show a finer timeline step without
	// any JavaScript measuring anything.
	if !contains(html, "@container") {
		t.Errorf("html missing an @container query gating higher-tier axis labels")
	}
	if !contains(html, `[data-tier="2"]{display:inline}`) || !contains(html, `[data-tier="3"]{display:inline}`) {
		t.Errorf("html missing tier 2/3 reveal rules")
	}
	// container-type lives on the chart-bearing card specifically, not on
	// every .ha-room: inline-size containment zeroes a box's intrinsic
	// width, and non-chart cards need theirs to shrink-to-fit their content.
	if !contains(html, `.ha-room[data-chart="true"]{container-type:inline-size;`) {
		t.Errorf("html missing container-type:inline-size on the chart-bearing room card, needed for @container to query its width")
	}
}

func TestRenderWidget_SizeClassApplied(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	// The full class attribute value, not a bare "ha-size-md" substring —
	// the static CSS block also contains "ha-size-md" as part of its
	// .ha-room.ha-size-md{...} selector, so a bare check would pass even if
	// no room's <div> ever got the class applied.
	if !contains(html, `class="ha-room ha-size-md"`) {
		t.Errorf("html missing the room's size class on its own element")
	}
}

func TestRenderWidget_TemperatureOnlyRoomOmitsLightsAndStatus(t *testing.T) {
	data := WidgetData{
		Rooms:         []RoomCardView{{Room: "Kitchen", HasTemperature: true, ChartHTML: "<svg>k</svg>"}},
		CardMinHeight: 130,
	}
	html := RenderWidget(data)
	if !contains(html, "Kitchen") || !contains(html, "<svg>k</svg>") {
		t.Errorf("html missing Kitchen's name or chart")
	}
	// Checking for the absence of the actual rendered elements' opening
	// tags, not just "ha-room-lights"/"ha-room-status"/"data-entity-id="
	// substrings — those all appear unconditionally elsewhere on the page
	// regardless of this room's data: the class names in the static
	// <style> block, and "data-entity-id="/"data-sensor-name=" inside the
	// bootstrap script's own querySelector template strings. The opening
	// tags below are real HTML tag syntax ("<span class=...") that neither
	// the CSS (which uses dot-prefixed selectors like ".ha-light[...]")
	// nor the JS (which builds selector strings, never HTML tags) ever
	// produces, so their absence is a genuine, unambiguous check.
	if contains(html, `<span class="ha-light"`) {
		t.Errorf("html has a light element for a room with no lights")
	}
	if contains(html, `<span class="ha-occ-chip"`) || contains(html, `<span class="ha-badge"`) {
		t.Errorf("html has a sensor badge for a room with no occupancy/contact")
	}
}

func TestRenderWidget_NoRoomsShowsEmptyMessage(t *testing.T) {
	html := RenderWidget(WidgetData{CardMinHeight: 130})
	if !contains(html, "no rooms") {
		t.Errorf("html missing empty-state message")
	}
}

func TestRenderWidget_EscapesRoomAndSensorNames(t *testing.T) {
	data := WidgetData{
		Rooms: []RoomCardView{{
			Room:      `<script>alert(1)</script>`,
			Occupancy: []SensorBadgeView{{Name: `<b>x</b>`, Attention: false, Label: "Clear"}},
		}},
		CardMinHeight: 130,
	}
	html := RenderWidget(data)
	if contains(html, "<script>alert(1)</script>") || contains(html, "<b>x</b>") {
		t.Errorf("html contains unescaped content, want it HTML-escaped")
	}
}

func TestRenderWidget_AppliesConfiguredCardMinHeight(t *testing.T) {
	data := sampleWidgetData()
	data.CardMinHeight = 200
	html := RenderWidget(data)
	if !contains(html, "min-height:200px") {
		t.Errorf("html missing configured base card min-height in CSS")
	}
	if !contains(html, "min-height:220px") {
		t.Errorf("html missing size-md min-height (base+20)")
	}
	if !contains(html, "min-height:330px") {
		t.Errorf("html missing size-lg min-height (base+130)")
	}
}

// TestRenderWidget_BarHeightMatchesWeathersFixedPixelFormula pins the
// ported bar geometry. Every earlier revision of this rule was an attempt
// to make a bar fill whatever height its flex-grown box happened to end up
// with — first a fixed-pixel cap per size tier, then a percentage of the
// container. Glance's own Weather widget does neither: its bar is a plain
// calc(20px + scale * 40px), i.e. a 20px floor plus a 40px span, and the
// chart box is exactly as tall as that plus its two label rows. Matching
// it is the whole point of the port, so this asserts the formula verbatim
// and that none of the old per-tier overrides came back.
func TestRenderWidget_BarHeightMatchesWeathersFixedPixelFormula(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if !contains(html, "height:calc(20px + var(--ha-bar-height,0) * 40px)") {
		t.Errorf("html missing Weather's own .ha-bar height formula")
	}
	if !contains(html, ".ha-bar, .ha-bar-cols:hover .ha-bar{") || !contains(html, "flex-shrink:0") {
		t.Errorf("html missing flex-shrink:0 on .ha-bar (a flex item of .ha-bar-col would otherwise be clamped)")
	}
	for _, stale := range []string{
		"height:calc(16px + var(--ha-bar-height,0) * (100% - 31px))",
		"height:calc(16px + var(--ha-bar-height,0) * 28px)",
		"height:calc(16px + var(--ha-bar-height,0) * 44px)",
		"height:calc(16px + var(--ha-bar-height,0) * 124px)",
	} {
		if contains(html, stale) {
			t.Errorf("html still has a pre-port .ha-bar height rule: %s", stale)
		}
	}
}

// TestRenderWidget_SizeMdCardDoesNotDoubleGrowFlexBasis is the regression
// test for "Bedroom's oversized size-md card pushes Kitchen to the next
// row": .ha-room.ha-size-md used to be flex:2 1 320px (double flex-grow,
// wide basis), which grabbed far more leftover row space than its actual
// vertically-stacked content (small light icons, one occupancy chip) needed,
// starving neighboring base-tier cards of room on the same line.
func TestRenderWidget_SizeMdCardDoesNotDoubleGrowFlexBasis(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if contains(html, "ha-size-md{flex:2 1 320px") {
		t.Errorf("html still has the old oversized size-md flex-grow/basis")
	}
	// Default (non-chart) size-md cards neither grow NOR shrink (flex:0 0
	// auto with a min-width floor, see TestRenderWidget_OnlyChartBearingCardsGrow)
	// — only a chart-bearing size-md card grows, and even then it's capped.
	if !contains(html, "ha-size-md{flex:0 0 auto;min-width:160px") {
		t.Errorf("html missing the reduced, non-growing size-md flex:0 0 auto;min-width:160px rule")
	}
}

func TestRenderWidget_BootstrapScriptCarriesLiveConfig(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if !contains(html, `data-live-url="/ha-widget/live.json"`) {
		t.Errorf("html missing data-live-url attribute")
	}
	if !contains(html, `data-poll-ms="10000"`) {
		t.Errorf("html missing data-poll-ms attribute")
	}
	if !contains(html, `data-pause-hidden="true"`) {
		t.Errorf("html missing data-pause-hidden attribute")
	}
	if !contains(html, "onerror=") {
		t.Errorf("html missing the onerror bootstrap trigger")
	}
}

// TestRenderWidget_OnlyChartBearingCardsGrow is the regression test for the
// "Bedroom/Hallway cards stretch into large empty right-side space" bug: a
// room with no temperature chart has content (icon row, status chip) that
// stacks in independent rows and never fills extra width, so flex-grow:1 on
// every tier just leaves dead space. Only chart-bearing cards (HasTemperature)
// should grow, and even then only up to a capped max-width so a lone chart
// card on a wide row doesn't stretch its now-fixed-pixel bars into a
// gap-riddled mess.
//
// The non-chart base rules also switched from a guessed fixed-pixel
// flex-basis (which was wider than a room's actual shrink-to-fit content
// needs, and wrong differently for every room's exact content mix) to
// flex-basis:auto with a small min-width floor — this lets the card
// shrink-to-fit its actual content while still guaranteeing a usable
// minimum for a room with just one tiny icon.
//
// flex-shrink is 0, not 1: with a chart card on the same row grabbing every
// spare pixel, a shrinkable neighbour got squeezed BELOW its content width,
// so its icon row no longer reached the card's right padding and the card
// read as having a fat right margin and a thin left one. Not shrinking means
// such a row wraps instead, which .ha-rooms' flex-wrap already handles.
func TestRenderWidget_OnlyChartBearingCardsGrow(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if !contains(html, ".ha-room{") || !contains(html, "flex:0 0 auto;min-width:120px") {
		t.Errorf("html missing the base tier's shrink-to-fit flex-basis:auto rule with its min-width floor")
	}
	if !contains(html, `.ha-room[data-chart="true"]{container-type:inline-size;flex:1 1 160px;max-width:420px}`) {
		t.Errorf("html missing the chart-bearing base tier's growing, capped override")
	}
	if !contains(html, `.ha-room.ha-size-md{flex:0 0 auto;min-width:160px`) {
		t.Errorf("html missing the size-md tier's shrink-to-fit flex-basis:auto rule with its min-width floor")
	}
	if !contains(html, `.ha-room.ha-size-md[data-chart="true"]{flex:1 1 200px;max-width:420px}`) {
		t.Errorf("html missing the chart-bearing size-md tier's growing, capped override")
	}
	if !contains(html, `.ha-room.ha-size-lg{flex:0 0 auto;min-width:200px`) {
		t.Errorf("html missing the size-lg tier's shrink-to-fit flex-basis:auto rule with its min-width floor")
	}
	if !contains(html, `.ha-room.ha-size-lg[data-chart="true"]{flex:1 1 340px;max-width:420px}`) {
		t.Errorf("html missing the chart-bearing size-lg tier's growing, capped override")
	}
}

func TestRenderRoomCard_DataChartAttributeReflectsHasTemperature(t *testing.T) {
	withChart := renderRoomCard(RoomCardView{Room: "Kitchen", HasTemperature: true, ChartHTML: "<svg></svg>"})
	if !contains(withChart, `data-chart="true"`) {
		t.Errorf("html = %q, want data-chart=\"true\" for a room with a temperature chart", withChart)
	}

	noChart := renderRoomCard(RoomCardView{Room: "Bedroom", HasTemperature: false})
	if !contains(noChart, `data-chart="false"`) {
		t.Errorf("html = %q, want data-chart=\"false\" for a room with no temperature chart", noChart)
	}
}

// TestRenderWidget_BarWidthMatchesWeather pins the ported bar geometry's
// other half. Percentage-based widths (the original `width:55%`) turned
// into ~44px blocks on a wide card; the fix was fixed pixels, then a
// per-size-tier ladder of them (6/7/9px, current 10/11/13px) so a big card
// got chunkier bars. Weather has no such ladder — one 6px bar, 10px when
// it's the current column or hovered — and matching Weather is the point,
// so the ladder is gone. The current column's emphasis is now a shared rule
// with :hover, which is what lets hovering any column light it up the same
// way the current one is lit at rest.
func TestRenderWidget_BarWidthMatchesWeather(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	for _, stale := range []string{
		"width:55%", "width:80%",
		".ha-room.ha-size-md .ha-bar{width:7px", ".ha-room.ha-size-lg .ha-bar{width:9px",
		".ha-room.ha-size-md .ha-bar-current{width:11px}", ".ha-room.ha-size-lg .ha-bar-current{width:13px}",
	} {
		if contains(html, stale) {
			t.Errorf("html still has a pre-port bar-width rule: %s", stale)
		}
	}
	if !contains(html, ".ha-bar, .ha-bar-cols:hover .ha-bar{") || !contains(html, "width:6px") {
		t.Errorf("html missing the single fixed-pixel .ha-bar width rule")
	}
	if !contains(html, ".ha-bar-col-current .ha-bar, .ha-bar-col:hover .ha-bar{") || !contains(html, "width:10px") {
		t.Errorf("html missing the shared current/hover wide-bar rule")
	}
}

// TestRenderWidget_DaylightHighlightMatchesWeather pins the ported daylight
// band. Its whole history — a full-height top:0;bottom:0 box that left empty
// tint above the tallest bar, then a bottom-anchored percentage height, then
// per-run positioned divs spanning the flex gap — was working around this
// widget's own layout choices. Weather's is one inset:0 div per daytime
// column with a 30px bottom fade and a 20px rounded corner at each end of
// the run, which works because Weather's columns have no gap between them
// (see chartCSS). Anything reintroducing a gap on .ha-bar-cols brings the
// seams back, so that's asserted too.
func TestRenderWidget_DaylightHighlightMatchesWeather(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	for _, stale := range []string{
		"top:0;bottom:0",
		"bottom:0;height:calc(100% - 22px)",
		".ha-room.ha-size-md .ha-bar-daylight{height:70px}",
		".ha-room.ha-size-lg .ha-bar-daylight{height:150px}",
	} {
		if contains(html, stale) {
			t.Errorf("html still has a pre-port daylight rule: %s", stale)
		}
	}
	if !contains(html, ".ha-bar-daylight{") || !contains(html, "position:absolute;inset:0") {
		t.Errorf("html missing Weather's inset:0 per-column daylight rule")
	}
	if !contains(html, "background:linear-gradient(0deg,transparent 30px,hsl(50,50%,30%,0.2))") {
		t.Errorf("html missing Weather's own daylight gradient")
	}
	if !contains(html, ".ha-bar-daylight-sunrise{border-radius:20px 0 0 0}") ||
		!contains(html, ".ha-bar-daylight-sunset{border-radius:0 20px 0 0}") {
		t.Errorf("html missing the run's rounded end-cap rules")
	}
	// No gap between columns — that is what makes per-column highlight divs
	// join up seamlessly instead of showing a stripe at every boundary.
	if !contains(html, ".ha-bar-cols{flex:1 0 auto;width:100%;display:flex;justify-content:center}") {
		t.Errorf("html missing Weather's gapless .ha-bar-cols row rule")
	}
	if !contains(html, "width:calc(100% / var(--ha-bar-cols,12))") {
		t.Errorf("html missing the 100%%/N column width rule")
	}
}

func TestRenderUnavailable_ContainsMessage(t *testing.T) {
	html := RenderUnavailable()
	if !contains(html, "Home Assistant unavailable") {
		t.Errorf("html = %q, want unavailable message", html)
	}
}

// TestRenderWidget_HoverRevealFollowsWeathersCascadeOrder pins the one part
// of the ported CSS that silently stops working if someone "tidies" it. In
// Weather the at-rest state (.ha-bar-value hidden, .ha-bar-col-time shown on
// columns 3/7/11, the current bar wide) is overridden by a row-level
// :hover copy of the same declarations, and only then re-enabled by a
// column-level :hover rule. The row copy has to be at least as specific as
// the at-rest reveal rules AND come after them, and the column copy after
// that — otherwise hovering either does nothing or lights two columns at
// once. Order in the emitted stylesheet is therefore load-bearing.
func TestRenderWidget_HoverRevealFollowsWeathersCascadeOrder(t *testing.T) {
	html := RenderWidget(sampleWidgetData())

	index := func(haystack, needle string) int { return strings.Index(haystack, needle) }

	ordered := func(label string, selectors ...string) {
		t.Helper()
		prev := -1
		for _, sel := range selectors {
			at := index(html, sel)
			if at == -1 {
				t.Errorf("%s: stylesheet missing rule %q", label, sel)
				return
			}
			if at <= prev {
				t.Errorf("%s: rule %q appears before the rule it must override", label, sel)
				return
			}
			prev = at
		}
	}

	ordered("value labels",
		".ha-bar-value, .ha-bar-cols:hover .ha-bar-value{",
		".ha-bar-col-current .ha-bar-value, .ha-bar-col:hover .ha-bar-value{",
	)
	ordered("bars",
		".ha-bar, .ha-bar-cols:hover .ha-bar{",
		".ha-bar-col-current .ha-bar, .ha-bar-col:hover .ha-bar{",
	)
	// Time labels invert the shape: the at-rest reveal is the nth-child rule,
	// so the row-hover copy must come after it to suppress it, and the
	// column-hover rule after that.
	ordered("time labels",
		".ha-bar-col:nth-child(3) .ha-bar-col-time,",
		".ha-bar-col-time, .ha-bar-cols:hover .ha-bar-col-time{",
		".ha-bar-col:hover .ha-bar-col-time{opacity:1;transform:translateY(0)}",
	)

	if !contains(html, ".ha-bar-col:nth-child(7) .ha-bar-col-time,") ||
		!contains(html, ".ha-bar-col:nth-child(11) .ha-bar-col-time{") {
		t.Errorf("html missing Weather's 3rd/7th/11th at-rest time-label columns")
	}
	// Visibility is CSS-only now; nothing in the markup may gate it.
	if contains(html, "ha-bar-col-time-visible") {
		t.Errorf("html still references the removed per-column time-label visibility class")
	}
}

// TestRenderWidget_ValueSignAndDegreeAreCSSPseudoElements pins the other
// half of Weather's value-label trick (see BarColumns): the markup carries
// bare absolute digits, and CSS supplies the "°" and any "-" as absolutely
// positioned pseudo-elements so neither one shifts the digits off-center
// above their bar.
func TestRenderWidget_ValueSignAndDegreeAreCSSPseudoElements(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if !contains(html, `.ha-bar-value::after{position:absolute;content:'\00b0';left:100%;color:var(--color-text-subdue)}`) {
		t.Errorf("html missing the degree-sign ::after rule")
	}
	if !contains(html, `.ha-bar-value.ha-bar-value-negative::before{position:absolute;content:'-';right:100%}`) {
		t.Errorf("html missing the negative-sign ::before rule")
	}
}

// TestRenderWidget_CardContentIsHorizontallyCentered is the regression test
// for "the right padding looks bigger than the left": a card is as wide as
// its widest row (usually the occupancy chip), so a shorter row — the light
// icons especially — used to sit flush left and dump all the leftover width
// against the right padding. Centering those rows splits the leftover evenly
// so both sides read as the same 14px gutter.
func TestRenderWidget_CardContentIsHorizontallyCentered(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if !contains(html, ".ha-room-lights{") || !contains(html, "align-items:center;justify-content:center;gap:10px}") {
		t.Errorf("html missing the centered light-icon row rule")
	}
	if !contains(html, ".ha-room-status{flex:none;display:flex;flex-direction:column;align-items:center;gap:5px}") {
		t.Errorf("html missing the centered status-column rule")
	}
}

// TestRenderWidget_ProjectedColumnsAreFadedNotHidden pins the styling of the
// chart's not-yet-happened half. Only the bar's opacity and the label's
// colour change: everything geometric stays shared with a measured column,
// so hovering a projected bucket still widens and lifts it the same way —
// it just never stops looking secondary.
func TestRenderWidget_ProjectedColumnsAreFadedNotHidden(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if !contains(html, ".ha-bar-projected{opacity:.4}") {
		t.Errorf("html missing the faded projected-bar rule")
	}
	if !contains(html, ".ha-bar-value-projected, .ha-bar-cols:hover .ha-bar-value-projected{color:var(--color-text-base)}") {
		t.Errorf("html missing the projected value-label colour rule (with its row-hover copy)")
	}
	// It must not get its own geometry or be hidden outright — either would
	// break the shared axis, the hover behaviour, or both.
	for _, forbidden := range []string{
		".ha-bar-projected{width", ".ha-bar-projected{height", ".ha-bar-projected{display",
		".ha-bar-projected{visibility",
	} {
		if contains(html, forbidden) {
			t.Errorf("projected columns must not override geometry or visibility: found %q", forbidden)
		}
	}
}
