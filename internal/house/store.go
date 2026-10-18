package house

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// Store keeps the uploaded plan on the data volume, so it survives a
// restart. It never fails to open: a missing, unreadable or invalid file is
// the same as no plan (the widget then renders as if the 3D view did not
// exist) until the Mac uploads a good one.
type Store struct {
	path string
	mu   sync.RWMutex
	plan *Plan
}

func OpenStore(path string) *Store {
	s := &Store{path: path}
	if path == "" {
		return s
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s
	}
	if err == nil {
		s.plan, err = ParsePlan(raw)
	}
	if err != nil {
		log.Printf("house: ignoring %s: %v", path, err)
		s.plan = nil
	}
	return s
}

// Current is the plan in use, nil when none has been uploaded.
func (s *Store) Current() *Plan {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.plan
}

// Save checks raw, writes it next to the old file and renames it over, and
// only then makes it Current. An invalid plan (*PlanError) or a failed
// write leaves both the file and Current as they were.
func (s *Store) Save(raw []byte) (*Plan, error) {
	p, err := ParsePlan(raw)
	if err != nil {
		return nil, err
	}
	if s.path == "" {
		return nil, fmt.Errorf("no plan file configured")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return nil, fmt.Errorf("create plan dir: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		os.Remove(tmp)
		return nil, fmt.Errorf("write plan: %w", err)
	}
	// WriteFile's mode is filtered by the umask; the file is meant to be
	// readable by whoever inspects the volume.
	os.Chmod(tmp, 0o644)
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		return nil, fmt.Errorf("replace plan: %w", err)
	}
	s.plan = p
	return p, nil
}
