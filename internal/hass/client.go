package hass

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	HTTPClient *http.Client
	BaseURL    string
	Token      string
}

func New(baseURL, token string) *Client {
	return &Client{
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Token:      token,
	}
}

type Room struct {
	ID        string
	Name      string
	EntityIDs []string
}

// areasTemplate renders every HA area to a JSON array via the Jinja functions
// areas()/area_name()/area_entities() — area_entities() already resolves
// entities assigned to an area either directly or through a device, matching
// how HA's own Areas UI groups things.
const areasTemplate = `{% set ns = namespace(areas=[]) %}` +
	`{% for a in areas() %}` +
	`{% set ns.areas = ns.areas + [{'id': a, 'name': area_name(a), 'entities': area_entities(a)}] %}` +
	`{% endfor %}` +
	`{{ ns.areas | tojson }}`

type templateRequest struct {
	Template string `json:"template"`
}

func (c *Client) FetchAreas(ctx context.Context) ([]Room, error) {
	payload, err := json.Marshal(templateRequest{Template: areasTemplate})
	if err != nil {
		return nil, fmt.Errorf("marshal template request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/template", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request areas: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("areas template returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read areas response: %w", err)
	}

	type rawArea struct {
		ID       string   `json:"id"`
		Name     string   `json:"name"`
		Entities []string `json:"entities"`
	}
	var rawAreas []rawArea
	if err := json.Unmarshal(bytes.TrimSpace(body), &rawAreas); err != nil {
		return nil, fmt.Errorf("parse areas response: %w", err)
	}

	rooms := make([]Room, len(rawAreas))
	for i, a := range rawAreas {
		rooms[i] = Room{ID: a.ID, Name: a.Name, EntityIDs: a.Entities}
	}
	return rooms, nil
}

type EntityState struct {
	EntityID     string
	Domain       string
	State        string
	FriendlyName string
	DeviceClass  string
}

func (c *Client) FetchStates(ctx context.Context) (map[string]EntityState, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/states", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request states: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("states returned status %d", resp.StatusCode)
	}

	type rawState struct {
		EntityID   string `json:"entity_id"`
		State      string `json:"state"`
		Attributes struct {
			FriendlyName string `json:"friendly_name"`
			DeviceClass  string `json:"device_class"`
		} `json:"attributes"`
	}
	var rawStates []rawState
	if err := json.NewDecoder(resp.Body).Decode(&rawStates); err != nil {
		return nil, fmt.Errorf("parse states response: %w", err)
	}

	states := make(map[string]EntityState, len(rawStates))
	for _, s := range rawStates {
		domain := s.EntityID
		if idx := strings.Index(s.EntityID, "."); idx != -1 {
			domain = s.EntityID[:idx]
		}
		name := s.Attributes.FriendlyName
		if name == "" {
			name = s.EntityID
		}
		states[s.EntityID] = EntityState{
			EntityID:     s.EntityID,
			Domain:       domain,
			State:        s.State,
			FriendlyName: name,
			DeviceClass:  s.Attributes.DeviceClass,
		}
	}
	return states, nil
}
