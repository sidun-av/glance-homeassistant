package render

import "testing"

func sampleWidgetData() WidgetData {
	return WidgetData{
		Title:       "Home",
		ChartHeight: 34,
		ChartStyle:  "sparkline",
		TemperatureRooms: []TemperatureRoomView{
			{Room: "Living Room", Value: "21.4°", SVG: "<svg>lr</svg>"},
			{Room: "Bedroom", NoData: true},
		},
		LightRooms: []LightRoomView{
			{Room: "Living Room", On: 2, Total: 3},
			{Room: "Office", On: 0, Total: 2},
		},
		Sensors: []SensorView{
			{Name: "Front Door", Attention: false, Label: "Closed"},
			{Name: "LR Window", Attention: true, Label: "Open"},
		},
		LiveURL:         "/ha-widget/live.json",
		PollIntervalMS:  10000,
		PauseWhenHidden: true,
	}
}

func TestRenderWidget_TemperatureSection(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if !contains(html, "Living Room") || !contains(html, "21.4") {
		t.Errorf("html missing populated temperature room content")
	}
	if !contains(html, "no data") {
		t.Errorf("html missing NoData fallback for Bedroom")
	}
	if !contains(html, "<svg>lr</svg>") {
		t.Errorf("html missing rendered sparkline SVG")
	}
}

func TestRenderWidget_LightsSection(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if !contains(html, `data-room="Living Room"`) {
		t.Errorf("html missing data-room attribute for live updates")
	}
	if !contains(html, "2/3 on") || !contains(html, "0/2 on") {
		t.Errorf("html missing light counts")
	}
	if !contains(html, "ha-on") {
		t.Errorf("html missing ha-on class for a room with a light on")
	}
}

func TestRenderWidget_SensorsSection(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	if !contains(html, `data-name="Front Door"`) || !contains(html, `data-name="LR Window"`) {
		t.Errorf("html missing data-name attributes for live updates")
	}
	if !contains(html, "Closed") || !contains(html, "Open") {
		t.Errorf("html missing sensor state labels")
	}
}

func TestRenderWidget_EmptySectionsShowFallbackMessages(t *testing.T) {
	data := WidgetData{Title: "Home", ChartStyle: "sparkline"}
	html := RenderWidget(data)
	if !contains(html, "no rooms with a temperature sensor") {
		t.Errorf("html missing empty-temperature fallback")
	}
	if !contains(html, "no rooms with lights") {
		t.Errorf("html missing empty-lights fallback")
	}
	if !contains(html, "no contact/motion sensors found") {
		t.Errorf("html missing empty-sensors fallback")
	}
}

func TestRenderWidget_EscapesRoomNames(t *testing.T) {
	data := sampleWidgetData()
	data.LightRooms = []LightRoomView{{Room: "<script>alert(1)</script>", On: 0, Total: 1}}
	html := RenderWidget(data)
	if contains(html, "<script>alert(1)</script>") {
		t.Errorf("html contains unescaped room name, want it HTML-escaped")
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

func TestRenderWidget_BarsStyleOmitsSeparateHeaderValue(t *testing.T) {
	data := sampleWidgetData()
	data.ChartStyle = "bars"
	html := RenderWidget(data)
	if !contains(html, "ha-temp-room-label") {
		t.Errorf("html missing bars-style room label wrapper")
	}
}

func TestRenderUnavailable_ContainsMessage(t *testing.T) {
	html := RenderUnavailable()
	if !contains(html, "Home Assistant unavailable") {
		t.Errorf("html = %q, want unavailable message", html)
	}
}
