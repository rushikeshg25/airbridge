package server

import (
	"airbridge/internal/store"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) uploadChunk(w http.ResponseWriter, r *http.Request) {
	offset, err := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
	if err != nil || offset < 0 || r.ContentLength <= 0 || r.ContentLength > store.ChunkSize {
		fail(w, 400, "Send up to 8 MiB with a valid Upload-Offset and Content-Length")
		return
	}
	// Bound slow clients without imposing a timeout on an entire multi-GB upload.
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(2 * time.Minute))
	defer http.NewResponseController(w).SetReadDeadline(time.Time{})
	f, err := s.files.Append(r.PathValue("id"), offset, r.ContentLength, r.Body)
	if err != nil {
		storageError(w, err)
		return
	}
	w.Header().Set("Upload-Offset", strconv.FormatInt(f.Offset, 10))
	reply(w, 200, f)
}

func (s *Server) finishUpload(w http.ResponseWriter, r *http.Request) {
	f, err := s.files.Finish(r.PathValue("id"))
	if err != nil {
		storageError(w, err)
		return
	}
	reply(w, 200, f)
}

// Fingerprint uses bounded samples for accidental-file-mismatch protection.
// This is an identity hint for resume, not a checksum of the entire file.
func (s *Server) fingerprint(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 128<<10))
	if err != nil {
		fail(w, 400, "Fingerprint sample is too large")
		return
	}
	hash := sha256.Sum256(body)
	reply(w, 200, map[string]string{"hash": hex.EncodeToString(hash[:])})
}
