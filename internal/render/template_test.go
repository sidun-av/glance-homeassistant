package render

import "testing"

func sampleRoomCard() RoomCardView {
	return RoomCardView{
		Room:           "Living Room",
		SizeClass:      "ha-size-md",
		Lit:            true,
		Occupied:       true,
		HasTemperature: true,
		TempValue:      "21.4°",
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

func TestRenderWidget_RoomCardIncludesTemperature(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if !contains(html, "Living Room") || !contains(html, "21.4") {
		t.Errorf("html missing temperature content")
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
	if !contains(html, "container-type:inline-size") {
		t.Errorf("html missing container-type:inline-size on the room card, needed for @container to query its width")
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
		Rooms:         []RoomCardView{{Room: "Kitchen", HasTemperature: true, TempValue: "25.0°", ChartHTML: "<svg>k</svg>"}},
		CardMinHeight: 130,
	}
	html := RenderWidget(data)
	if !contains(html, "Kitchen") || !contains(html, "25.0") {
		t.Errorf("html missing Kitchen's temperature")
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

// TestRenderWidget_BarHeightUsesPercentageWithFlexShrinkZero is the
// regression test for the "half the widget's height is empty above the
// tallest bar" bug: a fixed-pixel bar-height cap (the previous round's
// workaround for a flex-shrink-clamping bug) undershoots badly, since the
// real available content height for a bar varies by render context and is
// often much taller than any single fixed cap chosen up front.
//
// The flex-shrink-clamping bug's real cause is that .ha-bar is an
// unlabeled flex item of .ha-bar-col (a flex column), so it defaults to
// flex-shrink:1 — a percentage-based height that would exceed the
// container gets silently shrunk. Setting flex-shrink:0 explicitly stops
// the browser from ever clamping it, so a percentage-based height is safe
// again and adapts to every size tier automatically — no more per-tier
// guessed pixel maximums needed.
func TestRenderWidget_BarHeightUsesPercentageWithFlexShrinkZero(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if contains(html, "height:calc(16px + var(--ha-bar-height,0) * 28px)") {
		t.Errorf("html still has the old fixed-pixel base-tier .ha-bar height rule")
	}
	if !contains(html, ".ha-bar{") || !contains(html, "flex-shrink:0") {
		t.Errorf("html missing flex-shrink:0 on .ha-bar")
	}
	if !contains(html, "height:calc(16px + var(--ha-bar-height,0) * (100% - 31px))") {
		t.Errorf("html missing the percentage-based .ha-bar height formula with value-label headroom")
	}
	if contains(html, "height:calc(16px + var(--ha-bar-height,0) * 44px)") {
		t.Errorf("html still has the old fixed-pixel size-md .ha-bar height override")
	}
	if contains(html, "height:calc(16px + var(--ha-bar-height,0) * 124px)") {
		t.Errorf("html still has the old fixed-pixel size-lg .ha-bar height override")
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
	// Default (non-chart) size-md cards no longer grow at all (flex:0 1
	// auto with a min-width floor, see Fix 1 in
	// TestRenderWidget_OnlyChartBearingCardsGrow) — only a chart-bearing
	// size-md card grows, and even then it's capped.
	if !contains(html, "ha-size-md{flex:0 1 auto;min-width:160px") {
		t.Errorf("html missing the reduced, non-growing size-md flex:0 1 auto;min-width:160px rule")
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
func TestRenderWidget_OnlyChartBearingCardsGrow(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if !contains(html, ".ha-room{") || !contains(html, "flex:0 1 auto;min-width:120px") {
		t.Errorf("html missing the base tier's shrink-to-fit flex-basis:auto rule with its min-width floor")
	}
	if !contains(html, `.ha-room[data-chart="true"]{flex:1 1 160px;max-width:420px}`) {
		t.Errorf("html missing the chart-bearing base tier's growing, capped override")
	}
	if !contains(html, `.ha-room.ha-size-md{flex:0 1 auto;min-width:160px`) {
		t.Errorf("html missing the size-md tier's shrink-to-fit flex-basis:auto rule with its min-width floor")
	}
	if !contains(html, `.ha-room.ha-size-md[data-chart="true"]{flex:1 1 200px;max-width:420px}`) {
		t.Errorf("html missing the chart-bearing size-md tier's growing, capped override")
	}
	if !contains(html, `.ha-room.ha-size-lg{flex:0 1 auto;min-width:200px`) {
		t.Errorf("html missing the size-lg tier's shrink-to-fit flex-basis:auto rule with its min-width floor")
	}
	if !contains(html, `.ha-room.ha-size-lg[data-chart="true"]{flex:1 1 340px;max-width:420px}`) {
		t.Errorf("html missing the chart-bearing size-lg tier's growing, capped override")
	}
}

func TestRenderRoomCard_DataChartAttributeReflectsHasTemperature(t *testing.T) {
	withChart := renderRoomCard(RoomCardView{Room: "Kitchen", HasTemperature: true, TempValue: "20.0°", ChartHTML: "<svg></svg>"})
	if !contains(withChart, `data-chart="true"`) {
		t.Errorf("html = %q, want data-chart=\"true\" for a room with a temperature chart", withChart)
	}

	noChart := renderRoomCard(RoomCardView{Room: "Bedroom", HasTemperature: false})
	if !contains(noChart, `data-chart="false"`) {
		t.Errorf("html = %q, want data-chart=\"false\" for a room with no temperature chart", noChart)
	}
}

// TestRenderWidget_BarWidthUsesFixedPixelsNotPercentage is the regression
// test for the "bars look like blocky rectangles instead of slender bars"
// bug: percentage-based bar width (the old `width:55%`) is fragile on a
// flex-grown container (now also relevant per Fix 1's capped-growth chart
// cards), producing ~44px-wide blocks on a wide card instead of the ~6px
// bars Glance's real Weather widget uses. Fixed pixel widths are immune to
// this because they never depend on the container's resolved size.
func TestRenderWidget_BarWidthUsesFixedPixelsNotPercentage(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if contains(html, "width:55%") {
		t.Errorf("html contains the old percentage-based bar width, want a fixed-pixel width instead")
	}
	if !contains(html, ".ha-bar{") || !contains(html, "width:6px") {
		t.Errorf("html missing the base-tier fixed-pixel .ha-bar width rule")
	}
	if !contains(html, ".ha-room.ha-size-md .ha-bar{width:7px") {
		t.Errorf("html missing the size-md tier's scaled .ha-bar width rule")
	}
	if !contains(html, ".ha-room.ha-size-lg .ha-bar{width:9px") {
		t.Errorf("html missing the size-lg tier's scaled .ha-bar width rule")
	}
	if contains(html, "width:80%") {
		t.Errorf("html contains the old percentage-based current-bar width, want a fixed-pixel width instead")
	}
	if !contains(html, ".ha-bar-current{width:10px;opacity:1;background:color-mix(in srgb,var(--color-progress-value) 55%,var(--color-text-highlight))}") {
		t.Errorf("html missing the current bar's fixed-pixel width and brightened same-hue color")
	}
	if !contains(html, ".ha-room.ha-size-md .ha-bar-current{width:11px}") {
		t.Errorf("html missing the size-md tier's scaled current-bar width")
	}
	if !contains(html, ".ha-room.ha-size-lg .ha-bar-current{width:13px}") {
		t.Errorf("html missing the size-lg tier's scaled current-bar width")
	}
}

// TestRenderWidget_DaylightHighlightAnchoredToBarTrack is the regression
// test for the "daylight tint reads as a disconnected floating box" bug:
// .ha-bar-daylight used to span top:0;bottom:0 across the FULL height of
// .ha-bar-cols, which flex-grows taller than the actual bar+label content —
// leaving empty tinted space above the tallest possible bar. Anchoring the
// highlight to the bottom with a height matched to the bar track's real
// extent makes it read as an integrated highlight behind the bars.
//
// The height is now percentage-based (matching .ha-bar's own percentage
// formula, see TestRenderWidget_BarHeightUsesPercentageWithFlexShrinkZero)
// rather than a fixed pixel value sized to the old fixed bar-height caps —
// those old caps are gone, so a fixed daylight height could now clip a
// taller bar. The percentage formula scales consistently at every size
// tier, so the old per-tier overrides are removed entirely.
func TestRenderWidget_DaylightHighlightAnchoredToBarTrack(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if contains(html, "top:0;bottom:0") {
		t.Errorf("html still has the old full-height top:0;bottom:0 daylight rule")
	}
	if !contains(html, ".ha-bar-daylight{") || !contains(html, "bottom:0;height:calc(100% - 22px)") {
		t.Errorf("html missing the bottom-anchored, percentage-based daylight height rule")
	}
	if contains(html, ".ha-room.ha-size-md .ha-bar-daylight{height:70px}") {
		t.Errorf("html still has the old fixed-pixel size-md daylight height override")
	}
	if contains(html, ".ha-room.ha-size-lg .ha-bar-daylight{height:150px}") {
		t.Errorf("html still has the old fixed-pixel size-lg daylight height override")
	}
}

func TestRenderUnavailable_ContainsMessage(t *testing.T) {
	html := RenderUnavailable()
	if !contains(html, "Home Assistant unavailable") {
		t.Errorf("html = %q, want unavailable message", html)
	}
}
