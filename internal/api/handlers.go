package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/goxray/goxray/internal/admission"
	"github.com/goxray/goxray/internal/auth"
	"github.com/goxray/goxray/internal/detect"
	"github.com/goxray/goxray/internal/evaluate"
	"github.com/goxray/goxray/internal/jfrog"
	"github.com/goxray/goxray/internal/metrics"
	"github.com/goxray/goxray/internal/policy"
	"github.com/goxray/goxray/internal/policystore"
	"github.com/goxray/goxray/internal/store"
)

// Server holds API dependencies.
type Server struct {
	Store            *store.Store
	Policies         policystore.Store
	PolicySource     string
	Eval             *evaluate.Service
	JFrog            jfrog.Client
	Admit            *admission.Handler
	Auth             *auth.Service
	AdmissionDryRun  bool
	Version          string
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Health returns liveness.
func (s *Server) Health(w http.ResponseWriter, r *http.Request) {
	out := map[string]interface{}{"status": "ok"}
	if s.Version != "" {
		out["version"] = s.Version
	}
	if s.PolicySource != "" {
		out["policy_source"] = s.PolicySource
	}
	if s.Policies != nil {
		out["policies_ready"] = s.Policies.Ready(r.Context())
	}
	writeJSON(w, http.StatusOK, out)
}

// JFrogStatus GET /api/v1/integrations/jfrog
func (s *Server) JFrogStatus(w http.ResponseWriter, r *http.Request) {
	if s.JFrog == nil {
		writeJSON(w, http.StatusOK, jfrog.Status{Configured: false, Message: "client not initialized"})
		return
	}
	writeJSON(w, http.StatusOK, s.JFrog.Status(r.Context()))
}

// Evaluate POST /api/v1/evaluate
func (s *Server) Evaluate(w http.ResponseWriter, r *http.Request) {
	if s.Eval == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "evaluator not ready"})
		return
	}
	var req evaluate.Request
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.Image == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "image is required"})
		return
	}
	if req.Record {
		id, ok := auth.IdentityFrom(r.Context())
		if !ok || id.Role != auth.RoleAdmin {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin role required to record evaluations"})
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), evaluateTimeout())
	defer cancel()
	res, err := s.Eval.EvaluateImage(ctx, req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			writeJSON(w, http.StatusGatewayTimeout, map[string]string{
				"error": "evaluate timed out waiting for JFrog Xray (try a known indexed image or retry)",
			})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// evaluateTimeout matches JFrog client timeout plus headroom for policy evaluation.
func evaluateTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("JFROG_TIMEOUT"))
	if raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d >= 5*time.Second {
			return d + 15*time.Second
		}
	}
	return 60 * time.Second
}

// ListPolicies GET /api/v1/policies
func (s *Server) ListPolicies(w http.ResponseWriter, r *http.Request) {
	items, err := s.Policies.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if items == nil {
		items = []policy.Policy{}
	}
	writeJSON(w, http.StatusOK, items)
}

// CreatePolicy POST /api/v1/policies
func (s *Server) CreatePolicy(w http.ResponseWriter, r *http.Request) {
	var p policy.Policy
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if p.Name == "" || p.Action == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and action are required"})
		return
	}
	if p.Action != policy.ActionDeny && p.Action != policy.ActionWarn && p.Action != policy.ActionAudit {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action must be deny, warn, or audit"})
		return
	}
	if p.ID == "" {
		p.ID = policystore.SlugName(p.Name)
	}
	created, err := s.Policies.Create(r.Context(), p)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// GetPolicy GET /api/v1/policies/{id}
func (s *Server) GetPolicy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, err := s.Policies.Get(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// PutPolicy PUT /api/v1/policies/{id}
func (s *Server) PutPolicy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var p policy.Policy
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	p.ID = id
	updated, err := s.Policies.Update(r.Context(), p)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// PatchPolicy PATCH /api/v1/policies/{id}
func (s *Server) PatchPolicy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var patch map[string]json.RawMessage
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&patch); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	updated, err := s.Policies.Patch(r.Context(), id, patch)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// DeletePolicy DELETE /api/v1/policies/{id}
func (s *Server) DeletePolicy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := s.Policies.Delete(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListDetections GET /api/v1/detections
func (s *Server) ListDetections(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	items, err := s.Store.ListDetections(r.Context(), detect.Filter{
		Severity:    q.Get("severity"),
		Environment: q.Get("environment"),
		Namespace:   q.Get("namespace"),
		Registry:    q.Get("registry"),
		Outcome:     q.Get("outcome"),
		Q:           q.Get("q"),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if items == nil {
		items = []detect.Detection{}
	}
	writeJSON(w, http.StatusOK, items)
}

// StatsSummary GET /api/v1/stats/summary
func (s *Server) StatsSummary(w http.ResponseWriter, r *http.Request) {
	stats, err := s.Store.GetStatsSummary(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// Routes mounts API routes on a chi router.
func (s *Server) Routes(r chi.Router) {
	r.Get("/api/v1/health", s.Health)
	r.Get("/metrics", metrics.Handler().ServeHTTP)
	r.Get("/api/v1/config", s.Config)
	r.Post("/api/v1/auth/login", s.AuthLogin)
	r.Get("/api/v1/auth/oidc/login", s.OIDCLogin)
	r.Get("/api/v1/auth/oidc/callback", s.OIDCCallback)

	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAdmin)
		r.Post("/api/v1/policies", s.CreatePolicy)
		r.Put("/api/v1/policies/{id}", s.PutPolicy)
		r.Patch("/api/v1/policies/{id}", s.PatchPolicy)
		r.Delete("/api/v1/policies/{id}", s.DeletePolicy)
		r.Post("/api/v1/users", s.CreateUser)
		r.Patch("/api/v1/users/{id}", s.PatchUser)
		r.Delete("/api/v1/users/{id}", s.DeleteUser)
		r.Get("/api/v1/users", s.ListUsers)
		r.Put("/api/v1/settings/sso", s.PutSSOSettings)
		r.Post("/api/v1/settings/sso/test", s.TestSSOSettings)
	})

	r.Post("/api/v1/auth/logout", s.AuthLogout)
	r.Get("/api/v1/auth/me", s.AuthMe)
	r.Get("/api/v1/integrations/jfrog", s.JFrogStatus)
	r.Post("/api/v1/evaluate", s.Evaluate)
	r.Get("/api/v1/policies", s.ListPolicies)
	r.Get("/api/v1/policies/{id}", s.GetPolicy)
	r.Get("/api/v1/detections", s.ListDetections)
	r.Get("/api/v1/stats/summary", s.StatsSummary)
	r.Get("/api/v1/settings/sso", s.GetSSOSettings)
	if s.Admit != nil {
		r.Post("/validate", s.Admit.ServeHTTP)
		r.Post("/api/v1/admission/validate", s.Admit.ServeHTTP)
	}
}
