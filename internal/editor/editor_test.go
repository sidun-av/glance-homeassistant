package editor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sidun-av/glance-homeassistant/internal/layout"
)

type fakeStore struct {
	cur   *layout.Layout
	saved *layout.Layout
}

func (f *fakeStore) Current() *layout.Layout { return f.cur }
func (f *fakeStore) Seed() *layout.Layout    { return f.cur }
func (f *fakeStore) Save(l *layout.Layout) error {
	if err := l.Validate(); err != nil {
		return err
	}
	f.saved = l
	return nil
}
func (f *fakeStore) Preview(l *layout.Layout) (string, error) {
	return "<div class=\"ha-floorplan\">" + l.Rooms[0].Key + "</div>", nil
}

type fakeSource struct{}

func (fakeSource) Areas(context.Context) ([]Area, error) {
	return []Area{{ID: "bedroom", Name: "Bedroom", Entities: []Entity{{EntityID: "light.a", Name: "A", Domain: "light", Kind: "light", PlaceID: "light.a"}}}}, nil
}

func newMux(t *testing.T) (*http.ServeMux, *fakeStore) {
	t.Helper()
	st := &fakeStore{cur: &layout.Layout{Version: 1, Columns: 2, Rows: 1, Rooms: []layout.Room{{Key: "a", Area: "Bedroom", Cells: []layout.Cell{{0, 0}}}}}}
	h := &Handler{Store: st, Source: fakeSource{}}
	mux := http.NewServeMux()
	h.Register(mux, "")
	h.Register(mux, "/ha-widget")
	return mux, st
}

func TestPage_ServedUnderBothPaths(t *testing.T) {
	mux, _ := newMux(t)
	for _, path := range []string{"/edit", "/ha-widget/edit", "/ha-widget/edit/"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Floorplan editor") {
			t.Errorf("%s: %d", path, rec.Code)
		}
		wantBase := ""
		if strings.HasPrefix(path, "/ha-widget") {
			wantBase = "/ha-widget"
		}
		if !strings.Contains(rec.Body.String(), `const BASE="`+wantBase+`";`) {
			t.Errorf("%s: BASE not %q", path, wantBase)
		}
	}
}

func TestData_IncludesLayoutSeedAndAreas(t *testing.T) {
	mux, _ := newMux(t)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/ha-widget/edit/data.json", nil))
	body := rec.Body.String()
	for _, want := range []string{`"layout":`, `"seed":`, `"areas":[{"id":"bedroom"`, `"place_id":"light.a"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s in %s", want, body)
		}
	}
}

func TestLayout_PutValidatesAndSaves(t *testing.T) {
	mux, st := newMux(t)
	bad := `{"columns":2,"rows":1,"rooms":[{"key":"a","area":"Bedroom","cells":[[0,0],[5,5]]}]}`
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("PUT", "/edit/layout.json", strings.NewReader(bad)))
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "outside the 2x1 map") {
		t.Errorf("invalid layout: %d %s", rec.Code, rec.Body.String())
	}
	if st.saved != nil {
		t.Error("invalid layout must not be saved")
	}
	good := `{"columns":2,"rows":1,"rooms":[{"key":"a","area":"Bedroom","cells":[[0,0]],"entities":{"light.a":[0,0]}}]}`
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("PUT", "/edit/layout.json", strings.NewReader(good)))
	if rec.Code != 200 || st.saved == nil || st.saved.Rooms[0].Grid.Rows != 3 {
		t.Errorf("valid layout: %d %s saved=%+v", rec.Code, rec.Body.String(), st.saved)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("PUT", "/edit/layout.json", strings.NewReader("{nope")))
	if rec.Code != 400 {
		t.Errorf("malformed JSON: %d", rec.Code)
	}
}

func TestPreview_RendersUnsavedLayout(t *testing.T) {
	mux, st := newMux(t)
	body := `{"columns":1,"rows":1,"rooms":[{"key":"draft","area":"Bedroom","cells":[[0,0]]}]}`
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/edit/preview", strings.NewReader(body)))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "draft") {
		t.Errorf("preview: %d %s", rec.Code, rec.Body.String())
	}
	if st.saved != nil {
		t.Error("preview must not save")
	}
}
