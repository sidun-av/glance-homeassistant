package render

import "encoding/json"

type LiveLight struct {
	Room  string `json:"room"`
	On    int    `json:"on"`
	Total int    `json:"total"`
}

type LiveSensor struct {
	Name      string `json:"name"`
	Attention bool   `json:"attention"`
	Label     string `json:"label"`
}

type LivePayload struct {
	Lights  []LiveLight  `json:"lights"`
	Sensors []LiveSensor `json:"sensors"`
}

func RenderLive(lightRooms []LightRoomView, sensors []SensorView) ([]byte, error) {
	payload := LivePayload{
		Lights:  make([]LiveLight, len(lightRooms)),
		Sensors: make([]LiveSensor, len(sensors)),
	}
	for i, l := range lightRooms {
		payload.Lights[i] = LiveLight{Room: l.Room, On: l.On, Total: l.Total}
	}
	for i, s := range sensors {
		payload.Sensors[i] = LiveSensor{Name: s.Name, Attention: s.Attention, Label: s.Label}
	}
	return json.Marshal(payload)
}
