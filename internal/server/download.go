package server

import (
	"mime"
	"net/http"
)

func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	body, f, err := s.files.Download(r.PathValue("id"))
	if err != nil {
		storageError(w, err)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": f.Name}))
	w.Header().Set("ETag", `"`+f.ID+`"`)
	// ServeContent supports HEAD, byte ranges, and efficient streaming. Never
	// construct a Blob in browser memory for multi-gigabyte downloads.
	http.ServeContent(w, r, f.Name, f.Created, body)
}
