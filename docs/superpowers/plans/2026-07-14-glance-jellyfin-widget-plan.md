# glance-jellyfin Widget Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a new Go service, `glance-jellyfin`, exposed as a Glance extension widget, that shows the most recently added movies/TV shows from a Jellyfin library as a grid of poster cards.

**Architecture:** A new standalone repo at `/Users/sidun/GIT/glance-jellyfin`, structured identically to the existing `/Users/sidun/GIT/glance-homeassistant` repo: `main.go` + `config.go` at the root, an `internal/jellyfin` package for the Jellyfin HTTP client, an `internal/render` package for HTML generation, same Dockerfile/CI/config-file conventions. No client-side JS — `/widget` is polled by Glance on its own schedule; poster images are proxied through this service's own `/image/{itemId}` endpoint so the Jellyfin API key never reaches the browser.

**Tech Stack:** Go 1.23, stdlib `net/http` only, plus `gopkg.in/yaml.v3` for config parsing (same as `glance-homeassistant`).

## Global Constraints

- New, separate GitHub repo: `sidun-av/glance-jellyfin`. Not added to `glance-homeassistant`.
- No live-update JS and no `/live.json` endpoint — a periodic Glance `cache:` refresh (set in the user's `glance.yml`, out of scope here) is the only freshness mechanism.
- Poster images are always proxied server-side through this service's own `GET /image/{itemId}` — the Jellyfin API key (`X-Emby-Token`) must never be embedded in HTML or an `<img src>` sent to the browser.
- Card content is poster + title only — no year, rating, or type badge.
- Movies and TV shows render together in one grid sorted by date added, not separate tabs. This requires `GroupItems=false` on the Jellyfin "Latest Items" query (Jellyfin's default groups results per-library).
- Default item count is 12, overridable via the `LIMIT` env var / `limit` config field.
- `GET /healthz` (plain `200 ok`) is included for parity with `glance-homeassistant`.
- Editing the user's `glance.yml` to remove the old "Next Up"/"Releases"/"Latest Movies-Shows" widgets and add this one is a manual deployment step, out of scope for this repo's code.
- Every setting configurable via environment variable, taking priority over an optional mounted `config.yml` — same pattern as `glance-homeassistant`'s `config.go`.
- `gofmt -l .`, `go vet ./...`, and `go test ./...` must all be clean before every commit (matches this project's established convention).

---

### Task 1: Scaffold the new repo

**Files:**
- Create: `/Users/sidun/GIT/glance-jellyfin/go.mod`
- Create: `/Users/sidun/GIT/glance-jellyfin/.gitignore`
- Create: `/Users/sidun/GIT/glance-jellyfin/.dockerignore`
- Create: `/Users/sidun/GIT/glance-jellyfin/LICENSE`
- Create: `/Users/sidun/GIT/glance-jellyfin/main.go` (minimal stub, fully rewritten in Task 6)

**Interfaces:**
- Produces: a buildable, empty Go module at `github.com/sidun-av/glance-jellyfin`, ready for Task 2 onward. No functions yet.

- [ ] **Step 1: Create the directory and initialize git**

```bash
mkdir -p /Users/sidun/GIT/glance-jellyfin
cd /Users/sidun/GIT/glance-jellyfin
git init
```

- [ ] **Step 2: Create `go.mod`**

```
module github.com/sidun-av/glance-jellyfin

go 1.23
```

- [ ] **Step 3: Create `.gitignore`**

```
/glance-jellyfin
config.yml
```

- [ ] **Step 4: Create `.dockerignore`**

```
.git
docs
.github
*_test.go
glance-jellyfin
```

- [ ] **Step 5: Create `LICENSE`**

```
MIT License

Copyright (c) 2026 sidun-av

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

- [ ] **Step 6: Create a minimal `main.go` stub**

```go
package main

func main() {}
```

- [ ] **Step 7: Verify the module builds**

Run: `cd /Users/sidun/GIT/glance-jellyfin && go build ./...`
Expected: exits 0, no output.

- [ ] **Step 8: Commit**

```bash
cd /Users/sidun/GIT/glance-jellyfin
git add go.mod .gitignore .dockerignore LICENSE main.go
git commit -m "Scaffold glance-jellyfin repo"
```

---

### Task 2: Config loading

**Files:**
- Create: `/Users/sidun/GIT/glance-jellyfin/config.go`
- Test: `/Users/sidun/GIT/glance-jellyfin/config_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `type Config struct { Jellyfin JellyfinConfig; Title string; Limit int }`, `type JellyfinConfig struct { URL, Token, UserID, PublicURL string }`, `func LoadConfig(path string) (*Config, error)` — used by Task 6's `main()` and `newApp`.

- [ ] **Step 1: Write the failing tests**

Create `/Users/sidun/GIT/glance-jellyfin/config_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempConfig(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func setEnv(t *testing.T, name, value string) {
	t.Helper()
	prev, had := os.LookupEnv(name)
	if err := os.Setenv(name, value); err != nil {
		t.Fatalf("setenv %s: %v", name, err)
	}
	t.Cleanup(func() {
		if had {
			os.Setenv(name, prev)
		} else {
			os.Unsetenv(name)
		}
	})
}

func TestLoadConfig_Defaults(t *testing.T) {
	path := writeTempConfig(t, `
jellyfin:
  url: http://jellyfin:8096
  token: test-token
  user_id: test-user
  public_url: https://jellyfin.example.com
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Title != "Library" {
		t.Errorf("Title = %q, want %q", cfg.Title, "Library")
	}
	if cfg.Limit != 12 {
		t.Errorf("Limit = %d, want 12", cfg.Limit)
	}
	if cfg.Jellyfin.URL != "http://jellyfin:8096" {
		t.Errorf("Jellyfin.URL = %q, want http://jellyfin:8096", cfg.Jellyfin.URL)
	}
}

func TestLoadConfig_EnvOverrides(t *testing.T) {
	setEnv(t, "JELLYFIN_URL", "http://jellyfin.internal:8096")
	setEnv(t, "JELLYFIN_TOKEN", "secret-token")
	setEnv(t, "JELLYFIN_USER_ID", "env-user")
	setEnv(t, "JELLYFIN_PUBLIC_URL", "https://jf.example.com")
	setEnv(t, "TITLE", "Recently Added")
	setEnv(t, "LIMIT", "20")

	// No jellyfin block in the file at all — env vars alone must supply it.
	path := writeTempConfig(t, `title: ignored`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Jellyfin.URL != "http://jellyfin.internal:8096" {
		t.Errorf("Jellyfin.URL = %q, want env override", cfg.Jellyfin.URL)
	}
	if cfg.Jellyfin.Token != "secret-token" {
		t.Errorf("Jellyfin.Token = %q, want env override", cfg.Jellyfin.Token)
	}
	if cfg.Jellyfin.UserID != "env-user" {
		t.Errorf("Jellyfin.UserID = %q, want env override", cfg.Jellyfin.UserID)
	}
	if cfg.Jellyfin.PublicURL != "https://jf.example.com" {
		t.Errorf("Jellyfin.PublicURL = %q, want env override", cfg.Jellyfin.PublicURL)
	}
	if cfg.Title != "Recently Added" {
		t.Errorf("Title = %q, want env override", cfg.Title)
	}
	if cfg.Limit != 20 {
		t.Errorf("Limit = %d, want 20", cfg.Limit)
	}
}

func TestLoadConfig_MissingRequiredFieldsError(t *testing.T) {
	cases := []struct {
		name   string
		yaml   string
		wantIn string
	}{
		{"missing url", "jellyfin:\n  token: t\n  user_id: u\n  public_url: p", "jellyfin.url"},
		{"missing token", "jellyfin:\n  url: u\n  user_id: u\n  public_url: p", "jellyfin.token"},
		{"missing user_id", "jellyfin:\n  url: u\n  token: t\n  public_url: p", "jellyfin.user_id"},
		{"missing public_url", "jellyfin:\n  url: u\n  token: t\n  user_id: u", "jellyfin.public_url"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := writeTempConfig(t, c.yaml)
			_, err := LoadConfig(path)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), c.wantIn) {
				t.Errorf("error = %v, want it to mention %q", err, c.wantIn)
			}
		})
	}
}

func TestLoadConfig_InvalidLimitEnvErrors(t *testing.T) {
	setEnv(t, "LIMIT", "not-a-number")
	path := writeTempConfig(t, `
jellyfin:
  url: http://jellyfin:8096
  token: test-token
  user_id: test-user
  public_url: https://jellyfin.example.com
`)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid LIMIT, got nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/sidun/GIT/glance-jellyfin && go test ./... -run TestLoadConfig -v`
Expected: FAIL — `LoadConfig`/`Config` undefined (no `config.go` yet).

- [ ] **Step 3: Add the yaml dependency**

```bash
cd /Users/sidun/GIT/glance-jellyfin
go get gopkg.in/yaml.v3@v3.0.1
```

- [ ] **Step 4: Write `config.go`**

```go
package main

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Jellyfin JellyfinConfig `yaml:"jellyfin"`
	Title    string         `yaml:"title"`
	Limit    int            `yaml:"limit"`
}

type JellyfinConfig struct {
	URL       string `yaml:"url"`
	Token     string `yaml:"token"`
	UserID    string `yaml:"user_id"`
	PublicURL string `yaml:"public_url"`
}

func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := applyEnvOverrides(&cfg); err != nil {
		return nil, err
	}

	if cfg.Title == "" {
		cfg.Title = "Library"
	}
	if cfg.Limit == 0 {
		cfg.Limit = 12
	}

	if cfg.Jellyfin.URL == "" {
		return nil, fmt.Errorf("jellyfin.url is required")
	}
	if cfg.Jellyfin.Token == "" {
		return nil, fmt.Errorf("jellyfin.token is required")
	}
	if cfg.Jellyfin.UserID == "" {
		return nil, fmt.Errorf("jellyfin.user_id is required")
	}
	if cfg.Jellyfin.PublicURL == "" {
		return nil, fmt.Errorf("jellyfin.public_url is required")
	}
	if cfg.Limit < 0 {
		return nil, fmt.Errorf("limit must not be negative, got %d", cfg.Limit)
	}

	return &cfg, nil
}

// lookupNonEmptyEnv returns (value, true) only when the environment variable
// is actually set to a non-empty string — matters for GUI-driven deployments
// (e.g. Komodo) where an unfilled-in stack variable is passed through as an
// empty string rather than being absent.
func lookupNonEmptyEnv(name string) (string, bool) {
	v, ok := os.LookupEnv(name)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

func applyEnvOverrides(cfg *Config) error {
	if v, ok := lookupNonEmptyEnv("JELLYFIN_URL"); ok {
		cfg.Jellyfin.URL = v
	}
	if v, ok := lookupNonEmptyEnv("JELLYFIN_TOKEN"); ok {
		cfg.Jellyfin.Token = v
	}
	if v, ok := lookupNonEmptyEnv("JELLYFIN_USER_ID"); ok {
		cfg.Jellyfin.UserID = v
	}
	if v, ok := lookupNonEmptyEnv("JELLYFIN_PUBLIC_URL"); ok {
		cfg.Jellyfin.PublicURL = v
	}
	if v, ok := lookupNonEmptyEnv("TITLE"); ok {
		cfg.Title = v
	}
	if v, ok := lookupNonEmptyEnv("LIMIT"); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("env LIMIT=%q is not a valid integer: %w", v, err)
		}
		cfg.Limit = n
	}
	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/sidun/GIT/glance-jellyfin && go test ./... -v`
Expected: PASS, all `TestLoadConfig_*` cases green.

- [ ] **Step 6: Format/vet check and commit**

```bash
cd /Users/sidun/GIT/glance-jellyfin
gofmt -l .
go vet ./...
git add config.go config_test.go go.mod go.sum
git commit -m "Add config loading with env var overrides"
```

---

### Task 3: `internal/jellyfin` — fetch recently added items

**Files:**
- Create: `/Users/sidun/GIT/glance-jellyfin/internal/jellyfin/client.go`
- Test: `/Users/sidun/GIT/glance-jellyfin/internal/jellyfin/client_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks (standalone package).
- Produces: `type Item struct { ID, Name string; HasImage bool }`, `func New(baseURL, token, userID string) *Client`, `func (c *Client) FetchLatest(ctx context.Context, limit int) ([]Item, error)` — used by Task 6's `widgetHandler`. `Client` struct is extended (not replaced) by Task 4.

- [ ] **Step 1: Write the failing tests**

Create `/Users/sidun/GIT/glance-jellyfin/internal/jellyfin/client_test.go`:

```go
package jellyfin

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchLatest_ParsesItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/Users/test-user/Items/Latest" {
			t.Errorf("path = %s, want /Users/test-user/Items/Latest", r.URL.Path)
		}
		if got := r.Header.Get("X-Emby-Token"); got != "test-token" {
			t.Errorf("X-Emby-Token = %q, want %q", got, "test-token")
		}
		if got := r.URL.Query().Get("Limit"); got != "12" {
			t.Errorf("Limit query param = %q, want %q", got, "12")
		}
		if got := r.URL.Query().Get("GroupItems"); got != "false" {
			t.Errorf("GroupItems query param = %q, want %q", got, "false")
		}
		if got := r.URL.Query().Get("IncludeItemTypes"); got != "Movie,Series" {
			t.Errorf("IncludeItemTypes query param = %q, want %q", got, "Movie,Series")
		}
		fmt.Fprint(w, `[
			{"Id":"abc123","Name":"The Sheep Detectives","Type":"Series","ImageTags":{"Primary":"tag1"}},
			{"Id":"def456","Name":"No Poster Movie","Type":"Movie","ImageTags":{}}
		]`)
	}))
	defer server.Close()

	client := New(server.URL, "test-token", "test-user")
	items, err := client.FetchLatest(context.Background(), 12)
	if err != nil {
		t.Fatalf("FetchLatest: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].ID != "abc123" || items[0].Name != "The Sheep Detectives" || !items[0].HasImage {
		t.Errorf("items[0] = %+v, want {abc123, The Sheep Detectives, HasImage=true}", items[0])
	}
	if items[1].HasImage {
		t.Errorf("items[1].HasImage = true, want false (no ImageTags.Primary)")
	}
}

func TestFetchLatest_EmptyLibrary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[]`)
	}))
	defer server.Close()

	client := New(server.URL, "test-token", "test-user")
	items, err := client.FetchLatest(context.Background(), 12)
	if err != nil {
		t.Fatalf("FetchLatest: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("len(items) = %d, want 0", len(items))
	}
}

func TestFetchLatest_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := New(server.URL, "test-token", "test-user")
	_, err := client.FetchLatest(context.Background(), 12)
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %v, want it to mention status 500", err)
	}
}

func TestFetchLatest_MalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not json`)
	}))
	defer server.Close()

	client := New(server.URL, "test-token", "test-user")
	_, err := client.FetchLatest(context.Background(), 12)
	if err == nil {
		t.Fatal("expected error for malformed response, got nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/sidun/GIT/glance-jellyfin && go test ./internal/jellyfin/... -v`
Expected: FAIL — `New`/`Client`/`FetchLatest` undefined.

- [ ] **Step 3: Write `client.go`**

```go
package jellyfin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	HTTPClient *http.Client
	BaseURL    string
	Token      string
	UserID     string
}

func New(baseURL, token, userID string) *Client {
	return &Client{
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Token:      token,
		UserID:     userID,
	}
}

type Item struct {
	ID       string
	Name     string
	HasImage bool
}

func (c *Client) FetchLatest(ctx context.Context, limit int) ([]Item, error) {
	u := fmt.Sprintf("%s/Users/%s/Items/Latest", c.BaseURL, c.UserID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	q := req.URL.Query()
	q.Set("IncludeItemTypes", "Movie,Series")
	q.Set("Limit", strconv.Itoa(limit))
	q.Set("GroupItems", "false")
	req.URL.RawQuery = q.Encode()
	req.Header.Set("X-Emby-Token", c.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request latest items: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("latest items returned status %d", resp.StatusCode)
	}

	type rawItem struct {
		ID        string `json:"Id"`
		Name      string `json:"Name"`
		ImageTags struct {
			Primary string `json:"Primary"`
		} `json:"ImageTags"`
	}
	var rawItems []rawItem
	if err := json.NewDecoder(resp.Body).Decode(&rawItems); err != nil {
		return nil, fmt.Errorf("parse latest items response: %w", err)
	}

	items := make([]Item, len(rawItems))
	for i, r := range rawItems {
		items[i] = Item{ID: r.ID, Name: r.Name, HasImage: r.ImageTags.Primary != ""}
	}
	return items, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/sidun/GIT/glance-jellyfin && go test ./internal/jellyfin/... -v`
Expected: PASS, all four tests green.

- [ ] **Step 5: Format/vet check and commit**

```bash
cd /Users/sidun/GIT/glance-jellyfin
gofmt -l .
go vet ./...
git add internal/jellyfin/client.go internal/jellyfin/client_test.go
git commit -m "Add Jellyfin client: fetch recently added items"
```

---

### Task 4: `internal/jellyfin` — proxy poster images

**Files:**
- Modify: `/Users/sidun/GIT/glance-jellyfin/internal/jellyfin/client.go`
- Modify: `/Users/sidun/GIT/glance-jellyfin/internal/jellyfin/client_test.go`

**Interfaces:**
- Consumes: `Client` struct and `New` from Task 3.
- Produces: `type ImageResult struct { Body io.ReadCloser; ContentType string; StatusCode int }`, `func (c *Client) FetchImage(ctx context.Context, itemID string) (*ImageResult, error)` — used by Task 6's `imageHandler`. Caller owns closing `Body`.

- [ ] **Step 1: Write the failing tests**

Append to `/Users/sidun/GIT/glance-jellyfin/internal/jellyfin/client_test.go`:

```go
func TestFetchImage_StreamsBodyAndContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Items/abc123/Images/Primary" {
			t.Errorf("path = %s, want /Items/abc123/Images/Primary", r.URL.Path)
		}
		if got := r.Header.Get("X-Emby-Token"); got != "test-token" {
			t.Errorf("X-Emby-Token = %q, want %q", got, "test-token")
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("fake-jpeg-bytes"))
	}))
	defer server.Close()

	client := New(server.URL, "test-token", "test-user")
	result, err := client.FetchImage(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("FetchImage: %v", err)
	}
	defer result.Body.Close()

	if result.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", result.StatusCode)
	}
	if result.ContentType != "image/jpeg" {
		t.Errorf("ContentType = %q, want image/jpeg", result.ContentType)
	}
	body, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "fake-jpeg-bytes" {
		t.Errorf("body = %q, want %q", body, "fake-jpeg-bytes")
	}
}

func TestFetchImage_NonOKStatusReturnsStatusCodeNotError(t *testing.T) {
	// A 404 from Jellyfin (missing poster) is not a Go error — the caller
	// (main.go's imageHandler) decides what to do with a non-200
	// StatusCode.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := New(server.URL, "test-token", "test-user")
	result, err := client.FetchImage(context.Background(), "missing")
	if err != nil {
		t.Fatalf("FetchImage: %v", err)
	}
	defer result.Body.Close()
	if result.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want 404", result.StatusCode)
	}
}
```

Add `"io"` to the existing `import` block at the top of `client_test.go`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/sidun/GIT/glance-jellyfin && go test ./internal/jellyfin/... -run TestFetchImage -v`
Expected: FAIL — `FetchImage`/`ImageResult` undefined.

- [ ] **Step 3: Add `FetchImage` to `client.go`**

Add `"io"` to the existing `import` block, then append:

```go
type ImageResult struct {
	Body        io.ReadCloser
	ContentType string
	StatusCode  int
}

// FetchImage streams a poster image from Jellyfin. The caller owns Body and
// must close it. A non-200 StatusCode is not treated as an error — the
// caller (main.go's imageHandler) decides how to respond (e.g. a 404 for a
// missing poster).
func (c *Client) FetchImage(ctx context.Context, itemID string) (*ImageResult, error) {
	u := fmt.Sprintf("%s/Items/%s/Images/Primary", c.BaseURL, itemID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("X-Emby-Token", c.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request image: %w", err)
	}

	return &ImageResult{
		Body:        resp.Body,
		ContentType: resp.Header.Get("Content-Type"),
		StatusCode:  resp.StatusCode,
	}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/sidun/GIT/glance-jellyfin && go test ./internal/jellyfin/... -v`
Expected: PASS, all six tests in the package green.

- [ ] **Step 5: Format/vet check and commit**

```bash
cd /Users/sidun/GIT/glance-jellyfin
gofmt -l .
go vet ./...
git add internal/jellyfin/client.go internal/jellyfin/client_test.go
git commit -m "Add Jellyfin client: proxy poster images"
```

---

### Task 5: `internal/render` — poster grid HTML

**Files:**
- Create: `/Users/sidun/GIT/glance-jellyfin/internal/render/grid.go`
- Test: `/Users/sidun/GIT/glance-jellyfin/internal/render/grid_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks (standalone package; `main.go` in Task 6 supplies fully-resolved `CardView`s).
- Produces: `type CardView struct { Title, ImageSrc, Href string }`, `type WidgetData struct { Cards []CardView }`, `func RenderWidget(data WidgetData) string`, `func RenderUnavailable() string` — used by Task 6's `widgetHandler`.

- [ ] **Step 1: Write the failing tests**

Create `/Users/sidun/GIT/glance-jellyfin/internal/render/grid_test.go`:

```go
package render

import "testing"

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (func() bool {
		for i := 0; i+len(substr) <= len(s); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()
}

func count(s, substr string) int {
	n := 0
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			n++
		}
	}
	return n
}

func TestRenderWidget_RendersOneCardPerItem(t *testing.T) {
	html := RenderWidget(WidgetData{Cards: []CardView{
		{Title: "The Sheep Detectives", ImageSrc: "/image/abc123", Href: "https://jellyfin.example/web/#/details?id=abc123"},
		{Title: "Another Show", ImageSrc: "/image/def456", Href: "https://jellyfin.example/web/#/details?id=def456"},
	}})
	if got := count(html, `class="jf-card"`); got != 2 {
		t.Errorf("card count = %d, want 2", got)
	}
	if !contains(html, `src="/image/abc123"`) {
		t.Errorf("html missing card 1's image src: %q", html)
	}
	if !contains(html, `href="https://jellyfin.example/web/#/details?id=abc123"`) {
		t.Errorf("html missing card 1's href: %q", html)
	}
	if !contains(html, "The Sheep Detectives") {
		t.Errorf("html missing card 1's title")
	}
}

func TestRenderWidget_EmptyCardsShowsEmptyMessage(t *testing.T) {
	html := RenderWidget(WidgetData{Cards: nil})
	if !contains(html, "jf-empty") {
		t.Errorf("html missing empty-state message: %q", html)
	}
	if contains(html, `class="jf-card"`) {
		t.Errorf("html has a card when Cards is empty: %q", html)
	}
}

func TestRenderWidget_EscapesTitleAndHref(t *testing.T) {
	html := RenderWidget(WidgetData{Cards: []CardView{
		{Title: `<script>alert(1)</script>`, ImageSrc: "/image/x", Href: `"><script>`},
	}})
	if contains(html, "<script>alert(1)</script>") {
		t.Errorf("title not escaped: %q", html)
	}
	if contains(html, `href="">`) {
		t.Errorf("href not escaped: %q", html)
	}
}

func TestRenderUnavailable_ShowsMessage(t *testing.T) {
	html := RenderUnavailable()
	if !contains(html, "Jellyfin unavailable") {
		t.Errorf("html = %q, want unavailable message", html)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/sidun/GIT/glance-jellyfin && go test ./internal/render/... -v`
Expected: FAIL — `RenderWidget`/`WidgetData`/`CardView`/`RenderUnavailable` undefined.

- [ ] **Step 3: Write `grid.go`**

```go
package render

import (
	"fmt"
	"html"
	"strings"
)

type CardView struct {
	Title    string
	ImageSrc string
	Href     string
}

type WidgetData struct {
	Cards []CardView
}

func styleBlock() string {
	return `<style>
	.jf-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(100px,1fr));gap:10px}
	.jf-card{display:block;color:inherit;text-decoration:none}
	.jf-poster{width:100%;aspect-ratio:2/3;object-fit:cover;border-radius:6px;display:block;background:var(--color-widget-background-highlight)}
	.jf-title{font-size:11px;color:var(--color-text-highlight);margin-top:4px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
	.jf-empty{color:var(--color-text-subdue);font-size:.85em;padding:8px 0}
	.jf-unavailable{color:var(--color-text-subdue);padding:12px 0}
</style>`
}

func RenderWidget(data WidgetData) string {
	var b strings.Builder
	b.WriteString(styleBlock())

	if len(data.Cards) == 0 {
		b.WriteString(`<div class="jf-empty">no recently added items found</div>`)
		return b.String()
	}

	b.WriteString(`<div class="jf-grid">`)
	for _, c := range data.Cards {
		fmt.Fprintf(&b,
			`<a class="jf-card" href="%s" target="_blank" rel="noopener"><img class="jf-poster" src="%s" alt="%s" loading="lazy"><div class="jf-title">%s</div></a>`,
			html.EscapeString(c.Href), html.EscapeString(c.ImageSrc), html.EscapeString(c.Title), html.EscapeString(c.Title),
		)
	}
	b.WriteString(`</div>`)
	return b.String()
}

func RenderUnavailable() string {
	return styleBlock() + `<div class="jf-unavailable">Jellyfin unavailable</div>`
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/sidun/GIT/glance-jellyfin && go test ./internal/render/... -v`
Expected: PASS, all four tests green.

- [ ] **Step 5: Format/vet check and commit**

```bash
cd /Users/sidun/GIT/glance-jellyfin
gofmt -l .
go vet ./...
git add internal/render/grid.go internal/render/grid_test.go
git commit -m "Add poster grid HTML rendering"
```

---

### Task 6: Wire `main.go` — widget, image proxy, and healthz handlers

**Files:**
- Modify: `/Users/sidun/GIT/glance-jellyfin/main.go` (replaces Task 1's stub entirely)
- Create: `/Users/sidun/GIT/glance-jellyfin/main_test.go`

**Interfaces:**
- Consumes: `Config`/`LoadConfig` (Task 2), `jellyfin.New`/`Client`/`Item`/`ImageResult`/`FetchLatest`/`FetchImage` (Tasks 3–4), `render.CardView`/`WidgetData`/`RenderWidget`/`RenderUnavailable` (Task 5).
- Produces: `func newApp(cfg *Config) *app`, `func newMux(cfg *Config, a *app) *http.ServeMux` — final assembled service.

- [ ] **Step 1: Write the failing tests**

Create `/Users/sidun/GIT/glance-jellyfin/main_test.go`:

```go
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fakeJellyfinServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Users/test-user/Items/Latest":
			fmt.Fprint(w, `[
				{"Id":"abc123","Name":"The Sheep Detectives","Type":"Series","ImageTags":{"Primary":"tag1"}},
				{"Id":"def456","Name":"No Poster Movie","Type":"Movie","ImageTags":{}}
			]`)
		case r.Method == http.MethodGet && r.URL.Path == "/Items/abc123/Images/Primary":
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write([]byte("fake-jpeg-bytes"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func testConfig(jellyfinURL string) *Config {
	return &Config{
		Jellyfin: JellyfinConfig{
			URL:       jellyfinURL,
			Token:     "test-token",
			UserID:    "test-user",
			PublicURL: "https://jellyfin.example.com",
		},
		Title: "Library",
		Limit: 12,
	}
}

func TestWidgetHandler_EndToEnd(t *testing.T) {
	jf := fakeJellyfinServer(t)
	defer jf.Close()

	cfg := testConfig(jf.URL)
	mux := newMux(cfg, newApp(cfg))

	req := httptest.NewRequest(http.MethodGet, "/widget", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("Widget-Title") != "Library" {
		t.Errorf("Widget-Title = %q, want Library", rec.Header().Get("Widget-Title"))
	}
	if rec.Header().Get("Widget-Content-Type") != "html" {
		t.Errorf("Widget-Content-Type = %q, want html", rec.Header().Get("Widget-Content-Type"))
	}
	body := rec.Body.String()
	if !strings.Contains(body, "The Sheep Detectives") {
		t.Errorf("body missing item title")
	}
	if !strings.Contains(body, `src="/image/abc123"`) {
		t.Errorf("body missing image proxy src")
	}
	if !strings.Contains(body, `href="https://jellyfin.example.com/web/#/details?id=abc123"`) {
		t.Errorf("body missing click-through href")
	}
	if strings.Contains(body, "No Poster Movie") {
		t.Errorf("body includes an item with no poster image, want it skipped")
	}
}

func TestWidgetHandler_JellyfinUnavailable(t *testing.T) {
	jf := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer jf.Close()

	cfg := testConfig(jf.URL)
	mux := newMux(cfg, newApp(cfg))

	req := httptest.NewRequest(http.MethodGet, "/widget", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (service owns its degraded state)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Jellyfin unavailable") {
		t.Errorf("body = %s, want unavailable message", rec.Body.String())
	}
}

func TestImageHandler_ProxiesImageBytes(t *testing.T) {
	jf := fakeJellyfinServer(t)
	defer jf.Close()

	cfg := testConfig(jf.URL)
	mux := newMux(cfg, newApp(cfg))

	req := httptest.NewRequest(http.MethodGet, "/image/abc123", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg", rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=86400" {
		t.Errorf("Cache-Control = %q, want public, max-age=86400", rec.Header().Get("Cache-Control"))
	}
	if rec.Body.String() != "fake-jpeg-bytes" {
		t.Errorf("body = %q, want fake-jpeg-bytes", rec.Body.String())
	}
}

func TestImageHandler_MissingImageReturns404(t *testing.T) {
	jf := fakeJellyfinServer(t)
	defer jf.Close()

	cfg := testConfig(jf.URL)
	mux := newMux(cfg, newApp(cfg))

	req := httptest.NewRequest(http.MethodGet, "/image/does-not-exist", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHealthzHandler(t *testing.T) {
	cfg := testConfig("http://unused")
	mux := newMux(cfg, newApp(cfg))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/sidun/GIT/glance-jellyfin && go test . -v`
Expected: FAIL to compile — `newApp`/`newMux` undefined (Task 1's `main.go` stub has neither).

- [ ] **Step 3: Replace `main.go` entirely**

```go
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sidun-av/glance-jellyfin/internal/jellyfin"
	"github.com/sidun-av/glance-jellyfin/internal/render"
)

type app struct {
	cfg    *Config
	client *jellyfin.Client
}

func newApp(cfg *Config) *app {
	return &app{cfg: cfg, client: jellyfin.New(cfg.Jellyfin.URL, cfg.Jellyfin.Token, cfg.Jellyfin.UserID)}
}

func (a *app) widgetHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	w.Header().Set("Widget-Title", a.cfg.Title)
	w.Header().Set("Widget-Content-Type", "html")

	items, err := a.client.FetchLatest(ctx, a.cfg.Limit)
	if err != nil {
		log.Printf("jellyfin unavailable: %v", err)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, render.RenderUnavailable())
		return
	}

	publicURL := strings.TrimRight(a.cfg.Jellyfin.PublicURL, "/")
	var cards []render.CardView
	for _, it := range items {
		if !it.HasImage {
			continue
		}
		cards = append(cards, render.CardView{
			Title:    it.Name,
			ImageSrc: "/image/" + it.ID,
			Href:     publicURL + "/web/#/details?id=" + it.ID,
		})
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, render.RenderWidget(render.WidgetData{Cards: cards}))
}

func (a *app) imageHandler(w http.ResponseWriter, r *http.Request) {
	itemID := strings.TrimPrefix(r.URL.Path, "/image/")
	if itemID == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	result, err := a.client.FetchImage(ctx, itemID)
	if err != nil {
		log.Printf("fetch image %s: %v", itemID, err)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	defer result.Body.Close()

	if result.StatusCode != http.StatusOK {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", result.ContentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, result.Body)
}

func newMux(cfg *Config, a *app) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})
	mux.HandleFunc("/widget", a.widgetHandler)
	mux.HandleFunc("/image/", a.imageHandler)
	return mux
}

func main() {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "/config.yml"
	}
	cfg, err := LoadConfig(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	a := newApp(cfg)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, newMux(cfg, a)))
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/sidun/GIT/glance-jellyfin && go test ./... -v`
Expected: PASS, every test in the module green (config, jellyfin, render, main).

- [ ] **Step 5: Format/vet check and commit**

```bash
cd /Users/sidun/GIT/glance-jellyfin
gofmt -l .
go vet ./...
git add main.go main_test.go
git commit -m "Wire widget, image proxy, and healthz handlers"
```

---

### Task 7: Deployment parity files (Dockerfile, CI, README, example configs)

**Files:**
- Create: `/Users/sidun/GIT/glance-jellyfin/Dockerfile`
- Create: `/Users/sidun/GIT/glance-jellyfin/.github/workflows/ci.yml`
- Create: `/Users/sidun/GIT/glance-jellyfin/config.docker-default.yml`
- Create: `/Users/sidun/GIT/glance-jellyfin/config.example.yml`
- Create: `/Users/sidun/GIT/glance-jellyfin/docker-compose.example.yml`
- Create: `/Users/sidun/GIT/glance-jellyfin/README.md`

**Interfaces:**
- Consumes: the finished, tested service from Tasks 1–6. No new Go code.

- [ ] **Step 1: Create `Dockerfile`**

```dockerfile
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/glance-jellyfin .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/glance-jellyfin /glance-jellyfin
COPY config.docker-default.yml /config.yml
EXPOSE 8080
ENTRYPOINT ["/glance-jellyfin"]
```

- [ ] **Step 2: Create `.github/workflows/ci.yml`**

```bash
mkdir -p /Users/sidun/GIT/glance-jellyfin/.github/workflows
```

```yaml
name: CI

on:
  push:
    branches: [main]
    tags: ['v*']
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - name: Check gofmt
        run: |
          if [ -n "$(gofmt -l .)" ]; then
            gofmt -d .
            exit 1
          fi
      - run: go vet ./...
      - run: go test ./...

  docker:
    needs: test
    if: github.event_name == 'push'
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - uses: docker/metadata-action@v5
        id: meta
        with:
          images: ghcr.io/${{ github.repository }}
          tags: |
            type=raw,value=latest,enable={{is_default_branch}}
            type=semver,pattern={{version}}
      - uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
```

- [ ] **Step 3: Create `config.docker-default.yml`**

```yaml
# Baked into the Docker image as /config.yml. Every setting here can be
# overridden by an environment variable instead (see README.md's
# "Environment variable reference") — that's the intended way to configure
# this image from a GUI stack manager like Komodo, with no file to mount or
# edit at all. Only jellyfin.url/token/user_id/public_url are required, via
# JELLYFIN_URL/JELLYFIN_TOKEN/JELLYFIN_USER_ID/JELLYFIN_PUBLIC_URL.
#
# If you'd rather hand-edit a mounted config.yml instead, start from
# config.example.yml, which has the same fields with example values and
# comments.
```

- [ ] **Step 4: Create `config.example.yml`**

```yaml
# Every field below can also be set via an environment variable, which
# always takes priority over whatever's written here — see README.md's
# "Environment variable reference" for the full list (e.g. JELLYFIN_URL,
# JELLYFIN_TOKEN, ...). That's the intended way to configure this image from
# a GUI stack manager like Komodo, with no file to mount or edit at all.
# This file is for hand-editing a mounted config.yml instead.

jellyfin:
  url: http://jellyfin:8096   # env: JELLYFIN_URL
  token: replace-with-an-api-key   # env: JELLYFIN_TOKEN
  user_id: replace-with-a-jellyfin-user-id   # env: JELLYFIN_USER_ID
  public_url: https://jellyfin.example.com   # env: JELLYFIN_PUBLIC_URL — browser-facing base URL, used for each card's click-through link

title: Library   # env: TITLE

limit: 12   # env: LIMIT — number of most-recently-added items to show
```

- [ ] **Step 5: Create `docker-compose.example.yml`**

```yaml
services:
  glance-jellyfin:
    image: ghcr.io/sidun-av/glance-jellyfin:latest
    restart: unless-stopped
    environment:
      # Required — your Jellyfin instance, an API key, a user ID, and
      # Jellyfin's browser-facing URL for click-through links.
      - JELLYFIN_URL=${JELLYFIN_URL}
      - JELLYFIN_TOKEN=${JELLYFIN_TOKEN}
      - JELLYFIN_USER_ID=${JELLYFIN_USER_ID}
      - JELLYFIN_PUBLIC_URL=${JELLYFIN_PUBLIC_URL}
      # Optional — every one of these has a built-in default; only set the
      # ones you want to change. If you're deploying via a GUI stack manager
      # (e.g. Komodo, pointed at this repo), set these in its "Environment"
      # tab and you never need to touch a config file at all.
      - TITLE=${TITLE:-}
      - LIMIT=${LIMIT:-}
    # Optional: mount a hand-edited config.yml instead of (or alongside) the
    # environment variables above — see config.example.yml. Not needed for a
    # GUI/env-var-only setup; env vars always take priority over this file.
    # volumes:
    #   - ./glance-jellyfin/config.yml:/config.yml:ro
```

- [ ] **Step 6: Create `README.md`**

```markdown
# glance-jellyfin

A [Glance](https://github.com/glanceapp/glance) extension widget that shows the most recently
added movies and TV shows from a Jellyfin library as a grid of poster cards — click a poster to
open it in Jellyfin's own web UI.

## How it works

A small Go HTTP service Glance calls on its own schedule (a Glance
[extension widget](https://github.com/glanceapp/glance/blob/main/docs/extensions.md)). On each
request it asks Jellyfin for the most recently added movies/TV shows
(`GET /Users/{userId}/Items/Latest`), renders them as a poster grid, and proxies each poster image
through its own `/image/{itemId}` endpoint so your Jellyfin API key never reaches the browser.
There's no live-update mechanism — a media library doesn't change on a 10-second cadence, so
Glance's own `cache:` interval (see step 5 below) is all the freshness this needs.

## Setup

### 1. Create a Jellyfin API key

In Jellyfin: **Admin Dashboard → API Keys → +**. Give it a name (e.g. "glance-jellyfin") and copy
the key.

### 2. Find your Jellyfin user ID

The "Latest Items" endpoint this widget uses is scoped to a specific user's library
access/view. In Jellyfin: **Admin Dashboard → Users → (your user)** — the user ID is in the page's
URL (`.../userdetails?userId=<this-part>`).

### 3. Configure

Every setting can be set as an environment variable — no file to create or mount. Env vars always
take priority over `config.yml`, so the two approaches can be mixed if you want.

- `JELLYFIN_URL` — reachable from *this container* (e.g. `http://jellyfin:8096` over your
  Docker/LAN network).
- `JELLYFIN_TOKEN` — the API key from step 1.
- `JELLYFIN_USER_ID` — the user ID from step 2.
- `JELLYFIN_PUBLIC_URL` — Jellyfin's browser-facing base URL (e.g. `https://jellyfin.example.com`),
  used to build each poster's click-through link. Only needs to be reachable from *your browser*,
  not from this container.

See "Environment variable reference" below for the full list. If you'd rather hand-edit a file
instead, copy [`config.example.yml`](config.example.yml) to `config.yml`, mount it at `/config.yml`,
and skip the env vars it covers.

### 4. Run it alongside Glance

**Option A — Komodo (or any GUI stack manager that can pull a stack from a git repo):**

Point Komodo's Stack source at this repo (`sidun-av/glance-jellyfin`),
[`docker-compose.example.yml`](docker-compose.example.yml) as the compose file. Then set
`JELLYFIN_URL`/`JELLYFIN_TOKEN`/`JELLYFIN_USER_ID`/`JELLYFIN_PUBLIC_URL` (required) and any other
overrides you want in the stack's Environment tab — nothing to SSH in and edit. Add it to the same
Docker network as Jellyfin.

**Option B — plain `docker compose`:**

```yaml
services:
  glance-jellyfin:
    image: ghcr.io/sidun-av/glance-jellyfin:latest
    restart: unless-stopped
    environment:
      - JELLYFIN_URL=http://jellyfin:8096
      - JELLYFIN_TOKEN=${JELLYFIN_TOKEN}
      - JELLYFIN_USER_ID=${JELLYFIN_USER_ID}
      - JELLYFIN_PUBLIC_URL=https://jellyfin.example.com
```

Add it to the same Docker network as Jellyfin.

### 5. Add the widget to Glance

```yaml
- type: extension
  url: http://glance-jellyfin:8080/widget
  cache: 30m
  allow-potentially-dangerous-html: true
```

`cache: 30m` is intentionally slow — a media library only needs to look fresh every so often, not
live-updated.

## Environment variable reference

Every one of these overrides the matching `config.yml` field when set to a non-empty value — set
just the ones you want to change (e.g. in Komodo's stack Environment tab) and leave the rest unset
to use the built-in default (or whatever `config.yml` has, if you're mounting one).

| Env var | `config.yml` field | Default | Description |
|---|---|---|---|
| `JELLYFIN_URL` | `jellyfin.url` | — (required) | Jellyfin base URL, reachable from this container |
| `JELLYFIN_TOKEN` | `jellyfin.token` | — (required) | Jellyfin API key |
| `JELLYFIN_USER_ID` | `jellyfin.user_id` | — (required) | The Jellyfin user whose library access the "Latest Items" call uses |
| `JELLYFIN_PUBLIC_URL` | `jellyfin.public_url` | — (required) | Jellyfin's browser-facing base URL, used for each poster's click-through link |
| `TITLE` | `title` | `Library` | Widget title shown in Glance |
| `LIMIT` | `limit` | `12` | Number of most-recently-added items to show |

The service's own listen port and config-file path aren't `config.yml` fields — they're read from
the environment before any config is loaded, so they're always plain environment variables:

| Env var | Default | Description |
|---|---|---|
| `PORT` | `8080` | Port the HTTP server listens on |
| `CONFIG_PATH` | `/config.yml` | Path to the config file read at startup |

## Error handling

If Jellyfin is unreachable, the whole widget shows a single "Jellyfin unavailable" message instead
of Glance's generic widget-failed state. An item with no poster image is silently skipped rather
than shown broken. If a single poster fails to load after the widget already rendered, only that
card's image breaks (falls back to its alt text) — the rest of the grid is unaffected.

## Out of scope (for now)

Browsing the full library / pagination, separate Movies/TV Shows tabs, year/rating/type badges on
cards, and live/real-time updates — see the design spec for the reasoning behind each.

## Development

```bash
go test ./...
docker build -t glance-jellyfin:dev .
```
```

- [ ] **Step 7: Verify the Docker image builds**

Run: `cd /Users/sidun/GIT/glance-jellyfin && docker build -t glance-jellyfin:dev .`
Expected: builds successfully (exit 0). If Docker isn't available in this environment, skip this
step but still complete Step 8.

- [ ] **Step 8: Final format/vet/test check and commit**

```bash
cd /Users/sidun/GIT/glance-jellyfin
gofmt -l .
go vet ./...
go test ./...
git add Dockerfile .github/workflows/ci.yml config.docker-default.yml config.example.yml docker-compose.example.yml README.md
git commit -m "Add Dockerfile, CI workflow, example configs, and README"
```

---

### Task 8: Create the GitHub repo and push

**Files:** none (infrastructure step only).

**Interfaces:** none — this is the final step that makes the repo from Tasks 1–7 live on GitHub.

- [ ] **Step 1: Confirm repo visibility with the user before creating it**

`glance-homeassistant` and `glance-grafana-sparkline` are both pulled as Docker images without any
registry login in the user's `docker-compose.yml`, which means their GHCR packages are public —
their source repos are very likely public too. Confirm this assumption with the user (public vs.
private) before running the next step, since creating a new repo under their account is a visible,
not-trivially-reversible action.

- [ ] **Step 2: Create the repo and push**

```bash
cd /Users/sidun/GIT/glance-jellyfin
gh repo create sidun-av/glance-jellyfin --public --source=. --remote=origin --push
```

(Use `--private` instead of `--public` if the user's answer to Step 1 says private.)

- [ ] **Step 3: Verify CI runs and goes green**

```bash
gh run list --repo sidun-av/glance-jellyfin --branch main --limit 1
gh run view --repo sidun-av/glance-jellyfin <run-id> --json status,conclusion,jobs
```

Expected: both the `test` and `docker` jobs complete with `conclusion: success`, and
`ghcr.io/sidun-av/glance-jellyfin:latest` is published.

---

## Out of scope (matches the design spec)

- Editing the user's `glance.yml` to remove "Next Up"/"Releases"/"Latest Movies-Shows" and add
  this widget — a manual step once this image is published; ask the user for their current
  `glance.yml` at that point and hand them an exact diff.
- Full library browsing/pagination, separate Movies/TV tabs, year/rating/type badges, live updates
  — all explicitly rejected during brainstorming (see the design spec's "Out of scope" section).
