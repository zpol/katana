package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

type ctxKey string

const requestIDKey ctxKey = "request_id"

var stdLog = log.New(os.Stdout, "", 0)

// RequestID ensures every request has an X-Request-Id.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-Id", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

// SecurityHeaders adds baseline hardening headers.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; font-src 'self' https://fonts.gstatic.com; style-src-elem 'self' 'unsafe-inline' https://fonts.googleapis.com")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		next.ServeHTTP(w, r)
	})
}

// AuthToken is deprecated; use internal/auth.Service.Middleware instead.
// AuthToken requires X-Katana-Token on /api routes (except health and admission validate).
func AuthToken(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if r.Method == http.MethodOptions ||
				path == "/api/v1/health" ||
				path == "/metrics" ||
				path == "/validate" ||
				!strings.HasPrefix(path, "/api/") {
				next.ServeHTTP(w, r)
				return
			}
			got := r.Header.Get("X-Katana-Token")
			if token == "" || got != token {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// StructuredLogger logs method, path, status, duration, and request id as JSON.
func StructuredLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		payload, _ := json.Marshal(map[string]interface{}{
			"msg":         "request",
			"method":      r.Method,
			"path":        r.URL.Path,
			"status":      ww.Status(),
			"bytes":       ww.BytesWritten(),
			"duration_ms": time.Since(start).Milliseconds(),
			"request_id":  requestIDFrom(r.Context()),
			"remote":      r.RemoteAddr,
		})
		stdLog.Println(string(payload))
	})
}

// LogInfo writes a structured info log line.
func LogInfo(msg string, fields map[string]interface{}) {
	if fields == nil {
		fields = map[string]interface{}{}
	}
	fields["msg"] = msg
	fields["level"] = "info"
	b, _ := json.Marshal(fields)
	stdLog.Println(string(b))
}

// LogError writes a structured error log line.
func LogError(msg string, fields map[string]interface{}) {
	if fields == nil {
		fields = map[string]interface{}{}
	}
	fields["msg"] = msg
	fields["level"] = "error"
	b, _ := json.Marshal(fields)
	stdLog.Println(string(b))
}