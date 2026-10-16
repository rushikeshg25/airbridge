package store

import "os"

func (s *Store) Download(id string) (*os.File, File, error) {
	e, err := s.lookup(id)
	if err != nil {
		return nil, File{}, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.deleted {
		return nil, File{}, ErrNotFound
	}
	if !e.Complete {
		return nil, e.File, ErrConflict
	}
	body, err := os.Open(s.path(id, ".blob"))
	return body, e.File, err
}
