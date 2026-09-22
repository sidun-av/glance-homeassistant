package hass

import "sort"

type TemperatureRoom struct {
	Room      string
	EntityIDs []string
}

type Light struct {
	EntityID string
	Name     string
	On       bool
	Icon     string
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
				b.lights = append(b.lights, Light{
					EntityID: entityID,
					Name:     state.FriendlyName,
					On:       state.State == "on",
					Icon:     state.Icon,
				})

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
				b.devices = append(b.devices, Device{
					EntityID: entityID,
					Name:     state.FriendlyName,
					Domain:   state.Domain,
					Icon:     state.Icon,
					On:       on,
					Effect:   effect,
				})
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
	EntityID string
	Name     string
	Room     string // Area name, "" when unassigned
	State    string // playing, paused, idle, on, ...
	Title    string
	Artist   string
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
		out = append(out, MediaPlayer{EntityID: id, Name: st.FriendlyName, Room: roomOf[id], State: st.State, Title: st.MediaTitle, Artist: st.MediaArtist})
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
