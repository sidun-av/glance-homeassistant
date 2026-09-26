package hass

import (
	"math"
	"sort"
)

type TemperatureRoom struct {
	Room      string
	EntityIDs []string
}

type Light struct {
	EntityID string
	Name     string
	On       bool
	Icon     string

	// Has* come from supported_color_modes, which is a capability list
	// present even while the light is off — that's what the popover uses to
	// decide which controls to offer. The value fields are best-effort:
	// some integrations null out brightness/color while off.
	HasBrightness bool
	Brightness    int // 0..100 percent

	HasColorTemp    bool
	ColorTempKelvin int
	MinColorTempK   int // 0 = caller should fall back to a sane default range
	MaxColorTempK   int

	HasColor bool
	RGB      [3]int
}

type SensorEntity struct {
	Room      string
	Name      string
	Attention bool
	Label     string
}

// Device is any non-light, non-sensor entity worth a tile on the map: a
// fan, a climate unit, a media player, a vacuum, ... Effect says what it
// pushes into the room while On: "fan" (cool air), "heat" (warm air),
// "music" (notes drifting up from a playing media player) or "" (nothing
// animated).
type Device struct {
	EntityID string
	Name     string
	Domain   string
	Icon     string
	On       bool
	Effect   string

	// climate/water_heater: target temperature
	HasTargetTemp bool
	CurrentTemp   float64
	TargetTemp    float64
	MinTemp       float64
	MaxTemp       float64
	TempStep      float64
	// climate only: the modes HA offers (off/heat/cool/auto/...) and the
	// current one (the entity's state)
	HvacModes []string
	HvacMode  string

	// fan only
	HasSpeed       bool
	Percentage     int     // 0..100
	PercentageStep float64 // HA's percentage_step (100/speed_count); 1 when unknown
	HasOscillate   bool
	Oscillating    bool
}

type RoomCard struct {
	Room        string
	Temperature *TemperatureRoom
	Lights      []Light
	Occupancy   []SensorEntity
	Contacts    []SensorEntity
	Devices     []Device
	Weight      int
}

type ClassificationConfig struct {
	ContactDeviceClasses []string
	MotionDeviceClasses  []string
	// DeviceDomains lists the HA domains that become Device tiles.
	// DeviceExclude drops specific entity_ids from that (a switch that is
	// really a light's second channel, a "do not disturb" toggle, ...).
	DeviceDomains []string
	DeviceExclude []string
}

// DeviceEffect decides what a device visibly pushes into the room.
// Exported so the classification is testable on its own.
func DeviceEffect(state EntityState) (on bool, effect string) {
	switch state.Domain {
	case "fan":
		return state.State == "on", "fan"
	case "climate":
		switch state.HvacAction {
		case "heating":
			return true, "heat"
		case "cooling", "fan", "drying":
			return true, "fan"
		}
		// No hvac_action reported: treat any mode other than off as "on"
		// and pick the effect from the mode itself.
		switch state.State {
		case "heat":
			return true, "heat"
		case "cool", "fan_only", "dry":
			return true, "fan"
		case "off", "unavailable", "unknown", "":
			return false, ""
		}
		return true, ""
	case "water_heater":
		return state.State != "off" && state.State != "unavailable" && state.State != "unknown", "heat"
	case "cover":
		return state.State == "open" || state.State == "opening", ""
	case "lock":
		return state.State == "unlocked", ""
	case "vacuum":
		return state.State == "cleaning" || state.State == "returning", ""
	case "media_player":
		return state.State == "playing", "music"
	}
	return state.State == "on", ""
}

// colorModeCapabilities reads a light's supported_color_modes (present even
// while the light is off) and decides which popover controls apply. Any
// mode past plain on/off implies brightness; a color mode (hs/rgb/rgbw/
// rgbww/xy) implies both brightness and the swatch palette, since HA's
// light.turn_on always accepts rgb_color regardless of which of those the
// light natively uses internally.
func colorModeCapabilities(modes []string) (hasBrightness, hasColorTemp, hasColor bool) {
	for _, m := range modes {
		switch m {
		case "brightness":
			hasBrightness = true
		case "color_temp":
			hasBrightness, hasColorTemp = true, true
		case "hs", "rgb", "rgbw", "rgbww", "xy":
			hasBrightness, hasColor = true, true
		}
	}
	return
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

type roomBuilder struct {
	temp      *TemperatureRoom
	lights    []Light
	occupancy []SensorEntity
	contacts  []SensorEntity
	devices   []Device
}

// BuildModel classifies each area's entities into a per-room card:
// temperature (sensor, device_class "temperature"), lights (domain
// "light"), occupancy and contact (binary_sensor, device_class from cfg).
// A room with none of these classified is dropped entirely — there is
// nothing for its card to show.
func BuildModel(rooms []Room, states map[string]EntityState, cfg ClassificationConfig) []RoomCard {
	byRoom := make(map[string]*roomBuilder)

	for _, room := range rooms {
		for _, entityID := range room.EntityIDs {
			state, ok := states[entityID]
			if !ok {
				continue
			}

			b, exists := byRoom[room.Name]
			if !exists {
				b = &roomBuilder{}
				byRoom[room.Name] = b
			}

			switch {
			case state.Domain == "sensor" && state.DeviceClass == "temperature":
				if b.temp == nil {
					b.temp = &TemperatureRoom{Room: room.Name}
				}
				b.temp.EntityIDs = append(b.temp.EntityIDs, entityID)

			case state.Domain == "light":
				hasBrightness, hasColorTemp, hasColor := colorModeCapabilities(state.SupportedColorModes)
				l := Light{
					EntityID:      entityID,
					Name:          state.FriendlyName,
					On:            state.State == "on",
					Icon:          state.Icon,
					HasBrightness: hasBrightness,
					HasColorTemp:  hasColorTemp,
					HasColor:      hasColor,
				}
				if state.Brightness != nil {
					l.Brightness = int(math.Round(float64(*state.Brightness) * 100 / 255))
				}
				if state.ColorTempKelvin != nil {
					l.ColorTempKelvin = *state.ColorTempKelvin
				}
				if state.MinColorTempKelvin != nil {
					l.MinColorTempK = *state.MinColorTempKelvin
				}
				if state.MaxColorTempKelvin != nil {
					l.MaxColorTempK = *state.MaxColorTempKelvin
				}
				if len(state.RGBColor) == 3 {
					l.RGB = [3]int{state.RGBColor[0], state.RGBColor[1], state.RGBColor[2]}
				}
				b.lights = append(b.lights, l)

			case state.Domain == "binary_sensor" && contains(cfg.ContactDeviceClasses, state.DeviceClass):
				if state.State != "on" && state.State != "off" {
					continue
				}
				attention := state.State == "on"
				label := "Closed"
				if attention {
					label = "Open"
				}
				b.contacts = append(b.contacts, SensorEntity{Room: room.Name, Name: state.FriendlyName, Attention: attention, Label: label})

			case state.Domain == "binary_sensor" && contains(cfg.MotionDeviceClasses, state.DeviceClass):
				if state.State != "on" && state.State != "off" {
					continue
				}
				attention := state.State == "on"
				label := "Clear"
				if attention {
					label = "Occupied"
				}
				b.occupancy = append(b.occupancy, SensorEntity{Room: room.Name, Name: state.FriendlyName, Attention: attention, Label: label})

			case contains(cfg.DeviceDomains, state.Domain) && !contains(cfg.DeviceExclude, entityID):
				if state.State == "unavailable" || state.State == "unknown" {
					continue
				}
				on, effect := DeviceEffect(state)
				d := Device{
					EntityID: entityID,
					Name:     state.FriendlyName,
					Domain:   state.Domain,
					Icon:     state.Icon,
					On:       on,
					Effect:   effect,
				}
				if (state.Domain == "climate" || state.Domain == "water_heater") && state.TargetTemperature != nil {
					d.HasTargetTemp = true
					d.TargetTemp = *state.TargetTemperature
					if state.CurrentTemperature != nil {
						d.CurrentTemp = *state.CurrentTemperature
					}
					d.MinTemp, d.MaxTemp, d.TempStep = 16, 30, 0.5
					if state.MinTemp != nil {
						d.MinTemp = *state.MinTemp
					}
					if state.MaxTemp != nil {
						d.MaxTemp = *state.MaxTemp
					}
					if state.TempStep != nil {
						d.TempStep = *state.TempStep
					}
				}
				if state.Domain == "climate" {
					d.HvacModes, d.HvacMode = state.HvacModes, state.State
				}
				if state.Domain == "fan" {
					d.HasSpeed = state.SupportedFeatures&1 != 0 || state.Percentage != nil
					if state.Percentage != nil {
						d.Percentage = int(math.Round(*state.Percentage))
					}
					d.PercentageStep = 1
					if state.PercentageStep != nil && *state.PercentageStep > 0 {
						d.PercentageStep = *state.PercentageStep
					}
					d.HasOscillate = state.SupportedFeatures&2 != 0 || state.Oscillating != nil
					d.Oscillating = state.Oscillating != nil && *state.Oscillating
				}
				b.devices = append(b.devices, d)
			}
		}
	}

	cards := make([]RoomCard, 0, len(byRoom))
	for name, b := range byRoom {
		if b.temp == nil && len(b.lights) == 0 && len(b.occupancy) == 0 && len(b.contacts) == 0 && len(b.devices) == 0 {
			continue
		}
		weight := len(b.lights) + len(b.devices)
		if b.temp != nil {
			weight += 2
		}
		if len(b.occupancy) > 0 {
			weight++
		}
		if len(b.contacts) > 0 {
			weight++
		}
		cards = append(cards, RoomCard{
			Room:        name,
			Temperature: b.temp,
			Lights:      b.lights,
			Occupancy:   b.occupancy,
			Contacts:    b.contacts,
			Devices:     b.devices,
			Weight:      weight,
		})
	}

	sort.Slice(cards, func(i, j int) bool { return cards[i].Room < cards[j].Room })
	return cards
}

// MediaPlayer is one row of the "Now playing" panel.
type MediaPlayer struct {
	EntityID          string
	Name              string
	Room              string // Area name, "" when unassigned
	State             string // playing, paused, idle, on, ...
	Title             string
	Artist            string
	Position          float64
	Duration          float64
	PositionUpdatedAt string
	Picture           string  // HA-relative entity_picture, "" if none
	Volume            float64 // 0..1, -1 unknown
}

// BuildMediaPlayers lists every media_player that is reachable (not
// unavailable/unknown/off), with its Area when it has one, sorted so
// playing ones come first and the rest by name.
func BuildMediaPlayers(rooms []Room, states map[string]EntityState) []MediaPlayer {
	roomOf := map[string]string{}
	for _, r := range rooms {
		for _, id := range r.EntityIDs {
			roomOf[id] = r.Name
		}
	}
	var out []MediaPlayer
	for id, st := range states {
		if st.Domain != "media_player" {
			continue
		}
		switch st.State {
		case "unavailable", "unknown", "off", "":
			continue
		}
		out = append(out, MediaPlayer{EntityID: id, Name: st.FriendlyName, Room: roomOf[id], State: st.State, Title: st.MediaTitle, Artist: st.MediaArtist,
			Position: st.MediaPosition, Duration: st.MediaDuration, PositionUpdatedAt: st.MediaPositionUpdatedAt, Picture: st.EntityPicture, Volume: st.VolumeLevel})
	}
	rank := func(s string) int {
		switch s {
		case "playing":
			return 0
		case "paused":
			return 1
		}
		return 2
	}
	sort.Slice(out, func(i, j int) bool {
		if rank(out[i].State) != rank(out[j].State) {
			return rank(out[i].State) < rank(out[j].State)
		}
		return out[i].Name < out[j].Name
	})
	return out
}
