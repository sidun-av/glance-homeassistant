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
	"net"
	"net/http"
	"net/netip"
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
	// FrameAncestors is the page's CSP frame-ancestors source list: the
	// origins allowed to embed it (its controls must not be clickjackable
	// from any site a LAN visitor opens). Empty means "'self'".
	FrameAncestors string

	statesMu  sync.Mutex
	states    map[string]hass.RawState
	statesAt  time.Time
	stale     bool // a command ran since states was read
	batteryMu sync.Mutex
	batteryOf string            // plan ETag the battery sensors were looked up for
	batteries map[string]string // vacuum → battery sensor ("" none)
	batteryOK bool              // the lookup for batteryOf succeeded
	batteryAt time.Time         // when it last failed
	devMu     sync.Mutex
	devLocks  map[string]chan struct{}
	commands  tokenBucket

	clock func() time.Time // nil: time.Now (tests set it)
}

const (
	statesTTL     = 2 * time.Second
	batteryBudget = 5 * time.Second
	batteryRetry  = time.Minute
	stateBudget   = 5 * time.Second
	commandBudget = 10 * time.Second
	// commandRate and commandBurst bound /house/command for the whole
	// process: far more than a person tapping and dragging sliders sends,
	// far less than a script in a loop.
	commandRate     = 10 // per second
	commandBurst    = 30
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
// came straight over the LAN: the TCP peer must have a private address,
// and the reverse proxy in front of the dashboard adds forwarding headers,
// so anything carrying them is refused even with the right secret. Both
// are defence in depth; the secret is the control.
func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	if h.Secret == "" {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if !privatePeer(r.RemoteAddr) {
		log.Printf("house: plan upload refused: peer %s is not on a private network", r.RemoteAddr)
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

// privatePeer is true for a loopback, RFC 1918 or IPv6 unique-local
// (fc00::/7) address, IPv4-mapped forms included.
func privatePeer(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	return addr.IsLoopback() || addr.IsPrivate()
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
	states, err := h.rawStates(ctx, true)
	if err != nil {
		log.Printf("house: state: %v", err)
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	snap := Snapshot(p, states, h.batterySensors(p))
	snap["planVersion"] = p.ETag
	writeJSON(w, http.StatusOK, snap)
}

// rawStates is every entity's state, shared by all viewers for statesTTL
// (ten open dashboards polling every 10 s are one HA read, not ten). With
// fresh, a read made before the last command is not used: the panel's
// follow-up polls must see what the command did. A command's own
// validation passes fresh=false; it only needs the entity's attributes,
// so a burst of commands is not one full states download each.
func (h *Handler) rawStates(ctx context.Context, fresh bool) (map[string]hass.RawState, error) {
	h.statesMu.Lock()
	defer h.statesMu.Unlock()
	if h.states != nil && time.Since(h.statesAt) < statesTTL && !(fresh && h.stale) {
		return h.states, nil
	}
	states, err := h.HA.FetchRawStates(ctx)
	if err != nil {
		return nil, err
	}
	h.states, h.statesAt, h.stale = states, time.Now(), false
	return states, nil
}

// markStale makes the next fresh read ask HA again. It waits out a fetch
// in flight, so no snapshot taken before the command survives it.
func (h *Handler) markStale() {
	h.statesMu.Lock()
	h.stale = true
	h.statesMu.Unlock()
}

// batterySensors looks each vacuum's battery sensor up once per plan, with
// one template request. A failed lookup is not kept: it means "no sensor"
// (the vacuum's own battery_level, if any, is used meanwhile) and is tried
// again batteryRetry later. The lookup runs on its own deadline, not the
// viewer's request, so a closed tab or a slow states read cannot cost the
// battery.
func (h *Handler) batterySensors(p *Plan) map[string]string {
	h.batteryMu.Lock()
	defer h.batteryMu.Unlock()
	if h.batteryOf == p.ETag && (h.batteryOK || h.now().Sub(h.batteryAt) < batteryRetry) {
		return h.batteries
	}
	var vacuums []string
	for _, d := range p.Devices {
		if d.Kind == KindVacuum {
			vacuums = append(vacuums, d.EntityID)
		}
	}
	out := map[string]string{}
	ok := true
	if len(vacuums) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), batteryBudget)
		text, err := h.HA.RenderTemplate(ctx, BatteryTemplate(vacuums))
		cancel()
		if err != nil {
			log.Printf("house: battery sensors (retried in %s): %v", batteryRetry, err)
			text, ok = "", false
		}
		found := ParseBatterySensors(text)
		for _, v := range vacuums {
			out[v] = found[v]
		}
	}
	h.batteryOf, h.batteries, h.batteryOK, h.batteryAt = p.ETag, out, ok, h.now()
	return out
}

func (h *Handler) now() time.Time {
	if h.clock != nil {
		return h.clock()
	}
	return time.Now()
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
	if !h.commands.allow(h.now(), commandRate, commandBurst) {
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusTooManyRequests, "rate_limited")
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
	states, err := h.rawStates(ctx, false)
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

	// Commands to one entity run one after another, in arrival order; one
	// that waits past its deadline (HA hanging on the one before) gives up.
	lock := h.deviceLock(d.EntityID)
	select {
	case lock <- struct{}{}:
	case <-ctx.Done():
		log.Printf("house: %s %s %s → error: gave up waiting for the previous command: %v", d.Kind, d.AppID, strings.Join(keys, ","), ctx.Err())
		writeError(w, http.StatusBadGateway, "unavailable")
		return
	}
	defer func() { <-lock }()
	defer h.markStale()
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

// deviceLock is the entity's one-slot queue: send to take it, receive to
// let the next command in.
func (h *Handler) deviceLock(entity string) chan struct{} {
	h.devMu.Lock()
	defer h.devMu.Unlock()
	if h.devLocks == nil {
		h.devLocks = map[string]chan struct{}{}
	}
	if h.devLocks[entity] == nil {
		h.devLocks[entity] = make(chan struct{}, 1)
	}
	return h.devLocks[entity]
}

// tokenBucket is a plain token bucket; the zero value starts full.
type tokenBucket struct {
	mu     sync.Mutex
	tokens float64
	last   time.Time
}

func (b *tokenBucket) allow(now time.Time, rate, burst float64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.last.IsZero() {
		b.tokens = burst
	} else if dt := now.Sub(b.last).Seconds(); dt > 0 {
		b.tokens = min(burst, b.tokens+dt*rate)
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
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
	ancestors := h.FrameAncestors
	if ancestors == "" {
		ancestors = "'self'"
	}
	w.Header().Set("Content-Security-Policy", "frame-ancestors "+ancestors)
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		io.WriteString(w, page)
	}
}
