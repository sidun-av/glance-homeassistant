package render

import "encoding/json"

type LiveLight struct {
	EntityID string `json:"entity_id"`
	On       bool   `json:"on"`
}

type LiveSensor struct {
	Name      string `json:"name"`
	Attention bool   `json:"attention"`
	Label     string `json:"label"`
}

type LiveRoom struct {
	Room      string       `json:"room"`
	Lights    []LiveLight  `json:"lights"`
	Occupancy []LiveSensor `json:"occupancy"`
	Contacts  []LiveSensor `json:"contacts"`
}

type LivePayload struct {
	Rooms []LiveRoom `json:"rooms"`
}

// RenderLive builds the /live.json payload from the same RoomCardView data
// used to render the widget, so live updates always match one source of
// truth. A room with no lights, occupancy, or contacts is omitted from the
// payload entirely — its card never changes between polls, so there's
// nothing to send for it.
func RenderLive(rooms []RoomCardView) ([]byte, error) {
	payload := LivePayload{Rooms: []LiveRoom{}}
	for _, r := range rooms {
		if len(r.Lights) == 0 && len(r.Occupancy) == 0 && len(r.Contacts) == 0 {
			continue
		}
		lr := LiveRoom{
			Room:      r.Room,
			Lights:    make([]LiveLight, len(r.Lights)),
			Occupancy: make([]LiveSensor, len(r.Occupancy)),
			Contacts:  make([]LiveSensor, len(r.Contacts)),
		}
		for i, l := range r.Lights {
			lr.Lights[i] = LiveLight{EntityID: l.EntityID, On: l.On}
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
