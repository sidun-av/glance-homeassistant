package hass

import "testing"

func defaultClassificationConfig() ClassificationConfig {
	return ClassificationConfig{
		ContactDeviceClasses: []string{"door", "window", "garage_door", "opening"},
		MotionDeviceClasses:  []string{"motion", "occupancy"},
	}
}

func findCard(cards []RoomCard, room string) (RoomCard, bool) {
	for _, c := range cards {
		if c.Room == room {
			return c, true
		}
	}
	return RoomCard{}, false
}

func TestBuildModel_ClassifiesByDomainAndDeviceClass(t *testing.T) {
	rooms := []Room{
		{Name: "Living Room", EntityIDs: []string{"sensor.lr_temp", "light.lr_main", "binary_sensor.lr_window"}},
		{Name: "Bedroom", EntityIDs: []string{"light.bed_main", "binary_sensor.bed_motion"}},
	}
	states := map[string]EntityState{
		"sensor.lr_temp":           {EntityID: "sensor.lr_temp", Domain: "sensor", State: "21.4", DeviceClass: "temperature", FriendlyName: "LR Temp"},
		"light.lr_main":            {EntityID: "light.lr_main", Domain: "light", State: "on", FriendlyName: "LR Main", Icon: "mdi:track-light"},
		"binary_sensor.lr_window":  {EntityID: "binary_sensor.lr_window", Domain: "binary_sensor", State: "on", DeviceClass: "window", FriendlyName: "LR Window"},
		"light.bed_main":           {EntityID: "light.bed_main", Domain: "light", State: "off", FriendlyName: "Bed Main"},
		"binary_sensor.bed_motion": {EntityID: "binary_sensor.bed_motion", Domain: "binary_sensor", State: "off", DeviceClass: "motion", FriendlyName: "Bed Motion"},
	}

	cards := BuildModel(rooms, states, defaultClassificationConfig())
	if len(cards) != 2 {
		t.Fatalf("len(cards) = %d, want 2", len(cards))
	}

	lr, ok := findCard(cards, "Living Room")
	if !ok {
		t.Fatalf("Living Room card missing")
	}
	if lr.Temperature == nil || len(lr.Temperature.EntityIDs) != 1 || lr.Temperature.EntityIDs[0] != "sensor.lr_temp" {
		t.Errorf("Living Room.Temperature = %+v", lr.Temperature)
	}
	if len(lr.Lights) != 1 || !lr.Lights[0].On || lr.Lights[0].Icon != "mdi:track-light" || lr.Lights[0].EntityID != "light.lr_main" {
		t.Errorf("Living Room.Lights = %+v", lr.Lights)
	}
	if len(lr.Contacts) != 1 || lr.Contacts[0].Room != "Living Room" || !lr.Contacts[0].Attention || lr.Contacts[0].Label != "Open" {
		t.Errorf("Living Room.Contacts = %+v", lr.Contacts)
	}
	if len(lr.Occupancy) != 0 {
		t.Errorf("Living Room.Occupancy = %+v, want none", lr.Occupancy)
	}
	if lr.Weight != 4 { // temp(2) + 1 light + 0 occupancy + 1 contact
		t.Errorf("Living Room.Weight = %d, want 4", lr.Weight)
	}

	bed, ok := findCard(cards, "Bedroom")
	if !ok {
		t.Fatalf("Bedroom card missing")
	}
	if bed.Temperature != nil {
		t.Errorf("Bedroom.Temperature = %+v, want nil", bed.Temperature)
	}
	if len(bed.Lights) != 1 || bed.Lights[0].On {
		t.Errorf("Bedroom.Lights = %+v", bed.Lights)
	}
	if len(bed.Occupancy) != 1 || bed.Occupancy[0].Attention || bed.Occupancy[0].Label != "Clear" {
		t.Errorf("Bedroom.Occupancy = %+v", bed.Occupancy)
	}
	if bed.Weight != 2 { // 0 temp + 1 light + 1 occupancy + 0 contact
		t.Errorf("Bedroom.Weight = %d, want 2", bed.Weight)
	}
}

func TestBuildModel_RoomWithoutMatchingEntitiesIsOmitted(t *testing.T) {
	rooms := []Room{
		{Name: "Garage", EntityIDs: []string{"switch.garage_opener"}},
	}
	states := map[string]EntityState{
		"switch.garage_opener": {EntityID: "switch.garage_opener", Domain: "switch", State: "off", FriendlyName: "Garage Opener"},
	}

	cards := BuildModel(rooms, states, defaultClassificationConfig())
	if len(cards) != 0 {
		t.Errorf("cards = %+v, want none for a room with only an unclassified switch entity", cards)
	}
}

func TestBuildModel_RoomWithNoEntitiesAtAllIsOmitted(t *testing.T) {
	rooms := []Room{
		{Name: "Bathroom", EntityIDs: nil},
	}
	cards := BuildModel(rooms, map[string]EntityState{}, defaultClassificationConfig())
	if len(cards) != 0 {
		t.Errorf("cards = %+v, want none for an area with zero entities", cards)
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

	cards := BuildModel(rooms, states, defaultClassificationConfig())
	lr, ok := findCard(cards, "Living Room")
	if !ok {
		t.Fatalf("Living Room card missing")
	}
	if lr.Temperature == nil || len(lr.Temperature.EntityIDs) != 2 {
		t.Errorf("Living Room.Temperature = %+v, want both sensors collected", lr.Temperature)
	}
}

func TestBuildModel_SkipsUnavailableBinarySensor(t *testing.T) {
	rooms := []Room{
		{Name: "Hallway", EntityIDs: []string{"binary_sensor.hall_motion"}},
	}
	states := map[string]EntityState{
		"binary_sensor.hall_motion": {EntityID: "binary_sensor.hall_motion", Domain: "binary_sensor", State: "unavailable", DeviceClass: "motion", FriendlyName: "Hall Motion"},
	}

	cards := BuildModel(rooms, states, defaultClassificationConfig())
	if len(cards) != 0 {
		t.Errorf("cards = %+v, want none (only entity is an unavailable motion sensor)", cards)
	}
}

func TestBuildModel_MissingStateForEntityIsSkipped(t *testing.T) {
	rooms := []Room{
		{Name: "Office", EntityIDs: []string{"light.office_main"}},
	}
	states := map[string]EntityState{}

	cards := BuildModel(rooms, states, defaultClassificationConfig())
	if len(cards) != 0 {
		t.Errorf("cards = %+v, want none (entity missing from states map)", cards)
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

	cards := BuildModel(rooms, states, defaultClassificationConfig())
	if len(cards) != 2 || cards[0].Room != "Alpha Room" || cards[1].Room != "Zeta Room" {
		t.Errorf("cards = %+v, want alphabetical order", cards)
	}
}

func TestBuildModel_WeightCombinesAllSignals(t *testing.T) {
	rooms := []Room{
		{Name: "Living Room", EntityIDs: []string{
			"sensor.lr_temp", "light.lr_1", "light.lr_2", "light.lr_3",
			"binary_sensor.lr_motion", "binary_sensor.lr_window",
		}},
	}
	states := map[string]EntityState{
		"sensor.lr_temp":          {EntityID: "sensor.lr_temp", Domain: "sensor", State: "21.0", DeviceClass: "temperature", FriendlyName: "LR Temp"},
		"light.lr_1":              {EntityID: "light.lr_1", Domain: "light", State: "on", FriendlyName: "LR 1"},
		"light.lr_2":              {EntityID: "light.lr_2", Domain: "light", State: "on", FriendlyName: "LR 2"},
		"light.lr_3":              {EntityID: "light.lr_3", Domain: "light", State: "off", FriendlyName: "LR 3"},
		"binary_sensor.lr_motion": {EntityID: "binary_sensor.lr_motion", Domain: "binary_sensor", State: "on", DeviceClass: "occupancy", FriendlyName: "LR Motion"},
		"binary_sensor.lr_window": {EntityID: "binary_sensor.lr_window", Domain: "binary_sensor", State: "off", DeviceClass: "window", FriendlyName: "LR Window"},
	}

	cards := BuildModel(rooms, states, defaultClassificationConfig())
	lr, ok := findCard(cards, "Living Room")
	if !ok {
		t.Fatalf("Living Room card missing")
	}
	if lr.Weight != 7 { // temp(2) + 3 lights + occupancy(1) + contact(1)
		t.Errorf("Weight = %d, want 7", lr.Weight)
	}
}

func TestBuildModel_OccupancyAttentionLabel(t *testing.T) {
	rooms := []Room{
		{Name: "Hallway", EntityIDs: []string{"binary_sensor.hall_occupancy"}},
	}
	states := map[string]EntityState{
		"binary_sensor.hall_occupancy": {EntityID: "binary_sensor.hall_occupancy", Domain: "binary_sensor", State: "on", DeviceClass: "occupancy", FriendlyName: "Hall Occupancy"},
	}

	cards := BuildModel(rooms, states, defaultClassificationConfig())
	hall, ok := findCard(cards, "Hallway")
	if !ok {
		t.Fatalf("Hallway card missing")
	}
	if len(hall.Occupancy) != 1 || !hall.Occupancy[0].Attention || hall.Occupancy[0].Label != "Occupied" {
		t.Errorf("Hallway.Occupancy = %+v, want Attention=true Label=Occupied", hall.Occupancy)
	}
}
