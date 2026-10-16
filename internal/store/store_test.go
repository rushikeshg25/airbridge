package store

import (
	"bytes"
	"errors"
	"io"
	"testing"
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
	s, err = Open(dir, 100, 200)
	if err != nil {
		t.Fatal(err)
	}
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
