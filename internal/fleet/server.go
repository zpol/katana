package fleet

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	prom  *promClient
	store *Store
	token string
	web   string
}

func New(promURL, dbPath, token, webDist string) (*Server, error) {
	st, err := OpenStore(dbPath)
	if err != nil {
		return nil, err
	}
	return &Server{
		prom:  newPromClient(promURL),
		store: st,
		token: strings.TrimSpace(token),
		web:   webDist,
	}, nil
}

func (s *Server) Close() error { return s.store.Close() }

func (s *Server) StartSnapshotLoop(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	go func() {
		defer t.Stop()
		s.snapshotOnce(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.snapshotOnce(ctx)
			}
		}
	}()
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Use(s.auth)
	r.Get("/api/v1/health", s.health)
	r.Get("/api/v1/fleet", s.fleet)
	r.Get("/api/v1/fleet/overview", s.overview)
	r.Get("/api/v1/clusters/{name}", s.cluster)
	r.Get("/api/v1/history", s.history)
	s.mountStatic(r)
	return r
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.token == "" || !strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api/v1/health" {
			next.ServeHTTP(w, r)
			return
		}
		got := r.Header.Get("Authorization")
		if got == "Bearer "+s.token || r.Header.Get("X-Fleet-Token") == s.token {
			next.ServeHTTP(w, r)
			return
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "prometheus": s.prom.base})
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	ov := s.buildOverview(r.Context(), r.URL.Query().Get("window"))
	writeJSON(w, http.StatusOK, ov)
}

func (s *Server) fleet(w http.ResponseWriter, r *http.Request) {
	s.overview(w, r)
}

func (s *Server) cluster(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	ov := s.buildOverview(r.Context(), r.URL.Query().Get("window"))
	for _, c := range ov.Clusters {
		if c.Name == name {
			writeJSON(w, http.StatusOK, map[string]any{"cluster": c, "overview": ov})
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "cluster not found"})
}

func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListDeploys(r.Context(), 100)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, History{Deploys: items})
}

func (s *Server) mountStatic(r chi.Router) {
	webDist := s.web
	if webDist == "" {
		webDist = "web-fleet/dist"
	}
	webDist = filepath.Clean(webDist)
	st, err := os.Stat(webDist)
	if err != nil || !st.IsDir() {
		return
	}
	fileServer := http.FileServer(http.Dir(webDist))
	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/api/") {
			http.NotFound(w, req)
			return
		}
		path := filepath.Join(webDist, filepath.Clean("/"+req.URL.Path))
		rel, err := filepath.Rel(webDist, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			http.NotFound(w, req)
			return
		}
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			http.ServeFile(w, req, filepath.Join(webDist, "index.html"))
			return
		}
		fileServer.ServeHTTP(w, req)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
