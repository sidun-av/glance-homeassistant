package house

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestDistFS_IsUsable(t *testing.T) {
	d := DistFS()
	page, err := fs.ReadFile(d, "index.html")
	if err != nil {
		t.Fatalf("index.html: %v", err)
	}
	if !strings.Contains(string(page), "<head>") {
		t.Fatalf("index.html has no <head>")
	}
	refs := regexp.MustCompile(`(?:src|href)="([^"]*)"`).FindAllStringSubmatch(string(page), -1)
	if len(refs) == 0 {
		t.Fatalf("index.html references no assets")
	}
	for _, m := range refs {
		ref := m[1]
		if !strings.HasPrefix(ref, "./assets/") {
			t.Errorf("index.html references %q, want ./assets/…", ref)
			continue
		}
		if _, err := fs.Stat(d, strings.TrimPrefix(ref, "./")); err != nil {
			t.Errorf("%s: %v", ref, err)
		}
	}

	version, err := fs.ReadFile(d, "VERSION")
	if err != nil {
		t.Fatalf("VERSION: %v", err)
	}
	first, _, _ := strings.Cut(string(version), "\n")
	if !regexp.MustCompile(`^house-control [0-9a-f]{40}$`).MatchString(first) {
		t.Errorf("VERSION first line = %q", first)
	}
	if license, err := fs.ReadFile(d, "LICENSE"); err != nil || !strings.Contains(string(license), "MIT License") {
		t.Errorf("LICENSE: %v, want House Control's MIT licence", err)
	}
	if _, err := fs.Stat(d, "plans"); err == nil {
		t.Errorf("the demo plans/ were vendored")
	}
}

// The real build goes through the page handler: the boot config is
// injected and every asset index.html names is served.
func TestDistFS_ServedByThePage(t *testing.T) {
	h := &Handler{Store: OpenStore(""), Base: "/ha-widget/house", Lang: "ru", Dist: DistFS()}
	mux := http.NewServeMux()
	h.Register(mux, "")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/house/", nil))
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `<base href="/ha-widget/house/">`) || !strings.Contains(body, `"lang":"ru"`) {
		t.Fatalf("/house/: %d %.300s", rec.Code, body)
	}
	for _, m := range regexp.MustCompile(`(?:src|href)="\./(assets/[^"]*)"`).FindAllStringSubmatch(body, -1) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/house/"+m[1], nil))
		if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
			t.Errorf("/house/%s: %d", m[1], rec.Code)
		}
	}
	for _, path := range []string{"/house/VERSION", "/house/LICENSE"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404 (only the page and assets/ are served)", path, rec.Code)
		}
	}
}
