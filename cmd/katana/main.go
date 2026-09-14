package main

import (
	"context"
	"crypto/tls"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/goxray/goxray/internal/admission"
	"github.com/goxray/goxray/internal/api"
	"github.com/goxray/goxray/internal/auth"
	"github.com/goxray/goxray/internal/evaluate"
	"github.com/goxray/goxray/internal/jfrog"
	"github.com/goxray/goxray/internal/metrics"
	"github.com/goxray/goxray/internal/policy"
	"github.com/goxray/goxray/internal/policystore"
	"github.com/goxray/goxray/internal/store"
	"github.com/goxray/goxray/internal/version"
	"github.com/prometheus/client_golang/prometheus"
)

func main() {
	addr := env("KATANA_ADDR", "0.0.0.0:8080")
	dbPath := env("KATANA_DB_PATH", "./data/katana.db")
	jfURL := env("JFROG_URL", "")
	jfToken := env("JFROG_TOKEN", "")
	tlsCert := env("KATANA_TLS_CERT", "")
	tlsKey := env("KATANA_TLS_KEY", "")
	admissionDry := strings.EqualFold(env("KATANA_ADMISSION_DRY_RUN", "true"), "true")
	secureCookies := tlsCert != "" && tlsKey != ""

	st, err := store.Open(dbPath)
	if err != nil {
		api.LogError("open store", map[string]interface{}{"err": err.Error()})
		os.Exit(1)
	}
	defer st.Close()

	authCfg := auth.LoadConfig(secureCookies)
	authSvc, err := auth.NewService(st, authCfg)
	if err != nil {
		api.LogError("auth init", map[string]interface{}{"err": err.Error()})
		os.Exit(1)
	}
	if err := validateAuthStartup(context.Background(), st, authCfg); err != nil {
		api.LogError("auth config", map[string]interface{}{"err": err.Error()})
		os.Exit(1)
	}

	if err := st.SeedDefaults(context.Background()); err != nil {
		api.LogError("seed", map[string]interface{}{"err": err.Error()})
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	policyStore, policySource, err := policystore.New(ctx, st)
	if err != nil {
		api.LogError("policy store", map[string]interface{}{"err": err.Error(), "source": policySource})
		os.Exit(1)
	}
	defer policyStore.Close()

	jf := jfrog.NewHTTPClient(jfURL, jfToken)
	eval := &evaluate.Service{
		Policies:  policyStore,
		Events:    st,
		JFrog:     jf,
		Evaluator: policy.NewEvaluator(),
	}
	admit := admission.NewHandler(eval, admissionDry)

	metrics.MustRegister(prometheus.DefaultRegisterer)
	_ = prometheus.DefaultRegisterer.Register(metrics.NewRuntimeCollector(metrics.Runtime{
		Version:      version.Current(),
		PolicySource: policySource,
		DryRun:       admissionDry,
		Ready:        func() bool { return policyStore.Ready(context.Background()) },
		JFrogUp: func() bool {
			if jf == nil || !jf.Configured() {
				return false
			}
			return true
		},
	}))

	srvAPI := &api.Server{
		Store:           st,
		Policies:        policyStore,
		PolicySource:    policySource,
		Eval:            eval,
		JFrog:           jf,
		Admit:           admit,
		Auth:            authSvc,
		AdmissionDryRun: admissionDry,
		Version:         version.Current(),
	}
	r := chi.NewRouter()
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(api.RequestID)
	r.Use(api.SecurityHeaders)
	r.Use(api.StructuredLogger)
	r.Use(authSvc.Middleware)

	srvAPI.Routes(r)
	mountStatic(r)

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
	}

	go func() {
		api.LogInfo("katana listening", map[string]interface{}{
			"addr":            addr,
			"db":              dbPath,
			"jfrogConfigured": jf.Configured(),
			"admissionDryRun": admissionDry,
			"policySource":    policySource,
			"policiesReady":   policyStore.Ready(context.Background()),
			"tls":             secureCookies,
			"version":         version.Current(),
			"authMode":        authCfg.Mode,
			"localLogin":      authCfg.LocalEnabled(),
			"oidcConfigured":  authCfg.OIDCConfigured(),
		})
		var err error
		if secureCookies {
			httpSrv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
			err = httpSrv.ListenAndServeTLS(tlsCert, tlsKey)
		} else {
			err = httpSrv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			api.LogError("server failed", map[string]interface{}{"err": err.Error()})
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	api.LogInfo("shutdown complete", nil)
}

func validateAuthStartup(ctx context.Context, st *store.Store, cfg auth.Config) error {
	hasLegacy := cfg.LegacyToken != ""
	hasBootstrap := cfg.BootstrapAdminUser != "" && cfg.BootstrapAdminPass != ""
	hasOIDC := cfg.OIDCConfigured()
	n, err := st.CountLocalUsers(ctx)
	if err != nil {
		return err
	}
	if n == 0 && !hasBootstrap && !hasLegacy && !hasOIDC {
		return errAuth("configure KATANA_BOOTSTRAP_ADMIN_USER/PASSWORD, KATANA_TOKEN, or OIDC")
	}
	return nil
}

type authStartupError string

func (e authStartupError) Error() string { return string(e) }

func errAuth(msg string) error { return authStartupError(msg) }

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func mountStatic(r chi.Router) {
	webDist := filepath.Clean("web/dist")
	if st, err := os.Stat(webDist); err != nil || !st.IsDir() {
		api.LogInfo("web dist not found; UI not mounted", map[string]interface{}{"path": webDist})
		return
	}
	fileServer := http.FileServer(http.Dir(webDist))
	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/api/") || req.URL.Path == "/validate" || req.URL.Path == "/metrics" {
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
