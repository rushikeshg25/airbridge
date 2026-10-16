package store

import (
	"os"
	"path/filepath"
	"strings"
)

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.files[id]
	if e == nil {
		return ErrNotFound
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	// The tombstone records user intent if the process stops during removal.
	if err := os.Rename(s.path(id, ".json"), s.path(id, ".deleted")); err != nil {
		return err
	}
	suffix := ".part"
	if e.Complete {
		suffix = ".blob"
	}
	if err := os.Remove(s.path(id, suffix)); err != nil && !os.IsNotExist(err) {
		_ = os.Rename(s.path(id, ".deleted"), s.path(id, ".json"))
		return err
	}
	e.deleted = true
	delete(s.files, id)
	s.used -= e.Size
	return os.Remove(s.path(id, ".deleted"))
}

func (s *Store) recoverDeletes() error {
	paths, err := filepath.Glob(filepath.Join(s.dir, "*.deleted"))
	if err != nil {
		return err
	}
	for _, path := range paths {
		id := strings.TrimSuffix(filepath.Base(path), ".deleted")
		if !validID(id) {
			continue
		}
		for _, suffix := range []string{".part", ".blob", ".json.tmp", ".deleted"} {
			if err := os.Remove(s.path(id, suffix)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}
