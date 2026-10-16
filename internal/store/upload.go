package store

import (
	"io"
	"os"
)

const ChunkSize int64 = 8 << 20

// Append serializes each file independently. An interrupted request rolls back
// its bytes; other uploads remain available. RAM use does not grow with file size.
func (s *Store) Append(id string, offset, length int64, body io.Reader) (File, error) {
	e, err := s.lookup(id)
	if err != nil {
		return File{}, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.deleted {
		return File{}, ErrNotFound
	}
	if e.Complete || offset != e.Offset {
		return e.File, ErrConflict
	}
	if length <= 0 || length > ChunkSize || length > e.Size-e.Offset {
		return e.File, ErrInvalid
	}
	f, err := os.OpenFile(s.path(id, ".part"), os.O_WRONLY, 0600)
	if err != nil {
		return e.File, err
	}
	defer f.Close()
	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		return e.File, err
	}
	n, err := io.CopyN(f, body, length)
	if err != nil {
		if rollback := f.Truncate(offset); rollback != nil {
			return e.File, rollback
		}
		return e.File, err
	}
	e.Offset += n
	return e.File, nil
}
