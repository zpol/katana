package auth

import (
	"os"
	"strings"
	"time"
)

// Mode controls which authentication methods are active.
type Mode string

const (
	ModeLocal  Mode = "local"
	ModeOIDC   Mode = "oidc"
	ModeHybrid Mode = "hybrid"
)

// Config holds authentication settings loaded from environment.
type Config struct {
	Mode                 Mode
	LegacyToken          string
	SessionTTL           time.Duration
	SecureCookies        bool
	BootstrapAdminUser   string
	BootstrapAdminPass   string
	OIDCEnabled          bool
	OIDCIssuer           string
	OIDCClientID         string
	OIDCClientSecret     string
	OIDCRedirectURI      string
	OIDCScopes           []string
	OIDCGroupClaim       string
	OIDCEmailClaim       string
	OIDCAdminGroups      []string
	OIDCReadonlyGroups   []string
}

// LoadConfig reads auth configuration from environment variables.
func LoadConfig(secureCookies bool) Config {
	mode := Mode(strings.ToLower(strings.TrimSpace(os.Getenv("KATANA_AUTH_MODE"))))
	if mode == "" {
		mode = ModeLocal
	}
	oidcEnabled := strings.EqualFold(os.Getenv("KATANA_OIDC_ENABLED"), "true") ||
		mode == ModeOIDC || mode == ModeHybrid

	scopes := splitCSV(env("KATANA_OIDC_SCOPES", "openid profile email"))
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile", "email"}
	}

	return Config{
		Mode:               mode,
		LegacyToken:        strings.TrimSpace(os.Getenv("KATANA_TOKEN")),
		SessionTTL:         parseDuration(env("KATANA_SESSION_TTL", "8h"), 8*time.Hour),
		SecureCookies:      secureCookies,
		BootstrapAdminUser: strings.TrimSpace(os.Getenv("KATANA_BOOTSTRAP_ADMIN_USER")),
		BootstrapAdminPass: os.Getenv("KATANA_BOOTSTRAP_ADMIN_PASSWORD"),
		OIDCEnabled:        oidcEnabled,
		OIDCIssuer:         strings.TrimSpace(os.Getenv("KATANA_OIDC_ISSUER")),
		OIDCClientID:       strings.TrimSpace(os.Getenv("KATANA_OIDC_CLIENT_ID")),
		OIDCClientSecret:   os.Getenv("KATANA_OIDC_CLIENT_SECRET"),
		OIDCRedirectURI:    strings.TrimSpace(os.Getenv("KATANA_OIDC_REDIRECT_URI")),
		OIDCScopes:         scopes,
		OIDCGroupClaim:     env("KATANA_OIDC_GROUP_CLAIM", "groups"),
		OIDCEmailClaim:     env("KATANA_OIDC_EMAIL_CLAIM", "email"),
		OIDCAdminGroups:    splitCSV(os.Getenv("KATANA_OIDC_ADMIN_GROUPS")),
		OIDCReadonlyGroups: splitCSV(os.Getenv("KATANA_OIDC_READONLY_GROUPS")),
	}
}

// LocalEnabled returns true when username/password login is allowed.
func (c Config) LocalEnabled() bool {
	return c.Mode == ModeLocal || c.Mode == ModeHybrid
}

// OIDCConfigured returns true when OIDC can be attempted.
func (c Config) OIDCConfigured() bool {
	return c.OIDCEnabled && c.OIDCIssuer != "" && c.OIDCClientID != "" && c.OIDCClientSecret != ""
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseDuration(raw string, def time.Duration) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || d <= 0 {
		return def
	}
	return d
}
