package house

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenStore_MissingFileIsEmpty(t *testing.T) {
	s := OpenStore(filepath.Join(t.TempDir(), "house-plan.json"))
	if s.Current() != nil {
		t.Errorf("Current() = %+v, want nil", s.Current())
	}
}

func TestOpenStore_CorruptFileIsIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "house-plan.json")
	os.WriteFile(path, []byte(`{nope`), 0o644)
	s := OpenStore(path)
	if s.Current() != nil {
		t.Errorf("Current() = %+v, want nil for a corrupt file", s.Current())
	}
	// A good upload still works afterwards.
	if _, err := s.Save([]byte(fixturePlan)); err != nil {
		t.Fatalf("Save after a corrupt file: %v", err)
	}
}

func TestStore_SaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "house-plan.json") // parent dir is created
	s := OpenStore(path)
	p, err := s.Save([]byte(fixturePlan))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if s.Current() != p {
		t.Error("Current() is not the saved plan")
	}
	again := OpenStore(path).Current()
	if again == nil || again.ETag != p.ETag {
		t.Fatalf("reopened plan = %+v, want ETag %s", again, p.ETag)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644", info.Mode().Perm())
	}
}

func TestStore_InvalidSaveKeepsPrevious(t *testing.T) {
	path := filepath.Join(t.TempDir(), "house-plan.json")
	s := OpenStore(path)
	first, _ := s.Save([]byte(fixturePlan))
	if _, err := s.Save([]byte(`{"rooms":[]}`)); err == nil {
		t.Fatal("Save accepted a plan without rooms")
	}
	if s.Current() != first {
		t.Error("an invalid save replaced the current plan")
	}
	if got := OpenStore(path).Current(); got == nil || got.ETag != first.ETag {
		t.Error("an invalid save touched the file")
	}
}

func TestStore_SaveLeavesNoTmp(t *testing.T) {
	dir := t.TempDir()
	s := OpenStore(filepath.Join(dir, "house-plan.json"))
	if _, err := s.Save([]byte(fixturePlan)); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "house-plan.json" {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("dir holds %v, want only house-plan.json", names)
	}
}

func TestStore_NoPathCannotSave(t *testing.T) {
	s := OpenStore("")
	if s.Current() != nil {
		t.Error("an unconfigured store has a plan")
	}
	if _, err := s.Save([]byte(fixturePlan)); err == nil {
		t.Error("Save without a path succeeded")
	}
}
