package scan

import (
	"context"
	"strings"

	"github.com/zpol/katana/internal/imageutil"
	"github.com/zpol/katana/internal/store"
)

// ConfigProvider supplies live integration toggles (UI / app_settings).
type ConfigProvider interface {
	Integrations(ctx context.Context) store.IntegrationsSettings
}

// StaticConfig always returns the same settings (tests / fallback).
type StaticConfig struct {
	Cfg store.IntegrationsSettings
}

// Integrations implements ConfigProvider.
func (s StaticConfig) Integrations(context.Context) store.IntegrationsSettings { return s.Cfg }

// StoreConfig reads integrations from SQLite app_settings, merging defaults when never saved.
type StoreConfig struct {
	Store           *store.Store
	JFrogConfigured bool
	ECRConfigured   bool
}

// Integrations implements ConfigProvider.
func (s *StoreConfig) Integrations(ctx context.Context) store.IntegrationsSettings {
	def := store.DefaultIntegrations(s.JFrogConfigured, s.ECRConfigured)
	if s == nil || s.Store == nil {
		return def
	}
	has, err := s.Store.HasIntegrationsSettings(ctx)
	if err != nil || !has {
		return def
	}
	cfg, err := s.Store.GetIntegrationsSettings(ctx)
	if err != nil {
		return def
	}
	if cfg.AllowedRegistries == nil {
		cfg.AllowedRegistries = []string{}
	}
	return cfg
}

// Router picks JFrog or ECR Inspector by image registry + UI toggles.
type Router struct {
	Config ConfigProvider
	JFrog  Client
	ECR    Client
}

// Name implements Client.
func (r *Router) Name() string { return "router" }

// Configured is true if any enabled backend is configured.
func (r *Router) Configured() bool {
	if r == nil {
		return false
	}
	cfg := r.cfg(context.Background())
	if cfg.Scanners.JFrog.Enabled && r.JFrog != nil && r.JFrog.Configured() {
		return true
	}
	if cfg.Scanners.ECRInspector.Enabled && r.ECR != nil && r.ECR.Configured() {
		return true
	}
	return false
}

// Status aggregates backend status (primary message from enabled scanners).
func (r *Router) Status(ctx context.Context) Status {
	cfg := r.cfg(ctx)
	parts := []string{}
	ok := false
	configured := false
	if cfg.Scanners.JFrog.Enabled && r.JFrog != nil {
		st := r.JFrog.Status(ctx)
		configured = configured || st.Configured
		ok = ok || st.Reachable
		parts = append(parts, "jfrog: "+st.Message)
	}
	if cfg.Scanners.ECRInspector.Enabled && r.ECR != nil {
		st := r.ECR.Status(ctx)
		configured = configured || st.Configured
		ok = ok || st.Reachable
		parts = append(parts, "ecr_inspector: "+st.Message)
	}
	if len(parts) == 0 {
		return Status{Configured: false, Scanner: "router", Message: "no scanners enabled in Settings"}
	}
	return Status{
		Configured: configured,
		Reachable:  ok,
		Scanner:    "router",
		Message:    strings.Join(parts, "; "),
	}
}

// GetScanSummary routes to ECR for ECR hosts, otherwise JFrog, respecting enable flags.
func (r *Router) GetScanSummary(ctx context.Context, ref ArtifactRef) (*Summary, error) {
	cfg := r.cfg(ctx)
	host := imageutil.Parse(ref.ImageRef).Registry

	if imageutil.IsECRHost(host) {
		if !cfg.Scanners.ECRInspector.Enabled {
			return Unavailable(ref, "ecr_inspector", "ECR Inspector scanner disabled in Settings"), nil
		}
		if r.ECR == nil || !r.ECR.Configured() {
			return Unavailable(ref, "ecr_inspector", "ECR Inspector not configured (AWS credentials / region)"), nil
		}
		return r.ECR.GetScanSummary(ctx, ref)
	}

	if !cfg.Scanners.JFrog.Enabled {
		return Unavailable(ref, "jfrog", "JFrog Xray scanner disabled in Settings"), nil
	}
	if r.JFrog == nil || !r.JFrog.Configured() {
		return Unavailable(ref, "jfrog", "JFrog not configured: set JFROG_URL and JFROG_TOKEN"), nil
	}
	return r.JFrog.GetScanSummary(ctx, ref)
}

func (r *Router) cfg(ctx context.Context) store.IntegrationsSettings {
	if r != nil && r.Config != nil {
		return r.Config.Integrations(ctx)
	}
	return store.DefaultIntegrations(
		r != nil && r.JFrog != nil && r.JFrog.Configured(),
		r != nil && r.ECR != nil && r.ECR.Configured(),
	)
}
