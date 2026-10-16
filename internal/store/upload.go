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
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	e.mu.Lock()
	snapshot, deleted := e.File, e.deleted
	e.mu.Unlock()
	if deleted {
		return File{}, ErrNotFound
	}
	if snapshot.Complete || offset != snapshot.Offset {
		return snapshot, ErrConflict
	}
	if length <= 0 || length > ChunkSize || length > snapshot.Size-snapshot.Offset {
		return snapshot, ErrInvalid
	}
	f, err := os.OpenFile(s.path(id, ".part"), os.O_WRONLY, 0600)
	if err != nil {
		return snapshot, err
	}
	defer f.Close()
	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		return snapshot, err
	}
	n, err := io.CopyN(f, body, length)
	if err != nil {
		if rollback := f.Truncate(offset); rollback != nil {
			return snapshot, rollback
		}
		return snapshot, err
	}
	snapshot.Offset += n
	e.mu.Lock()
	e.File = snapshot
	e.mu.Unlock()
	return snapshot, nil
}

// Finish publishes only a complete, synced body. A second call is safe after
// losing the first response. Recovery recognizes the renamed body on restart.
func (s *Store) Finish(id string) (File, error) {
	e, err := s.lookup(id)
	if err != nil {
		return File{}, err
	}
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	e.mu.Lock()
	snapshot, deleted := e.File, e.deleted
	e.mu.Unlock()
	if deleted {
		return File{}, ErrNotFound
	}
	if snapshot.Complete {
		return snapshot, nil
	}
	if snapshot.Offset != snapshot.Size {
		return snapshot, ErrConflict
	}
	body, err := os.OpenFile(s.path(id, ".part"), os.O_WRONLY, 0600)
	if err != nil {
		return snapshot, err
	}
	err = body.Sync()
	closeErr := body.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return snapshot, err
	}
	if err = os.Rename(s.path(id, ".part"), s.path(id, ".blob")); err != nil {
		return snapshot, err
	}
	snapshot.Complete = true
	e.mu.Lock()
	e.File = snapshot
	e.mu.Unlock()
	// Completion is recoverable from .blob even if this metadata update fails.
	if err = s.save(snapshot); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}
