package render

import "encoding/json"

type LiveLight struct {
	EntityID   string `json:"entity_id"`
	On         bool   `json:"on"`
	Brightness int    `json:"brightness"` // 0..100; meaningful only when the tile's data-has-brightness is true
	ColorTemp  int    `json:"color_temp"` // Kelvin; meaningful only when data-has-color-temp is true
	RGB        []int  `json:"rgb"`        // [r,g,b]; nil when the light has no color mode
}

type LiveSensor struct {
	Name      string `json:"name"`
	Attention bool   `json:"attention"`
	Label     string `json:"label"`
}

type LiveDevice struct {
	EntityID    string  `json:"entity_id"`
	On          bool    `json:"on"`
	Effect      string  `json:"effect"`
	CurrentTemp float64 `json:"current_temp"` // climate only; meaningful when data-has-target-temp is true
	TargetTemp  float64 `json:"target_temp"`
}

type LiveRoom struct {
	Room      string       `json:"room"`
	Lights    []LiveLight  `json:"lights"`
	Occupancy []LiveSensor `json:"occupancy"`
	Contacts  []LiveSensor `json:"contacts"`
	Devices   []LiveDevice `json:"devices"`
}

type LiveMedia struct {
	EntityID   string  `json:"entity_id"`
	State      string  `json:"state"`
	StateLabel string  `json:"state_label"`
	Title      string  `json:"title"`
	Artist     string  `json:"artist"`
	Position   float64 `json:"position"`
	Duration   float64 `json:"duration"`
	PositionAt int64   `json:"position_at"`
	ArtURL     string  `json:"art_url"`
	Volume     float64 `json:"volume"`
}

type LivePayload struct {
	Rooms []LiveRoom  `json:"rooms"`
	Media []LiveMedia `json:"media"`
}

// RenderLive builds the /live.json payload from the same RoomCardView data
// used to render the widget, so live updates always match one source of
// truth. A room with no lights, occupancy, or contacts is omitted from the
// payload entirely — its card never changes between polls, so there's
// nothing to send for it.
func RenderLive(rooms []RoomCardView, media ...MediaView) ([]byte, error) {
	payload := LivePayload{Rooms: []LiveRoom{}, Media: []LiveMedia{}}
	for _, m := range media {
		payload.Media = append(payload.Media, LiveMedia{EntityID: m.EntityID, State: m.State, StateLabel: stateLabel(m.State), Title: m.Title, Artist: m.Artist,
			Position: m.Position, Duration: m.Duration, PositionAt: m.PositionAt, ArtURL: m.ArtURL, Volume: m.Volume})
	}
	for _, r := range rooms {
		if len(r.Lights) == 0 && len(r.Occupancy) == 0 && len(r.Contacts) == 0 && len(r.Devices) == 0 {
			continue
		}
		lr := LiveRoom{
			Room:      r.Room,
			Lights:    make([]LiveLight, len(r.Lights)),
			Occupancy: make([]LiveSensor, len(r.Occupancy)),
			Contacts:  make([]LiveSensor, len(r.Contacts)),
			Devices:   make([]LiveDevice, len(r.Devices)),
		}
		for i, d := range r.Devices {
			lr.Devices[i] = LiveDevice{EntityID: d.EntityID, On: d.On, Effect: d.Effect, CurrentTemp: d.CurrentTemp, TargetTemp: d.TargetTemp}
		}
		for i, l := range r.Lights {
			lv := LiveLight{EntityID: l.EntityID, On: l.On, Brightness: l.Brightness, ColorTemp: l.ColorTempKelvin}
			if l.HasColor {
				lv.RGB = []int{l.RGB[0], l.RGB[1], l.RGB[2]}
			}
			lr.Lights[i] = lv
		}
		for i, o := range r.Occupancy {
			lr.Occupancy[i] = LiveSensor{Name: o.Name, Attention: o.Attention, Label: o.Label}
		}
		for i, c := range r.Contacts {
			lr.Contacts[i] = LiveSensor{Name: c.Name, Attention: c.Attention, Label: c.Label}
		}
		payload.Rooms = append(payload.Rooms, lr)
	}
	return json.Marshal(payload)
}
