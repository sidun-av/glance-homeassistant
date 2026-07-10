package hass

import "testing"

func defaultClassificationConfig() ClassificationConfig {
	return ClassificationConfig{
		ContactDeviceClasses: []string{"door", "window", "garage_door", "opening"},
		MotionDeviceClasses:  []string{"motion", "occupancy"},
	}
}

func TestBuildModel_ClassifiesByDomainAndDeviceClass(t *testing.T) {
	rooms := []Room{
		{Name: "Living Room", EntityIDs: []string{"sensor.lr_temp", "light.lr_main", "binary_sensor.lr_window"}},
		{Name: "Bedroom", EntityIDs: []string{"light.bed_main", "binary_sensor.bed_motion"}},
	}
	states := map[string]EntityState{
		"sensor.lr_temp":         {EntityID: "sensor.lr_temp", Domain: "sensor", State: "21.4", DeviceClass: "temperature", FriendlyName: "LR Temp"},
		"light.lr_main":          {EntityID: "light.lr_main", Domain: "light", State: "on", FriendlyName: "LR Main"},
		"binary_sensor.lr_window": {EntityID: "binary_sensor.lr_window", Domain: "binary_sensor", State: "on", DeviceClass: "window", FriendlyName: "LR Window"},
		"light.bed_main":         {EntityID: "light.bed_main", Domain: "light", State: "off", FriendlyName: "Bed Main"},
		"binary_sensor.bed_motion": {EntityID: "binary_sensor.bed_motion", Domain: "binary_sensor", State: "off", DeviceClass: "motion", FriendlyName: "Bed Motion"},
	}

	model := BuildModel(rooms, states, defaultClassificationConfig())

	if len(model.TemperatureRooms) != 1 || model.TemperatureRooms[0].Room != "Living Room" {
		t.Fatalf("TemperatureRooms = %+v", model.TemperatureRooms)
	}
	if len(model.TemperatureRooms[0].EntityIDs) != 1 || model.TemperatureRooms[0].EntityIDs[0] != "sensor.lr_temp" {
		t.Errorf("TemperatureRooms[0].EntityIDs = %v", model.TemperatureRooms[0].EntityIDs)
	}

	if len(model.LightRooms) != 2 {
		t.Fatalf("LightRooms = %+v", model.LightRooms)
	}
	byRoom := map[string]LightRoom{}
	for _, lr := range model.LightRooms {
		byRoom[lr.Room] = lr
	}
	if byRoom["Living Room"].On != 1 || byRoom["Living Room"].Total != 1 {
		t.Errorf("Living Room lights = %+v, want On=1 Total=1", byRoom["Living Room"])
	}
	if byRoom["Bedroom"].On != 0 || byRoom["Bedroom"].Total != 1 {
		t.Errorf("Bedroom lights = %+v, want On=0 Total=1", byRoom["Bedroom"])
	}

	if len(model.Sensors) != 2 {
		t.Fatalf("Sensors = %+v", model.Sensors)
	}
	byName := map[string]SensorEntity{}
	for _, s := range model.Sensors {
		byName[s.Name] = s
	}
	if !byName["LR Window"].Attention || byName["LR Window"].Label != "Open" {
		t.Errorf("LR Window sensor = %+v, want Attention=true Label=Open", byName["LR Window"])
	}
	if byName["Bed Motion"].Attention || byName["Bed Motion"].Label != "Clear" {
		t.Errorf("Bed Motion sensor = %+v, want Attention=false Label=Clear", byName["Bed Motion"])
	}
}

func TestBuildModel_RoomWithoutMatchingEntitiesIsOmitted(t *testing.T) {
	rooms := []Room{
		{Name: "Garage", EntityIDs: []string{"switch.garage_opener"}},
	}
	states := map[string]EntityState{
		"switch.garage_opener": {EntityID: "switch.garage_opener", Domain: "switch", State: "off", FriendlyName: "Garage Opener"},
	}

	model := BuildModel(rooms, states, defaultClassificationConfig())

	if len(model.TemperatureRooms) != 0 || len(model.LightRooms) != 0 || len(model.Sensors) != 0 {
		t.Errorf("model = %+v, want all sections empty for a room with only an unclassified switch entity", model)
	}
}

func TestBuildModel_MultipleTemperatureSensorsInOneRoom(t *testing.T) {
	rooms := []Room{
		{Name: "Living Room", EntityIDs: []string{"sensor.lr_temp_1", "sensor.lr_temp_2"}},
	}
	states := map[string]EntityState{
		"sensor.lr_temp_1": {EntityID: "sensor.lr_temp_1", Domain: "sensor", State: "21.0", DeviceClass: "temperature", FriendlyName: "LR Temp 1"},
		"sensor.lr_temp_2": {EntityID: "sensor.lr_temp_2", Domain: "sensor", State: "22.0", DeviceClass: "temperature", FriendlyName: "LR Temp 2"},
	}

	model := BuildModel(rooms, states, defaultClassificationConfig())

	if len(model.TemperatureRooms) != 1 {
		t.Fatalf("TemperatureRooms = %+v", model.TemperatureRooms)
	}
	if len(model.TemperatureRooms[0].EntityIDs) != 2 {
		t.Errorf("EntityIDs = %v, want both sensors collected under one room", model.TemperatureRooms[0].EntityIDs)
	}
}

func TestBuildModel_SkipsUnavailableBinarySensor(t *testing.T) {
	rooms := []Room{
		{Name: "Hallway", EntityIDs: []string{"binary_sensor.hall_motion"}},
	}
	states := map[string]EntityState{
		"binary_sensor.hall_motion": {EntityID: "binary_sensor.hall_motion", Domain: "binary_sensor", State: "unavailable", DeviceClass: "motion", FriendlyName: "Hall Motion"},
	}

	model := BuildModel(rooms, states, defaultClassificationConfig())

	if len(model.Sensors) != 0 {
		t.Errorf("Sensors = %+v, want unavailable sensor excluded", model.Sensors)
	}
}

func TestBuildModel_MissingStateForEntityIsSkipped(t *testing.T) {
	rooms := []Room{
		{Name: "Office", EntityIDs: []string{"light.office_main"}},
	}
	states := map[string]EntityState{} // entity not present in states at all

	model := BuildModel(rooms, states, defaultClassificationConfig())

	if len(model.LightRooms) != 0 {
		t.Errorf("LightRooms = %+v, want none (entity missing from states map)", model.LightRooms)
	}
}

func TestBuildModel_SortsAlphabetically(t *testing.T) {
	rooms := []Room{
		{Name: "Zeta Room", EntityIDs: []string{"light.zeta"}},
		{Name: "Alpha Room", EntityIDs: []string{"light.alpha"}},
	}
	states := map[string]EntityState{
		"light.zeta":  {EntityID: "light.zeta", Domain: "light", State: "on", FriendlyName: "Zeta Light"},
		"light.alpha": {EntityID: "light.alpha", Domain: "light", State: "on", FriendlyName: "Alpha Light"},
	}

	model := BuildModel(rooms, states, defaultClassificationConfig())

	if len(model.LightRooms) != 2 || model.LightRooms[0].Room != "Alpha Room" || model.LightRooms[1].Room != "Zeta Room" {
		t.Errorf("LightRooms = %+v, want alphabetical order", model.LightRooms)
	}
}
