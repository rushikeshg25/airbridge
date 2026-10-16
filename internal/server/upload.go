package server

import (
	"airbridge/internal/store"
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
