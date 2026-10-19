package house

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/sidun-av/glance-homeassistant/internal/hass"
)

const (
	testSecret = "s3cret"
	testBase   = "/ha-widget/house"
)

// fakeHA stands in for Home Assistant and records what it was asked.
type fakeHA struct {
	mu          sync.Mutex
	states      map[string]hass.RawState
	statesErr   error
	template    string
	templateErr error
	templateCtx bool // RenderTemplate fails when its context is done
	callErr     error
	fetches     int
	templates   int
	calls       []Call
}

func (f *fakeHA) FetchRawStates(ctx context.Context) (map[string]hass.RawState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fetches++
	if f.statesErr != nil {
		return nil, f.statesErr
	}
	out := map[string]hass.RawState{}
	for k, v := range f.states {
		out[k] = v
	}
	return out, nil
}

func (f *fakeHA) RenderTemplate(ctx context.Context, tpl string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.templates++
	if f.templateCtx && ctx.Err() != nil {
		return "", ctx.Err()
	}
	return f.template, f.templateErr
}

func (f *fakeHA) CallServiceData(ctx context.Context, domain, service string, data map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, Call{Domain: domain, Service: service, Data: data})
	return f.callErr
}

func (f *fakeHA) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// rawAttrs round-trips attributes through JSON, as they arrive from HA.
func rawAttrs(v map[string]any) map[string]any {
	b, _ := json.Marshal(v)
	var out map[string]any
	json.Unmarshal(b, &out)
	return out
}

func newFakeHA() *fakeHA {
	return &fakeHA{
		states: map[string]hass.RawState{
			"light.demo_lamp":           {EntityID: "light.demo_lamp", State: "off", Attributes: map[string]any{}},
			"switch.demo_plug":          {EntityID: "switch.demo_plug", State: "on", Attributes: map[string]any{}},
			"climate.demo_heater":       {EntityID: "climate.demo_heater", State: "heat", Attributes: rawAttrs(map[string]any{"hvac_modes": []string{"off", "heat"}, "temperature": 21, "preset_modes": []string{"none", "eco"}, "fan_modes": []string{"low", "high"}})},
			"sensor.demo_temp":          {EntityID: "sensor.demo_temp", State: "21.5", Attributes: map[string]any{"unit_of_measurement": "°C"}},
			"vacuum.demo_robot":         {EntityID: "vacuum.demo_robot", State: "docked", Attributes: rawAttrs(map[string]any{"fan_speed_list": []string{"Quiet", "Turbo"}, "supported_features": 4 | 16 | 32 | 8192})},
			"sensor.demo_robot_battery": {EntityID: "sensor.demo_robot_battery", State: "42", Attributes: map[string]any{"device_class": "battery"}},
		},
		template: "vacuum.demo_robot\tsensor.demo_robot_battery\n",
	}
}

var testDist = fstest.MapFS{
	"index.html":  {Data: []byte(`<!doctype html><html><head><title>x</title><script type="module" src="./assets/a.js"></script></head><body></body></html>`)},
	"assets/a.js": {Data: []byte(`console.log(1)`)},
}

// newTestHandler serves the generic fixture plan (Task 5's) from a temp
// store, with every route registered under "" and testBase's prefix.
func newTestHandler(t *testing.T, ha *fakeHA, withPlan bool) (*Handler, *http.ServeMux) {
	t.Helper()
	store := OpenStore(filepath.Join(t.TempDir(), "house-plan.json"))
	if withPlan {
		if _, err := store.Save([]byte(fixturePlan)); err != nil {
			t.Fatal(err)
		}
	}
	h := &Handler{Store: store, HA: ha, Secret: testSecret, Base: testBase, Lang: "ru", Dist: testDist}
	mux := http.NewServeMux()
	h.Register(mux, "")
	h.Register(mux, "/ha-widget")
	return h, mux
}

// lanPeer is the TCP peer of every test request unless a test says
// otherwise: plan uploads are taken only from a private address.
const lanPeer = "10.0.0.7:50123"

func do(mux http.Handler, method, target string, body string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.RemoteAddr = lanPeer
	for i := 0; i+1 < len(headers); i += 2 {
		if headers[i] == "RemoteAddr" {
			req.RemoteAddr = headers[i+1]
			continue
		}
		if headers[i] == "Host" {
			req.Host = headers[i+1]
			continue
		}
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func command(mux http.Handler, body string) *httptest.ResponseRecorder {
	return do(mux, http.MethodPost, "/house/command", body, "Content-Type", "application/json")
}

func errorBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("body %q is not JSON: %v", rec.Body.String(), err)
	}
	return out
}

func TestPlan_GetNoPlan404(t *testing.T) {
	_, mux := newTestHandler(t, newFakeHA(), false)
	for _, path := range []string{"/house/plan", "/ha-widget/house/plan"} {
		rec := do(mux, http.MethodGet, path, "")
		if rec.Code != http.StatusNotFound || strings.TrimSpace(rec.Body.String()) != `{"error":"no_plan"}` {
			t.Errorf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestPlan_GetServesFilteredWithETagAnd304(t *testing.T) {
	// No battery sensor known: the plan goes out exactly as stored (see
	// TestPlan_BatteryHint for the other case).
	ha := newFakeHA()
	ha.template = ""
	h, mux := newTestHandler(t, ha, true)
	rec := do(mux, http.MethodGet, "/house/plan", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	etag := rec.Header().Get("ETag")
	if etag != h.Store.Current().ETag {
		t.Errorf("ETag = %q, want %q", etag, h.Store.Current().ETag)
	}
	if rec.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	if !bytes.Equal(rec.Body.Bytes(), h.Store.Current().Served()) {
		t.Error("body is not the served (filtered) plan")
	}
	if strings.Contains(rec.Body.String(), "c.hue") {
		t.Error("served plan carries the Hue lamp")
	}
	again := do(mux, http.MethodGet, "/house/plan", "", "If-None-Match", etag)
	if again.Code != http.StatusNotModified || again.Body.Len() != 0 {
		t.Errorf("If-None-Match: %d %q", again.Code, again.Body.String())
	}
}

func TestPlan_Put(t *testing.T) {
	good := `{"rooms":[{"id":"z"}],"lights":[{"id":"z.lamp","link":{"service":"homeassistant","id":"light.demo_lamp"}}]}`
	auth := "Bearer " + testSecret
	cases := []struct {
		name    string
		secret  string
		body    string
		headers []string
		code    int
		errCode string
		reason  string
	}{
		{"uploads disabled", "", good, []string{"Authorization", "Bearer "}, 403, "forbidden", ""},
		{"public peer", testSecret, good, []string{"Authorization", auth, "RemoteAddr", "203.0.113.9:4000"}, 403, "forbidden", ""},
		{"through the proxy", testSecret, good, []string{"Authorization", auth, "X-Forwarded-For", "1.2.3.4"}, 403, "forbidden", ""},
		{"real ip header", testSecret, good, []string{"Authorization", auth, "X-Real-IP", "1.2.3.4"}, 403, "forbidden", ""},
		{"forwarded header", testSecret, good, []string{"Authorization", auth, "Forwarded", "for=1.2.3.4"}, 403, "forbidden", ""},
		{"no bearer", testSecret, good, nil, 401, "unauthorized", ""},
		{"wrong bearer", testSecret, good, []string{"Authorization", "Bearer nope"}, 401, "unauthorized", ""},
		{"oversize", testSecret, `{"rooms":[{}],"pad":"` + strings.Repeat("x", MaxPlanBytes) + `"}`, []string{"Authorization", auth}, 413, "too_large", ""},
		{"no rooms", testSecret, `{"rooms":[]}`, []string{"Authorization", auth}, 400, "invalid_plan", "no_rooms"},
		{"a token inside", testSecret, `{"rooms":[{}],"token":"x"}`, []string{"Authorization", auth}, 400, "invalid_plan", "forbidden_key"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, mux := newTestHandler(t, newFakeHA(), true)
			h.Secret = c.secret
			before := h.Store.Current().ETag
			rec := do(mux, http.MethodPut, "/house/plan", c.body, c.headers...)
			if rec.Code != c.code {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, c.code, rec.Body.String())
			}
			body := errorBody(t, rec)
			if body["error"] != c.errCode {
				t.Errorf("error = %v, want %s", body["error"], c.errCode)
			}
			if c.reason != "" && body["reason"] != c.reason {
				t.Errorf("reason = %v, want %s", body["reason"], c.reason)
			}
			if h.Store.Current().ETag != before {
				t.Error("a refused upload replaced the plan")
			}
		})
	}

	t.Run("good", func(t *testing.T) {
		_, mux := newTestHandler(t, newFakeHA(), true)
		rec := do(mux, http.MethodPut, "/house/plan", good, "Authorization", auth, "Content-Type", "application/json")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
		}
		body := errorBody(t, rec)
		want, _ := ParsePlan([]byte(good))
		if body["ok"] != true || body["etag"] != want.ETag {
			t.Errorf("body = %v, want ok and etag %s", body, want.ETag)
		}
		if got := do(mux, http.MethodGet, "/house/plan", "").Header().Get("ETag"); got != want.ETag {
			t.Errorf("GET ETag after upload = %q, want %q", got, want.ETag)
		}
	})

	t.Run("method", func(t *testing.T) {
		_, mux := newTestHandler(t, newFakeHA(), true)
		if rec := do(mux, http.MethodDelete, "/house/plan", ""); rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("DELETE = %d", rec.Code)
		}
	})
}

func TestPrivatePeer(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:1":         true,
		"10.1.2.3:1":          true,
		"172.16.0.1:1":        true,
		"172.31.255.255:1":    true,
		"192.168.0.10:1":      true,
		"[::1]:1":             true,
		"[fd12:3456::1]:1":    true,
		"[::ffff:10.0.0.1]:1": true,
		"172.32.0.1:1":        false,
		"203.0.113.9:1":       false,
		"[2001:db8::1]:1":     false,
		"[fe80::1]:1":         false,
		"169.254.1.1:1":       false,
		"":                    false,
		"not an address":      false,
	} {
		if got := privatePeer(addr); got != want {
			t.Errorf("privatePeer(%q) = %v, want %v", addr, got, want)
		}
	}
}

func TestState_HAUnavailable(t *testing.T) {
	ha := newFakeHA()
	ha.statesErr = errors.New("connection refused")
	_, mux := newTestHandler(t, ha, true)
	rec := do(mux, http.MethodGet, "/house/state", "")
	if rec.Code != http.StatusServiceUnavailable || strings.TrimSpace(rec.Body.String()) != `{"error":"unavailable"}` {
		t.Errorf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestState_NoPlan404(t *testing.T) {
	_, mux := newTestHandler(t, newFakeHA(), false)
	rec := do(mux, http.MethodGet, "/house/state", "")
	if rec.Code != http.StatusNotFound || strings.TrimSpace(rec.Body.String()) != `{"error":"no_plan"}` {
		t.Errorf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestState_ShapeAndPlanVersion(t *testing.T) {
	ha := newFakeHA()
	_, mux := newTestHandler(t, ha, true)
	rec := do(mux, http.MethodGet, "/ha-widget/house/state", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	keys := []string{}
	for k := range body {
		keys = append(keys, k)
	}
	for _, k := range []string{"lights", "acs", "vacuums", "features", "planVersion"} {
		if _, ok := body[k]; !ok {
			t.Errorf("state lacks %s (has %v)", k, keys)
		}
	}
	if len(body) != 5 {
		t.Errorf("state keys = %v, want exactly five", keys)
	}
	// The panel reloads when planVersion is not the ETag it got the plan with.
	if etag := do(mux, http.MethodGet, "/house/plan", "").Header().Get("ETag"); body["planVersion"] != etag {
		t.Errorf("planVersion = %v, want the plan's ETag %s", body["planVersion"], etag)
	}
	robot := body["vacuums"].(map[string]any)["h.robot"].(map[string]any)
	if robot["battery"] != 42.0 || robot["status"] != "docked" {
		t.Errorf("robot = %v, want docked with battery 42 from the template's sensor", robot)
	}
	heat := body["acs"].(map[string]any)["k.heat"].(map[string]any)
	if heat["roomTemp"] != 21.5 {
		t.Errorf("heater = %v", heat)
	}
	if _, ok := body["lights"].(map[string]any)["c.hue"]; ok {
		t.Error("a dropped device has state")
	}

	do(mux, http.MethodGet, "/house/state", "")
	if ha.fetches != 1 {
		t.Errorf("FetchRawStates called %d times for two reads within 2 s, want 1", ha.fetches)
	}
	if ha.templates != 1 {
		t.Errorf("battery template rendered %d times, want once per plan", ha.templates)
	}
}

func TestState_BatteryTemplateFailureMeansNoSensor(t *testing.T) {
	ha := newFakeHA()
	ha.templateErr = errors.New("template boom")
	_, mux := newTestHandler(t, ha, true)
	rec := do(mux, http.MethodGet, "/house/state", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	robot := body["vacuums"].(map[string]any)["h.robot"].(map[string]any)
	if _, ok := robot["battery"]; ok {
		t.Errorf("robot = %v, want no battery", robot)
	}
}

// One transient template failure (an expired or cancelled context) must
// not lose the battery sensor for the plan's lifetime: it is asked again
// after the back-off, and HA's answer then counts.
func TestState_BatteryLookupRetriedAfterAFailure(t *testing.T) {
	ha := newFakeHA()
	ha.templateErr = context.DeadlineExceeded
	h, mux := newTestHandler(t, ha, true)
	now := time.Unix(1_700_000_000, 0)
	h.clock = func() time.Time { return now }
	battery := func() any {
		t.Helper()
		var body map[string]any
		rec := do(mux, http.MethodGet, "/house/state", "")
		json.Unmarshal(rec.Body.Bytes(), &body)
		return body["vacuums"].(map[string]any)["h.robot"].(map[string]any)["battery"]
	}
	if b := battery(); b != nil {
		t.Fatalf("battery = %v while the template fails", b)
	}
	ha.mu.Lock()
	ha.templateErr = nil
	ha.mu.Unlock()
	if b := battery(); b != nil || ha.templates != 1 {
		t.Errorf("within the back-off: battery %v, templates %d, want none and 1", b, ha.templates)
	}
	now = now.Add(batteryRetry)
	if b := battery(); b != 42.0 || ha.templates != 2 {
		t.Errorf("after the back-off: battery %v, templates %d, want 42 and 2", b, ha.templates)
	}
	now = now.Add(time.Hour)
	if b := battery(); b != 42.0 || ha.templates != 2 {
		t.Errorf("a success is kept: battery %v, templates %d", b, ha.templates)
	}
}

// The lookup does not run on the viewer's request context.
func TestState_BatteryLookupOutlivesTheRequest(t *testing.T) {
	ha := newFakeHA()
	ha.templateCtx = true
	_, mux := newTestHandler(t, ha, true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/house/state", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	robot, _ := body["vacuums"].(map[string]any)["h.robot"].(map[string]any)
	if robot["battery"] != 42.0 {
		t.Errorf("a cancelled request: %d %s, want the battery from the sensor", rec.Code, rec.Body.String())
	}
}

func TestCommand_ValidatesAndCalls(t *testing.T) {
	ha := newFakeHA()
	_, mux := newTestHandler(t, ha, true)

	rec := command(mux, `{"kind":"light","id":"a.lamp","patch":{"on":true},"state":{"on":true}}`)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"ok":true}` {
		t.Fatalf("lamp on: %d %s", rec.Code, rec.Body.String())
	}
	if len(ha.calls) != 1 || ha.calls[0].Domain != "light" || ha.calls[0].Service != "turn_on" || ha.calls[0].Data["entity_id"] != "light.demo_lamp" {
		t.Errorf("calls = %+v", ha.calls)
	}

	bad := []struct {
		body   string
		failed []any
	}{
		{`{"kind":"ac","id":"k.heat","patch":{"target":35}}`, []any{"target"}},
		{`{"kind":"ac","id":"k.heat","patch":{"mode":"cool"}}`, []any{"mode"}},
		{`{"kind":"ac","id":"k.heat","patch":{"sleep":true}}`, []any{"sleep"}},
		{`{"kind":"ac","id":"k.heat","patch":{"fan":"turbo","swing":"sideways"}}`, []any{"fan", "swing"}},
		{`{"kind":"light","id":"a.lamp","patch":{"brightness":0.01,"temp":9000,"color":"red","on":"yes"}}`, []any{"brightness", "color", "on", "temp"}},
		{`{"kind":"vacuum","id":"h.robot","patch":{"fan":"Nope"}}`, []any{"fan"}},
		{`{"kind":"vacuum","id":"h.robot","patch":{"status":"docked"}}`, []any{"status"}},
	}
	for _, c := range bad {
		rec := command(mux, c.body)
		body := errorBody(t, rec)
		if rec.Code != http.StatusBadRequest || body["error"] != "bad_patch" || !reflect.DeepEqual(body["failed"], c.failed) {
			t.Errorf("%s → %d %v, want 400 bad_patch failed %v", c.body, rec.Code, body, c.failed)
		}
	}

	// Setting a white temperature clears the colour: the panel sends
	// color null with it.
	ha.calls = nil
	rec = command(mux, `{"kind":"light","id":"a.lamp","patch":{"temp":3000,"color":null}}`)
	if rec.Code != http.StatusOK || len(ha.calls) != 1 || ha.calls[0].Data["color_temp_kelvin"] != 3000 || ha.calls[0].Data["rgb_color"] != nil {
		t.Errorf("temp + color null: %d %s calls %+v", rec.Code, rec.Body.String(), ha.calls)
	}

	ha.calls = nil
	rec = command(mux, `{"kind":"vacuum","id":"h.robot","patch":{"status":"cleaning","fan":"Turbo"},"state":{"status":"cleaning"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("vacuum: %d %s", rec.Code, rec.Body.String())
	}
	if len(ha.calls) != 2 || ha.calls[0].Service != "start" || ha.calls[1].Service != "set_fan_speed" {
		t.Errorf("vacuum calls = %+v", ha.calls)
	}

	ha.calls = nil
	rec = command(mux, `{"kind":"ac","id":"k.heat","patch":{"target":22.5,"eco":true},"state":{"mode":"heat"}}`)
	if rec.Code != http.StatusOK || len(ha.calls) != 2 {
		t.Errorf("heater: %d %s calls %+v", rec.Code, rec.Body.String(), ha.calls)
	}
}

func TestCommand_DropsTheStateCache(t *testing.T) {
	ha := newFakeHA()
	_, mux := newTestHandler(t, ha, true)
	do(mux, http.MethodGet, "/house/state", "")
	command(mux, `{"kind":"light","id":"a.lamp","patch":{"on":true}}`)
	before := ha.fetches
	do(mux, http.MethodGet, "/house/state", "")
	if ha.fetches != before+1 {
		t.Errorf("state after a command was served from the cache")
	}
}

// Commands validate against the cached states: a burst of them is not one
// full states download each, while a state read after a command still is.
func TestCommand_BurstSharesOneStatesRead(t *testing.T) {
	ha := newFakeHA()
	_, mux := newTestHandler(t, ha, true)
	for i := 0; i < 5; i++ {
		if rec := command(mux, `{"kind":"light","id":"a.lamp","patch":{"on":true}}`); rec.Code != http.StatusOK {
			t.Fatalf("command %d: %d %s", i, rec.Code, rec.Body.String())
		}
	}
	if ha.fetches != 1 {
		t.Errorf("FetchRawStates called %d times for 5 commands within 2 s, want 1", ha.fetches)
	}
	do(mux, http.MethodGet, "/house/state", "")
	do(mux, http.MethodGet, "/house/state", "")
	if ha.fetches != 2 {
		t.Errorf("FetchRawStates called %d times, want one more for the reads after the commands", ha.fetches)
	}
}

func TestCommand_RateLimited(t *testing.T) {
	ha := newFakeHA()
	h, mux := newTestHandler(t, ha, true)
	now := time.Unix(1_700_000_000, 0)
	h.clock = func() time.Time { return now }
	dry := func() *httptest.ResponseRecorder {
		return do(mux, http.MethodPost, "/house/command?dry_run=1", `{"kind":"light","id":"a.lamp","patch":{"on":true}}`, "Content-Type", "application/json")
	}
	for i := 0; i < commandBurst; i++ {
		if rec := dry(); rec.Code != http.StatusOK {
			t.Fatalf("command %d of the burst: %d", i, rec.Code)
		}
	}
	rec := dry()
	if rec.Code != http.StatusTooManyRequests || errorBody(t, rec)["error"] != "rate_limited" || rec.Header().Get("Retry-After") != "1" {
		t.Fatalf("past the burst: %d %s %v", rec.Code, rec.Body.String(), rec.Header())
	}
	now = now.Add(time.Second)
	for i := 0; i < commandRate; i++ {
		if rec := dry(); rec.Code != http.StatusOK {
			t.Fatalf("a second later, command %d: %d", i, rec.Code)
		}
	}
	if rec := dry(); rec.Code != http.StatusTooManyRequests {
		t.Errorf("past the refill: %d", rec.Code)
	}
}

// blockingHA hangs every service call until released.
type blockingHA struct {
	*fakeHA
	entered chan struct{}
	release chan struct{}
}

func (b *blockingHA) CallServiceData(ctx context.Context, domain, service string, data map[string]any) error {
	b.entered <- struct{}{}
	<-b.release
	return b.fakeHA.CallServiceData(ctx, domain, service, data)
}

func TestCommand_QueuedCommandGivesUpWithItsContext(t *testing.T) {
	ha := &blockingHA{fakeHA: newFakeHA(), entered: make(chan struct{}, 1), release: make(chan struct{})}
	h, mux := newTestHandler(t, ha.fakeHA, true)
	h.HA = ha
	body := `{"kind":"light","id":"a.lamp","patch":{"on":true}}`
	first := make(chan int)
	go func() { first <- command(mux, body).Code }()
	<-ha.entered // the first command holds the lamp

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/house/command", strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { mux.ServeHTTP(rec, req); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the queued command ignored its context")
	}
	if rec.Code != http.StatusBadGateway || errorBody(t, rec)["error"] != "unavailable" {
		t.Errorf("queued command: %d %s", rec.Code, rec.Body.String())
	}
	close(ha.release)
	if code := <-first; code != http.StatusOK {
		t.Errorf("first command: %d", code)
	}
	if n := ha.callCount(); n != 1 {
		t.Errorf("HA got %d calls, want only the first command's", n)
	}
}

func TestCommand_RejectsUnknownAndForeign(t *testing.T) {
	ha := newFakeHA()
	_, mux := newTestHandler(t, ha, true)
	cases := []struct {
		body, code string
	}{
		{`{"kind":"light","id":"z.nowhere","patch":{"on":true}}`, "unknown_device"},
		{`{"kind":"light","id":"c.hue","patch":{"on":true}}`, "unknown_device"},
		{`{"kind":"light","id":"f.fan","patch":{"on":true}}`, "unknown_device"},
		{`{"kind":"ac","id":"a.lamp","patch":{"on":true}}`, "unknown_device"},
		{`{"kind":"light","id":"a.lamp","patch":{"on":true,"entity_id":"lock.front"}}`, "bad_patch"},
		{`{"kind":"vacuum","id":"h.robot","patch":{"target":20}}`, "bad_patch"},
		{`{"kind":"light","id":"a.lamp"}`, "bad_patch"},
		{`{"kind":"light","id":"a.lamp","patch":[1]}`, "bad_request"},
		{`not json`, "bad_request"},
	}
	for _, c := range cases {
		rec := command(mux, c.body)
		body := errorBody(t, rec)
		if rec.Code != http.StatusBadRequest || body["error"] != c.code {
			t.Errorf("%s → %d %v, want 400 %s", c.body, rec.Code, body, c.code)
		}
	}
	rec := command(mux, `{"kind":"light","id":"a.lamp","patch":{"on":true,"entity_id":"lock.front"}}`)
	if failed := errorBody(t, rec)["failed"]; !reflect.DeepEqual(failed, []any{"entity_id"}) {
		t.Errorf("failed = %v", failed)
	}
	if n := ha.callCount(); n != 0 {
		t.Errorf("HA got %d calls, want none", n)
	}
}

func TestCommand_RequiresJSONAndPost(t *testing.T) {
	ha := newFakeHA()
	_, mux := newTestHandler(t, ha, true)
	body := `{"kind":"light","id":"a.lamp","patch":{"on":true}}`
	rec := do(mux, http.MethodPost, "/house/command", body, "Content-Type", "text/plain")
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("text/plain = %d", rec.Code)
	}
	rec = do(mux, http.MethodPost, "/house/command", body)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("no content type = %d", rec.Code)
	}
	rec = do(mux, http.MethodGet, "/house/command", "")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d", rec.Code)
	}
	rec = do(mux, http.MethodPost, "/house/command", body, "Content-Type", "application/json; charset=utf-8", "Origin", "https://evil.example")
	if rec.Code != http.StatusOK {
		t.Errorf("application/json; charset=utf-8 = %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("command answers with CORS headers")
	}
	rec = do(mux, http.MethodPost, "/house/command", `{"kind":"light","id":"a.lamp","patch":{"on":true},"pad":"`+strings.Repeat("x", 9<<10)+`"}`, "Content-Type", "application/json")
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("9 KiB body = %d", rec.Code)
	}
	if n := ha.callCount(); n != 1 {
		t.Errorf("HA got %d calls, want only the one good command", n)
	}
}

func TestCommand_DryRunMakesNoCall(t *testing.T) {
	ha := newFakeHA()
	_, mux := newTestHandler(t, ha, true)
	rec := do(mux, http.MethodPost, "/house/command?dry_run=1", `{"kind":"light","id":"a.lamp","patch":{"on":true,"brightness":0.5}}`, "Content-Type", "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("dry run: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		OK    bool   `json:"ok"`
		Calls []Call `json:"calls"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if !body.OK || len(body.Calls) != 1 || body.Calls[0].Service != "turn_on" || body.Calls[0].Data["brightness_pct"] != 50.0 {
		t.Errorf("dry run body = %s", rec.Body.String())
	}
	if n := ha.callCount(); n != 0 {
		t.Errorf("a dry run made %d HA calls", n)
	}
}

func TestCommand_HAErrorIs502WithCode(t *testing.T) {
	ha := newFakeHA()
	ha.callErr = errors.New(`Post "http://ha.example.invalid:8123/api/services/light/turn_on": connection refused`)
	_, mux := newTestHandler(t, ha, true)
	rec := command(mux, `{"kind":"light","id":"a.lamp","patch":{"on":true}}`)
	if rec.Code != http.StatusBadGateway || strings.TrimSpace(rec.Body.String()) != `{"error":"unavailable"}` {
		t.Errorf("got %d %s", rec.Code, rec.Body.String())
	}

	// States unreachable, or the entity gone from HA: the same answer.
	ha2 := newFakeHA()
	ha2.statesErr = errors.New("boom")
	_, mux2 := newTestHandler(t, ha2, true)
	if rec := command(mux2, `{"kind":"light","id":"a.lamp","patch":{"on":true}}`); rec.Code != http.StatusBadGateway || errorBody(t, rec)["error"] != "unavailable" {
		t.Errorf("states error: %d %s", rec.Code, rec.Body.String())
	}
	ha3 := newFakeHA()
	delete(ha3.states, "light.demo_lamp")
	_, mux3 := newTestHandler(t, ha3, true)
	if rec := command(mux3, `{"kind":"light","id":"a.lamp","patch":{"on":true}}`); rec.Code != http.StatusBadGateway || errorBody(t, rec)["error"] != "unavailable" {
		t.Errorf("entity missing: %d %s", rec.Code, rec.Body.String())
	}
	if ha3.callCount() != 0 {
		t.Error("a call went to an entity HA does not know")
	}
}

func TestCommand_NoPlan404(t *testing.T) {
	_, mux := newTestHandler(t, newFakeHA(), false)
	rec := command(mux, `{"kind":"light","id":"a.lamp","patch":{"on":true}}`)
	if rec.Code != http.StatusNotFound || errorBody(t, rec)["error"] != "no_plan" {
		t.Errorf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestPage_InjectsBootAndBase(t *testing.T) {
	h, mux := newTestHandler(t, newFakeHA(), true)
	for _, path := range []string{"/house/", "/house", "/ha-widget/house/", "/ha-widget/house"} {
		rec := do(mux, http.MethodGet, path, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d", path, rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `<head><base href="/ha-widget/house/"><script>window.__HOUSE_BOOT__={"host":"/ha-widget/house","lang":"ru","embed":true}</script><title>x</title>`) {
			t.Errorf("%s: page = %s", path, body)
		}
		if rec.Header().Get("Cache-Control") != "no-cache" || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
			t.Errorf("%s: headers = %v", path, rec.Header())
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: no nosniff", path)
		}
		if csp := rec.Header().Get("Content-Security-Policy"); csp != "frame-ancestors 'self'" {
			t.Errorf("%s: Content-Security-Policy = %q", path, csp)
		}
	}
	h.FrameAncestors = "'self' http://glance.lan:3000"
	if csp := do(mux, http.MethodGet, "/house/", "").Header().Get("Content-Security-Policy"); csp != "frame-ancestors 'self' http://glance.lan:3000" {
		t.Errorf("with FrameAncestors: Content-Security-Policy = %q", csp)
	}
	h.FrameAncestors = ""
	rec := do(mux, http.MethodGet, "/ha-widget/house/assets/a.js", "")
	if rec.Code != http.StatusOK || rec.Body.String() != "console.log(1)" {
		t.Fatalf("asset: %d %q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Errorf("asset Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "javascript") {
		t.Errorf("asset Content-Type = %q", rec.Header().Get("Content-Type"))
	}
	for _, path := range []string{"/house/assets/missing.js", "/house/assets/", "/house/assets", "/house/index.html", "/house/other", "/house/plans/home.json"} {
		if rec := do(mux, http.MethodGet, path, ""); rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", path, rec.Code)
		}
	}
	// A path that climbs out is never served (the mux cleans it into a
	// redirect; the page handler itself refuses it outright).
	if rec := do(mux, http.MethodGet, "/house/../config.yml", ""); rec.Code == http.StatusOK {
		t.Errorf("/house/../config.yml: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.page("").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/house/../config.yml", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("page handler, /house/../config.yml: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.page("").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/house/assets/../index.html", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("page handler, assets/../index.html: %d", rec.Code)
	}
	if rec := do(mux, http.MethodPost, "/house/", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST page: %d", rec.Code)
	}
}

func TestPage_NoDist404(t *testing.T) {
	h, mux := newTestHandler(t, newFakeHA(), true)
	h.Dist = nil
	if rec := do(mux, http.MethodGet, "/house/", ""); rec.Code != http.StatusNotFound {
		t.Errorf("no dist: %d", rec.Code)
	}
}

func TestHouse_NeverLeaksTokenOrHAURL(t *testing.T) {
	const haURL, token = "https://ha.example.invalid", "tok-123"
	leak := errors.New(`Get "` + haURL + `/api/states?token=` + token + `": dial tcp: no such host (Bearer ` + token + `)`)
	check := func(t *testing.T, what string, rec *httptest.ResponseRecorder) {
		t.Helper()
		dump := rec.Body.String()
		for k, v := range rec.Header() {
			dump += k + ": " + strings.Join(v, ",") + "\n"
		}
		if strings.Contains(dump, haURL) || strings.Contains(dump, "ha.example.invalid") || strings.Contains(dump, token) {
			t.Errorf("%s leaks HA details: %s", what, dump)
		}
	}
	requests := func(mux http.Handler) map[string]*httptest.ResponseRecorder {
		cmd := `{"kind":"light","id":"a.lamp","patch":{"on":true}}`
		return map[string]*httptest.ResponseRecorder{
			"GET plan":       do(mux, http.MethodGet, "/house/plan", ""),
			"PUT plan bad":   do(mux, http.MethodPut, "/house/plan", `{"rooms":[]}`, "Authorization", "Bearer "+testSecret),
			"PUT plan auth":  do(mux, http.MethodPut, "/house/plan", fixturePlan, "Authorization", "Bearer "+token),
			"GET state":      do(mux, http.MethodGet, "/house/state", ""),
			"POST command":   command(mux, cmd),
			"dry run":        do(mux, http.MethodPost, "/house/command?dry_run=1", cmd, "Content-Type", "application/json"),
			"bad command":    command(mux, `{"kind":"light","id":"a.lamp","patch":{"x":1}}`),
			"page":           do(mux, http.MethodGet, "/house/", ""),
			"asset":          do(mux, http.MethodGet, "/house/assets/a.js", ""),
			"missing asset":  do(mux, http.MethodGet, "/house/assets/nope.js", ""),
			"command method": do(mux, http.MethodGet, "/house/command", ""),
		}
	}

	t.Run("HA failing", func(t *testing.T) {
		ha := newFakeHA()
		ha.statesErr, ha.templateErr, ha.callErr = leak, leak, leak
		_, mux := newTestHandler(t, ha, true)
		for what, rec := range requests(mux) {
			check(t, what, rec)
		}
	})
	t.Run("HA calls failing", func(t *testing.T) {
		ha := newFakeHA()
		ha.templateErr, ha.callErr = leak, leak
		_, mux := newTestHandler(t, ha, true)
		for what, rec := range requests(mux) {
			check(t, what, rec)
		}
	})
	t.Run("HA fine", func(t *testing.T) {
		_, mux := newTestHandler(t, newFakeHA(), true)
		for what, rec := range requests(mux) {
			check(t, what, rec)
		}
	})
	t.Run("no plan", func(t *testing.T) {
		ha := newFakeHA()
		ha.statesErr = leak
		_, mux := newTestHandler(t, ha, false)
		for what, rec := range requests(mux) {
			check(t, what, rec)
		}
	})
}
