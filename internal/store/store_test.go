package store

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestResumePublishAndDownload(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, 100, 200)
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.Create("hello.txt", 11, "phone")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Append(f.ID, 0, 6, bytes.NewBufferString("hello ")); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s, err = Open(dir, 100, 200)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	f, _ = s.Get(f.ID)
	if f.Offset != 6 {
		t.Fatalf("offset %d", f.Offset)
	}
	if _, err = s.Finish(f.ID); !errors.Is(err, ErrConflict) {
		t.Fatal("published incomplete file", err)
	}
	if _, err = s.Append(f.ID, 6, 5, bytes.NewBufferString("world")); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Finish(f.ID); err != nil {
		t.Fatal(err)
	}
	body, _, err := s.Download(f.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	actual, _ := io.ReadAll(body)
	if string(actual) != "hello world" {
		t.Fatalf("body %q", actual)
	}
	if _, err = s.Finish(f.ID); err != nil {
		t.Fatal("finish not idempotent", err)
	}
}
func TestInvalidQuotaAndDelete(t *testing.T) {
	s, _ := Open(t.TempDir(), 100, 100)
	t.Cleanup(func() { _ = s.Close() })
	for _, name := range []string{"../secret", "..", "a/b", "a\\b", "bad\nname", ""} {
		if _, err := s.Create(name, 1, "device"); !errors.Is(err, ErrInvalid) {
			t.Errorf("accepted %q", name)
		}
	}
	f, _ := s.Create("one", 80, "device")
	if _, err := s.Create("two", 30, "device"); !errors.Is(err, ErrQuota) {
		t.Fatal("quota not enforced")
	}
	if err := s.Delete(f.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create("two", 100, "device"); err != nil {
		t.Fatal("quota not released", err)
	}
}
func TestInterruptedAndStaleChunks(t *testing.T) {
	s, _ := Open(t.TempDir(), 100, 200)
	t.Cleanup(func() { _ = s.Close() })
	f, _ := s.Create("file", 10, "device")
	if _, err := s.Append(f.ID, 0, 5, bytes.NewBufferString("ab")); err == nil {
		t.Fatal("short write accepted")
	}
	f, _ = s.Get(f.ID)
	if f.Offset != 0 {
		t.Fatal("partial bytes kept")
	}
	if _, err := s.Append(f.ID, 0, 5, bytes.NewBufferString("hello")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append(f.ID, 0, 5, bytes.NewBufferString("again")); !errors.Is(err, ErrConflict) {
		t.Fatal("stale offset accepted")
	}
}

func TestExclusiveDirectoryLock(t *testing.T) {
	dir := t.TempDir()
	first, err := Open(dir, 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := Open(dir, 100, 100); err == nil {
		other.Close()
		t.Fatal("second writer accepted")
	}
	first.Close()
	second, err := Open(dir, 100, 100)
	if err != nil {
		t.Fatal("lock not released", err)
	}
	second.Close()
}
func TestZeroByteAndConcurrentOffset(t *testing.T) {
	s, _ := Open(t.TempDir(), 100, 200)
	defer s.Close()
	empty, _ := s.Create("empty", 0, "device")
	if _, err := s.Finish(empty.ID); err != nil {
		t.Fatal(err)
	}
	f, _ := s.Create("race", 4, "device")
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { _, err := s.Append(f.ID, 0, 4, bytes.NewBufferString("data")); results <- err }()
	}
	wins := 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			wins++
		} else if !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	}
	if wins != 1 {
		t.Fatalf("expected one writer, got %d", wins)
	}
}

// A slow body must not prevent the room from listing files or accepting new ones.
func TestLibraryRemainsResponsiveDuringUpload(t *testing.T) {
	s, err := Open(t.TempDir(), 100, 200)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	f, _ := s.Create("slow", 4, "phone")
	reader, writer := io.Pipe()
	started := make(chan struct{})
	uploaded := make(chan error, 1)
	go func() {
		_, err := s.Append(f.ID, 0, 4, &signalReader{Reader: reader, started: started})
		uploaded <- err
	}()
	<-started
	listed := make(chan struct{})
	go func() { s.List(); close(listed) }()
	responsive := true
	select {
	case <-listed:
	case <-time.After(150 * time.Millisecond):
		responsive = false
	}
	// Always release the stalled upload, including when testing the broken version.
	_, _ = writer.Write([]byte("data"))
	_ = writer.Close()
	if err := <-uploaded; err != nil {
		t.Fatal(err)
	}
	<-listed
	if !responsive {
		t.Fatal("room library blocked behind a slow upload body")
	}
}

type signalReader struct {
	io.Reader
	started chan struct{}
	once    sync.Once
}

func (r *signalReader) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	return r.Reader.Read(p)
}

func TestRecoveryWithLiteralDirectoryName(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "room[1]")
	s, err := Open(dir, 100, 200)
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.Create("saved", 0, "device")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Finish(f.ID); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(dir, 100, 200)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.Get(f.ID); err != nil {
		t.Fatal("saved file disappeared after restart in a path containing brackets", err)
	}
}
