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

type RoomCard struct {
	Room        string
	Temperature *TemperatureRoom
	Lights      []Light
	Occupancy   []SensorEntity
	Contacts    []SensorEntity
	Weight      int
}

type ClassificationConfig struct {
	ContactDeviceClasses []string
	MotionDeviceClasses  []string
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
			}
		}
	}

	cards := make([]RoomCard, 0, len(byRoom))
	for name, b := range byRoom {
		if b.temp == nil && len(b.lights) == 0 && len(b.occupancy) == 0 && len(b.contacts) == 0 {
			continue
		}
		weight := len(b.lights)
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
			Weight:      weight,
		})
	}

	sort.Slice(cards, func(i, j int) bool { return cards[i].Room < cards[j].Room })
	return cards
}
