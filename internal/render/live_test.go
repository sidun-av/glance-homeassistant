package render

import (
	"encoding/json"
	"testing"
)

func TestRenderLive_MarshalsRoomsWithLightsAndSensors(t *testing.T) {
	rooms := []RoomCardView{
		{
			Room:      "Bedroom",
			Lights:    []LightView{{EntityID: "light.bed_1", On: false}, {EntityID: "light.bed_2", On: true}},
			Occupancy: []SensorBadgeView{{Name: "Bed Motion", Attention: true, Label: "Occupied"}},
		},
	}
	body, err := RenderLive(rooms)
	if err != nil {
		t.Fatalf("RenderLive: %v", err)
	}

	var parsed struct {
		Rooms []struct {
			Room   string `json:"room"`
			Lights []struct {
				EntityID string `json:"entity_id"`
				On       bool   `json:"on"`
			} `json:"lights"`
			Occupancy []struct {
				Name      string `json:"name"`
				Attention bool   `json:"attention"`
				Label     string `json:"label"`
			} `json:"occupancy"`
		} `json:"rooms"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(parsed.Rooms) != 1 || parsed.Rooms[0].Room != "Bedroom" {
		t.Fatalf("Rooms = %+v", parsed.Rooms)
	}
	if len(parsed.Rooms[0].Lights) != 2 || parsed.Rooms[0].Lights[1].EntityID != "light.bed_2" || !parsed.Rooms[0].Lights[1].On {
		t.Errorf("Lights = %+v", parsed.Rooms[0].Lights)
	}
	if len(parsed.Rooms[0].Occupancy) != 1 || !parsed.Rooms[0].Occupancy[0].Attention || parsed.Rooms[0].Occupancy[0].Label != "Occupied" {
		t.Errorf("Occupancy = %+v", parsed.Rooms[0].Occupancy)
	}
}

func TestRenderLive_OmitsTemperatureOnlyRoom(t *testing.T) {
	rooms := []RoomCardView{
		{Room: "Kitchen", HasTemperature: true, TempValue: "25.0°", ChartSVG: "<svg></svg>"},
	}
	body, err := RenderLive(rooms)
	if err != nil {
		t.Fatalf("RenderLive: %v", err)
	}
	if contains(string(body), "Kitchen") {
		t.Errorf("body = %s, want temperature-only room omitted from live payload", body)
	}
}

func TestRenderLive_EmptyInputProducesEmptyRoomsArrayNotNull(t *testing.T) {
	body, err := RenderLive(nil)
	if err != nil {
		t.Fatalf("RenderLive: %v", err)
	}
	if !contains(string(body), `"rooms":[]`) {
		t.Errorf("body = %s, want \"rooms\":[] not null", body)
	}
}

func TestRenderLive_RoomWithOnlyLightsOmitsNullOccupancyAndContacts(t *testing.T) {
	rooms := []RoomCardView{
		{Room: "Hallway", Lights: []LightView{{EntityID: "light.hall", On: true}}},
	}
	body, err := RenderLive(rooms)
	if err != nil {
		t.Fatalf("RenderLive: %v", err)
	}
	if !contains(string(body), `"occupancy":[]`) || !contains(string(body), `"contacts":[]`) {
		t.Errorf("body = %s, want empty (not null) occupancy/contacts arrays", body)
	}
}
