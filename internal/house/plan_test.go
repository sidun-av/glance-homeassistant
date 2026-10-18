package house

import (
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// fixturePlan is a generic House Control plan: two Home Assistant lamps
// (a light and a switch), a heater with its own thermometer and a robot,
// next to devices the web view must never show — a Hue lamp, an unlinked
// lamp, a Smart Life lamp, a fan entity linked as a lamp and an AC linked
// to a light entity.
const fixturePlan = `{
  "version": 1,
  "name": "Demo",
  "rooms": [{"id": "a", "name": "Room A", "rect": [0, 0, 4, 4]}],
  "furniture": [{"type": "sofa"}],
  "scenes": [{"id": "evening", "name": "Evening", "lights": {"a.lamp": true}}],
  "lights": [
    {"id": "a.lamp", "room": "a", "capabilities": ["brightness", "temp"], "link": {"service": "homeassistant", "id": "light.demo_lamp"}},
    {"id": "b.plug", "room": "a", "link": {"service": "homeassistant", "id": "switch.demo_plug"}},
    {"id": "c.hue", "room": "a", "link": {"service": "hue", "id": "1"}},
    {"id": "d.free", "room": "a"},
    {"id": "e.tuya", "room": "a", "tuya": {"deviceId": "x"}},
    {"id": "f.fan", "room": "a", "link": {"service": "homeassistant", "id": "fan.demo"}}
  ],
  "acs": [
    {"id": "k.heat", "room": "a", "link": {"service": "homeassistant", "id": "climate.demo_heater"}, "sensor": {"service": "homeassistant", "id": "sensor.demo_temp"}, "min": 10, "max": 28},
    {"id": "k.bad", "room": "a", "link": {"service": "homeassistant", "id": "light.x"}}
  ],
  "vacuums": [
    {"id": "h.robot", "room": "a", "link": {"service": "homeassistant", "id": "vacuum.demo_robot"}}
  ]
}`

func servedIDs(t *testing.T, served map[string]any, key string) []string {
	t.Helper()
	list, _ := served[key].([]any)
	ids := []string{}
	for _, d := range list {
		ids = append(ids, d.(map[string]any)["id"].(string))
	}
	return ids
}

func TestParsePlan_FiltersToHomeAssistantDevices(t *testing.T) {
	p, err := ParsePlan([]byte(fixturePlan))
	if err != nil {
		t.Fatalf("ParsePlan: %v", err)
	}
	want := []Device{
		{AppID: "a.lamp", Kind: KindLight, EntityID: "light.demo_lamp", Capabilities: []string{"brightness", "temp"}},
		{AppID: "b.plug", Kind: KindLight, EntityID: "switch.demo_plug"},
		{AppID: "k.heat", Kind: KindAC, EntityID: "climate.demo_heater", SensorID: "sensor.demo_temp", Min: 10, Max: 28},
		{AppID: "h.robot", Kind: KindVacuum, EntityID: "vacuum.demo_robot"},
	}
	if !reflect.DeepEqual(p.Devices, want) {
		t.Fatalf("Devices = %+v\nwant %+v", p.Devices, want)
	}
	if d, ok := p.Device("k.heat"); !ok || d.EntityID != "climate.demo_heater" {
		t.Errorf("Device(k.heat) = %+v, %v", d, ok)
	}
	if _, ok := p.Device("c.hue"); ok {
		t.Error("Device(c.hue) found; a Hue lamp must be dropped")
	}

	var served map[string]any
	if err := json.Unmarshal(p.Served(), &served); err != nil {
		t.Fatalf("Served() is not JSON: %v", err)
	}
	if got := servedIDs(t, served, "lights"); !reflect.DeepEqual(got, []string{"a.lamp", "b.plug"}) {
		t.Errorf("served lights = %v", got)
	}
	if got := servedIDs(t, served, "acs"); !reflect.DeepEqual(got, []string{"k.heat"}) {
		t.Errorf("served acs = %v", got)
	}
	if got := servedIDs(t, served, "vacuums"); !reflect.DeepEqual(got, []string{"h.robot"}) {
		t.Errorf("served vacuums = %v", got)
	}
	var orig map[string]any
	json.Unmarshal([]byte(fixturePlan), &orig)
	for _, key := range []string{"rooms", "furniture", "scenes", "name", "version"} {
		if !reflect.DeepEqual(served[key], orig[key]) {
			t.Errorf("served %s = %v, want unchanged %v", key, served[key], orig[key])
		}
	}
}

func TestParsePlan_ACDefaultsAndNonHASensorDropped(t *testing.T) {
	raw := `{"rooms":[{"id":"a"}],"acs":[{"id":"k.ac","link":{"service":"homeassistant","id":"climate.demo_heater"},"sensor":{"service":"hue","id":"7"}}]}`
	p, err := ParsePlan([]byte(raw))
	if err != nil {
		t.Fatalf("ParsePlan: %v", err)
	}
	if len(p.Devices) != 1 {
		t.Fatalf("Devices = %+v", p.Devices)
	}
	d := p.Devices[0]
	if d.Min != 16 || d.Max != 30 || d.SensorID != "" {
		t.Errorf("ac = %+v, want Min 16 Max 30 and no sensor", d)
	}
	var served map[string]any
	json.Unmarshal(p.Served(), &served)
	ac := served["acs"].([]any)[0].(map[string]any)
	if _, ok := ac["sensor"]; ok {
		t.Errorf("served ac keeps a non-HA sensor: %v", ac)
	}
}

// An entity id that is not one Home Assistant could have (a list, a space,
// capitals) would be served as a device that always fails; it is dropped
// like any other unsupported link, and so is such an AC sensor.
func TestParsePlan_MalformedEntityIDsDropped(t *testing.T) {
	raw := `{"rooms":[{"id":"a"}],"lights":[` +
		`{"id":"l.list","link":{"service":"homeassistant","id":"light.a,light.b"}},` +
		`{"id":"l.space","link":{"service":"homeassistant","id":"light.a b"}},` +
		`{"id":"l.caps","link":{"service":"homeassistant","id":"light.Lamp"}},` +
		`{"id":"l.two","link":{"service":"homeassistant","id":"light.a.b"}},` +
		`{"id":"l.good","link":{"service":"homeassistant","id":"light.demo_lamp"}}],` +
		`"acs":[{"id":"k.ac","link":{"service":"homeassistant","id":"climate.demo_heater"},"sensor":{"service":"homeassistant","id":"sensor.t,sensor.u"}}]}`
	p, err := ParsePlan([]byte(raw))
	if err != nil {
		t.Fatalf("ParsePlan: %v", err)
	}
	var ids []string
	for _, d := range p.Devices {
		ids = append(ids, d.AppID)
	}
	if !reflect.DeepEqual(ids, []string{"l.good", "k.ac"}) {
		t.Errorf("devices = %v, want only l.good and k.ac", ids)
	}
	if d, _ := p.Device("k.ac"); d.SensorID != "" {
		t.Errorf("AC sensor = %q, want none", d.SensorID)
	}
	if strings.Contains(string(p.Served()), "sensor.t") {
		t.Errorf("served plan keeps the malformed sensor: %s", p.Served())
	}
}

func TestParsePlan_Rejects(t *testing.T) {
	cases := map[string]struct {
		raw    string
		reason string
	}{
		"array":                 {`[]`, "not_object"},
		"not json":              {`{nope`, "not_object"},
		"no rooms":              {`{"rooms":[]}`, "no_rooms"},
		"rooms missing":         {`{"lights":[]}`, "no_rooms"},
		"token anywhere":        {`{"rooms":[{}],"x":{"Token":"y"}}`, "forbidden_key"},
		"access_token":          {`{"rooms":[{"access_token":"y"}]}`, "forbidden_key"},
		"password":              {`{"rooms":[{}],"a":{"b":{"PASSWORD":1}}}`, "forbidden_key"},
		"secret":                {`{"rooms":[{}],"secret":""}`, "forbidden_key"},
		"accessToken in array":  {`{"rooms":[{}],"lights":[{"id":"x","link":{"accessToken":"z"}}]}`, "forbidden_key"},
		"too large":             {`{"rooms":[{}],"pad":"` + strings.Repeat("x", MaxPlanBytes) + `"}`, "too_large"},
		"rooms is not an array": {`{"rooms":{"a":1}}`, "no_rooms"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParsePlan([]byte(c.raw))
			var pe *PlanError
			if !errors.As(err, &pe) {
				t.Fatalf("err = %v, want *PlanError", err)
			}
			if pe.Reason != c.reason {
				t.Errorf("Reason = %q, want %q", pe.Reason, c.reason)
			}
		})
	}
	if _, err := ParsePlan(make([]byte, MaxPlanBytes+1)); err == nil || err.(*PlanError).Reason != "too_large" {
		t.Errorf("MaxPlanBytes+1 bytes: err = %v, want too_large", err)
	}
}

func TestParsePlan_ETagIsStable(t *testing.T) {
	a, _ := ParsePlan([]byte(fixturePlan))
	b, _ := ParsePlan([]byte(fixturePlan))
	if a.ETag != b.ETag {
		t.Errorf("same bytes, ETags %s and %s", a.ETag, b.ETag)
	}
	if !regexp.MustCompile(`^"[0-9a-f]{16}"$`).MatchString(a.ETag) {
		t.Errorf("ETag %s does not look like \"<16 hex>\"", a.ETag)
	}
	c, _ := ParsePlan([]byte(strings.Replace(fixturePlan, `"Demo"`, `"Demp"`, 1)))
	if c.ETag == a.ETag {
		t.Error("one byte changed, ETag did not")
	}
}
