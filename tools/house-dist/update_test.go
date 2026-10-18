package housedist

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// buildScript stands in for `vite build`: it writes dist/ with a page that
// references what PAGE_REFS lists, the asset files, and a demo plans/.
const buildScript = `import fs from 'node:fs';
fs.rmSync('dist', {recursive: true, force: true});
fs.mkdirSync('dist/assets', {recursive: true});
fs.mkdirSync('dist/plans', {recursive: true});
fs.writeFileSync('dist/plans/home.json', '{}');
fs.writeFileSync('dist/assets/index-abc.js', 'console.log(1)');
fs.writeFileSync('dist/assets/index-abc.css', 'body{}');
const refs = fs.readFileSync('refs.txt', 'utf8').trim().split('\n');
fs.writeFileSync('dist/index.html', '<!doctype html><html><head>' +
  refs.map(r => r.endsWith('.css') ? '<link rel="stylesheet" href="' + r + '">' : '<script type="module" src="' + r + '"></script>').join('') +
  '</head><body></body></html>');
`

func need(t *testing.T, tools ...string) {
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not found", tool)
		}
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

const fakeLock = `{"name":"fake","lockfileVersion":3,"requires":true,"packages":{
"":{"name":"fake","dependencies":{"rtlib":"file:deps/rtlib"},"devDependencies":{"devtool":"file:deps/devtool"}},
"deps/devtool":{"version":"9.9.9","dev":true,"license":"MIT"},
"deps/rtlib":{"version":"1.2.3","license":"ISC"},
"node_modules/devtool":{"resolved":"deps/devtool","link":true},
"node_modules/rtlib":{"resolved":"deps/rtlib","link":true}}}`

// fakeSource makes a House Control-shaped git repo whose build references
// refs, and returns it with the commit's full sha.
func fakeSource(t *testing.T, refs ...string) (string, string) {
	src := t.TempDir()
	files := map[string]string{
		"LICENSE":               "MIT License\n\nCopyright (c) test\n",
		"README.md":             "not vendored\n",
		"web/package.json":      `{"name":"fake","private":true,"type":"module","scripts":{"build":"node build.mjs"},"dependencies":{"rtlib":"file:deps/rtlib"},"devDependencies":{"devtool":"file:deps/devtool"}}`,
		"web/package-lock.json": fakeLock,
		"web/build.mjs":         buildScript,
		"web/refs.txt":          strings.Join(refs, "\n") + "\n",
		// A runtime dependency (bundled, so its licence ships) and a
		// build tool (not bundled, so it does not).
		"web/deps/rtlib/package.json":   `{"name":"rtlib","version":"1.2.3","license":"ISC"}`,
		"web/deps/rtlib/LICENSE":        "ISC License\n\nCopyright (c) rtlib authors\n",
		"web/deps/devtool/package.json": `{"name":"devtool","version":"9.9.9","license":"MIT"}`,
		"web/deps/devtool/LICENSE":      "MIT License\n\nCopyright (c) devtool authors\n",
	}
	for name, body := range files {
		path := filepath.Join(src, name)
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git(t, src, "init", "-q")
	git(t, src, "add", ".")
	git(t, src, "commit", "-q", "-m", "fake")
	// A dirty working tree must not leak into the build.
	os.WriteFile(filepath.Join(src, "web", "refs.txt"), []byte("/elsewhere.js\n"), 0o644)
	return src, git(t, src, "rev-parse", "HEAD")
}

func run(t *testing.T, src, out string, args ...string) (string, error) {
	cmd := exec.Command("sh", append([]string{"update.sh"}, args...)...)
	cmd.Env = append(os.Environ(),
		"HOUSE_CONTROL_SRC="+src,
		"HOUSE_DIST_OUT="+out,
		"TMPDIR="+t.TempDir(),
		"npm_config_cache="+t.TempDir(),
		"npm_config_update_notifier=false",
	)
	b, err := cmd.CombinedOutput()
	return string(b), err
}

func TestUpdate_VendorsTheBuild(t *testing.T) {
	need(t, "git", "node", "npm")
	src, sha := fakeSource(t, "./assets/index-abc.js", "./assets/index-abc.css")
	out := filepath.Join(t.TempDir(), "dist")
	os.MkdirAll(filepath.Join(out, "assets"), 0o755)
	os.WriteFile(filepath.Join(out, "assets", "stale.js"), []byte("old"), 0o644)

	log, err := run(t, src, out, sha[:12])
	if err != nil {
		t.Fatalf("update.sh: %v\n%s", err, log)
	}
	if !strings.Contains(log, "vendored house-control "+sha) {
		t.Errorf("log lacks the vendored line:\n%s", log)
	}
	for _, name := range []string{"index.html", "assets/index-abc.js", "assets/index-abc.css", "LICENSE", "THIRD_PARTY_LICENSES", "VERSION"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, name := range []string{"plans", "assets/stale.js", "README.md", "package.json"} {
		if _, err := os.Stat(filepath.Join(out, name)); err == nil {
			t.Errorf("%s should not be in the vendored build", name)
		}
	}
	version, _ := os.ReadFile(filepath.Join(out, "VERSION"))
	want := regexp.MustCompile(`^house-control ` + sha + `\nsource: github\.com/sidun-av/house-control \(branch feature/browser-host\)\nbuilt: \d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ\n$`)
	if !want.Match(version) {
		t.Errorf("VERSION = %q", version)
	}
	if license, _ := os.ReadFile(filepath.Join(out, "LICENSE")); !strings.HasPrefix(string(license), "MIT License") {
		t.Errorf("LICENSE = %q", license)
	}
	third, _ := os.ReadFile(filepath.Join(out, "THIRD_PARTY_LICENSES"))
	if !strings.Contains(string(third), "rtlib 1.2.3 (ISC)") || !strings.Contains(string(third), "Copyright (c) rtlib authors") {
		t.Errorf("THIRD_PARTY_LICENSES lacks the runtime dependency:\n%s", third)
	}
	if strings.Contains(string(third), "devtool") {
		t.Errorf("THIRD_PARTY_LICENSES names a dev dependency:\n%s", third)
	}
	if matches, _ := filepath.Glob(out + ".*"); len(matches) != 0 {
		t.Errorf("staging directories left behind: %v", matches)
	}
	if st := git(t, src, "status", "--porcelain"); st != "M web/refs.txt" {
		t.Errorf("the source checkout changed: %q", st)
	}
}

func TestUpdate_RefusesAPageOutsideAssets(t *testing.T) {
	need(t, "git", "node", "npm")
	for _, ref := range []string{"/assets/index-abc.js", "https://cdn.example.invalid/x.js", "./assets/missing.js"} {
		src, sha := fakeSource(t, "./assets/index-abc.css", ref)
		out := filepath.Join(t.TempDir(), "dist")
		os.MkdirAll(out, 0o755)
		os.WriteFile(filepath.Join(out, "index.html"), []byte("kept"), 0o644)
		log, err := run(t, src, out, sha)
		if err == nil {
			t.Errorf("%s: update.sh succeeded:\n%s", ref, log)
		}
		if b, _ := os.ReadFile(filepath.Join(out, "index.html")); string(b) != "kept" {
			t.Errorf("%s: the previous build was replaced", ref)
		}
	}
}

func TestUpdate_RefusesAnUnknownCommit(t *testing.T) {
	need(t, "git")
	src, _ := fakeSource(t, "./assets/index-abc.js")
	for _, args := range [][]string{{"0123456789abcdef0123456789abcdef01234567"}, {}, {"a", "b"}} {
		if log, err := run(t, src, filepath.Join(t.TempDir(), "dist"), args...); err == nil {
			t.Errorf("%v: update.sh succeeded:\n%s", args, log)
		}
	}
	if log, err := run(t, filepath.Join(t.TempDir(), "nope"), t.TempDir(), "HEAD"); err == nil || !strings.Contains(log, "HOUSE_CONTROL_SRC") {
		t.Errorf("missing checkout: %v\n%s", err, log)
	}
}
