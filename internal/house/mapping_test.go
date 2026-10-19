package house

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/sidun-av/glance-homeassistant/internal/hass"
)

// These port House Control's core/Tests/HouseCoreTests/ServiceTests.swift
// (the Home Assistant part) value for value, so the web view behaves like
// the Mac app on the same entities.

func st(id, state string, attrs map[string]any) hass.RawState {
	if attrs == nil {
		attrs = map[string]any{}
	}
	return hass.RawState{EntityID: id, State: state, Attributes: attrs}
}

// decoded round-trips v through JSON, the way attributes arrive from HA.
func decoded(t *testing.T, v map[string]any) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(v)
	var out map[string]any
	json.Unmarshal(raw, &out)
	return out
}

func services(calls []Call) []string {
	out := []string{}
	for _, c := range calls {
		out = append(out, c.Domain+"."+c.Service)
	}
	return out
}

func assertServices(t *testing.T, calls []Call, want ...string) {
	t.Helper()
	got := []string{}
	for _, c := range calls {
		got = append(got, c.Service)
	}
	if want == nil {
		want = []string{}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("services = %v, want %v", got, want)
	}
}

func assertData(t *testing.T, c Call, want map[string]any) {
	t.Helper()
	if !reflect.DeepEqual(c.Data, want) {
		t.Errorf("data = %#v, want %#v", c.Data, want)
	}
}

func assertMap(t *testing.T, got, want map[string]string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func eq(t *testing.T, what string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %#v, want %#v", what, got, want)
	}
}

func TestReading_Light(t *testing.T) {
	white := ReadingFrom(st("light.ceiling", "on", decoded(t, map[string]any{"brightness": 255, "color_mode": "color_temp", "color_temp_kelvin": 2631})), nil).Light
	eq(t, "on", white["on"], true)
	eq(t, "brightness", white["brightness"], 1.0)
	eq(t, "temp", white["temp"], 2631.0)
	if c, ok := white["color"]; !ok || c != nil {
		t.Errorf("color = %#v, %v; want an explicit null", c, ok)
	}
	colour := ReadingFrom(st("light.kitchen", "on", decoded(t, map[string]any{"brightness": 180, "color_mode": "hs", "rgb_color": []int{255, 64, 112}})), nil).Light
	eq(t, "color", colour["color"], "#ff4070")
	if ReadingFrom(st("light.x", "unavailable", nil), nil).Online {
		t.Error("an unavailable light is online")
	}
	// Brightness only while on, clamped to 5%…100%.
	off := ReadingFrom(st("light.x", "off", decoded(t, map[string]any{"brightness": 200})), nil).Light
	if _, ok := off["brightness"]; ok || off["on"] != false {
		t.Errorf("off light = %v", off)
	}
	dim := ReadingFrom(st("light.x", "on", decoded(t, map[string]any{"brightness": 1})), nil).Light
	eq(t, "dim brightness", dim["brightness"], 0.05)
	eq(t, "switch", ReadingFrom(st("switch.p", "on", nil), nil).Light, map[string]any{"on": true})
}

func TestReading_ClimateIdle(t *testing.T) {
	idle := func(state string, action any) any {
		attrs := map[string]any{"hvac_modes": []string{"off", "heat"}, "temperature": 21}
		if action != nil {
			attrs["hvac_action"] = action
		}
		return ReadingFrom(st("climate.heater", state, decoded(t, attrs)), nil).AC["idle"]
	}
	eq(t, "heat/idle", idle("heat", "idle"), true)
	eq(t, "heat/heating", idle("heat", "heating"), false)
	eq(t, "heat/none", idle("heat", nil), false)
	eq(t, "off/off", idle("off", "off"), false)
}

func TestReading_Climate(t *testing.T) {
	r := ReadingFrom(st("climate.hvac", "fan_only", decoded(t, map[string]any{
		"hvac_modes": []string{"off", "heat", "cool", "auto", "dry", "fan_only"}, "temperature": 21,
		"current_temperature": 22, "fan_modes": []string{"on_low", "on_high", "auto_low", "auto_high", "off"},
		"fan_mode": "on_high", "swing_modes": []string{"auto", "1", "off"}, "swing_mode": "off",
		"preset_modes": []string{"home", "eco"}, "preset_mode": "eco",
	})), nil)
	eq(t, "on", r.AC["on"], true)
	eq(t, "mode", r.AC["mode"], "fan")
	eq(t, "target", r.AC["target"], 21.0)
	eq(t, "roomTemp", r.AC["roomTemp"], 22.0)
	eq(t, "fan", r.AC["fan"], "high")
	eq(t, "swing", r.AC["swing"], "off")
	eq(t, "eco", r.AC["eco"], true)
	eq(t, "idle", r.AC["idle"], false)
	if _, ok := r.AC["sleep"]; ok {
		t.Error("sleep reported for an entity without a sleep preset")
	}
	eq(t, "features.modes", r.Features["modes"], []string{"cool", "heat", "dry", "fan", "auto"})
	eq(t, "features.fans", r.Features["fans"], []string{"low", "high"})
	eq(t, "features.swing", r.Features["swing"], true)
	eq(t, "features.eco", r.Features["eco"], true)
	eq(t, "features.sleep", r.Features["sleep"], false)

	// A heat_cool-only thermostat reports a range instead of one set-point.
	rng := ReadingFrom(st("climate.ecobee", "heat_cool", decoded(t, map[string]any{"hvac_modes": []string{"off", "heat_cool"}, "target_temp_low": 21, "target_temp_high": 24})), nil)
	eq(t, "range mode", rng.AC["mode"], "auto")
	eq(t, "range target", rng.AC["target"], 23.0)

	off := ReadingFrom(st("climate.hvac", "off", decoded(t, map[string]any{"hvac_modes": []string{"off", "heat"}})), nil)
	eq(t, "off on", off.AC["on"], false)
	if _, ok := off.AC["mode"]; ok {
		t.Error("an off climate has a mode")
	}
}

func TestReading_Sensor(t *testing.T) {
	c := ReadingFrom(st("sensor.demo_temp", "21.5", map[string]any{"unit_of_measurement": "°C"}), nil)
	if c.Temperature == nil || *c.Temperature != 21.5 {
		t.Errorf("°C = %v", c.Temperature)
	}
	f := ReadingFrom(st("sensor.demo_temp", "212", map[string]any{"unit_of_measurement": "°F"}), nil)
	if f.Temperature == nil || *f.Temperature != 100 {
		t.Errorf("°F = %v", f.Temperature)
	}
	if ReadingFrom(st("sensor.demo_temp", "abc", nil), nil).Temperature != nil {
		t.Error("a non-number sensor has a temperature")
	}
}

func TestModeAndFanTables(t *testing.T) {
	assertMap(t, ModeTable([]string{"off", "cool", "heat_cool", "fan_only"}), map[string]string{"cool": "cool", "auto": "heat_cool", "fan": "fan_only"})
	assertMap(t, FanTable([]string{"on_low", "on_high", "auto_low", "off"}), map[string]string{"low": "on_low", "high": "on_high"})
	for name, want := range map[string]string{"Turbo": "turbo", "very high": "turbo", "High": "high", "medium": "medium", "Mid": "medium", "Quiet": "low", "auto": "auto", "swirl": ""} {
		if got := FanPanel(name); got != want {
			t.Errorf("FanPanel(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestCalls_LightAndSwitch(t *testing.T) {
	off := Calls("light.a", map[string]any{"on": false}, nil, nil)
	assertServices(t, off, "turn_off")
	dim := Calls("light.a", map[string]any{"brightness": 0.5, "temp": 3000.0}, nil, nil)
	assertServices(t, dim, "turn_on")
	assertData(t, dim[0], map[string]any{"entity_id": "light.a", "brightness_pct": 50, "color_temp_kelvin": 3000})
	col := Calls("light.a", map[string]any{"color": "#ff8000"}, nil, nil)
	assertData(t, col[0], map[string]any{"entity_id": "light.a", "rgb_color": []int{255, 128, 0}})
	assertServices(t, Calls("switch.p", map[string]any{"on": true}, nil, nil), "turn_on")
	assertServices(t, Calls("switch.p", map[string]any{"on": false}, nil, nil), "turn_off")
	assertServices(t, Calls("switch.p", map[string]any{"brightness": 0.5}, nil, nil))
	assertServices(t, Calls("light.a", map[string]any{}, nil, nil))
	if c := Calls("light.a", map[string]any{"on": true}, nil, nil); len(c) != 1 || c[0].Domain != "light" {
		t.Errorf("on = %+v", c)
	}
}

func TestCalls_Climate(t *testing.T) {
	attrs := map[string]any{"hvac_modes": []any{"off", "cool", "heat_cool", "fan_only"}, "fan_modes": []any{"on_low", "on_high", "auto_low", "off"}}
	on := Calls("climate.x", map[string]any{"on": true}, map[string]any{"mode": "auto"}, attrs)
	if len(on) == 0 || on[0].Service != "set_hvac_mode" || on[0].Data["hvac_mode"] != "heat_cool" {
		t.Errorf("on = %+v", on)
	}
	ch := Calls("climate.x", map[string]any{"target": 23.0, "fan": "high", "eco": true}, map[string]any{"mode": "cool"}, attrs)
	got := map[string]bool{}
	for _, c := range ch {
		got[c.Service] = true
	}
	eq(t, "services", got, map[string]bool{"set_temperature": true, "set_fan_mode": true, "set_preset_mode": true})
	for _, c := range ch {
		switch c.Service {
		case "set_temperature":
			eq(t, "temperature", c.Data["temperature"], 23.0)
		case "set_fan_mode":
			eq(t, "fan_mode", c.Data["fan_mode"], "on_high")
		case "set_preset_mode":
			eq(t, "preset_mode", c.Data["preset_mode"], "eco")
		}
	}
	offCalls := Calls("climate.x", map[string]any{"on": false}, nil, attrs)
	if len(offCalls) != 1 || offCalls[0].Data["hvac_mode"] != "off" {
		t.Errorf("off = %+v", offCalls)
	}
	// A mode the entity lacks falls back to one it has (deterministically
	// the first in the panel's order, here cool).
	fb := Calls("climate.x", map[string]any{"mode": "heat"}, nil, attrs)
	eq(t, "fallback mode", fb[0].Data["hvac_mode"], "cool")
	sw := Calls("climate.x", map[string]any{"swing": "l3", "sleep": false}, nil, attrs)
	eq(t, "swing/sleep", services(sw), []string{"climate.set_swing_mode", "climate.set_preset_mode"})
	eq(t, "swing_mode", sw[0].Data["swing_mode"], "on")
	eq(t, "preset none", sw[1].Data["preset_mode"], "none")
}

func TestReading_Vacuum(t *testing.T) {
	// PAUSE | RETURN_HOME | FAN_SPEED | BATTERY | STATE | START
	r := ReadingFrom(st("vacuum.robo", "cleaning", decoded(t, map[string]any{
		"battery_level": 61, "fan_speed": "Turbo", "fan_speed_list": []string{"Quiet", "Turbo"},
		"supported_features": 4 | 16 | 32 | 64 | 4096 | 8192,
	})), nil)
	eq(t, "status", r.Vacuum["status"], "cleaning")
	eq(t, "battery", r.Vacuum["battery"], 61.0)
	eq(t, "fan", r.Vacuum["fan"], "Turbo")
	eq(t, "fans", r.Features["fans"], []string{"Quiet", "Turbo"})
	eq(t, "canStart", r.Features["canStart"], true)
	eq(t, "canPause", r.Features["canPause"], true)
	eq(t, "canReturn", r.Features["canReturn"], true)
	if r.AC != nil {
		t.Error("a vacuum has an ac reading")
	}

	// No fan speeds, no way home and no start: the panel offers none of them.
	basic := ReadingFrom(st("vacuum.basic", "returning", decoded(t, map[string]any{"fan_speed_list": []string{"Max"}, "supported_features": 8})), nil)
	eq(t, "basic status", basic.Vacuum["status"], "returning")
	if _, ok := basic.Vacuum["battery"]; ok {
		t.Error("basic has a battery")
	}
	eq(t, "basic fans", basic.Features["fans"], []string{})
	eq(t, "basic canStart", basic.Features["canStart"], false)
	eq(t, "basic canPause", basic.Features["canPause"], true) // through stop
	eq(t, "basic canReturn", basic.Features["canReturn"], false)
	if ReadingFrom(st("vacuum.x", "unavailable", nil), nil).Online {
		t.Error("unavailable vacuum is online")
	}
	eq(t, "mopping", ReadingFrom(st("vacuum.x", "mopping", nil), nil).Vacuum["status"], "idle")
	// No supported_features at all: assumed to do the usual things.
	all := ReadingFrom(st("vacuum.x", "docked", nil), nil)
	eq(t, "default canStart", all.Features["canStart"], true)
	eq(t, "default canReturn", all.Features["canReturn"], true)
}

func TestReading_VacuumBatterySensor(t *testing.T) {
	eq(t, "level 66", batteryLevel(st("sensor.robo_battery", "66", nil)), ptr(66))
	eq(t, "level 100.4", batteryLevel(st("sensor.robo_battery", "100.4", nil)), ptr(100))
	if batteryLevel(st("sensor.robo_battery", "unavailable", nil)) != nil {
		t.Error("unavailable battery has a level")
	}
	state := st("vacuum.robo", "docked", decoded(t, map[string]any{"supported_features": 16 | 8192}))
	eq(t, "sensor battery", ReadingFrom(state, ptr(66)).Vacuum["battery"], 66.0)
	if _, ok := ReadingFrom(state, nil).Vacuum["battery"]; ok {
		t.Error("battery without a sensor or battery_level")
	}
	// The sensor wins over a deprecated battery_level that is still there.
	both := st("vacuum.robo", "docked", decoded(t, map[string]any{"battery_level": 20}))
	eq(t, "sensor wins", ReadingFrom(both, ptr(66)).Vacuum["battery"], 66.0)
	eq(t, "battery_level", ReadingFrom(both, nil).Vacuum["battery"], 20.0)
}

func ptr(v float64) *float64 { return &v }

func TestCalls_VacuumPauseFallsBackToStop(t *testing.T) {
	svc := func(patch map[string]any, attrs map[string]any) []string {
		return services(Calls("vacuum.robo", patch, nil, attrs))
	}
	eq(t, "cleaning", svc(map[string]any{"status": "cleaning"}, nil), []string{"vacuum.start"})
	eq(t, "paused", svc(map[string]any{"status": "paused"}, nil), []string{"vacuum.pause"})
	eq(t, "paused 4", svc(map[string]any{"status": "paused"}, map[string]any{"supported_features": 4.0}), []string{"vacuum.pause"})
	eq(t, "paused stop", svc(map[string]any{"status": "paused"}, map[string]any{"supported_features": float64(8 | 8192)}), []string{"vacuum.stop"})
	eq(t, "returning", svc(map[string]any{"status": "returning"}, nil), []string{"vacuum.return_to_base"})
	// A status the panel cannot ask for is never sent.
	eq(t, "docked", svc(map[string]any{"status": "docked"}, nil), []string{})
	fan := Calls("vacuum.robo", map[string]any{"fan": "Turbo"}, nil, nil)
	assertServices(t, fan, "set_fan_speed")
	assertData(t, fan[0], map[string]any{"entity_id": "vacuum.robo", "fan_speed": "Turbo"})
}

func TestBatteryTemplateAndParse(t *testing.T) {
	tpl := BatteryTemplate([]string{"vacuum.a", "vacuum.b"})
	want := "{% for v in ['vacuum.a','vacuum.b'] %}{% set d = device_id(v) %}{% set ns = namespace(b='') %}" +
		"{% if d %}{% for e in device_entities(d) %}" +
		"{% if not ns.b and e.startswith('sensor.') and state_attr(e, 'device_class') == 'battery' %}{% set ns.b = e %}{% endif %}" +
		"{% endfor %}{% endif %}{{ v }}\t{{ ns.b }}\n{% endfor %}"
	if tpl != want {
		t.Errorf("template =\n%q\nwant\n%q", tpl, want)
	}
	// An id that could break out of the quoted list is never put in a template.
	if bad := BatteryTemplate([]string{"vacuum.a'] %}{{ x }}{% for v in ['"}); strings.Contains(bad, "x }}") {
		t.Errorf("template carries an unsafe id: %q", bad)
	}
	eq(t, "parse", ParseBatterySensors("vacuum.a\tsensor.a_battery\nvacuum.b\t\n"), map[string]string{"vacuum.a": "sensor.a_battery"})
	eq(t, "parse swift", ParseBatterySensors("vacuum.robo\tsensor.robo_battery\nvacuum.old\t\nvacuum.gone\tNone\nvacuum.last\tsensor.last_battery"),
		map[string]string{"vacuum.robo": "sensor.robo_battery", "vacuum.last": "sensor.last_battery"})
}

func snapshotPlan(t *testing.T) *Plan {
	t.Helper()
	p, err := ParsePlan([]byte(`{"rooms":[{"id":"a"}],
	  "lights":[{"id":"a.lamp","link":{"service":"homeassistant","id":"light.demo_lamp"}},
	            {"id":"b.plug","link":{"service":"homeassistant","id":"switch.demo_plug"}},
	            {"id":"c.gone","link":{"service":"homeassistant","id":"light.demo_gone"}}],
	  "acs":[{"id":"k.heat","link":{"service":"homeassistant","id":"climate.demo_heater"},"sensor":{"service":"homeassistant","id":"sensor.demo_temp"}}],
	  "vacuums":[{"id":"h.robot","link":{"service":"homeassistant","id":"vacuum.demo_robot"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSnapshot_MergeRules(t *testing.T) {
	p := snapshotPlan(t)
	states := map[string]hass.RawState{
		"light.demo_lamp":           st("light.demo_lamp", "off", nil),
		"switch.demo_plug":          st("switch.demo_plug", "unavailable", nil),
		"climate.demo_heater":       st("climate.demo_heater", "heat", decoded(t, map[string]any{"hvac_modes": []string{"off", "heat"}, "temperature": 22, "current_temperature": 19})),
		"sensor.demo_temp":          st("sensor.demo_temp", "21.5", map[string]any{"unit_of_measurement": "°C"}),
		"vacuum.demo_robot":         st("vacuum.demo_robot", "docked", decoded(t, map[string]any{"supported_features": 16 | 8192})),
		"sensor.demo_robot_battery": st("sensor.demo_robot_battery", "42.4", nil),
	}
	batteries := map[string]string{"vacuum.demo_robot": "sensor.demo_robot_battery"}
	snap := Snapshot(p, states, batteries)

	lights := snap["lights"].(map[string]any)
	eq(t, "lights", lights, map[string]any{"a.lamp": map[string]any{"on": false}}) // plug offline, c.gone missing
	acs := snap["acs"].(map[string]any)
	heat := acs["k.heat"].(map[string]any)
	eq(t, "roomTemp from sensor", heat["roomTemp"], 21.5)
	eq(t, "heat mode", heat["mode"], "heat")
	vac := snap["vacuums"].(map[string]any)["h.robot"].(map[string]any)
	eq(t, "battery", vac["battery"], 42.0)
	features := snap["features"].(map[string]any)
	if _, ok := features["k.heat"]; !ok {
		t.Error("no ac features")
	}
	if _, ok := features["h.robot"]; !ok {
		t.Error("no vacuum features")
	}

	// An unreachable robot says so, without features; an offline AC keeps
	// only its room reading.
	states["vacuum.demo_robot"] = st("vacuum.demo_robot", "unavailable", nil)
	states["climate.demo_heater"] = st("climate.demo_heater", "unavailable", nil)
	snap = Snapshot(p, states, batteries)
	eq(t, "unavailable robot", snap["vacuums"].(map[string]any)["h.robot"], map[string]any{"status": "unavailable"})
	features = snap["features"].(map[string]any)
	if _, ok := features["h.robot"]; ok {
		t.Error("an unavailable robot has features")
	}
	eq(t, "offline ac", snap["acs"].(map[string]any)["k.heat"], map[string]any{"roomTemp": 21.5})

	// Nothing known: every section is an empty object, never null.
	snap = Snapshot(p, map[string]hass.RawState{}, nil)
	raw, _ := json.Marshal(snap)
	eq(t, "empty", string(raw), `{"acs":{},"features":{},"lights":{},"vacuums":{}}`)
}
