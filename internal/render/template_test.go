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
		ChartSVG:       "<svg>lr</svg>",
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
	if !contains(html, `data-sensor-name="LR Motion"`) || !contains(html, "Occupied") {
		t.Errorf("html missing occupancy chip")
	}
	if !contains(html, `data-sensor-name="LR Window"`) || !contains(html, "Open") {
		t.Errorf("html missing contact badge")
	}
}

func TestRenderWidget_RoomCardCarriesLitAndOccupiedState(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	// One combined substring, not separate data-room/data-lit/data-occupied
	// checks — the static CSS block contains data-lit="true" and
	// data-occupied="true" on their own (as part of its attribute
	// selectors), so a bare check for either would pass regardless of what
	// the room's own <div> actually carries. This exact three-attribute
	// sequence only ever appears on the rendered room element.
	if !contains(html, `data-room="Living Room" data-lit="true" data-occupied="true">`) {
		t.Errorf("html missing the room's data-room/data-lit/data-occupied attributes")
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
		Rooms:         []RoomCardView{{Room: "Kitchen", HasTemperature: true, TempValue: "25.0°", ChartSVG: "<svg>k</svg>"}},
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

func TestRenderUnavailable_ContainsMessage(t *testing.T) {
	html := RenderUnavailable()
	if !contains(html, "Home Assistant unavailable") {
		t.Errorf("html = %q, want unavailable message", html)
	}
}
