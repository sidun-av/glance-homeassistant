package house

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// The install script is run for real under sh, against an httptest server
// that is the real install handler plus the app download. Only what would
// reach outside the test is stubbed on PATH: launchctl, open, pgrep and
// pkill (a running app), and the account's names. osascript is the real one
// for JXA; anything else it is asked is logged, and nothing should be.

const (
	stubFullName = "Demo User"
	stubComputer = "Demo Mac"
	appBundle    = "House Control.app"
	appBundleID  = "com.example.HouseControl"
)

var scriptStubs = map[string]string{
	// JXA is a real dependency of the script; AppleScript is not wanted at
	// all (it can raise an Automation dialog).
	"osascript": `if [ "$1" = -l ]; then exec /usr/bin/osascript "$@"; fi
echo "osascript $*" >> "$STUB_LOG"`,
	// pgrep reports a running app while $STUB_DIR/running exists; pkill
	// stops it.
	"pgrep": `echo "pgrep $*" >> "$STUB_LOG"
[ -f "$STUB_DIR/running" ] && echo 4242`,
	"pkill": `echo "pkill $*" >> "$STUB_LOG"
rm -f "$STUB_DIR/running"`,
	"open": `echo "open $*" >> "$STUB_LOG"`,
	// launchctl knows no loaded job; bootstrap fails while
	// $STUB_DIR/bootstrap-fails counts down.
	"launchctl": `echo "launchctl $*" >> "$STUB_LOG"
case $1 in
print) exit 113 ;;
bootstrap)
	n=$(cat "$STUB_DIR/bootstrap-fails" 2>/dev/null || echo 0)
	if [ "$n" -gt 0 ]; then
		echo $((n - 1)) > "$STUB_DIR/bootstrap-fails"
		echo "Bootstrap failed: 5: Input/output error" >&2
		exit 5
	fi ;;
esac`,
	"id":     `case "$1" in -F) echo '` + stubFullName + `' ;; *) exec /usr/bin/id "$@" ;; esac`,
	"scutil": `echo '` + stubComputer + `'`,
	// sw_vers reports $STUB_DIR/macos-version, else a current macOS.
	"sw_vers": `cat "$STUB_DIR/macos-version" 2>/dev/null || echo 15.1`,
}

type scriptHarness struct {
	t                      *testing.T
	h                      *Handler
	mux                    *http.ServeMux
	persons                *fakePersons
	srv                    *httptest.Server
	home, apps, stubs, log string
	zip                    []byte
	sha                    string
	mu                     sync.Mutex
	plan                   http.HandlerFunc // replaces GET /house/plan when set
	keyStatus              int              // answers POST …/key with this status when set
}

func newScriptHarness(t *testing.T) *scriptHarness {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("the install script runs on macOS only")
	}
	h, mux, persons, _ := newInstallHandler(t)
	root := t.TempDir()
	s := &scriptHarness{t: t, h: h, mux: mux, persons: persons,
		home: filepath.Join(root, "home"), apps: filepath.Join(root, "Apps"),
		stubs: filepath.Join(root, "stubs"), log: filepath.Join(root, "stub.log")}
	for _, dir := range []string{s.home, s.apps, s.stubs, filepath.Join(root, "tmp")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range scriptStubs {
		if err := os.WriteFile(filepath.Join(s.stubs, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	s.zip = appZip(t, "v1")
	sum := sha256.Sum256(s.zip)
	s.sha = hex.EncodeToString(sum[:])
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		plan, keyStatus, zip := s.plan, s.keyStatus, s.zip
		s.mu.Unlock()
		switch {
		case r.URL.Path == "/app.zip":
			w.Write(zip)
		case r.URL.Path == "/house/plan" && plan != nil:
			plan(w, r)
		case strings.HasSuffix(r.URL.Path, "/key") && keyStatus != 0:
			writeError(w, keyStatus, "forbidden")
		default:
			mux.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(s.srv.Close)
	return s
}

// appZip is a release zip as ditto makes it: a fake "House Control.app"
// whose executable says which build it is.
func appZip(t *testing.T, build string) []byte {
	t.Helper()
	dir := t.TempDir()
	exe := filepath.Join(dir, appBundle, "Contents", "MacOS", "HouseControl")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("#!/bin/sh\necho "+build+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	info := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>CFBundleIdentifier</key><string>` + appBundleID + `</string></dict></plist>
`
	if err := os.WriteFile(filepath.Join(dir, appBundle, "Contents", "Info.plist"), []byte(info), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "app.zip")
	if b, err := exec.Command("ditto", "-c", "-k", "--keepParent", filepath.Join(dir, appBundle), out).CombinedOutput(); err != nil {
		t.Fatalf("ditto: %v %s", err, b)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func (s *scriptHarness) base() string { return s.srv.URL + "/house" }

// script renders the install script for a new code, with the app's hash
// (or another one).
func (s *scriptHarness) script(sha string) string {
	s.t.Helper()
	var out bytes.Buffer
	vars := installVars{Base: s.base(), Code: newCode(s.t, s.mux), AppURL: s.srv.URL + "/app.zip", AppSHA256: sha}
	if err := installScript.Execute(&out, vars); err != nil {
		s.t.Fatal(err)
	}
	return out.String()
}

func (s *scriptHarness) env() []string {
	return []string{
		"HOME=" + s.home,
		"HC_APPLICATIONS_DIR=" + s.apps,
		"HC_LAUNCH_DAEMONS_DIR=" + filepath.Join(filepath.Dir(s.home), "LaunchDaemons"),
		"TMPDIR=" + filepath.Join(filepath.Dir(s.home), "tmp"),
		"PATH=" + s.stubs + ":/usr/bin:/bin:/usr/sbin:/sbin",
		"STUB_LOG=" + s.log,
		"STUB_DIR=" + s.stubs,
		"LANG=en_US.UTF-8",
	}
}

// run pipes script into sh as `curl … | sh -s <args>` does.
func (s *scriptHarness) run(script string, args ...string) (string, error) {
	s.t.Helper()
	cmd := exec.Command("sh", append([]string{"-s"}, args...)...)
	cmd.Stdin = strings.NewReader(script)
	cmd.Env = s.env()
	cmd.Dir = s.home
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (s *scriptHarness) pull() (string, error) {
	s.t.Helper()
	cmd := exec.Command("/bin/sh", s.data("plan-pull.sh"))
	cmd.Env = s.env()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (s *scriptHarness) data(name string) string {
	return filepath.Join(s.home, "Library", "Application Support", "HouseControl", name)
}

func (s *scriptHarness) agent(label string) string {
	return filepath.Join(s.home, "Library", "LaunchAgents", label+".plist")
}

func (s *scriptHarness) stubLog() string {
	b, _ := os.ReadFile(s.log)
	return string(b)
}

func (s *scriptHarness) install() string {
	s.t.Helper()
	out, err := s.run(s.script(s.sha))
	if err != nil {
		s.t.Fatalf("install: %v\n%s", err, out)
	}
	return out
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("%s: %v: %s", path, err, b)
	}
	return v
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func appBuild(t *testing.T, app string) string {
	t.Helper()
	b, err := exec.Command(filepath.Join(app, "Contents", "MacOS", "HouseControl")).Output()
	if err != nil {
		t.Fatalf("run installed app: %v", err)
	}
	return strings.TrimSpace(string(b))
}

func TestInstallScript_Installs(t *testing.T) {
	s := newScriptHarness(t)
	// What the app already keeps in settings.json stays.
	if err := os.MkdirAll(filepath.Dir(s.data("settings.json")), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.data("settings.json"), []byte(`{"plan":"home","sidebar":{"open":true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// An older build is installed and running.
	if err := os.MkdirAll(filepath.Join(s.apps, appBundle, "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.stubs, "running"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	out := s.install()
	if !strings.Contains(out, "Готово: House Control в строке меню.") {
		t.Errorf("no final line:\n%s", out)
	}
	if s.persons.count() != 1 || s.persons.calls[0] != [2]string{stubFullName, stubComputer} {
		t.Errorf("keys minted: %v", s.persons.calls)
	}

	ha := readJSON(t, s.data("homeassistant.json"))
	if ha["url"] != testHAURL || ha["token"] != testKeyToken || ha["name"] != "Home Assistant" {
		t.Errorf("homeassistant.json = %v", ha)
	}
	if fi, err := os.Stat(s.data("homeassistant.json")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("homeassistant.json mode: %v %v", fi.Mode(), err)
	}
	settings := readJSON(t, s.data("settings.json"))
	if settings["plan"] != "custom" || settings["lang"] != "ru" || settings["sidebar"] == nil {
		t.Errorf("settings.json = %v", settings)
	}
	plan := readJSON(t, s.data("plan.json"))
	if plan["name"] != "Demo" || plan["rooms"] == nil {
		t.Errorf("plan.json = %v", plan)
	}
	if b, _ := os.ReadFile(s.data("plan.etag")); len(bytes.TrimSpace(b)) == 0 {
		t.Error("no plan.etag")
	}

	app := filepath.Join(s.apps, appBundle)
	if appBuild(t, app) != "v1" {
		t.Error("the new build is not installed")
	}
	entries, _ := os.ReadDir(s.apps)
	if len(entries) != 1 {
		t.Errorf("apps dir holds more than the app: %v", entries)
	}

	log := s.stubLog()
	uid := strings.TrimSpace(string(must(exec.Command("id", "-u").Output())))
	for _, want := range []string{
		"pkill -TERM -U " + uid + " -f /House Control.app/Contents/MacOS/HouseControl",
		"launchctl bootstrap gui/" + uid + " " + s.agent("local.house-control.plan-pull"),
		"launchctl bootstrap gui/" + uid + " " + s.agent("local.house-control.open"),
		"open -a " + app,
	} {
		if !strings.Contains(log, want) {
			t.Errorf("stub log lacks %q:\n%s", want, log)
		}
	}
	pull, _ := os.ReadFile(s.agent("local.house-control.plan-pull"))
	for _, want := range []string{"<key>RunAtLoad</key>", "<integer>600</integer>", s.data("plan-pull.sh")} {
		if !strings.Contains(string(pull), want) {
			t.Errorf("plan-pull agent lacks %q:\n%s", want, pull)
		}
	}
	open, _ := os.ReadFile(s.agent("local.house-control.open"))
	if !strings.Contains(string(open), "<string>"+app+"</string>") || strings.Contains(string(open), "StartInterval") {
		t.Errorf("open agent:\n%s", open)
	}
	for _, f := range []string{s.agent("local.house-control.plan-pull"), s.agent("local.house-control.open")} {
		if b, err := exec.Command("plutil", "-lint", f).CombinedOutput(); err != nil {
			t.Errorf("plutil: %s", b)
		}
		// macOS's "Background Items Added" notice names the app, read from
		// the downloaded bundle.
		if b, err := exec.Command("/usr/libexec/PlistBuddy", "-c", "Print AssociatedBundleIdentifiers:0", f).Output(); err != nil || strings.TrimSpace(string(b)) != appBundleID {
			t.Errorf("%s: AssociatedBundleIdentifiers = %q (%v), want [%s]", f, b, err, appBundleID)
		}
	}
	notice := "macOS покажет уведомление «Добавлены фоновые объекты» — это House Control, так и должно быть."
	if i, j := strings.Index(out, notice), strings.Index(out, "Готово"); i < 0 || i > j {
		t.Errorf("no Background Items line before the end:\n%s", out)
	}

	puller, _ := os.ReadFile(s.data("plan-pull.sh"))
	if !strings.Contains(string(puller), "BASE='"+s.base()+"'") {
		t.Errorf("puller has no BASE:\n%s", puller)
	}
	if !strings.Contains(string(puller), "-H 'X-House-Client: mac'") {
		t.Errorf("puller does not say it is a household Mac:\n%s", puller)
	}
	for _, secret := range []string{testKeyToken, "/install/"} {
		if strings.Contains(string(puller), secret) {
			t.Errorf("the puller carries %q", secret)
		}
	}
	if strings.Contains(out, testKeyToken) || strings.Contains(log, testKeyToken) {
		t.Error("the key reached the output or a command line")
	}
	if strings.Contains(log, "osascript") {
		t.Errorf("AppleScript was run:\n%s", log)
	}
	uninstaller, _ := os.ReadFile(s.data("uninstall.sh"))
	if strings.Contains(string(uninstaller), testKeyToken) || strings.Contains(string(uninstaller), "/install/") || !strings.Contains(string(uninstaller), "\nuninstall\n") {
		t.Errorf("uninstall.sh:\n%s", uninstaller)
	}
	if left, _ := os.ReadDir(filepath.Join(filepath.Dir(s.home), "tmp")); len(left) != 0 {
		t.Errorf("temp files left: %v", left)
	}
}

func must(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}

// curlSh runs the command the widget copies: curl -fsSL '<url>' | sh.
func (s *scriptHarness) curlSh(url string) (string, error) {
	s.t.Helper()
	cmd := exec.Command("sh", "-c", "curl -fsSL '"+url+"' | sh")
	cmd.Env = s.env()
	cmd.Dir = s.home
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestInstallScript_SameCommandTwice(t *testing.T) {
	s := newScriptHarness(t)
	s.h.AppURL, s.h.AppSHA256 = s.srv.URL+"/app.zip", s.sha
	url := s.srv.URL + testBase + "/install/" + newCode(t, s.mux)
	if out, err := s.curlSh(url); err != nil {
		t.Fatalf("first run: %v\n%s", err, out)
	}
	ha, _ := os.ReadFile(s.data("homeassistant.json"))
	os.Remove(s.log)

	out, err := s.curlSh(url)
	if err == nil {
		t.Fatalf("second run succeeded:\n%s", out)
	}
	host := strings.TrimPrefix(s.srv.URL, "http://")
	if strings.TrimSpace(out) != "Ссылка устарела — откройте "+host+" и скопируйте команду заново." {
		t.Errorf("second run, want only the expired line:\n%s", out)
	}
	if again, _ := os.ReadFile(s.data("homeassistant.json")); !bytes.Equal(again, ha) {
		t.Error("homeassistant.json changed")
	}
	if log := s.stubLog(); log != "" {
		t.Errorf("the second run went on:\n%s", log)
	}
	if s.persons.count() != 1 {
		t.Errorf("minted %d keys, want 1", s.persons.count())
	}
}

// The script's own answer to a code used up between fetching the script
// and asking for the key.
func TestInstallScript_CodeUsedMeanwhile(t *testing.T) {
	s := newScriptHarness(t)
	script := s.script(s.sha)
	if out, err := s.run(script); err != nil {
		t.Fatalf("first run: %v\n%s", err, out)
	}
	out, err := s.run(script)
	host := strings.TrimPrefix(s.srv.URL, "http://")
	if err == nil || !strings.Contains(out, "Ссылка устарела — откройте "+host+" и скопируйте команду заново.") {
		t.Errorf("second run: %v\n%s", err, out)
	}
}

func TestInstallScript_HashMismatchTouchesNothing(t *testing.T) {
	s := newScriptHarness(t)
	out, err := s.run(s.script(strings.Repeat("0", 64)))
	if err == nil {
		t.Fatalf("installed with a wrong hash:\n%s", out)
	}
	if !strings.Contains(out, "контрольная сумма") {
		t.Errorf("no hash line:\n%s", out)
	}
	for _, dir := range []string{s.home, s.apps, filepath.Join(filepath.Dir(s.home), "tmp")} {
		if entries, _ := os.ReadDir(dir); len(entries) != 0 {
			t.Errorf("%s touched: %v", dir, entries)
		}
	}
	if s.persons.count() != 0 {
		t.Error("a key was minted")
	}
}

func TestInstallScript_NotAtHome(t *testing.T) {
	s := newScriptHarness(t)
	s.mu.Lock()
	s.keyStatus = http.StatusForbidden
	s.mu.Unlock()
	out, err := s.run(s.script(s.sha))
	if err == nil {
		t.Fatalf("installed away from home:\n%s", out)
	}
	if !strings.Contains(out, "Установка работает только из дома: подключитесь к домашнему Wi-Fi и попробуйте снова.") {
		t.Errorf("no not-at-home line:\n%s", out)
	}
	if entries, _ := os.ReadDir(s.home); len(entries) != 0 {
		t.Errorf("HOME touched: %v", entries)
	}
}

func TestInstallScript_UnwritableApplications(t *testing.T) {
	s := newScriptHarness(t)
	if err := os.Chmod(s.apps, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(s.apps, 0o755) })
	s.install()
	app := filepath.Join(s.home, "Applications", appBundle)
	if appBuild(t, app) != "v1" {
		t.Error("not installed into ~/Applications")
	}
	open, _ := os.ReadFile(s.agent("local.house-control.open"))
	if !strings.Contains(string(open), "<string>"+app+"</string>") {
		t.Errorf("open agent does not start ~/Applications' copy:\n%s", open)
	}
	if !exists(s.data("homeassistant.json")) || !exists(s.data("plan.json")) {
		t.Error("the rest of the install is missing")
	}
}

func TestInstallScript_PullerKeepsPlanOnAnythingButAPlan(t *testing.T) {
	s := newScriptHarness(t)
	s.install()
	before, _ := os.ReadFile(s.data("plan.json"))

	answers := map[string]http.HandlerFunc{
		"login page": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<!doctype html><title>Sign in</title>"))
		},
		"redirect": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/login", http.StatusFound)
		},
		"unauthorized": func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) },
		"array":        func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`[{"rooms":[]}]`)) },
		"no rooms":     func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"name":"x"}`)) },
		"truncated":    func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"rooms":[`)) },
	}
	for name, answer := range answers {
		s.mu.Lock()
		s.plan = answer
		s.mu.Unlock()
		out, _ := s.pull()
		if after, _ := os.ReadFile(s.data("plan.json")); !bytes.Equal(after, before) {
			t.Errorf("%s replaced the plan: %s (%s)", name, after, out)
		}
	}

	// Unchanged on the server: a 304, nothing rewritten.
	s.mu.Lock()
	s.plan = nil
	s.mu.Unlock()
	if out, err := s.pull(); err != nil {
		t.Fatalf("pull: %v %s", err, out)
	}
	if after, _ := os.ReadFile(s.data("plan.json")); !bytes.Equal(after, before) {
		t.Error("an unchanged plan was rewritten")
	}

	// A new plan on the server replaces the old one.
	newPlan := strings.Replace(fixturePlan, `"name": "Demo"`, `"name": "Demo 2"`, 1)
	if _, err := s.h.Store.Save([]byte(newPlan)); err != nil {
		t.Fatal(err)
	}
	if out, err := s.pull(); err != nil {
		t.Fatalf("pull: %v %s", err, out)
	}
	if plan := readJSON(t, s.data("plan.json")); plan["name"] != "Demo 2" {
		t.Errorf("plan not updated: %v", plan["name"])
	}
	if b, _ := os.ReadFile(filepath.Join(s.home, "Library", "Logs", "house-control-plan-pull.log")); len(b) == 0 {
		t.Error("the puller logged nothing")
	}
}

func TestInstallScript_Uninstall(t *testing.T) {
	s := newScriptHarness(t)
	script := s.script(s.sha)
	if out, err := s.run(script); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	s.pull() // leaves a log
	out, err := s.run(script, "uninstall")
	if err != nil {
		t.Fatalf("uninstall: %v\n%s", err, out)
	}
	for _, gone := range []string{
		filepath.Join(s.apps, appBundle),
		s.agent("local.house-control.plan-pull"), s.agent("local.house-control.open"),
		s.data("plan-pull.sh"), s.data("plan.etag"), s.data("homeassistant.json"), s.data("uninstall.sh"),
		filepath.Join(s.home, "Library", "Logs", "house-control-plan-pull.log"),
	} {
		if exists(gone) {
			t.Errorf("%s is still there", gone)
		}
	}
	for _, kept := range []string{s.data("plan.json"), s.data("settings.json")} {
		if !exists(kept) {
			t.Errorf("%s was removed", kept)
		}
	}
	if !strings.Contains(out, "plan.json") || !strings.Contains(s.stubLog(), "launchctl bootout") {
		t.Errorf("uninstall output:\n%s\nlog:\n%s", out, s.stubLog())
	}
	if s.persons.count() != 1 {
		t.Error("uninstall asked for a key")
	}
}

func TestInstallScript_LocalUninstaller(t *testing.T) {
	s := newScriptHarness(t)
	s.install()
	cmd := exec.Command("/bin/sh", s.data("uninstall.sh"))
	cmd.Env = s.env()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("uninstall.sh: %v\n%s", err, out)
	}
	for _, gone := range []string{
		filepath.Join(s.apps, appBundle), s.agent("local.house-control.plan-pull"), s.agent("local.house-control.open"),
		s.data("plan-pull.sh"), s.data("homeassistant.json"), s.data("uninstall.sh"),
	} {
		if exists(gone) {
			t.Errorf("%s is still there", gone)
		}
	}
	if !exists(s.data("plan.json")) || !strings.Contains(string(out), "House Control удалён") {
		t.Errorf("uninstall.sh output:\n%s", out)
	}
}

// The owner's Mac runs the plan pusher: installing there would overwrite
// its plan with the pulled one. Nothing is downloaded and no key minted.
func TestInstallScript_RefusesTheOwnersMac(t *testing.T) {
	for name, dir := range map[string]func(s *scriptHarness) string{
		"agent":  func(s *scriptHarness) string { return filepath.Join(s.home, "Library", "LaunchAgents") },
		"daemon": func(s *scriptHarness) string { return filepath.Join(filepath.Dir(s.home), "LaunchDaemons") },
	} {
		t.Run(name, func(t *testing.T) {
			s := newScriptHarness(t)
			plist := filepath.Join(dir(s), "com.glance-homeassistant.house-plan-push.plist")
			if err := os.MkdirAll(filepath.Dir(plist), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(plist, []byte("<plist/>"), 0o644); err != nil {
				t.Fatal(err)
			}
			out, err := s.run(s.script(s.sha))
			if err == nil || strings.TrimSpace(out) != "Этот Mac — главный: на нём план редактируют, сюда устанавливать не нужно." {
				t.Errorf("owner's Mac: %v\n%s", err, out)
			}
			if s.persons.count() != 0 || exists(s.data("plan.json")) || s.stubLog() != "" {
				t.Errorf("went on: %d keys, log:\n%s", s.persons.count(), s.stubLog())
			}
		})
	}
}

func TestInstallScript_MacOSVersion(t *testing.T) {
	s := newScriptHarness(t)
	if err := os.WriteFile(filepath.Join(s.stubs, "macos-version"), []byte("12.7.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := s.run(s.script(s.sha))
	if err == nil || !strings.Contains(out, "House Control работает на macOS 13 и новее, а на этом Mac macOS 12.7.4.") {
		t.Errorf("macOS 12: %v\n%s", err, out)
	}
	if s.persons.count() != 0 || exists(s.data("plan.json")) {
		t.Error("macOS 12 went on")
	}

	if err := os.WriteFile(filepath.Join(s.stubs, "macos-version"), []byte("13.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.install()
}

// launchd takes a moment to drop a booted-out job; a bootstrap refused
// meanwhile is tried again, quietly.
func TestInstallScript_BootstrapRetried(t *testing.T) {
	s := newScriptHarness(t)
	if err := os.WriteFile(filepath.Join(s.stubs, "bootstrap-fails"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := s.install()
	if strings.Contains(out, "Bootstrap failed") || strings.Count(s.stubLog(), "launchctl bootstrap") != 3 {
		t.Errorf("output:\n%s\nlog:\n%s", out, s.stubLog())
	}

	s2 := newScriptHarness(t)
	if err := os.WriteFile(filepath.Join(s2.stubs, "bootstrap-fails"), []byte("9\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := s2.run(s2.script(s2.sha))
	if err == nil || strings.Contains(out, "Bootstrap failed") || !strings.Contains(out, "Не удалось включить автозапуск") {
		t.Errorf("a bootstrap that keeps failing: %v\n%s", err, out)
	}
}

// No English from curl: a server that cannot be reached is one Russian line.
func TestInstallScript_UnreachableServerSaysSoInRussian(t *testing.T) {
	s := newScriptHarness(t)
	script := s.script(s.sha)
	s.srv.Close()
	out, err := s.run(script)
	if err == nil || strings.Contains(out, "curl") || !strings.Contains(out, "Не удалось скачать приложение") {
		t.Errorf("%v\n%s", err, out)
	}
}
