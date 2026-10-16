package store

import (
	"os"
	"strings"
)

func (s *Store) Delete(id string) error {
	e, err := s.lookup(id)
	if err != nil {
		return err
	}
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.deleted {
		return ErrNotFound
	}
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
	s.mu.Lock()
	delete(s.files, id)
	s.used -= e.Size
	s.mu.Unlock()
	return os.Remove(s.path(id, ".deleted"))
}

func (s *Store) recoverDeletes() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".deleted") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".deleted")
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
