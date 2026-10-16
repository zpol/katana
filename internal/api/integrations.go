package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/zpol/katana/internal/policy"
	"github.com/zpol/katana/internal/scan"
	"github.com/zpol/katana/internal/store"
)

const registryAllowlistPolicyID = "registry-allowlist"

// GetIntegrationsSettings GET /api/v1/settings/integrations
func (s *Server) GetIntegrationsSettings(w http.ResponseWriter, r *http.Request) {
	cfg := s.effectiveIntegrations(r.Context())
	writeJSON(w, http.StatusOK, cfg)
}

// PutIntegrationsSettings PUT /api/v1/settings/integrations
func (s *Server) PutIntegrationsSettings(w http.ResponseWriter, r *http.Request) {
	var cfg store.IntegrationsSettings
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	cfg.AllowedRegistries = normalizeRegistryList(cfg.AllowedRegistries)
	if err := s.Store.SaveIntegrationsSettings(r.Context(), cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := s.syncRegistryAllowlistPolicy(r.Context(), cfg.AllowedRegistries); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"settings": cfg,
			"warning":  "saved settings but failed to sync Registry Allowlist policy: " + err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// ECRStatus GET /api/v1/integrations/ecr
func (s *Server) ECRStatus(w http.ResponseWriter, r *http.Request) {
	if s.ECR == nil {
		writeJSON(w, http.StatusOK, scan.Status{Configured: false, Scanner: "ecr_inspector", Message: "client not initialized"})
		return
	}
	writeJSON(w, http.StatusOK, s.ECR.Status(r.Context()))
}

func (s *Server) effectiveIntegrations(ctx context.Context) store.IntegrationsSettings {
	jfOK := s.JFrog != nil && s.JFrog.Configured()
	ecrOK := s.ECR != nil && s.ECR.Configured()
	def := store.DefaultIntegrations(jfOK, ecrOK)
	if s.Store == nil {
		return def
	}
	has, err := s.Store.HasIntegrationsSettings(ctx)
	if err != nil || !has {
		// Prefer registries from existing Registry Allowlist policy when present.
		if regs := s.registriesFromPolicy(ctx); len(regs) > 0 {
			def.AllowedRegistries = regs
		}
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

func (s *Server) registriesFromPolicy(ctx context.Context) []string {
	if s.Policies == nil {
		return nil
	}
	p, err := s.Policies.Get(ctx, registryAllowlistPolicyID)
	if err != nil {
		return nil
	}
	return normalizeRegistryList(p.Match.RegistryAllowlist)
}

func normalizeRegistryList(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, r := range in {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		key := strings.ToLower(r)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, r)
	}
	return out
}

func (s *Server) syncRegistryAllowlistPolicy(ctx context.Context, registries []string) error {
	if s.Policies == nil {
		return nil
	}
	registries = normalizeRegistryList(registries)
	p, err := s.Policies.Get(ctx, registryAllowlistPolicyID)
	if err != nil {
		// Create if missing.
		np := policy.Policy{
			ID:          registryAllowlistPolicyID,
			Name:        "Registry Allowlist",
			Description: "Only allow images from approved registries (managed from Settings → Integrations)",
			Enabled:     len(registries) > 0,
			Action:      policy.ActionDeny,
			Match:       policy.MatchCriteria{RegistryAllowlist: registries},
			Exceptions:  []string{"kube-system", "kube-public", "kube-node-lease", "katana-system"},
			DenyMessage: `Image {image} uses registry "{registry}" which is not on the approved list. Policy: {policy}.
Pull from an approved registry configured in Settings → Integrations.`,
		}
		_, err = s.Policies.Create(ctx, np)
		return err
	}
	p.Match.RegistryAllowlist = registries
	p.Enabled = len(registries) > 0
	if p.Description == "" {
		p.Description = "Only allow images from approved registries (managed from Settings → Integrations)"
	}
	_, err = s.Policies.Update(ctx, p)
	return err
}
