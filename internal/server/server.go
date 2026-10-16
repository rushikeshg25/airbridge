package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"airbridge/internal/store"
	"airbridge/internal/web"
)

type session struct {
	Name    string
	Expires time.Time
}
type attempt struct {
	Count int
	Until time.Time
}
type Server struct {
	files    *store.Store
	cfg      Config
	mu       sync.Mutex
	sessions map[string]session
	attempts map[string]attempt
}

func New(cfg Config) (*Server, error) {
	files, err := store.Open(cfg.Directory, cfg.MaxFile, cfg.Quota)
	if err != nil {
		return nil, err
	}
	return &Server{files: files, cfg: cfg, sessions: make(map[string]session), attempts: make(map[string]attempt)}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/join", s.join)
	mux.HandleFunc("POST /api/leave", s.require(s.leave))
	mux.HandleFunc("GET /api/room", s.require(s.room))
	mux.HandleFunc("GET /api/files", s.require(s.listFiles))
	mux.HandleFunc("POST /api/files", s.require(s.createFile))
	mux.HandleFunc("GET /api/files/{id}", s.require(s.getFile))
	mux.HandleFunc("PATCH /api/files/{id}", s.require(s.uploadChunk))
	mux.HandleFunc("POST /api/files/{id}/complete", s.require(s.finishUpload))
	mux.Handle("/", web.Handler())
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("Cache-Control", "no-store")
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		configured, _, _ := net.SplitHostPort(s.cfg.Address)
		if net.ParseIP(host) == nil && host != "localhost" && host != configured {
			fail(w, 403, "Use the local IP address shown by the host")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Host != r.Host || (u.Scheme != "http" && u.Scheme != "https") {
				fail(w, 403, "Cross-origin requests are not allowed")
				return
			}
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			fail(w, 403, "Open Airbridge directly to continue")
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, message string) {
	reply(w, status, map[string]string{"error": message})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		fail(w, 400, "Invalid request")
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		fail(w, 400, "Expected one JSON object")
		return false
	}
	return true
}
func (s *Server) join(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PIN  string `json:"pin"`
		Name string `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, a := range s.attempts {
		if now.After(a.Until) {
			delete(s.attempts, key)
		}
	}
	a := s.attempts[ip]
	if a.Count >= 10 {
		fail(w, 429, "Too many attempts. Try again in one minute.")
		return
	}
	if len(s.attempts) >= 4096 {
		fail(w, 503, "Room is busy. Try again shortly.")
		return
	}
	if a.Count == 0 {
		a.Until = now.Add(time.Minute)
	}
	a.Count++
	s.attempts[ip] = a
	if subtle.ConstantTimeCompare([]byte(in.PIN), []byte(s.cfg.PIN)) != 1 {
		fail(w, 401, "That room code does not match. Check the host terminal.")
		return
	}
	for key, v := range s.sessions {
		if now.After(v.Expires) {
			delete(s.sessions, key)
		}
	}
	if len(s.sessions) >= 128 {
		fail(w, 503, "Room is full. Leave from an unused device.")
		return
	}
	if c, err := r.Cookie("airbridge"); err == nil {
		delete(s.sessions, c.Value)
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		fail(w, 500, "Could not create session")
		return
	}
	name := strings.TrimSpace(in.Name)
	if len(name) == 0 || len(name) > 60 {
		name = "Guest device"
	}
	key := hex.EncodeToString(token)
	s.sessions[key] = session{Name: name, Expires: now.Add(24 * time.Hour)}
	delete(s.attempts, ip)
	http.SetCookie(w, &http.Cookie{Name: "airbridge", Value: key, Path: "/", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: 86400})
	reply(w, 200, map[string]bool{"ok": true})
}
func (s *Server) require(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("airbridge")
		if err != nil {
			fail(w, 401, "Join the room to continue")
			return
		}
		s.mu.Lock()
		sess, ok := s.sessions[cookie.Value]
		s.mu.Unlock()
		if !ok || time.Now().After(sess.Expires) {
			fail(w, 401, "Your session ended. Join the room again.")
			return
		}
		next(w, r)
	}
}
func (s *Server) leave(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie("airbridge")
	s.mu.Lock()
	delete(s.sessions, c.Value)
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "airbridge", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	reply(w, 200, map[string]bool{"ok": true})
}
func (s *Server) room(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie("airbridge")
	s.mu.Lock()
	sess := s.sessions[c.Value]
	s.mu.Unlock()
	reply(w, 200, map[string]any{"name": s.cfg.Name, "device": sess.Name, "urls": URLs(s.cfg.Address), "maxFile": s.cfg.MaxFile, "quota": s.cfg.Quota, "chunkSize": 8 << 20})
}
