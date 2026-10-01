package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// ScannerToggle enables/disables a scan backend from the UI.
type ScannerToggle struct {
	Enabled bool `json:"enabled"`
}

// IntegrationsSettings is persisted in app_settings (no secrets).
type IntegrationsSettings struct {
	AllowedRegistries []string `json:"allowed_registries"`
	Scanners          struct {
		JFrog        ScannerToggle `json:"jfrog"`
		ECRInspector ScannerToggle `json:"ecr_inspector"`
	} `json:"scanners"`
}

const integrationsSettingsKey = "integrations"

// HasIntegrationsSettings reports whether the UI has ever saved integrations config.
func (s *Store) HasIntegrationsSettings(ctx context.Context) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM app_settings WHERE key = ?`, integrationsSettingsKey).Scan(&n)
	return n > 0, err
}

// DefaultIntegrations returns sensible defaults from which backends are configured in-process.
func DefaultIntegrations(jfrogConfigured, ecrConfigured bool) IntegrationsSettings {
	var out IntegrationsSettings
	out.Scanners.JFrog.Enabled = jfrogConfigured
	out.Scanners.ECRInspector.Enabled = ecrConfigured
	return out
}

// GetIntegrationsSettings reads integration overrides from app_settings.
func (s *Store) GetIntegrationsSettings(ctx context.Context) (IntegrationsSettings, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value_json FROM app_settings WHERE key = ?`, integrationsSettingsKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return IntegrationsSettings{}, nil
	}
	if err != nil {
		return IntegrationsSettings{}, err
	}
	var out IntegrationsSettings
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return IntegrationsSettings{}, err
	}
	return out, nil
}

// SaveIntegrationsSettings persists integration overrides.
func (s *Store) SaveIntegrationsSettings(ctx context.Context, cfg IntegrationsSettings) error {
	if cfg.AllowedRegistries == nil {
		cfg.AllowedRegistries = []string{}
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	ts := nowRFC3339()
	_, err = s.db.ExecContext(ctx, `
INSERT INTO app_settings (key, value_json, updated_at) VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`,
		integrationsSettingsKey, string(b), ts,
	)
	return err
}
