package houseplanpush

import (
	"bytes"
	"io"
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

const secret = "s3cret"

type request struct {
	method, path, auth, contentType string
	body                            []byte
}

type recorder struct {
	mu     sync.Mutex
	reqs   []request
	status int
}

func (rec *recorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	rec.mu.Lock()
	rec.reqs = append(rec.reqs, request{r.Method, r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Content-Type"), body})
	status := rec.status
	rec.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if status != 0 {
		w.WriteHeader(status)
		io.WriteString(w, `{"error":"save_failed"}`+"\n")
		return
	}
	io.WriteString(w, `{"ok":true,"etag":"\"0123456789abcdef\""}`+"\n")
}

func (rec *recorder) count() int {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return len(rec.reqs)
}

type env struct {
	t                            *testing.T
	home, plan, conf, state, tmp string
}

func skipUnlessUnix(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts for macOS")
	}
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not installed")
	}
}

func newEnv(t *testing.T, serverURL string) *env {
	t.Helper()
	skipUnlessUnix(t)
	root := t.TempDir()
	e := &env{
		t:     t,
		home:  filepath.Join(root, "home"),
		plan:  filepath.Join(root, "home", "plan dir", "plan.json"),
		conf:  filepath.Join(root, "home", ".config", "glance-house"),
		state: filepath.Join(root, "home", "state"),
		tmp:   filepath.Join(root, "tmp"),
	}
	for _, d := range []string{filepath.Dir(e.plan), e.conf, e.tmp} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	e.write(filepath.Join(e.conf, "url"), serverURL+"/\n", 0o600)
	e.write(filepath.Join(e.conf, "secret"), secret+"\n", 0o600)
	return e
}

func (e *env) write(path, content string, mode os.FileMode) {
	e.t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		e.t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		e.t.Fatal(err)
	}
}

// run executes script with the test's private HOME and paths; it never sees
// the real home directory.
func (e *env) run(script string, extraEnv []string, args ...string) (string, int) {
	e.t.Helper()
	cmd := exec.Command("sh", append([]string{script}, args...)...)
	cmd.Env = append([]string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + e.home,
		"TMPDIR=" + e.tmp,
		"PLAN_FILE=" + e.plan,
		"CONF_DIR=" + e.conf,
		"STATE_DIR=" + e.state,
	}, extraEnv...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		e.t.Fatal(err)
	}
	if strings.Contains(out.String(), secret) {
		e.t.Fatalf("%s printed the secret:\n%s", script, out.String())
	}
	return out.String(), code
}

func (e *env) push() (string, int) { return e.run("push.sh", nil) }

func TestPush_PushesOnceThenSkipsUntilChanged(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(rec)
	defer srv.Close()
	e := newEnv(t, srv.URL)
	plan := `{"rooms":[{"id":"r1"}],"lights":[]}` + "\n"
	e.write(e.plan, plan, 0o644)

	out, code := e.push()
	if code != 0 || !strings.Contains(out, "pushed ") {
		t.Fatalf("first run: code %d, output:\n%s", code, out)
	}
	if rec.count() != 1 {
		t.Fatalf("requests = %d, want 1", rec.count())
	}
	r := rec.reqs[0]
	if r.method != http.MethodPut || r.path != "/house/plan" {
		t.Errorf("request = %s %s, want PUT /house/plan", r.method, r.path)
	}
	if r.auth != "Bearer "+secret {
		t.Errorf("Authorization = %q", r.auth)
	}
	if r.contentType != "application/json" {
		t.Errorf("Content-Type = %q", r.contentType)
	}
	if string(r.body) != plan {
		t.Errorf("body = %q, want the plan file byte for byte", r.body)
	}

	out, code = e.push()
	if code != 0 || !strings.Contains(out, "unchanged") || rec.count() != 1 {
		t.Fatalf("second run: code %d, requests %d, output:\n%s", code, rec.count(), out)
	}

	e.write(e.plan, `{"rooms":[{"id":"r2"}]}`, 0o644)
	out, code = e.push()
	if code != 0 || !strings.Contains(out, "pushed ") || rec.count() != 2 {
		t.Fatalf("after edit: code %d, requests %d, output:\n%s", code, rec.count(), out)
	}
}

func TestPush_NewServerURLPushesAgain(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(rec)
	defer srv.Close()
	e := newEnv(t, srv.URL)
	e.write(e.plan, `{"rooms":[1]}`, 0o644)
	if _, code := e.push(); code != 0 || rec.count() != 1 {
		t.Fatalf("first push: code %d, requests %d", code, rec.count())
	}
	rec2 := &recorder{}
	srv2 := httptest.NewServer(rec2)
	defer srv2.Close()
	e.write(filepath.Join(e.conf, "url"), srv2.URL, 0o600)
	if out, code := e.push(); code != 0 || rec2.count() != 1 {
		t.Fatalf("after url change: code %d, requests %d, output:\n%s", code, rec2.count(), out)
	}
}

func TestPush_ServerErrorIsRetriedLater(t *testing.T) {
	rec := &recorder{status: http.StatusInternalServerError}
	srv := httptest.NewServer(rec)
	defer srv.Close()
	e := newEnv(t, srv.URL)
	e.write(e.plan, `{"rooms":[1]}`, 0o644)

	out, code := e.push()
	if code != 0 || !strings.Contains(out, "push failed") || !strings.Contains(out, "HTTP 500") {
		t.Fatalf("code %d, output:\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(e.state, "last")); !os.IsNotExist(err) {
		t.Fatalf("last written after a failed push (err %v)", err)
	}
	rec.status = 0
	if out, _ := e.push(); !strings.Contains(out, "pushed ") || rec.count() != 2 {
		t.Fatalf("retry: requests %d, output:\n%s", rec.count(), out)
	}
}

func TestPush_UnreachableServerExitsZero(t *testing.T) {
	srv := httptest.NewServer(&recorder{})
	url := srv.URL
	srv.Close()
	e := newEnv(t, url)
	e.write(e.plan, `{"rooms":[1]}`, 0o644)
	out, code := e.push()
	if code != 0 || !strings.Contains(out, "push failed (curl 7") {
		t.Fatalf("code %d, output:\n%s", code, out)
	}
}

func TestPush_RefusesReadableSecret(t *testing.T) {
	for _, mode := range []os.FileMode{0o644, 0o640, 0o604, 0o620} {
		rec := &recorder{}
		srv := httptest.NewServer(rec)
		e := newEnv(t, srv.URL)
		e.write(e.plan, `{"rooms":[1]}`, 0o644)
		e.write(filepath.Join(e.conf, "secret"), secret, mode)
		out, code := e.push()
		if code != 1 || rec.count() != 0 || !strings.Contains(out, "readable by others") {
			t.Errorf("mode %o: code %d, requests %d, output:\n%s", mode, code, rec.count(), out)
		}
		srv.Close()
	}
}

func TestPush_MissingConfigIsAnError(t *testing.T) {
	e := newEnv(t, "http://127.0.0.1:9")
	os.Remove(filepath.Join(e.conf, "url"))
	if out, code := e.push(); code != 1 || !strings.Contains(out, "missing") {
		t.Fatalf("code %d, output:\n%s", code, out)
	}
}

func TestPush_NoPlanFile(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(rec)
	defer srv.Close()
	e := newEnv(t, srv.URL)
	out, code := e.push()
	if code != 0 || !strings.Contains(out, "no plan file") || rec.count() != 0 {
		t.Fatalf("code %d, requests %d, output:\n%s", code, rec.count(), out)
	}
}

func TestPush_TrimsAnOversizedLog(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(rec)
	defer srv.Close()
	e := newEnv(t, srv.URL)
	e.write(e.plan, `{"rooms":[1]}`, 0o644)
	logFile := filepath.Join(e.home, "push.log")
	e.write(logFile, strings.Repeat(strings.Repeat("x", 99)+"\n", 11000), 0o644)
	if _, code := e.run("push.sh", []string{"LOG_FILE=" + logFile}); code != 0 {
		t.Fatalf("code %d", code)
	}
	b, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), "\n"); n != 2000 {
		t.Fatalf("log has %d lines after trimming, want 2000", n)
	}
}

func TestPlistTemplate(t *testing.T) {
	b, err := os.ReadFile("com.glance-homeassistant.house-plan-push.plist.in")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<key>WatchPaths</key>", "<key>StartInterval</key>", "<integer>300</integer>", "<key>RunAtLoad</key>", "<key>ThrottleInterval</key>", "@PLAN_FILE@", "@PUSH_SH@", "@LOG_FILE@", "@USERNAME@"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("plist template lacks %s", want)
		}
	}
}

// TestInstall_InPrivateHome runs install.sh and uninstall.sh with a
// throwaway HOME and a fake launchctl that only records its arguments, so
// nothing is installed on the machine running the test.
func TestInstall_InPrivateHome(t *testing.T) {
	e := newEnv(t, "http://lan-host.invalid:8081")
	calls := filepath.Join(e.home, "launchctl.calls")
	fake := filepath.Join(e.tmp, "launchctl")
	e.write(fake, "#!/bin/sh\nprintf '%s\\n' \"$*\" >>'"+calls+"'\n", 0o755)
	fakeEnv := []string{"LAUNCHCTL=" + fake, "SUDO="}
	label := "com.glance-homeassistant.house-plan-push"
	agent := filepath.Join(e.home, "Library", "LaunchAgents", label+".plist")
	installed := filepath.Join(e.home, "Library", "Application Support", "glance-house", "push.sh")

	// Dry run: prints the plist, writes and runs nothing.
	out, code := e.run("install.sh", fakeEnv, "--daemon", "--dry-run")
	if code != 0 || !strings.Contains(out, "<key>UserName</key>") || !strings.Contains(out, "would run: ") {
		t.Fatalf("dry run: code %d, output:\n%s", code, out)
	}
	if _, err := os.Stat(calls); !os.IsNotExist(err) {
		t.Fatal("dry run called launchctl")
	}
	if _, err := os.Stat(filepath.Join(e.home, "Library")); !os.IsNotExist(err) {
		t.Fatal("dry run wrote under HOME/Library")
	}

	out, code = e.run("install.sh", fakeEnv)
	if code != 0 {
		t.Fatalf("install: code %d, output:\n%s", code, out)
	}
	plist, err := os.ReadFile(agent)
	if err != nil {
		t.Fatalf("agent plist not written: %v\n%s", err, out)
	}
	for _, want := range []string{
		"<string>" + label + "</string>",
		"<string>" + installed + "</string>",
		"<key>PLAN_FILE</key>\n\t\t<string>" + e.plan + "</string>",
		"<key>WatchPaths</key>\n\t<array>\n\t\t<string>" + e.plan + "</string>",
		filepath.Join(e.home, "Library", "Logs", "glance-house-push.log"),
	} {
		if !strings.Contains(string(plist), want) {
			t.Errorf("plist lacks %q:\n%s", want, plist)
		}
	}
	if strings.Contains(string(plist), "@") || strings.Contains(string(plist), "UserName") {
		t.Errorf("agent plist has a leftover placeholder or UserName:\n%s", plist)
	}
	if plutil, err := exec.LookPath("plutil"); err == nil {
		if b, err := exec.Command(plutil, "-lint", agent).CombinedOutput(); err != nil {
			t.Errorf("plutil -lint: %v\n%s", err, b)
		}
	}
	if fi, err := os.Stat(installed); err != nil || fi.Mode().Perm() != 0o755 {
		t.Fatalf("installed push.sh: %v %v", fi, err)
	}
	got, _ := os.ReadFile(calls)
	uid := strings.TrimSpace(mustOutput(t, "id", "-u"))
	want := "bootout gui/" + uid + "/" + label + "\nbootstrap gui/" + uid + " " + agent + "\n"
	if string(got) != want {
		t.Errorf("launchctl calls:\n%s\nwant:\n%s", got, want)
	}

	// Again: idempotent.
	if out, code := e.run("install.sh", fakeEnv); code != 0 {
		t.Fatalf("second install: code %d, output:\n%s", code, out)
	}

	out, code = e.run("uninstall.sh", fakeEnv)
	if code != 0 {
		t.Fatalf("uninstall: code %d, output:\n%s", code, out)
	}
	for _, gone := range []string{agent, installed} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Errorf("%s still there after uninstall", gone)
		}
	}
	if _, err := os.Stat(filepath.Join(e.conf, "secret")); err != nil {
		t.Errorf("uninstall removed the config: %v", err)
	}
}

func TestInstall_RefusesWithoutPrivateSecret(t *testing.T) {
	e := newEnv(t, "http://lan-host.invalid:8081")
	fakeEnv := []string{"LAUNCHCTL=/usr/bin/false", "SUDO="}
	e.write(filepath.Join(e.conf, "secret"), secret, 0o644)
	if out, code := e.run("install.sh", fakeEnv); code != 1 || !strings.Contains(out, "readable by others") {
		t.Fatalf("readable secret: code %d, output:\n%s", code, out)
	}
	os.Remove(filepath.Join(e.conf, "secret"))
	if out, code := e.run("install.sh", fakeEnv); code != 1 || !strings.Contains(out, "secret first") {
		t.Fatalf("missing secret: code %d, output:\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(e.home, "Library")); !os.IsNotExist(err) {
		t.Fatal("a refused install wrote under HOME/Library")
	}
}

func mustOutput(t *testing.T, name string, args ...string) string {
	t.Helper()
	b, err := exec.Command(name, args...).Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
