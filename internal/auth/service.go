package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/zpol/katana/internal/store"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookie   = "katana_session"
	oidcStateCookie = "katana_oidc_state"
)

// Identity is the authenticated caller attached to request context.
type Identity struct {
	UserID   string
	Username string
	Role     string
	Source   string // local | oidc | legacy_token
}

// Service coordinates local login, sessions, OIDC, and legacy tokens.
type Service struct {
	Store  *store.Store
	Config Config
	OIDC   *OIDCProvider
}

// NewService builds an auth service and runs bootstrap if needed.
func NewService(st *store.Store, cfg Config) (*Service, error) {
	s := &Service{Store: st, Config: cfg}
	if cfg.OIDCConfigured() {
		oidc, err := NewOIDCProvider(cfg)
		if err != nil {
			return nil, fmt.Errorf("oidc: %w", err)
		}
		s.OIDC = oidc
	}
	if err := s.bootstrapAdmin(context.Background()); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) bootstrapAdmin(ctx context.Context) error {
	if s.Config.BootstrapAdminUser == "" || s.Config.BootstrapAdminPass == "" {
		return nil
	}
	n, err := s.Store.CountLocalUsers(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(s.Config.BootstrapAdminPass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.Store.CreateUser(ctx, store.User{
		Username: s.Config.BootstrapAdminUser,
		Role:     RoleAdmin,
		Source:   "local",
	}, string(hash))
	return err
}

// EffectiveSSO merges env config with DB overrides.
func (s *Service) EffectiveSSO(ctx context.Context) store.SSOSettings {
	db, _ := s.Store.GetSSOSettings(ctx)
	out := store.SSOSettings{
		Enabled:         s.Config.OIDCEnabled,
		Issuer:          s.Config.OIDCIssuer,
		ClientID:        s.Config.OIDCClientID,
		RedirectURI:     s.Config.OIDCRedirectURI,
		Scopes:          append([]string(nil), s.Config.OIDCScopes...),
		GroupClaim:      s.Config.OIDCGroupClaim,
		EmailClaim:      s.Config.OIDCEmailClaim,
		AdminGroups:     append([]string(nil), s.Config.OIDCAdminGroups...),
		ReadonlyGroups:  append([]string(nil), s.Config.OIDCReadonlyGroups...),
		ClientSecretSet: s.Config.OIDCClientSecret != "",
	}
	if db.Issuer != "" {
		out.Issuer = db.Issuer
	}
	if db.ClientID != "" {
		out.ClientID = db.ClientID
	}
	if db.RedirectURI != "" {
		out.RedirectURI = db.RedirectURI
	}
	if len(db.Scopes) > 0 {
		out.Scopes = db.Scopes
	}
	if db.GroupClaim != "" {
		out.GroupClaim = db.GroupClaim
	}
	if db.EmailClaim != "" {
		out.EmailClaim = db.EmailClaim
	}
	if len(db.AdminGroups) > 0 {
		out.AdminGroups = db.AdminGroups
	}
	if len(db.ReadonlyGroups) > 0 {
		out.ReadonlyGroups = db.ReadonlyGroups
	}
	if db.Enabled || s.Config.OIDCEnabled {
		out.Enabled = true
	}
	out.Scopes = nonEmptySlice(out.Scopes)
	out.AdminGroups = nonEmptySlice(out.AdminGroups)
	out.ReadonlyGroups = nonEmptySlice(out.ReadonlyGroups)
	return out
}

func nonEmptySlice(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

// AuthenticateRequest resolves identity from session cookie or legacy token.
func (s *Service) AuthenticateRequest(r *http.Request) (Identity, error) {
	if tok := strings.TrimSpace(r.Header.Get("X-Katana-Token")); tok != "" && s.Config.LegacyToken != "" && tok == s.Config.LegacyToken {
		return Identity{Username: "api-token", Role: RoleAdmin, Source: "legacy_token"}, nil
	}
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		sess, err := s.Store.GetSession(r.Context(), c.Value)
		if err != nil {
			return Identity{}, errUnauthorized("invalid session")
		}
		u, err := s.Store.GetUserByID(r.Context(), sess.UserID)
		if err != nil {
			return Identity{}, errUnauthorized("user not found")
		}
		return Identity{UserID: u.ID, Username: u.Username, Role: u.Role, Source: u.Source}, nil
	}
	return Identity{}, errUnauthorized("authentication required")
}

// LoginLocal validates credentials and creates a session cookie.
func (s *Service) LoginLocal(w http.ResponseWriter, r *http.Request, username, password string) (Identity, error) {
	if !s.Config.LocalEnabled() {
		return Identity{}, errForbidden("local login disabled")
	}
	u, hash, err := s.Store.GetUserByUsername(r.Context(), username)
	if err != nil {
		return Identity{}, errUnauthorized("invalid credentials")
	}
	if hash == "" {
		return Identity{}, errUnauthorized("invalid credentials")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return Identity{}, errUnauthorized("invalid credentials")
	}
	sess, err := s.Store.CreateSession(r.Context(), u.ID, s.Config.SessionTTL)
	if err != nil {
		return Identity{}, err
	}
	s.setSessionCookie(w, sess.ID)
	return Identity{UserID: u.ID, Username: u.Username, Role: u.Role, Source: u.Source}, nil
}

// Logout clears the session cookie.
func (s *Service) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		_ = s.Store.DeleteSession(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.Config.SecureCookies,
	})
}

func (s *Service) setSessionCookie(w http.ResponseWriter, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    sessionID,
		Path:     "/",
		MaxAge:   int(s.Config.SessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.Config.SecureCookies,
	})
}

// CreateLocalUser adds a local user (admin only).
func (s *Service) CreateLocalUser(ctx context.Context, username, password, role string) (store.User, error) {
	if !ValidRole(role) {
		return store.User{}, fmt.Errorf("invalid role")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return store.User{}, err
	}
	return s.Store.CreateUser(ctx, store.User{
		Username: username,
		Role:     role,
		Source:   "local",
	}, string(hash))
}

// MapGroupsToRole resolves OIDC group membership to a KATANA role.
func (s *Service) MapGroupsToRole(ctx context.Context, groups []string) (string, error) {
	sso := s.EffectiveSSO(ctx)
	groupSet := make(map[string]struct{}, len(groups))
	for _, g := range groups {
		groupSet[strings.ToLower(strings.TrimSpace(g))] = struct{}{}
	}
	for _, g := range sso.AdminGroups {
		if _, ok := groupSet[strings.ToLower(strings.TrimSpace(g))]; ok {
			return RoleAdmin, nil
		}
	}
	for _, g := range sso.ReadonlyGroups {
		if _, ok := groupSet[strings.ToLower(strings.TrimSpace(g))]; ok {
			return RoleReadonly, nil
		}
	}
	return "", errForbidden("no matching OIDC group for KATANA access")
}

// UpsertOIDCUser links or updates an OIDC identity.
func (s *Service) UpsertOIDCUser(ctx context.Context, sub, username, email, role string) (store.User, error) {
	if u, err := s.Store.GetUserByExternalSub(ctx, sub); err == nil {
		_ = s.Store.UpdateOIDCUser(ctx, u.ID, role, email)
		u.Role = role
		u.Email = email
		return u, nil
	}
	return s.Store.CreateUser(ctx, store.User{
		Username:    username,
		Role:        role,
		Source:      "oidc",
		Email:       email,
		ExternalSub: sub,
	}, "")
}

func randomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

type oidcFlow struct {
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
}

func setOIDCFlowCookie(w http.ResponseWriter, flow oidcFlow, secure bool) error {
	b, err := json.Marshal(flow)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookie,
		Value:    base64.RawURLEncoding.EncodeToString(b),
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
	return nil
}

func readOIDCFlow(r *http.Request) (oidcFlow, error) {
	c, err := r.Cookie(oidcStateCookie)
	if err != nil || c.Value == "" {
		return oidcFlow{}, errors.New("invalid oidc state")
	}
	raw, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil {
		return oidcFlow{}, errors.New("invalid oidc state")
	}
	var flow oidcFlow
	if err := json.Unmarshal(raw, &flow); err != nil || flow.State == "" {
		return oidcFlow{}, errors.New("invalid oidc state")
	}
	return flow, nil
}

// ExtractGroups parses group claim from ID token claims map.
func ExtractGroups(claims map[string]interface{}, claimName string) []string {
	if claimName == "" {
		claimName = "groups"
	}
	raw, ok := claims[claimName]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []interface{}:
		var out []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	case string:
		return splitCSV(v)
	default:
		return nil
	}
}

// ClaimsJSON helper for logging/debug (no secrets).
func ClaimsJSON(claims map[string]interface{}) string {
	b, _ := json.Marshal(claims)
	return string(b)
}

func errUnauthorized(msg string) error { return &HTTPError{Code: http.StatusUnauthorized, Msg: msg} }
func errForbidden(msg string) error    { return &HTTPError{Code: http.StatusForbidden, Msg: msg} }

// HTTPError is an auth error with HTTP status.
type HTTPError struct {
	Code int
	Msg  string
}

func (e *HTTPError) Error() string { return e.Msg }
