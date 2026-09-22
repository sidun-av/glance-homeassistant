// Package editor serves the browser page that edits the floorplan and the
// JSON endpoints behind it. It owns nothing itself: the app hands it a
// Store (current layout + save) and a Source (Home Assistant areas and
// entities), and a Preview renderer.
package editor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/sidun-av/glance-homeassistant/internal/layout"
)

// Entity is what the editor lists for an area.
type Entity struct {
	EntityID string `json:"entity_id"`
	Name     string `json:"name"`
	Domain   string `json:"domain"`
	Icon     string `json:"icon,omitempty"`
	IconSVG  string `json:"icon_svg,omitempty"` // the glyph the map would draw, for the editor's chips
	// Kind: light, device, contact, motion, temperature or other. The
	// first three are what the map draws as tiles; motion/temperature are
	// shown for information; "other" is hidden behind "show all".
	Kind string `json:"kind"`
	// PlaceID is the id the layout places this entity by: entity_id for
	// lights/devices, the friendly name for contacts (the widget keys
	// contact badges by name).
	PlaceID string `json:"place_id"`
}

type Area struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Entities []Entity `json:"entities"`
}

type Store interface {
	Current() *layout.Layout                // never nil (seeded from config when no file)
	Save(*layout.Layout) error              // validates, persists, applies
	Seed() *layout.Layout                   // the config-defined layout, for "reset"
	Preview(*layout.Layout) (string, error) // widget HTML for an unsaved layout
}

type Source interface {
	Areas(ctx context.Context) ([]Area, error)
}

type Handler struct {
	Store  Store
	Source Source
	// Prefix is public_url when it is a path ("/ha-widget"), so links in
	// the page work through a reverse proxy that keeps the prefix.
	Prefix string
}

// Register mounts the editor under base ("" or the proxy prefix).
func (h *Handler) Register(mux *http.ServeMux, base string) {
	mux.HandleFunc(base+"/edit", h.page)
	mux.HandleFunc(base+"/edit/", h.page)
	mux.HandleFunc(base+"/edit/data.json", h.data)
	mux.HandleFunc(base+"/edit/layout.json", h.layout)
	mux.HandleFunc(base+"/edit/preview", h.preview)
}

func (h *Handler) page(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/edit") && !strings.HasSuffix(r.URL.Path, "/edit/") {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	base := strings.TrimSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/edit")
	io.WriteString(w, strings.ReplaceAll(pageHTML, "{{BASE}}", base))
}

func (h *Handler) data(w http.ResponseWriter, r *http.Request) {
	areas, err := h.Source.Areas(r.Context())
	if err != nil {
		http.Error(w, "home assistant unavailable: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, map[string]any{"layout": h.Store.Current(), "seed": h.Store.Seed(), "areas": areas})
}

func (h *Handler) layout(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, h.Store.Current())
	case http.MethodPut, http.MethodPost:
		l, err := readLayout(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := h.Store.Save(l); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, l)
	default:
		w.Header().Set("Allow", "GET, PUT")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) preview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST a layout", http.StatusMethodNotAllowed)
		return
	}
	l, err := readLayout(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := l.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	html, err := h.Store.Preview(l)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, html)
}

func readLayout(r *http.Request) (*layout.Layout, error) {
	var l layout.Layout
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&l); err != nil {
		return nil, fmt.Errorf("invalid layout JSON: %w", err)
	}
	return &l, nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}
