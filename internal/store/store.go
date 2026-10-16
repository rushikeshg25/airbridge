// Package store persists upload metadata and file bodies without buffering whole files.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrNotFound = errors.New("file not found")
	ErrInvalid  = errors.New("invalid file name or size")
	ErrQuota    = errors.New("room storage limit reached; remove files to make space")
	ErrConflict = errors.New("upload offset changed; refresh and resume")
)

type File struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Offset   int64     `json:"offset"`
	Complete bool      `json:"complete"`
	Created  time.Time `json:"created"`
	Sender   string    `json:"sender"`
}
type entry struct {
	mu sync.Mutex
	File
	deleted bool
}
type Store struct {
	mu               sync.RWMutex
	dir              string
	max, quota, used int64
	files            map[string]*entry
}

func Open(dir string, max, quota int64) (*Store, error) {
	if max <= 0 || quota < max {
		return nil, ErrInvalid
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, max: max, quota: quota, files: make(map[string]*entry)}
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var f File
		if err := json.Unmarshal(data, &f); err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		id := strings.TrimSuffix(filepath.Base(path), ".json")
		if !validID(id) || f.ID != id || !validName(f.Name) || f.Size < 0 {
			return nil, fmt.Errorf("invalid metadata: %s", path)
		}
		info, err := os.Stat(s.path(id, ".blob"))
		if err == nil {
			f.Complete = true
		} else if os.IsNotExist(err) {
			info, err = os.Stat(s.path(id, ".part"))
			f.Complete = false
		}
		if err != nil {
			return nil, fmt.Errorf("missing file body for %s: %w", id, err)
		}
		f.Offset = info.Size()
		if f.Offset > f.Size || (f.Complete && f.Offset != f.Size) {
			return nil, fmt.Errorf("unexpected size for %s", id)
		}
		s.files[id] = &entry{File: f}
		s.used += f.Size
	}
	return s, nil
}
func validID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}
func validName(name string) bool {
	return utf8.ValidString(name) && len(name) > 0 && len(name) <= 240 && name != "." && name != ".." && !strings.ContainsAny(name, "/\\") && strings.IndexFunc(name, unicode.IsControl) < 0
}
func (s *Store) path(id, suffix string) string { return filepath.Join(s.dir, id+suffix) }
func (s *Store) save(f File) error {
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	path := s.path(f.ID, ".json")
	tmp, err := os.OpenFile(path+".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}
func (s *Store) Create(name string, size int64, sender string) (File, error) {
	if !validName(name) || size < 0 || size > s.max {
		return File{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if size > s.quota-s.used || len(s.files) >= 10000 {
		return File{}, ErrQuota
	}
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return File{}, err
	}
	f := File{ID: hex.EncodeToString(bytes), Name: name, Size: size, Created: time.Now().UTC(), Sender: sender}
	body, err := os.OpenFile(s.path(f.ID, ".part"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return File{}, err
	}
	if err = body.Close(); err != nil {
		return File{}, err
	}
	if err = s.save(f); err != nil {
		_ = os.Remove(s.path(f.ID, ".part"))
		return File{}, err
	}
	s.files[f.ID] = &entry{File: f}
	s.used += size
	return f, nil
}
func (s *Store) lookup(id string) (*entry, error) {
	s.mu.RLock()
	f := s.files[id]
	s.mu.RUnlock()
	if f == nil {
		return nil, ErrNotFound
	}
	return f, nil
}
func (s *Store) Get(id string) (File, error) {
	e, err := s.lookup(id)
	if err != nil {
		return File{}, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.deleted {
		return File{}, ErrNotFound
	}
	return e.File, nil
}
func (s *Store) List() ([]File, int64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	files := make([]File, 0, len(s.files))
	for _, e := range s.files {
		e.mu.Lock()
		files = append(files, e.File)
		e.mu.Unlock()
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Created.After(files[j].Created) })
	return files, s.used
}
