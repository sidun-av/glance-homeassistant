// Package house serves House Control's 3D home inside the widget: the
// plan uploaded from the owner's Mac, the state of its Home Assistant
// devices in House Control's own vocabulary, and the commands its panel
// sends, all under <public_url>/house/. The mapping between Home Assistant
// and the panel is a port of House Control's Swift HomeAssistantService.
package house

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// MaxPlanBytes caps an uploaded plan. A real plan is a few tens of KiB.
const MaxPlanBytes = 1 << 20

type Kind string

const (
	KindLight  Kind = "light"
	KindAC     Kind = "ac"
	KindVacuum Kind = "vacuum"
)

// Device is one plan device the web view may show and control: linked to
// Home Assistant, through an entity of a domain that fits its kind.
type Device struct {
	AppID    string // the plan's id, which the panel and /house/state use
	Kind     Kind
	EntityID string
	SensorID string // ac only: a sensor.* that reports the room, "" if none
	Min, Max float64
	// Capabilities is a light's "brightness" / "temp" / "color", from the plan.
	Capabilities []string
}

// Plan is a parsed, checked plan: the devices the web view serves, and the
// filtered JSON it hands the browser.
type Plan struct {
	// ETag is `"` + the first 16 hex digits of sha256(raw) + `"`: it names
	// this upload, and /house/state reports it as planVersion so an open 3D
	// view reloads when the plan changes.
	ETag    string
	Devices []Device
	served  []byte
	byID    map[string]int
}

// PlanError says why an uploaded plan was refused. Reason is one of
// too_large, not_object, no_rooms, forbidden_key.
type PlanError struct {
	Reason string
}

func (e *PlanError) Error() string { return "invalid plan: " + e.Reason }

// Served is the plan as the browser gets it: every top-level key as
// uploaded, except that lights, acs and vacuums keep only the devices in
// Devices (so nothing in the web view is simulated or silently inert).
func (p *Plan) Served() []byte { return p.served }

// batteryHints is "vacuum\tsensor\n" for each served vacuum batteries
// names a well-formed battery sensor for, in plan order.
func (p *Plan) batteryHints(batteries map[string]string) (map[string]string, string) {
	hints := map[string]string{}
	var key strings.Builder
	for _, d := range p.Devices {
		s := batteries[d.EntityID]
		if d.Kind != KindVacuum || !safeEntityID.MatchString(s) || !hasDomain(s, []string{"sensor."}) {
			continue
		}
		hints[d.EntityID] = s
		key.WriteString(d.EntityID + "\t" + s + "\n")
	}
	return hints, key.String()
}

// ETagWith is the ETag of ServedWith(batteries): the plan's own ETag when
// no vacuum gains a battery sensor, otherwise one that covers the sensors
// too.
func (p *Plan) ETagWith(batteries map[string]string) string {
	_, key := p.batteryHints(batteries)
	if key == "" {
		return p.ETag
	}
	sum := sha256.Sum256([]byte(p.ETag + "\n" + key))
	return `"` + hex.EncodeToString(sum[:])[:16] + `"`
}

// ServedWith is Served with each vacuum's link carrying "battery": the
// sensor batteries names for it, so that a Mac whose key cannot look the
// sensor up (a non-admin key cannot render templates) still reads it.
func (p *Plan) ServedWith(batteries map[string]string) ([]byte, error) {
	hints, _ := p.batteryHints(batteries)
	if len(hints) == 0 {
		return p.served, nil
	}
	dec := json.NewDecoder(bytes.NewReader(p.served))
	dec.UseNumber()
	var top map[string]any
	if err := dec.Decode(&top); err != nil {
		return nil, fmt.Errorf("decode served plan: %w", err)
	}
	vacuums, _ := top["vacuums"].([]any)
	for _, item := range vacuums {
		obj, _ := item.(map[string]any)
		link, _ := obj["link"].(map[string]any)
		if entity, ok := haLink(link); ok && hints[entity] != "" {
			link["battery"] = hints[entity]
		}
	}
	out, err := json.Marshal(top)
	if err != nil {
		return nil, fmt.Errorf("encode plan: %w", err)
	}
	return out, nil
}

// Device looks a served device up by its plan id.
func (p *Plan) Device(appID string) (Device, bool) {
	i, ok := p.byID[appID]
	if !ok {
		return Device{}, false
	}
	return p.Devices[i], true
}

// forbiddenKeys may not appear anywhere in a plan: it carries entity ids,
// never credentials.
var forbiddenKeys = map[string]bool{"token": true, "accesstoken": true, "access_token": true, "password": true, "secret": true}

// deviceLists are the plan's device arrays, with the entity domains each
// kind may be linked to.
var deviceLists = []struct {
	key     string
	kind    Kind
	domains []string
}{
	{"lights", KindLight, []string{"light.", "switch."}},
	{"acs", KindAC, []string{"climate."}},
	{"vacuums", KindVacuum, []string{"vacuum."}},
}

// ParsePlan checks an uploaded plan and keeps the devices the web view can
// drive (see Plan.Served). Errors are *PlanError.
func ParsePlan(raw []byte) (*Plan, error) {
	if len(raw) > MaxPlanBytes {
		return nil, &PlanError{"too_large"}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // numbers go back to the browser exactly as uploaded
	var top map[string]any
	if err := dec.Decode(&top); err != nil || top == nil {
		return nil, &PlanError{"not_object"}
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, &PlanError{"not_object"}
	}
	if rooms, ok := top["rooms"].([]any); !ok || len(rooms) == 0 {
		return nil, &PlanError{"no_rooms"}
	}
	if hasForbiddenKey(top) {
		return nil, &PlanError{"forbidden_key"}
	}

	sum := sha256.Sum256(raw)
	p := &Plan{ETag: `"` + hex.EncodeToString(sum[:])[:16] + `"`, byID: map[string]int{}}
	for _, list := range deviceLists {
		items, ok := top[list.key].([]any)
		if !ok {
			continue
		}
		kept := []any{}
		for _, item := range items {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			d, ok := deviceFrom(obj, list.kind, list.domains)
			if !ok {
				continue
			}
			if _, dup := p.byID[d.AppID]; dup {
				continue
			}
			if d.Kind == KindAC && d.SensorID == "" {
				delete(obj, "sensor")
			}
			p.byID[d.AppID] = len(p.Devices)
			p.Devices = append(p.Devices, d)
			kept = append(kept, obj)
		}
		top[list.key] = kept
	}
	served, err := json.Marshal(top)
	if err != nil {
		return nil, fmt.Errorf("encode plan: %w", err)
	}
	p.served = served
	return p, nil
}

// deviceFrom reads one plan device, ok only when it is linked to Home
// Assistant through a well-formed entity id (lowercase letters, digits and
// underscores around one dot) whose domain fits the kind. An AC's sensor
// link is kept under the same rule.
func deviceFrom(obj map[string]any, kind Kind, domains []string) (Device, bool) {
	id, _ := obj["id"].(string)
	if id == "" {
		return Device{}, false
	}
	entity, ok := haLink(obj["link"])
	if !ok || !safeEntityID.MatchString(entity) || !hasDomain(entity, domains) {
		return Device{}, false
	}
	d := Device{AppID: id, Kind: kind, EntityID: entity}
	switch kind {
	case KindLight:
		if caps, ok := obj["capabilities"].([]any); ok {
			for _, c := range caps {
				if s, ok := c.(string); ok {
					d.Capabilities = append(d.Capabilities, s)
				}
			}
		}
	case KindAC:
		if sensor, ok := haLink(obj["sensor"]); ok && safeEntityID.MatchString(sensor) && hasDomain(sensor, []string{"sensor."}) {
			d.SensorID = sensor
		}
		// The panel's own defaults (web/src/config.js).
		d.Min, d.Max = 16, 30
		if v, ok := num(obj, "min"); ok {
			d.Min = v
		}
		if v, ok := num(obj, "max"); ok {
			d.Max = v
		}
	}
	return d, true
}

// haLink is the entity id of a `{"service": "homeassistant", "id": …}` link.
func haLink(v any) (string, bool) {
	link, ok := v.(map[string]any)
	if !ok || link["service"] != "homeassistant" {
		return "", false
	}
	id, _ := link["id"].(string)
	return id, id != ""
}

func hasDomain(entity string, domains []string) bool {
	for _, d := range domains {
		if strings.HasPrefix(entity, d) && len(entity) > len(d) {
			return true
		}
	}
	return false
}

func hasForbiddenKey(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			if forbiddenKeys[strings.ToLower(k)] || hasForbiddenKey(child) {
				return true
			}
		}
	case []any:
		for _, child := range x {
			if hasForbiddenKey(child) {
				return true
			}
		}
	}
	return false
}

// num reads a number the way Swift's Dictionary.double does: a JSON number
// (float64 or json.Number) or a numeric string.
func num(m map[string]any, key string) (float64, bool) {
	switch v := m[key].(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	}
	return 0, false
}
