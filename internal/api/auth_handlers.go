package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/zpol/katana/internal/auth"
	"github.com/zpol/katana/internal/store"
)

// Config GET /api/v1/config — public runtime flags for the UI.
func (s *Server) Config(w http.ResponseWriter, r *http.Request) {
	out := map[string]interface{}{
		"admission_dry_run": s.AdmissionDryRun,
		"version":           s.Version,
	}
	if s.Auth != nil {
		out["auth_mode"] = string(s.Auth.Config.Mode)
		out["local_login_enabled"] = s.Auth.Config.LocalEnabled()
		out["oidc_enabled"] = s.Auth.Config.OIDCConfigured() || s.Auth.EffectiveSSO(r.Context()).Enabled
	}
	writeJSON(w, http.StatusOK, out)
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// AuthLogin POST /api/v1/auth/login
func (s *Server) AuthLogin(w http.ResponseWriter, r *http.Request) {
	if s.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auth not configured"})
		return
	}
	var req loginRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	id, err := s.Auth.LoginLocal(w, r, req.Username, req.Password)
	if err != nil {
		code := http.StatusUnauthorized
		msg := err.Error()
		if he, ok := err.(*auth.HTTPError); ok {
			code = he.Code
			msg = he.Msg
		}
		writeJSON(w, code, map[string]string{"error": msg})
		return
	}
	writeJSON(w, http.StatusOK, identityResponse(id))
}

// AuthLogout POST /api/v1/auth/logout
func (s *Server) AuthLogout(w http.ResponseWriter, r *http.Request) {
	if s.Auth != nil {
		s.Auth.Logout(w, r)
	}
	w.WriteHeader(http.StatusNoContent)
}

// AuthMe GET /api/v1/auth/me
func (s *Server) AuthMe(w http.ResponseWriter, r *http.Request) {
	id, ok := auth.IdentityFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	writeJSON(w, http.StatusOK, identityResponse(id))
}

func identityResponse(id auth.Identity) map[string]interface{} {
	return map[string]interface{}{
		"user_id":  id.UserID,
		"username": id.Username,
		"role":     id.Role,
		"source":   id.Source,
	}
}

// OIDCLogin GET /api/v1/auth/oidc/login
func (s *Server) OIDCLogin(w http.ResponseWriter, r *http.Request) {
	if s.Auth == nil || s.Auth.OIDC == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "oidc not configured"})
		return
	}
	if err := s.Auth.OIDC.LoginRedirect(w, r); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
}

// OIDCCallback GET /api/v1/auth/oidc/callback
func (s *Server) OIDCCallback(w http.ResponseWriter, r *http.Request) {
	if s.Auth == nil || s.Auth.OIDC == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "oidc not configured"})
		return
	}
	id, err := s.Auth.OIDC.HandleCallback(w, r, s.Auth)
	if err != nil {
		code := http.StatusUnauthorized
		msg := err.Error()
		if he, ok := err.(*auth.HTTPError); ok {
			code = he.Code
			msg = he.Msg
		}
		writeJSON(w, code, map[string]string{"error": msg})
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
	_ = id
}

// ListUsers GET /api/v1/users
func (s *Server) ListUsers(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.ListUsers(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if items == nil {
		items = []store.User{}
	}
	writeJSON(w, http.StatusOK, items)
}

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// CreateUser POST /api/v1/users
func (s *Server) CreateUser(w http.ResponseWriter, r *http.Request) {
	if s.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auth not configured"})
		return
	}
	var req createUserRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.Username == "" || req.Password == "" || req.Role == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username, password, and role are required"})
		return
	}
	u, err := s.Auth.CreateLocalUser(r.Context(), req.Username, req.Password, req.Role)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

type patchUserRequest struct {
	Role     *string `json:"role"`
	Password *string `json:"password"`
}

// PatchUser PATCH /api/v1/users/{id}
func (s *Server) PatchUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req patchUserRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.Role != nil {
		if !auth.ValidRole(*req.Role) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid role"})
			return
		}
		if err := s.Store.UpdateUserRole(r.Context(), id, *req.Role); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	if req.Password != nil && *req.Password != "" {
		hash, err := hashPassword(*req.Password)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if err := s.Store.UpdateUserPassword(r.Context(), id, hash); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	u, err := s.Store.GetUserByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// DeleteUser DELETE /api/v1/users/{id}
func (s *Server) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	u, err := s.Store.GetUserByID(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if u.Role == auth.RoleAdmin {
		n, err := s.Store.CountAdminUsers(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if n <= 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot delete the last admin user"})
			return
		}
	}
	if err := s.Store.DeleteUser(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetSSOSettings GET /api/v1/settings/sso
func (s *Server) GetSSOSettings(w http.ResponseWriter, r *http.Request) {
	if s.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auth not configured"})
		return
	}
	writeJSON(w, http.StatusOK, s.Auth.EffectiveSSO(r.Context()))
}

// PutSSOSettings PUT /api/v1/settings/sso
func (s *Server) PutSSOSettings(w http.ResponseWriter, r *http.Request) {
	if s.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auth not configured"})
		return
	}
	var req store.SSOSettings
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	req.ClientSecretSet = s.Auth.Config.OIDCClientSecret != ""
	if err := s.Store.SaveSSOSettings(r.Context(), req); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, s.Auth.EffectiveSSO(r.Context()))
}

// TestSSOSettings POST /api/v1/settings/sso/test
func (s *Server) TestSSOSettings(w http.ResponseWriter, r *http.Request) {
	if s.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auth not configured"})
		return
	}
	sso := s.Auth.EffectiveSSO(r.Context())
	if sso.Issuer == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "issuer not configured"})
		return
	}
	if s.Auth.OIDC != nil {
		if err := s.Auth.OIDC.TestDiscovery(r.Context()); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "OIDC discovery reachable"})
		return
	}
	// Provider not initialized at startup — still test discovery URL.
	if err := testIssuerDiscovery(r, sso.Issuer); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "issuer discovery reachable (restart KATANA after saving SSO to activate OIDC login)",
	})
}
