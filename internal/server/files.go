package server

import (
	"airbridge/internal/store"
	"errors"
	"log"
	"net/http"
)

func storageError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		fail(w, 404, err.Error())
	case errors.Is(err, store.ErrInvalid):
		fail(w, 400, err.Error())
	case errors.Is(err, store.ErrQuota):
		fail(w, 507, err.Error())
	case errors.Is(err, store.ErrConflict):
		fail(w, 409, err.Error())
	default:
		log.Printf("storage: %v", err)
		fail(w, 500, "Storage operation failed. Check host disk space and permissions.")
	}
}
func (s *Server) listFiles(w http.ResponseWriter, r *http.Request) {
	files, used := s.files.List()
	reply(w, 200, map[string]any{"files": files, "used": used, "quota": s.cfg.Quota})
}
func (s *Server) createFile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	}
	if !decode(w, r, &in) {
		return
	}
	c, _ := r.Cookie("airbridge")
	s.mu.Lock()
	sender := s.sessions[c.Value].Name
	s.mu.Unlock()
	f, err := s.files.Create(in.Name, in.Size, sender)
	if err != nil {
		storageError(w, err)
		return
	}
	reply(w, 201, f)
}
func (s *Server) getFile(w http.ResponseWriter, r *http.Request) {
	f, err := s.files.Get(r.PathValue("id"))
	if err != nil {
		storageError(w, err)
		return
	}
	reply(w, 200, f)
}

func (s *Server) deleteFile(w http.ResponseWriter, r *http.Request) {
	if err := s.files.Delete(r.PathValue("id")); err != nil {
		storageError(w, err)
		return
	}
	reply(w, 200, map[string]bool{"ok": true})
}
