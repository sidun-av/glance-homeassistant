package hass

import (
	"strings"
	"testing"
)

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

func TestBuildModel_DevicesByDomainWithEffects(t *testing.T) {
	rooms := []Room{{Name: "Office", EntityIDs: []string{"fan.desk", "climate.ac", "switch.heater_plug", "switch.child_lock", "media_player.tv", "vacuum.bot"}}}
	states := map[string]EntityState{
		"fan.desk":           {EntityID: "fan.desk", Domain: "fan", State: "on", FriendlyName: "Desk fan"},
		"climate.ac":         {EntityID: "climate.ac", Domain: "climate", State: "heat", HvacAction: "heating", FriendlyName: "AC", Icon: "mdi:air-conditioner"},
		"switch.heater_plug": {EntityID: "switch.heater_plug", Domain: "switch", State: "off", FriendlyName: "Heater plug"},
		"switch.child_lock":  {EntityID: "switch.child_lock", Domain: "switch", State: "on", FriendlyName: "Child lock"},
		"media_player.tv":    {EntityID: "media_player.tv", Domain: "media_player", State: "playing", FriendlyName: "TV"},
		"vacuum.bot":         {EntityID: "vacuum.bot", Domain: "vacuum", State: "unavailable", FriendlyName: "Bot"},
	}
	cfg := defaultClassificationConfig()
	cfg.DeviceDomains = []string{"fan", "climate", "switch", "media_player", "vacuum"}
	cfg.DeviceExclude = []string{"switch.child_lock"}

	cards := BuildModel(rooms, states, cfg)
	card, ok := findCard(cards, "Office")
	if !ok {
		t.Fatal("Office card missing — devices alone must keep a room")
	}
	want := map[string][2]interface{}{
		"fan.desk":           {true, "fan"},
		"climate.ac":         {true, "heat"},
		"switch.heater_plug": {false, ""},
		"media_player.tv":    {true, "music"},
	}
	if len(card.Devices) != len(want) {
		t.Fatalf("devices = %+v, want %d (child_lock excluded, unavailable vacuum skipped)", card.Devices, len(want))
	}
	for _, d := range card.Devices {
		w, ok := want[d.EntityID]
		if !ok {
			t.Errorf("unexpected device %s", d.EntityID)
			continue
		}
		if d.On != w[0].(bool) || d.Effect != w[1].(string) {
			t.Errorf("%s: on=%v effect=%q, want on=%v effect=%q", d.EntityID, d.On, d.Effect, w[0], w[1])
		}
	}
	if card.Weight != 4 {
		t.Errorf("weight = %d, want 4 (one per device)", card.Weight)
	}
}

func TestDeviceEffect_ClimateWithoutHvacActionUsesMode(t *testing.T) {
	cases := []struct {
		state, want string
		on          bool
	}{{"cool", "fan", true}, {"heat", "heat", true}, {"off", "", false}, {"auto", "", true}}
	for _, c := range cases {
		on, eff := DeviceEffect(EntityState{Domain: "climate", State: c.state})
		if on != c.on || eff != c.want {
			t.Errorf("climate %q: on=%v effect=%q, want on=%v effect=%q", c.state, on, eff, c.on, c.want)
		}
	}
}

func TestBuildMediaPlayers_CarriesVolume(t *testing.T) {
	states := map[string]EntityState{"media_player.a": {EntityID: "media_player.a", Domain: "media_player", State: "playing", VolumeLevel: 0.4}}
	if got := BuildMediaPlayers(nil, states); len(got) != 1 || got[0].Volume != 0.4 {
		t.Errorf("got %+v", got)
	}
}

func TestBuildMediaPlayers(t *testing.T) {
	rooms := []Room{{Name: "Kitchen", EntityIDs: []string{"media_player.kitchen"}}}
	states := map[string]EntityState{
		"media_player.kitchen": {EntityID: "media_player.kitchen", Domain: "media_player", State: "idle", FriendlyName: "Kitchen"},
		"media_player.bed":     {EntityID: "media_player.bed", Domain: "media_player", State: "playing", FriendlyName: "Bed", MediaTitle: "Song", MediaArtist: "Band"},
		"media_player.tv":      {EntityID: "media_player.tv", Domain: "media_player", State: "unavailable", FriendlyName: "TV"},
		"light.x":              {EntityID: "light.x", Domain: "light", State: "on"},
	}
	got := BuildMediaPlayers(rooms, states)
	if len(got) != 2 || got[0].EntityID != "media_player.bed" || got[0].Title != "Song" || got[1].Room != "Kitchen" {
		t.Errorf("got %+v", got)
	}
}

func TestBuildModel_FanAndClimateControls(t *testing.T) {
	pct, step, osc := 66.4, 33.33, true
	tgt, cur := 21.5, 19.0
	rooms := []Room{{Name: "R", EntityIDs: []string{"fan.f", "fan.plain", "climate.c", "water_heater.w"}}}
	states := map[string]EntityState{
		"fan.f":          {EntityID: "fan.f", Domain: "fan", State: "on", SupportedFeatures: 3, Percentage: &pct, PercentageStep: &step, Oscillating: &osc},
		"fan.plain":      {EntityID: "fan.plain", Domain: "fan", State: "off"},
		"climate.c":      {EntityID: "climate.c", Domain: "climate", State: "heat", TargetTemperature: &tgt, CurrentTemperature: &cur, HvacModes: []string{"off", "heat"}},
		"water_heater.w": {EntityID: "water_heater.w", Domain: "water_heater", State: "eco", TargetTemperature: &tgt},
	}
	cfg := defaultClassificationConfig()
	cfg.DeviceDomains = []string{"fan", "climate", "water_heater"}
	cards := BuildModel(rooms, states, cfg)
	if len(cards) != 1 {
		t.Fatalf("cards = %d", len(cards))
	}
	by := map[string]Device{}
	for _, d := range cards[0].Devices {
		by[d.EntityID] = d
	}
	if f := by["fan.f"]; !f.HasSpeed || f.Percentage != 66 || f.PercentageStep != 33.33 || !f.HasOscillate || !f.Oscillating {
		t.Errorf("fan.f = %+v", f)
	}
	if f := by["fan.plain"]; f.HasSpeed || f.HasOscillate || f.PercentageStep != 1 {
		t.Errorf("fan.plain = %+v, want no speed/oscillate controls", f)
	}
	if c := by["climate.c"]; !c.HasTargetTemp || c.TargetTemp != 21.5 || c.HvacMode != "heat" || len(c.HvacModes) != 2 {
		t.Errorf("climate.c = %+v", c)
	}
	if w := by["water_heater.w"]; !w.HasTargetTemp || w.TargetTemp != 21.5 || len(w.HvacModes) != 0 {
		t.Errorf("water_heater.w = %+v", w)
	}
}

func TestDeviceExtras_SiblingsOfTheSameDevice(t *testing.T) {
	min0, max480, max100 := 0.0, 480.0, 100.0
	room := Room{Name: "K", Devices: []DeviceGroup{{Name: "Fan 2", EntityIDs: []string{"fan.f", "select.angle", "number.timer", "number.speed", "switch.lock", "button.x", "switch.gone"}}}}
	states := map[string]EntityState{
		"fan.f":        {Domain: "fan", State: "on"},
		"select.angle": {Domain: "select", State: "140", FriendlyName: "Fan 2 Horizontal Angle", Options: []string{"30", "140"}},
		"number.timer": {Domain: "number", State: "0", FriendlyName: "Fan 2 Power Off Delay Time", Min: &min0, Max: &max480, Unit: "minutes"},
		"number.speed": {Domain: "number", State: "42", FriendlyName: "Fan 2 Motor Control", Min: &min0, Max: &max100},
		"switch.lock":  {Domain: "switch", State: "off", FriendlyName: "Fan 2 Physical Control Locked"},
		"button.x":     {Domain: "button", State: "unknown"},
		"switch.gone":  {Domain: "switch", State: "unavailable"},
	}
	got := deviceExtras("fan.f", room, states, nil)
	var names []string
	for _, e := range got {
		names = append(names, e.Domain+":"+e.Name)
	}
	want := "select:Swing angle,number:Off timer,switch:Child lock"
	if strings.Join(names, ",") != want {
		t.Errorf("extras = %v, want %s (fan speed number, buttons and unavailable ones dropped)", names, want)
	}
}
