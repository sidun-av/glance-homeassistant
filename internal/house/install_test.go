package house

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testAppURL    = "https://example.invalid/releases/HouseControl.zip"
	testAppSHA256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	testHAURL     = "http://ha.example.invalid:8123"
	testKeyToken  = "demo-person-token"
)

// fakePersons stands in for Home Assistant's user and token minting.
type fakePersons struct {
	mu    sync.Mutex
	err   error
	calls [][2]string
}

func (f *fakePersons) MintPersonKey(ctx context.Context, person, computer string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, [2]string{person, computer})
	if f.err != nil {
		return "", f.err
	}
	return testKeyToken, nil
}

func (f *fakePersons) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// newInstallHandler is newTestHandler with installs enabled and a clock
// the test moves.
func newInstallHandler(t *testing.T) (*Handler, *http.ServeMux, *fakePersons, *time.Time) {
	t.Helper()
	h, mux := newTestHandler(t, newFakeHA(), true)
	persons := &fakePersons{}
	h.Persons, h.HAURL, h.AppURL, h.AppSHA256 = persons, testHAURL, testAppURL, testAppSHA256
	// example.com is httptest's default Host.
	h.InstallHosts = []string{"example.com", "dash.example.com", "other.example"}
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	h.clock = func() time.Time { return now }
	return h, mux, persons, &now
}

var codeRe = regexp.MustCompile(`^[a-z2-7]{26}$`)

func newCode(t *testing.T, mux http.Handler) string {
	t.Helper()
	rec := do(mux, http.MethodPost, "/ha-widget/house/install/code", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("POST code: %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("POST code body %q: %v", rec.Body.String(), err)
	}
	code := body["code"]
	if !codeRe.MatchString(code) {
		t.Fatalf("code = %q, want 26 lowercase base32 characters", code)
	}
	if body["path"] != testBase+"/install/"+code {
		t.Errorf("path = %q, want %s/install/%s", body["path"], testBase, code)
	}
	if _, ok := body["command"]; ok {
		t.Error("the response carries a command; the widget builds it from path")
	}
	return code
}

func key(mux http.Handler, code string, headers ...string) *httptest.ResponseRecorder {
	return do(mux, http.MethodPost, "/ha-widget/house/install/"+code+"/key",
		`{"person":"  Demo Person ","computer":"Demo MacBook"}`,
		append([]string{"Content-Type", "application/json"}, headers...)...)
}

func TestInstall_CodeExpires(t *testing.T) {
	_, mux, persons, now := newInstallHandler(t)
	code := newCode(t, mux)
	if rec := do(mux, http.MethodGet, "/ha-widget/house/install/"+code, ""); rec.Code != http.StatusOK {
		t.Fatalf("fresh script: %d", rec.Code)
	}
	*now = now.Add(30*time.Minute + time.Second)
	expiredStub(t, do(mux, http.MethodGet, "/ha-widget/house/install/"+code, ""), code)
	rec := key(mux, code)
	if rec.Code != http.StatusGone || errorBody(t, rec)["error"] != "expired" {
		t.Errorf("expired key: %d %s, want 410 expired", rec.Code, rec.Body.String())
	}
	if persons.count() != 0 {
		t.Error("an expired code minted a key")
	}
}

func TestInstall_CodeSingleUse(t *testing.T) {
	_, mux, persons, _ := newInstallHandler(t)
	code := newCode(t, mux)
	// Fetching the script does not use the code up.
	for i := 0; i < 2; i++ {
		if rec := do(mux, http.MethodGet, "/house/install/"+code, ""); rec.Code != http.StatusOK {
			t.Fatalf("script fetch %d: %d", i, rec.Code)
		}
	}
	rec := key(mux, code)
	if rec.Code != http.StatusOK {
		t.Fatalf("key: %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	var body struct {
		HomeAssistant map[string]string `json:"homeassistant"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	want := map[string]string{"url": testHAURL, "token": testKeyToken, "name": "Home Assistant"}
	if len(body.HomeAssistant) != 3 || body.HomeAssistant["url"] != want["url"] || body.HomeAssistant["token"] != want["token"] || body.HomeAssistant["name"] != want["name"] {
		t.Errorf("key body = %s", rec.Body.String())
	}
	if persons.calls[0] != [2]string{"Demo Person", "Demo MacBook"} {
		t.Errorf("minted for %q, want trimmed names", persons.calls[0])
	}

	again := key(mux, code)
	if again.Code != http.StatusGone || errorBody(t, again)["error"] != "expired" {
		t.Errorf("second key: %d %s, want 410 expired", again.Code, again.Body.String())
	}
	expiredStub(t, do(mux, http.MethodGet, "/house/install/"+code, ""), code)
	expiredStub(t, do(mux, http.MethodGet, "/house/install/"+strings.Repeat("a", 26), ""), "")
	if persons.count() != 1 {
		t.Errorf("minted %d keys, want 1", persons.count())
	}
	if rec := key(mux, "aaaaaaaaaaaaaaaaaaaaaaaaaa"); rec.Code != http.StatusGone {
		t.Errorf("unknown code: %d, want 410", rec.Code)
	}
}

// expiredStub checks the answer to a used, expired or unknown code: a
// script that sends the person back to the dashboard and fails, with
// neither the code nor the app in it.
func expiredStub(t *testing.T, rec *httptest.ResponseRecorder, code string) {
	t.Helper()
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("stale code: %d %q, want a 200 text/plain script", rec.Code, rec.Header().Get("Content-Type"))
	}
	script := rec.Body.String()
	if !strings.HasPrefix(script, "#!/bin/sh\n") || !strings.Contains(script, "'Ссылка устарела — откройте example.com и скопируйте команду заново.' >&2") || !strings.Contains(script, "\nexit 1\n") {
		t.Errorf("stale code script:\n%s", script)
	}
	for _, secret := range []string{testAppURL, testAppSHA256, "CODE=", "/install/"} {
		if strings.Contains(script, secret) {
			t.Errorf("stale code script carries %q", secret)
		}
	}
	if code != "" && strings.Contains(script, code) {
		t.Error("stale code script carries the code")
	}
}

func TestInstall_StaleCodeStillNeedsThisHost(t *testing.T) {
	_, mux, _, _ := newInstallHandler(t)
	rec := do(mux, http.MethodGet, "/house/install/"+strings.Repeat("a", 26), "", "Host", "evil.example")
	if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "evil") {
		t.Errorf("foreign Host: %d %s, want 403", rec.Code, rec.Body.String())
	}
}

func TestKey_RefusesPublicClient(t *testing.T) {
	_, mux, persons, _ := newInstallHandler(t)
	code := newCode(t, mux)
	for name, headers := range map[string][]string{
		"public real ip":          {"X-Real-IP", "203.0.113.9"},
		"forwarded, no real ip":   {"X-Forwarded-For", "192.168.0.20"},
		"rfc 7239, no real ip":    {"Forwarded", "for=192.168.0.20"},
		"public peer":             {"RemoteAddr", "203.0.113.9:4000"},
		"garbage real ip":         {"X-Real-IP", "not-an-ip"},
		"public real ip, lan xff": {"X-Real-IP", "203.0.113.9", "X-Forwarded-For", "192.168.0.20"},
	} {
		rec := key(mux, code, headers...)
		if rec.Code != http.StatusForbidden || errorBody(t, rec)["error"] != "forbidden" {
			t.Errorf("%s: %d %s, want 403 forbidden", name, rec.Code, rec.Body.String())
		}
	}
	if persons.count() != 0 {
		t.Fatal("a refused request minted a key")
	}
	if rec := key(mux, code); rec.Code != http.StatusOK {
		t.Errorf("after the refusals the code is gone: %d %s", rec.Code, rec.Body.String())
	}
}

func TestKey_AllowsLANThroughProxy(t *testing.T) {
	_, mux, _, _ := newInstallHandler(t)
	code := newCode(t, mux)
	rec := key(mux, code, "X-Real-IP", "192.168.0.20", "X-Forwarded-For", "192.168.0.20")
	if rec.Code != http.StatusOK {
		t.Errorf("LAN client through the proxy: %d %s", rec.Code, rec.Body.String())
	}
}

func TestKey_ValidatesNames(t *testing.T) {
	_, mux, persons, _ := newInstallHandler(t)
	code := newCode(t, mux)
	long := strings.Repeat("я", 81)
	for _, body := range []string{
		`{"person":"","computer":"Mac"}`,
		`{"person":"   ","computer":"Mac"}`,
		`{"person":"Demo","computer":""}`,
		`{"person":"` + long + `","computer":"Mac"}`,
		`{"person":"Demo\u0007","computer":"Mac"}`,
		`{"person":1,"computer":"Mac"}`,
		`not json`,
	} {
		rec := do(mux, http.MethodPost, "/house/install/"+code+"/key", body, "Content-Type", "application/json")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s, want 400", body, rec.Code, rec.Body.String())
		}
	}
	if rec := do(mux, http.MethodPost, "/house/install/"+code+"/key", `{"person":"Demo","computer":"Mac"}`); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("no JSON content type: %d", rec.Code)
	}
	ok := `{"person":"` + strings.Repeat("я", 80) + `","computer":"Mac"}`
	if rec := do(mux, http.MethodPost, "/house/install/"+code+"/key", ok, "Content-Type", "application/json"); rec.Code != http.StatusOK {
		t.Errorf("80 runes: %d %s", rec.Code, rec.Body.String())
	}
	if persons.count() != 1 {
		t.Errorf("minted %d keys, want 1", persons.count())
	}
}

func TestKey_MintFailureKeepsTheCode(t *testing.T) {
	_, mux, persons, _ := newInstallHandler(t)
	code := newCode(t, mux)
	persons.err = errors.New(`dial "` + testHAURL + `": refused`)
	rec := key(mux, code)
	if rec.Code != http.StatusBadGateway || strings.Contains(rec.Body.String(), "example.invalid") {
		t.Errorf("mint failure: %d %s, want 502 with a bare code", rec.Code, rec.Body.String())
	}
	persons.err = nil
	if rec := key(mux, code); rec.Code != http.StatusOK {
		t.Errorf("retry after a failure: %d %s", rec.Code, rec.Body.String())
	}
}

func TestInstall_ScriptHasNoSecrets(t *testing.T) {
	h, mux, _, _ := newInstallHandler(t)
	h.Secret = "upload-secret-xyz"
	code := newCode(t, mux)
	rec := do(mux, http.MethodGet, "/ha-widget/house/install/"+code, "",
		"X-Forwarded-Proto", "https", "X-Real-IP", "192.168.0.20")
	if rec.Code != http.StatusOK {
		t.Fatalf("script: %d %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q", ct)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	script := rec.Body.String()
	if !strings.HasPrefix(script, "#!/bin/sh\n") {
		t.Errorf("script does not start with a shebang: %.40q", script)
	}
	for _, want := range []string{
		"BASE='https://example.com" + testBase + "'",
		"CODE='" + code + "'",
		"APP_URL='" + testAppURL + "'",
		"APP_SHA256='" + testAppSHA256 + "'",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("script lacks %s", want)
		}
	}
	check := exec.Command("sh", "-n")
	check.Stdin = strings.NewReader(script)
	if out, err := check.CombinedOutput(); err != nil {
		t.Errorf("sh -n: %v %s", err, out)
	}
	for _, secret := range []string{testKeyToken, "upload-secret-xyz", testHAURL, "ha.example.invalid"} {
		if strings.Contains(script, secret) {
			t.Errorf("script carries %q", secret)
		}
	}
}

func TestInstall_ScriptBaseFromRequest(t *testing.T) {
	h, mux, _, _ := newInstallHandler(t)
	code := newCode(t, mux)
	get := func(headers ...string) *httptest.ResponseRecorder {
		return do(mux, http.MethodGet, "/house/install/"+code, "", headers...)
	}
	if s := get("Host", "dash.example.com:8443").Body.String(); !strings.Contains(s, "BASE='http://dash.example.com:8443"+testBase+"'") {
		t.Errorf("plain http with a port: %s", s)
	}
	if s := get("X-Forwarded-Proto", "gopher").Body.String(); !strings.Contains(s, "BASE='http://example.com"+testBase+"'") {
		t.Errorf("an unknown scheme is ignored: %s", s)
	}
	for _, host := range []string{"evil.com'; rm -rf ~; '", "a b", "$(id)"} {
		if rec := get("Host", host); rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "rm -rf") {
			t.Errorf("host %q: %d %s, want 403", host, rec.Code, rec.Body.String())
		}
	}
	// An origin public_url: Base is already absolute.
	h.Base = "http://192.0.2.1:8081/house"
	if s := get("Host", "other.example").Body.String(); !strings.Contains(s, "BASE='http://192.0.2.1:8081/house'") {
		t.Errorf("absolute Base: %s", s)
	}
}

func TestInstall_DisabledWithoutApp(t *testing.T) {
	cases := map[string]func(h *Handler){
		"no app url":  func(h *Handler) { h.AppURL = "" },
		"no app hash": func(h *Handler) { h.AppSHA256 = "" },
		"no persons":  func(h *Handler) { h.Persons = nil },
		"no plan":     func(h *Handler) { h.Store = OpenStore(t.TempDir() + "/none.json") },
	}
	for name, disable := range cases {
		t.Run(name, func(t *testing.T) {
			h, mux, persons, _ := newInstallHandler(t)
			code := newCode(t, mux)
			disable(h)
			for what, rec := range map[string]*httptest.ResponseRecorder{
				"code":   do(mux, http.MethodPost, "/house/install/code", ""),
				"script": do(mux, http.MethodGet, "/house/install/"+code, ""),
				"key":    key(mux, code),
			} {
				if rec.Code != http.StatusNotFound || errorBody(t, rec)["error"] != "not_found" {
					t.Errorf("%s: %d %s, want 404 not_found", what, rec.Code, rec.Body.String())
				}
			}
			if persons.count() != 0 {
				t.Error("a disabled install minted a key")
			}
		})
	}
}

func TestInstall_CodeRateLimited(t *testing.T) {
	_, mux, _, now := newInstallHandler(t)
	for i := 0; i < 10; i++ {
		newCode(t, mux)
	}
	rec := do(mux, http.MethodPost, "/house/install/code", "")
	if rec.Code != http.StatusTooManyRequests || errorBody(t, rec)["error"] != "rate_limited" {
		t.Errorf("11th code in a minute: %d %s, want 429", rec.Code, rec.Body.String())
	}
	*now = now.Add(6 * time.Second)
	newCode(t, mux)
}

func TestInstall_ExpiredCodesPruned(t *testing.T) {
	h, mux, _, now := newInstallHandler(t)
	newCode(t, mux)
	newCode(t, mux)
	*now = now.Add(31 * time.Minute)
	newCode(t, mux)
	h.installMu.Lock()
	n := len(h.installCodes)
	h.installMu.Unlock()
	if n != 1 {
		t.Errorf("%d codes kept, want only the live one", n)
	}
}

func TestInstall_Methods(t *testing.T) {
	_, mux, _, _ := newInstallHandler(t)
	code := newCode(t, mux)
	for target, method := range map[string]string{
		"/house/install/code":             http.MethodGet,
		"/house/install/" + code:          http.MethodPost,
		"/house/install/" + code + "/key": http.MethodGet,
	} {
		if rec := do(mux, method, target, ""); rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: %d, want 405", method, target, rec.Code)
		}
	}
	for _, target := range []string{"/house/install/", "/house/install/" + code + "/nope", "/house/install/NOT-A-CODE"} {
		if rec := do(mux, http.MethodGet, target, ""); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: %d, want 404", target, rec.Code)
		}
	}
}

func TestPlan_BatteryHint(t *testing.T) {
	plain := newFakeHA()
	plain.template = ""
	_, plainMux := newTestHandler(t, plain, true)
	plainRec := do(plainMux, http.MethodGet, "/house/plan", "")

	h, mux := newTestHandler(t, newFakeHA(), true)
	rec := do(mux, http.MethodGet, "/house/plan", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET plan: %d", rec.Code)
	}
	var served map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &served); err != nil {
		t.Fatal(err)
	}
	link := served["vacuums"].([]any)[0].(map[string]any)["link"].(map[string]any)
	if link["battery"] != "sensor.demo_robot_battery" || link["id"] != "vacuum.demo_robot" {
		t.Errorf("vacuum link = %v, want the battery sensor added", link)
	}
	etag := rec.Header().Get("ETag")
	if etag == "" || etag == plainRec.Header().Get("ETag") || etag == h.Store.Current().ETag {
		t.Errorf("ETag %q must differ from the plain plan's %q", etag, plainRec.Header().Get("ETag"))
	}
	if strings.Contains(string(h.Store.Current().Served()), "battery") {
		t.Error("the stored plan was changed")
	}
	if again := do(mux, http.MethodGet, "/house/plan", "", "If-None-Match", etag); again.Code != http.StatusNotModified {
		t.Errorf("If-None-Match with the hinted ETag: %d", again.Code)
	}
	// The plan pusher asks with the ETag of the bytes it uploaded.
	if again := do(mux, http.MethodGet, "/house/plan", "", "If-None-Match", h.Store.Current().ETag); again.Code != http.StatusNotModified {
		t.Errorf("If-None-Match with the uploaded plan's ETag: %d", again.Code)
	}
	// A household Mac's puller that still holds the plain plan (pulled
	// before the sensor was known) gets the hinted one.
	pulled := do(mux, http.MethodGet, "/house/plan", "", "If-None-Match", h.Store.Current().ETag, "X-House-Client", "mac")
	if pulled.Code != http.StatusOK || pulled.Header().Get("ETag") != etag || !strings.Contains(pulled.Body.String(), "sensor.demo_robot_battery") {
		t.Errorf("the puller with the plain ETag: %d %q, want 200 with the hinted plan", pulled.Code, pulled.Header().Get("ETag"))
	}
	if again := do(mux, http.MethodGet, "/house/plan", "", "If-None-Match", etag, "X-House-Client", "mac"); again.Code != http.StatusNotModified {
		t.Errorf("the puller with the hinted ETag: %d, want 304", again.Code)
	}
	// The panel reloads when planVersion differs from the plan's ETag.
	state := do(mux, http.MethodGet, "/house/state", "")
	var body map[string]any
	json.Unmarshal(state.Body.Bytes(), &body)
	if body["planVersion"] != etag {
		t.Errorf("planVersion = %v, want the served ETag %s", body["planVersion"], etag)
	}
}

// A page on another site whose name was re-pointed at this server (DNS
// rebinding) is same-origin with it in the browser; only Host and Origin
// tell it apart.
func TestInstall_RefusesForeignHost(t *testing.T) {
	_, mux, persons, _ := newInstallHandler(t)
	code := newCode(t, mux)
	forbidden := func(t *testing.T, what string, rec *httptest.ResponseRecorder) {
		t.Helper()
		if rec.Code != http.StatusForbidden || errorBody(t, rec)["error"] != "forbidden" {
			t.Errorf("%s: %d %s, want 403 forbidden", what, rec.Code, rec.Body.String())
		}
	}
	foreign := []string{"Host", "attacker.example"}
	forbidden(t, "POST code", do(mux, http.MethodPost, "/house/install/code", "", foreign...))
	forbidden(t, "GET script", do(mux, http.MethodGet, "/house/install/"+code, "", foreign...))
	forbidden(t, "POST key", key(mux, code, foreign...))
	forbidden(t, "POST key, foreign host with the server's port", key(mux, code, "Host", "attacker.example:8081"))
	for _, origin := range []string{"http://attacker.example", "https://attacker.example:443", "null", "file://", "ftp://example.com"} {
		forbidden(t, "POST code, Origin "+origin, do(mux, http.MethodPost, "/house/install/code", "", "Origin", origin))
		forbidden(t, "POST key, Origin "+origin, key(mux, code, "Origin", origin))
	}
	if persons.count() != 0 {
		t.Fatal("a refused request minted a key")
	}

	for name, headers := range map[string][]string{
		"allowlisted host, any case":     {"Host", "Dash.Example.COM:8443", "Origin", "https://dash.example.com:8443"},
		"IPv4 literal with port":         {"Host", "192.0.2.1:8081", "Origin", "http://192.0.2.1:8081"},
		"IPv6 literal":                   {"Host", "[2001:db8::1]:8081"},
		"no Origin (curl)":               nil,
		"IP-literal Origin, listed Host": {"Origin", "http://192.0.2.1:8081"},
	} {
		code := newCode(t, mux)
		if rec := do(mux, http.MethodGet, "/house/install/"+code, "", headers...); rec.Code != http.StatusOK {
			t.Errorf("%s: script %d %s", name, rec.Code, rec.Body.String())
		}
		if rec := key(mux, code, headers...); rec.Code != http.StatusOK {
			t.Errorf("%s: key %d %s", name, rec.Code, rec.Body.String())
		}
	}
	// The refusals left the first code usable.
	if rec := key(mux, code); rec.Code != http.StatusOK {
		t.Errorf("after the refusals: %d %s", rec.Code, rec.Body.String())
	}
}
