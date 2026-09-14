package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type ctxKey int

const identityKey ctxKey = 1

// WithIdentity stores identity on context.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey, id)
}

// IdentityFrom returns identity from context.
func IdentityFrom(ctx context.Context) (Identity, bool) {
	v, ok := ctx.Value(identityKey).(Identity)
	return v, ok
}

// Middleware authenticates API requests (except public paths).
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r) {
			next.ServeHTTP(w, r)
			return
		}
		id, err := s.AuthenticateRequest(r)
		if err != nil {
			code := http.StatusUnauthorized
			msg := err.Error()
			if he, ok := err.(*HTTPError); ok {
				code = he.Code
				msg = he.Msg
			}
			writeJSON(w, code, map[string]string{"error": msg})
			return
		}
		next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
	})
}

// RequireAdmin wraps handlers that mutate state.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := IdentityFrom(r.Context())
		if !ok || id.Role != RoleAdmin {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin role required"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isPublicPath(r *http.Request) bool {
	if r.Method == http.MethodOptions {
		return true
	}
	path := r.URL.Path
	switch path {
	case "/api/v1/health", "/api/v1/config", "/metrics":
		return true
	case "/api/v1/auth/login", "/api/v1/auth/oidc/login", "/api/v1/auth/oidc/callback":
		return true
	case "/validate":
		// kube-apiserver calls this path; /api/v1/admission/validate requires auth.
		return true
	}
	return !strings.HasPrefix(path, "/api/")
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
