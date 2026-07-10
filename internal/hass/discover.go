package hass

import "sort"

type Model struct {
	TemperatureRooms []TemperatureRoom
	LightRooms       []LightRoom
	Sensors          []SensorEntity
}

type TemperatureRoom struct {
	Room      string
	EntityIDs []string
}

type LightRoom struct {
	Room  string
	On    int
	Total int
}

type SensorEntity struct {
	Name      string
	Attention bool
	Label     string
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

func BuildModel(rooms []Room, states map[string]EntityState, cfg ClassificationConfig) Model {
	tempByRoom := make(map[string]*TemperatureRoom)
	lightByRoom := make(map[string]*LightRoom)
	var sensors []SensorEntity

	for _, room := range rooms {
		for _, entityID := range room.EntityIDs {
			state, ok := states[entityID]
			if !ok {
				continue
			}

			switch {
			case state.Domain == "sensor" && state.DeviceClass == "temperature":
				tr, exists := tempByRoom[room.Name]
				if !exists {
					tr = &TemperatureRoom{Room: room.Name}
					tempByRoom[room.Name] = tr
				}
				tr.EntityIDs = append(tr.EntityIDs, entityID)

			case state.Domain == "light":
				lr, exists := lightByRoom[room.Name]
				if !exists {
					lr = &LightRoom{Room: room.Name}
					lightByRoom[room.Name] = lr
				}
				lr.Total++
				if state.State == "on" {
					lr.On++
				}

			case state.Domain == "binary_sensor" && contains(cfg.ContactDeviceClasses, state.DeviceClass):
				if state.State != "on" && state.State != "off" {
					continue
				}
				attention := state.State == "on"
				label := "Closed"
				if attention {
					label = "Open"
				}
				sensors = append(sensors, SensorEntity{Name: state.FriendlyName, Attention: attention, Label: label})

			case state.Domain == "binary_sensor" && contains(cfg.MotionDeviceClasses, state.DeviceClass):
				if state.State != "on" && state.State != "off" {
					continue
				}
				attention := state.State == "on"
				label := "Clear"
				if attention {
					label = "Motion"
				}
				sensors = append(sensors, SensorEntity{Name: state.FriendlyName, Attention: attention, Label: label})
			}
		}
	}

	model := Model{}
	for _, tr := range tempByRoom {
		model.TemperatureRooms = append(model.TemperatureRooms, *tr)
	}
	for _, lr := range lightByRoom {
		model.LightRooms = append(model.LightRooms, *lr)
	}
	model.Sensors = sensors

	sort.Slice(model.TemperatureRooms, func(i, j int) bool { return model.TemperatureRooms[i].Room < model.TemperatureRooms[j].Room })
	sort.Slice(model.LightRooms, func(i, j int) bool { return model.LightRooms[i].Room < model.LightRooms[j].Room })
	sort.Slice(model.Sensors, func(i, j int) bool { return model.Sensors[i].Name < model.Sensors[j].Name })

	return model
}
