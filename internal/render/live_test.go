package render

import (
	"encoding/json"
	"testing"
)

func TestRenderLive_MarshalsLightsAndSensors(t *testing.T) {
	body, err := RenderLive(
		[]LightRoomView{{Room: "Living Room", On: 2, Total: 3}},
		[]SensorView{{Name: "Front Door", Attention: false, Label: "Closed"}},
	)
	if err != nil {
		t.Fatalf("RenderLive: %v", err)
	}

	var parsed struct {
		Lights []struct {
			Room  string `json:"room"`
			On    int    `json:"on"`
			Total int    `json:"total"`
		} `json:"lights"`
		Sensors []struct {
			Name      string `json:"name"`
			Attention bool   `json:"attention"`
			Label     string `json:"label"`
		} `json:"sensors"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(parsed.Lights) != 1 || parsed.Lights[0].Room != "Living Room" || parsed.Lights[0].On != 2 || parsed.Lights[0].Total != 3 {
		t.Errorf("Lights = %+v", parsed.Lights)
	}
	if len(parsed.Sensors) != 1 || parsed.Sensors[0].Name != "Front Door" || parsed.Sensors[0].Attention != false || parsed.Sensors[0].Label != "Closed" {
		t.Errorf("Sensors = %+v", parsed.Sensors)
	}
}

func TestRenderLive_EmptyInputProducesEmptyArraysNotNull(t *testing.T) {
	body, err := RenderLive(nil, nil)
	if err != nil {
		t.Fatalf("RenderLive: %v", err)
	}
	if !contains(string(body), `"lights":[]`) {
		t.Errorf("body = %s, want \"lights\":[] not null", body)
	}
	if !contains(string(body), `"sensors":[]`) {
		t.Errorf("body = %s, want \"sensors\":[] not null", body)
	}
}
