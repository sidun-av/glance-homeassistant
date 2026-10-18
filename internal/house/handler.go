package house

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"html"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sidun-av/glance-homeassistant/internal/hass"
)

// HA is what the handler needs from Home Assistant; *hass.Client has it.
type HA interface {
	FetchRawStates(ctx context.Context) (map[string]hass.RawState, error)
	RenderTemplate(ctx context.Context, tpl string) (string, error)
	CallServiceData(ctx context.Context, domain, service string, data map[string]any) error
}

// Handler serves the 3D home under <prefix>/house/:
//
//	GET  /house/plan     the stored plan, filtered (ETag, 304 on If-None-Match)
//	PUT  /house/plan     upload from the owner's Mac (bearer secret, LAN only)
//	GET  /house/state    device state in House Control's applyRemote shape + planVersion
//	POST /house/command  {kind,id,patch,state} → Home Assistant service calls
//	GET  /house/         House Control's page with its boot config
//	GET  /house/assets/* the page's files
//
// Bodies toward the browser carry error codes only; Home Assistant's own
// errors (which name its URL) go to the server log.
type Handler struct {
	Store  *Store
	HA     HA
	Secret string // PUT /house/plan bearer; empty disables uploads
	// Base is the browser path of the page without a trailing slash, e.g.
	// "/ha-widget/house": the page's <base href> and its API host.
	Base string
	Lang string // "en" or "ru", handed to the page
	Dist fs.FS  // House Control's build: index.html and assets/

	statesMu  sync.Mutex
	states    map[string]hass.RawState
	statesAt  time.Time
	batteryMu sync.Mutex
	batteryOf string            // plan ETag the battery sensors were looked up for
	batteries map[string]string // vacuum → battery sensor ("" none)
	devMu     sync.Mutex
	devLocks  map[string]*sync.Mutex
}

const (
	statesTTL       = 2 * time.Second
	stateBudget     = 5 * time.Second
	commandBudget   = 10 * time.Second
	maxCommandBytes = 8 << 10
)

// Register mounts every /house route under prefix ("" or a proxy prefix).
func (h *Handler) Register(mux *http.ServeMux, prefix string) {
	mux.Handle(prefix+"/house/plan", nosniff(http.HandlerFunc(h.plan)))
	mux.Handle(prefix+"/house/state", nosniff(http.HandlerFunc(h.state)))
	mux.Handle(prefix+"/house/command", nosniff(http.HandlerFunc(h.command)))
	page := nosniff(h.page(prefix))
	mux.Handle(prefix+"/house/", page)
	mux.Handle(prefix+"/house", page)
}

func nosniff(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		status, body = http.StatusInternalServerError, []byte(`{"error":"internal"}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(body)
	w.Write([]byte("\n"))
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

// --- /house/plan ---

func (h *Handler) plan(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		p := h.Store.Current()
		if p == nil {
			writeError(w, http.StatusNotFound, "no_plan")
			return
		}
		w.Header().Set("ETag", p.ETag)
		w.Header().Set("Cache-Control", "no-cache")
		if etagMatches(r.Header.Get("If-None-Match"), p.ETag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			w.Write(p.Served())
		}
	case http.MethodPut:
		h.upload(w, r)
	default:
		w.Header().Set("Allow", "GET, HEAD, PUT")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
	}
}

func etagMatches(header, etag string) bool {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimPrefix(strings.TrimSpace(part), "W/")
		if part == etag || part == "*" {
			return true
		}
	}
	return false
}

// upload takes a plan from the owner's Mac. It is only for a request that
// came straight over the LAN: the reverse proxy in front of the dashboard
// adds forwarding headers, and anything carrying them is refused even with
// the right secret.
func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	if h.Secret == "" {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP", "Forwarded"} {
		if r.Header.Get(header) != "" {
			log.Printf("house: plan upload refused: %s present (came through a proxy)", header)
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
	}
	if !bearerMatches(r.Header.Get("Authorization"), h.Secret) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, MaxPlanBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request")
		return
	}
	if len(raw) > MaxPlanBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large")
		return
	}
	p, err := h.Store.Save(raw)
	var pe *PlanError
	switch {
	case errors.As(err, &pe):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_plan", "reason": pe.Reason})
		return
	case err != nil:
		log.Printf("house: plan upload: %v", err)
		writeError(w, http.StatusInternalServerError, "save_failed")
		return
	}
	log.Printf("house: plan saved (%d devices) from %s", len(p.Devices), r.RemoteAddr)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "etag": p.ETag})
}

// bearerMatches compares in constant time (hashing first so that the
// length of the secret does not show either).
func bearerMatches(header, secret string) bool {
	got, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		return false
	}
	a, b := sha256.Sum256([]byte(got)), sha256.Sum256([]byte(secret))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}

// --- /house/state ---

func (h *Handler) state(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	p := h.Store.Current()
	if p == nil {
		writeError(w, http.StatusNotFound, "no_plan")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), stateBudget)
	defer cancel()
	states, err := h.rawStates(ctx)
	if err != nil {
		log.Printf("house: state: %v", err)
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	snap := Snapshot(p, states, h.batterySensors(ctx, p))
	snap["planVersion"] = p.ETag
	writeJSON(w, http.StatusOK, snap)
}

// rawStates is every entity's state, shared by all viewers for statesTTL
// (ten open dashboards polling every 10 s are one HA read, not ten).
func (h *Handler) rawStates(ctx context.Context) (map[string]hass.RawState, error) {
	h.statesMu.Lock()
	defer h.statesMu.Unlock()
	if h.states != nil && time.Since(h.statesAt) < statesTTL {
		return h.states, nil
	}
	states, err := h.HA.FetchRawStates(ctx)
	if err != nil {
		return nil, err
	}
	h.states, h.statesAt = states, time.Now()
	return states, nil
}

// dropStates makes the next read ask HA again: after a command, the panel's
// follow-up polls must see what the command did.
func (h *Handler) dropStates() {
	h.statesMu.Lock()
	h.states = nil
	h.statesMu.Unlock()
}

// batterySensors looks each vacuum's battery sensor up once per plan, with
// one template request. A lookup that fails counts as "no sensor" until
// the plan changes (as in House Control): the vacuum's own battery_level,
// if any, is used meanwhile.
func (h *Handler) batterySensors(ctx context.Context, p *Plan) map[string]string {
	h.batteryMu.Lock()
	defer h.batteryMu.Unlock()
	if h.batteryOf == p.ETag {
		return h.batteries
	}
	var vacuums []string
	for _, d := range p.Devices {
		if d.Kind == KindVacuum {
			vacuums = append(vacuums, d.EntityID)
		}
	}
	out := map[string]string{}
	if len(vacuums) > 0 {
		text, err := h.HA.RenderTemplate(ctx, BatteryTemplate(vacuums))
		if err != nil {
			log.Printf("house: battery sensors: %v", err)
			text = ""
		}
		found := ParseBatterySensors(text)
		for _, v := range vacuums {
			out[v] = found[v]
		}
	}
	h.batteryOf, h.batteries = p.ETag, out
	return out
}

// --- /house/command ---

// allowedKeys are the patch keys the panel sends, per kind.
var allowedKeys = map[Kind]map[string]bool{
	KindLight:  {"on": true, "brightness": true, "temp": true, "color": true},
	KindAC:     {"on": true, "mode": true, "target": true, "fan": true, "swing": true, "sleep": true, "eco": true},
	KindVacuum: {"status": true, "fan": true},
}

var (
	hexColorRe  = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	swingValues = map[string]bool{"swing": true, "off": true, "l1": true, "l2": true, "l3": true, "l4": true, "l5": true}
	vacStatuses = map[string]bool{"cleaning": true, "paused": true, "returning": true}
)

type commandRequest struct {
	Kind  Kind           `json:"kind"`
	ID    string         `json:"id"`
	Patch map[string]any `json:"patch"`
	State map[string]any `json:"state"`
}

func (h *Handler) command(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	// Only the same-origin page sends JSON; a cross-site form cannot.
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxCommandBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request")
		return
	}
	if len(raw) > maxCommandBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large")
		return
	}
	var req commandRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request")
		return
	}
	p := h.Store.Current()
	if p == nil {
		writeError(w, http.StatusNotFound, "no_plan")
		return
	}
	d, ok := p.Device(req.ID)
	if !ok || d.Kind != req.Kind {
		writeError(w, http.StatusBadRequest, "unknown_device")
		return
	}
	if req.Patch == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_patch", "failed": []string{}})
		return
	}
	keys := sortedKeys(req.Patch)
	var failed []string
	for _, k := range keys {
		if !allowedKeys[d.Kind][k] {
			failed = append(failed, k)
		}
	}
	if len(failed) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_patch", "failed": failed})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), commandBudget)
	defer cancel()
	states, err := h.rawStates(ctx)
	if err != nil {
		log.Printf("house: %s %s %s → error: %v", d.Kind, d.AppID, strings.Join(keys, ","), err)
		writeError(w, http.StatusBadGateway, "unavailable")
		return
	}
	entity, ok := states[d.EntityID]
	if !ok {
		log.Printf("house: %s %s %s → error: Home Assistant does not know %s", d.Kind, d.AppID, strings.Join(keys, ","), d.EntityID)
		writeError(w, http.StatusBadGateway, "unavailable")
		return
	}
	if failed := checkPatch(d, req.Patch, entity.Attributes); len(failed) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_patch", "failed": failed})
		return
	}

	calls := Calls(d.EntityID, req.Patch, req.State, entity.Attributes)
	if r.URL.Query().Get("dry_run") == "1" {
		log.Printf("house: %s %s %s → dry run (%d calls)", d.Kind, d.AppID, strings.Join(keys, ","), len(calls))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "calls": calls})
		return
	}
	if len(calls) == 0 {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	// Commands to one entity run one after another, in arrival order.
	lock := h.deviceLock(d.EntityID)
	lock.Lock()
	defer lock.Unlock()
	defer h.dropStates()
	for _, c := range calls {
		if err := h.HA.CallServiceData(ctx, c.Domain, c.Service, c.Data); err != nil {
			log.Printf("house: %s %s %s → error: %v", d.Kind, d.AppID, strings.Join(keys, ","), err)
			writeError(w, http.StatusBadGateway, "unavailable")
			return
		}
	}
	log.Printf("house: %s %s %s → ok", d.Kind, d.AppID, strings.Join(keys, ","))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) deviceLock(entity string) *sync.Mutex {
	h.devMu.Lock()
	defer h.devMu.Unlock()
	if h.devLocks == nil {
		h.devLocks = map[string]*sync.Mutex{}
	}
	if h.devLocks[entity] == nil {
		h.devLocks[entity] = &sync.Mutex{}
	}
	return h.devLocks[entity]
}

// checkPatch returns the patch keys whose values the device or its entity
// cannot take, sorted.
func checkPatch(d Device, patch, attrs map[string]any) []string {
	var failed []string
	for _, k := range sortedKeys(patch) {
		v := patch[k]
		ok := false
		switch d.Kind {
		case KindLight:
			switch k {
			case "on":
				_, ok = v.(bool)
			case "brightness":
				f, isNum := v.(float64)
				ok = isNum && f >= 0.05 && f <= 1
			case "temp":
				f, isNum := v.(float64)
				ok = isNum && f >= 2200 && f <= 6500
			case "color":
				// null: the panel clears the colour when it sets a white
				// temperature (store.js sanitizeLight); nothing to send.
				s, isStr := v.(string)
				ok = v == nil || (isStr && hexColorRe.MatchString(s))
			}
		case KindAC:
			switch k {
			case "on":
				_, ok = v.(bool)
			case "mode":
				s, _ := v.(string)
				_, ok = ModeTable(strs(attrs, "hvac_modes"))[s]
			case "target":
				f, isNum := v.(float64)
				ok = isNum && f >= d.Min && f <= d.Max
			case "fan":
				s, _ := v.(string)
				_, ok = FanTable(strs(attrs, "fan_modes"))[s]
			case "swing":
				s, _ := v.(string)
				ok = swingValues[s]
			case "sleep", "eco":
				_, isBool := v.(bool)
				ok = isBool && contains(strs(attrs, "preset_modes"), k)
			}
		case KindVacuum:
			switch k {
			case "status":
				s, _ := v.(string)
				ok = vacStatuses[s]
			case "fan":
				s, isStr := v.(string)
				ok = isStr && contains(strs(attrs, "fan_speed_list"), s)
			}
		}
		if !ok {
			failed = append(failed, k)
		}
	}
	return failed
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// --- the page ---

type bootConfig struct {
	Host  string `json:"host"`
	Lang  string `json:"lang"`
	Embed bool   `json:"embed"`
}

// page serves House Control's index.html at <prefix>/house and
// <prefix>/house/, with a <base> so its ./assets/ resolve under Base and
// its boot config (API host, language, embed layout), and the build's
// files under assets/. Nothing else under /house/ exists.
func (h *Handler) page(prefix string) http.Handler {
	root := prefix + "/house"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		rel, ok := strings.CutPrefix(r.URL.Path, root)
		if !ok || h.Dist == nil {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		if rel == "" || rel == "/" {
			h.index(w, r)
			return
		}
		name := strings.TrimPrefix(rel, "/")
		if !strings.HasPrefix(name, "assets/") || !fs.ValidPath(name) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		info, err := fs.Stat(h.Dist, name)
		if err != nil || info.IsDir() {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		data, err := fs.ReadFile(h.Dist, name)
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		// Vite names every asset by its content hash.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
	})
}

func (h *Handler) index(w http.ResponseWriter, r *http.Request) {
	raw, err := fs.ReadFile(h.Dist, "index.html")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	boot, _ := json.Marshal(bootConfig{Host: h.Base, Lang: h.Lang, Embed: true}) // escapes <, >, &
	head := `<head><base href="` + html.EscapeString(h.Base+"/") + `"><script>window.__HOUSE_BOOT__=` + string(boot) + `</script>`
	page := strings.Replace(string(raw), "<head>", head, 1)
	if page == string(raw) {
		log.Printf("house: index.html has no <head>")
		writeError(w, http.StatusInternalServerError, "bad_page")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		io.WriteString(w, page)
	}
}
