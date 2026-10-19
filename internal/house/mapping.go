package house

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/sidun-av/glance-homeassistant/internal/hass"
)

// The translation between Home Assistant and House Control's panel, ported
// from House Control's HomeAssistantService.swift (reading(from:battery:),
// calls(for:…), modeTable, batteryTemplate, batterySensors(from:)),
// FanNames in DeviceService.swift and DeviceHub.pollServices. The panel
// speaks its own vocabulary — a light is on / brightness 0…1 / temp in
// kelvin / color "#rrggbb", an AC is on / target / mode / fan / swing /
// sleep / eco, a robot vacuum is status / battery / fan — so the browser
// never sees what is underneath.

// panelModes are the panel's AC modes, in its order, with the climate
// entity's names for each (the first one the entity has wins).
var panelModes = []struct {
	panel string
	names []string
}{
	{"cool", []string{"cool"}}, {"heat", []string{"heat"}}, {"dry", []string{"dry"}},
	{"fan", []string{"fan_only"}}, {"auto", []string{"auto", "heat_cool"}},
}

// ModeTable is the panel's AC modes and the climate entity's names for them.
func ModeTable(hvacModes []string) map[string]string {
	out := map[string]string{}
	for _, m := range panelModes {
		for _, name := range m.names {
			if contains(hvacModes, name) {
				out[m.panel] = name
				break
			}
		}
	}
	return out
}

// FanOrder is the panel's five fan speeds, slowest to fastest after auto.
var FanOrder = []string{"auto", "low", "medium", "high", "turbo"}

// FanPanel maps whatever a platform calls a fan speed onto the panel's
// five; "" when it is none of them.
func FanPanel(name string) string {
	n := strings.ToLower(name)
	for _, w := range []string{"turbo", "max", "strong", "highest", "powerful", "boost", "very high"} {
		if strings.Contains(n, w) {
			return "turbo"
		}
	}
	switch {
	case strings.Contains(n, "high"):
		return "high"
	case strings.Contains(n, "med") || strings.Contains(n, "mid"):
		return "medium"
	case strings.Contains(n, "low") || strings.Contains(n, "quiet") || strings.Contains(n, "silent"):
		return "low"
	case strings.Contains(n, "auto"):
		return "auto"
	}
	return ""
}

// FanTable is, for each panel fan speed, the platform's own name for it
// (the first one that maps; "off" is never a speed).
func FanTable(names []string) map[string]string {
	out := map[string]string{}
	for _, name := range names {
		if strings.ToLower(name) == "off" {
			continue
		}
		if p := FanPanel(name); p != "" {
			if _, taken := out[p]; !taken {
				out[p] = name
			}
		}
	}
	return out
}

// Reading is what one entity reports, in the panel's terms.
type Reading struct {
	Online bool
	Light  map[string]any
	AC     map[string]any
	// Vacuum: status (cleaning, docked, returning, paused, idle or error),
	// battery 0…100 and fan (the entity's own name) when it reports them.
	Vacuum map[string]any
	// Features: what an AC can do (modes, fans, swing, sleep, eco) or what
	// a vacuum can do (fans, canStart, canPause, canReturn).
	Features map[string]any
	// Temperature: a sensor's reading in °C.
	Temperature *float64
}

// Home Assistant's VacuumEntityFeature bits that matter here.
const (
	vacuumPause      = 4
	vacuumStop       = 8
	vacuumReturnHome = 16
	vacuumFanSpeed   = 32
	vacuumStart      = 8192
)

// ReadingFrom reads one entity. battery is a vacuum's charge from its
// battery sensor, when it has one.
func ReadingFrom(s hass.RawState, battery *float64) Reading {
	id, value, attrs := s.EntityID, s.State, s.Attributes
	if attrs == nil {
		attrs = map[string]any{}
	}
	r := Reading{Online: value != "unavailable" && value != "unknown"}
	if !r.Online {
		return r
	}
	switch {
	case strings.HasPrefix(id, "light.") || strings.HasPrefix(id, "switch."):
		light := map[string]any{"on": value == "on"}
		if b, ok := num(attrs, "brightness"); ok && value == "on" {
			light["brightness"] = clamp(b/255, 0.05, 1)
		}
		switch str(attrs, "color_mode") {
		case "color_temp":
			if k, ok := num(attrs, "color_temp_kelvin"); ok {
				light["temp"] = clamp(k, 2200, 6500)
				light["color"] = nil
			}
		case "hs", "xy", "rgb", "rgbw", "rgbww":
			if rgb, ok := attrs["rgb_color"].([]any); ok && len(rgb) == 3 {
				c := make([]float64, 0, 3)
				for _, v := range rgb {
					if f, ok := v.(float64); ok {
						c = append(c, f)
					}
				}
				if len(c) == 3 {
					light["color"] = hexColor(c[0]/255, c[1]/255, c[2]/255)
				}
			}
		}
		r.Light = light
	case strings.HasPrefix(id, "climate."):
		modes := ModeTable(strs(attrs, "hvac_modes"))
		ac := map[string]any{"on": value != "off"}
		// On, but not running right now: a thermostat whose room is already warm (or cool) enough.
		action, _ := attrs["hvac_action"].(string)
		ac["idle"] = value != "off" && (action == "idle" || action == "off")
		for _, m := range panelModes {
			if name, ok := modes[m.panel]; ok && name == value {
				ac["mode"] = m.panel
				break
			}
		}
		if t, ok := num(attrs, "temperature"); ok {
			ac["target"] = t
		} else if lo, ok := num(attrs, "target_temp_low"); ok {
			if hi, ok := num(attrs, "target_temp_high"); ok {
				ac["target"] = math.Round((lo + hi) / 2)
			}
		}
		if room, ok := num(attrs, "current_temperature"); ok {
			ac["roomTemp"] = room
		}
		fans := FanTable(strs(attrs, "fan_modes"))
		if fan := FanPanel(str(attrs, "fan_mode")); fan != "" {
			if _, ok := fans[fan]; ok {
				ac["fan"] = fan
			}
		}
		swings := strs(attrs, "swing_modes")
		if swing, ok := attrs["swing_mode"].(string); ok && len(swings) > 0 {
			if swing == "off" {
				ac["swing"] = "off"
			} else {
				ac["swing"] = "swing"
			}
		}
		presets := strs(attrs, "preset_modes")
		preset := str(attrs, "preset_mode")
		if contains(presets, "sleep") {
			ac["sleep"] = preset == "sleep"
		}
		if contains(presets, "eco") {
			ac["eco"] = preset == "eco"
		}
		r.AC = ac
		modeList, fanList := []string{}, []string{}
		for _, m := range panelModes {
			if _, ok := modes[m.panel]; ok {
				modeList = append(modeList, m.panel)
			}
		}
		for _, f := range FanOrder {
			if _, ok := fans[f]; ok {
				fanList = append(fanList, f)
			}
		}
		r.Features = map[string]any{
			"modes": modeList, "fans": fanList,
			"swing": contains(swings, "off") && len(swings) > 1,
			"sleep": contains(presets, "sleep"), "eco": contains(presets, "eco"),
		}
	case strings.HasPrefix(id, "vacuum."):
		r.Vacuum, r.Features = vacuumReading(value, attrs, battery)
	case strings.HasPrefix(id, "sensor."):
		if t, err := strconv.ParseFloat(value, 64); err == nil && !math.IsNaN(t) && !math.IsInf(t, 0) {
			if str(attrs, "unit_of_measurement") == "°F" {
				t = (t - 32) * 5 / 9
			}
			r.Temperature = &t
		}
	}
	return r
}

// vacuumReading is a vacuum entity's state and what it can do. A vacuum
// that does not say what it supports is assumed to do the usual things.
// Without a battery sensor, the battery_level Home Assistant deprecated in
// 2025.8 is used while integrations still report it.
func vacuumReading(value string, attrs map[string]any, battery *float64) (map[string]any, map[string]any) {
	status := "idle"
	if contains([]string{"cleaning", "docked", "returning", "paused", "idle", "error"}, value) {
		status = value
	}
	state := map[string]any{"status": status}
	if battery != nil {
		state["battery"] = clamp(math.Round(*battery), 0, 100)
	} else if level, ok := num(attrs, "battery_level"); ok {
		state["battery"] = clamp(math.Round(level), 0, 100)
	}
	bits, known := num(attrs, "supported_features")
	has := func(bit int) bool { return !known || int(bits)&bit != 0 }
	speeds := []string{}
	if has(vacuumFanSpeed) {
		speeds = append(speeds, strs(attrs, "fan_speed_list")...)
	}
	if fan, ok := attrs["fan_speed"].(string); ok && contains(speeds, fan) {
		state["fan"] = fan
	}
	return state, map[string]any{
		"fans":      speeds,
		"canStart":  has(vacuumStart),
		"canPause":  has(vacuumPause) || has(vacuumStop),
		"canReturn": has(vacuumReturnHome),
	}
}

// batteryLevel is a battery sensor's charge, 0…100, when it has one.
func batteryLevel(s hass.RawState) *float64 {
	level, err := strconv.ParseFloat(s.State, 64)
	if err != nil || math.IsNaN(level) || math.IsInf(level, 0) {
		return nil
	}
	v := clamp(math.Round(level), 0, 100)
	return &v
}

// Call is one Home Assistant service call.
type Call struct {
	Domain  string         `json:"domain"`
	Service string         `json:"service"`
	Data    map[string]any `json:"data"`
}

// Calls turns a change made in the panel into Home Assistant service calls.
// state is the device's whole new state (an AC switched on goes back to its
// mode); attrs are the entity's current attributes, for the entity's own
// names of fan speeds and modes and a vacuum's supported_features.
func Calls(entityID string, patch, state, attrs map[string]any) []Call {
	domain, _, _ := strings.Cut(entityID, ".")
	call := func(service string, data map[string]any) Call {
		d := map[string]any{"entity_id": entityID}
		for k, v := range data {
			d[k] = v
		}
		return Call{Domain: domain, Service: service, Data: d}
	}
	on, hasOn := patch["on"].(bool)

	switch domain {
	case "switch":
		if !hasOn {
			return []Call{}
		}
		return []Call{call(onOff(on), nil)}
	case "light":
		if hasOn && !on {
			return []Call{call("turn_off", nil)}
		}
		data := map[string]any{}
		if b, ok := patch["brightness"].(float64); ok {
			data["brightness_pct"] = int(math.Round(b * 100))
		}
		if k, ok := patch["temp"].(float64); ok {
			data["color_temp_kelvin"] = int(k)
		}
		if hex, ok := patch["color"].(string); ok {
			if rgb, ok := parseHex(hex); ok {
				data["rgb_color"] = rgb
			}
		}
		if len(data) == 0 && !(hasOn && on) {
			return []Call{}
		}
		return []Call{call("turn_on", data)}
	case "vacuum":
		out := []Call{}
		// The panel sends the status it wants; the vacuum works out how to get there.
		switch patch["status"] {
		case "cleaning":
			out = append(out, call("start", nil))
		case "paused":
			canPause := true
			if bits, ok := num(attrs, "supported_features"); ok {
				canPause = int(bits)&vacuumPause != 0 || int(bits)&vacuumStop == 0
			}
			if canPause {
				out = append(out, call("pause", nil))
			} else {
				out = append(out, call("stop", nil))
			}
		case "returning":
			out = append(out, call("return_to_base", nil))
		}
		if fan, ok := patch["fan"].(string); ok && fan != "" {
			out = append(out, call("set_fan_speed", map[string]any{"fan_speed": fan}))
		}
		return out
	case "climate":
	default:
		return []Call{}
	}

	modes := ModeTable(strs(attrs, "hvac_modes"))
	fans := FanTable(strs(attrs, "fan_modes"))
	out := []Call{}
	if hasOn && !on {
		return []Call{call("set_hvac_mode", map[string]any{"hvac_mode": "off"})}
	}
	// Picking a mode is what switches a climate entity on.
	if _, modeSet := patch["mode"]; (hasOn && on) || modeSet {
		mode, ok := patch["mode"].(string)
		if !ok {
			if mode, ok = state["mode"].(string); !ok {
				mode = "cool"
			}
		}
		name, ok := modes[mode]
		if !ok {
			// Swift takes "any" of the entity's modes here; the first in the
			// panel's order keeps it deterministic.
			for _, m := range panelModes {
				if n, has := modes[m.panel]; has {
					name, ok = n, true
					break
				}
			}
		}
		if ok {
			out = append(out, call("set_hvac_mode", map[string]any{"hvac_mode": name}))
		}
	}
	if t, ok := patch["target"].(float64); ok {
		out = append(out, call("set_temperature", map[string]any{"temperature": t}))
	}
	if fan, ok := patch["fan"].(string); ok {
		if name, ok := fans[fan]; ok {
			out = append(out, call("set_fan_mode", map[string]any{"fan_mode": name}))
		}
	}
	if swing, ok := patch["swing"].(string); ok {
		mode := "on"
		if swing == "off" {
			mode = "off"
		}
		out = append(out, call("set_swing_mode", map[string]any{"swing_mode": mode}))
	}
	for _, preset := range []string{"sleep", "eco"} {
		if enabled, ok := patch[preset].(bool); ok {
			name := "none"
			if enabled {
				name = preset
			}
			out = append(out, call("set_preset_mode", map[string]any{"preset_mode": name}))
		}
	}
	return out
}

// safeEntityID is what an entity id looks like in Home Assistant; only
// such ids are put inside a template.
var safeEntityID = regexp.MustCompile(`^[a-z0-9_]+\.[a-z0-9_]+$`)

// BatteryTemplate renders one line per vacuum, "vacuum.x<TAB>sensor.y": the
// first sensor of its device whose device_class is battery, or nothing
// after the tab. (Home Assistant 2025.8 moved a vacuum's battery off the
// vacuum entity onto a sensor of the same device.)
func BatteryTemplate(vacuums []string) string {
	quoted := []string{}
	for _, v := range vacuums {
		if safeEntityID.MatchString(v) {
			quoted = append(quoted, "'"+v+"'")
		}
	}
	return "{% for v in [" + strings.Join(quoted, ",") + "] %}{% set d = device_id(v) %}{% set ns = namespace(b='') %}" +
		"{% if d %}{% for e in device_entities(d) %}" +
		"{% if not ns.b and e.startswith('sensor.') and state_attr(e, 'device_class') == 'battery' %}{% set ns.b = e %}{% endif %}" +
		"{% endfor %}{% endif %}{{ v }}\t{{ ns.b }}\n{% endfor %}"
}

// ParseBatterySensors reads BatteryTemplate's answer: vacuum → its battery
// sensor, for the vacuums that have one.
func ParseBatterySensors(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 2 {
			continue
		}
		vac, sensor := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if strings.HasPrefix(vac, "vacuum.") && strings.HasPrefix(sensor, "sensor.") {
			out[vac] = sensor
		}
	}
	return out
}

// Snapshot is the state of every served device in the shape House
// Control's applyRemote takes, keyed by plan id, merged the way
// DeviceHub.pollServices does: a light only while reachable; a robot that
// cannot be reached says "unavailable" (the panel must not go on showing
// its last status as if it were true); an AC's room reading comes from its
// own sensor when it has one. An entity Home Assistant does not know is
// left out. batteries maps a vacuum to its battery sensor ("" or absent:
// none).
func Snapshot(p *Plan, states map[string]hass.RawState, batteries map[string]string) map[string]any {
	lights, acs, vacuums, features := map[string]any{}, map[string]any{}, map[string]any{}, map[string]any{}
	if p != nil {
		for _, d := range p.Devices {
			s, known := states[d.EntityID]
			switch d.Kind {
			case KindLight:
				if !known {
					continue
				}
				if r := ReadingFrom(s, nil); r.Online && r.Light != nil {
					lights[d.AppID] = r.Light
				}
			case KindVacuum:
				if !known {
					continue
				}
				var battery *float64
				if sensor := batteries[d.EntityID]; sensor != "" {
					if bs, ok := states[sensor]; ok {
						battery = batteryLevel(bs)
					}
				}
				r := ReadingFrom(s, battery)
				if !r.Online {
					vacuums[d.AppID] = map[string]any{"status": "unavailable"}
				} else if r.Vacuum != nil {
					vacuums[d.AppID] = r.Vacuum
					if r.Features != nil {
						features[d.AppID] = r.Features
					}
				}
			case KindAC:
				ac := map[string]any{}
				var r Reading
				if known {
					r = ReadingFrom(s, nil)
					for k, v := range r.AC {
						ac[k] = v
					}
				}
				if d.SensorID != "" {
					if ss, ok := states[d.SensorID]; ok {
						if t := ReadingFrom(ss, nil).Temperature; t != nil {
							ac["roomTemp"] = *t
						}
					}
				}
				if len(ac) > 0 {
					acs[d.AppID] = ac
				}
				if r.Features != nil {
					features[d.AppID] = r.Features
				}
			}
		}
	}
	return map[string]any{"lights": lights, "acs": acs, "vacuums": vacuums, "features": features}
}

// --- small helpers mirroring Swift's casts ---

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

func onOff(on bool) string {
	if on {
		return "turn_on"
	}
	return "turn_off"
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func str(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

// strs is a list attribute's strings (other elements are skipped).
func strs(m map[string]any, key string) []string {
	var out []string
	switch v := m[key].(type) {
	case []any:
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	case []string:
		out = append(out, v...)
	}
	return out
}

// hexColor is ColorMath.hex(r:g:b:): "#rrggbb" of 0…1 channels.
func hexColor(r, g, b float64) string {
	byteOf := func(v float64) int { return int(math.Round(clamp(v, 0, 1) * 255)) }
	return fmt.Sprintf("#%02x%02x%02x", byteOf(r), byteOf(g), byteOf(b))
}

// parseHex is ColorMath.rgb as 0…255 integers: "#ff8000" → [255 128 0].
func parseHex(s string) ([]int, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return nil, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return nil, false
	}
	return []int{int(v >> 16 & 0xff), int(v >> 8 & 0xff), int(v & 0xff)}, true
}
