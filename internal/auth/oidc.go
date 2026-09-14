package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// OIDCProvider wraps generic OIDC discovery and token exchange.
type OIDCProvider struct {
	cfg      Config
	oauth    *oauth2.Config
	issuer   string
	authURL  string
	tokenURL string
	jwksURL  string
	jwks     jwksCache
}

type oidcDiscovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

// NewOIDCProvider discovers endpoints from the issuer.
func NewOIDCProvider(cfg Config) (*OIDCProvider, error) {
	issuer := strings.TrimRight(cfg.OIDCIssuer, "/")
	discURL := issuer + "/.well-known/openid-configuration"
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discURL, nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discovery GET: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discovery status %d", res.StatusCode)
	}
	var disc oidcDiscovery
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&disc); err != nil {
		return nil, err
	}
	redirect := cfg.OIDCRedirectURI
	if redirect == "" {
		return nil, fmt.Errorf("KATANA_OIDC_REDIRECT_URI is required")
	}
	if strings.TrimSpace(disc.JWKSURI) == "" {
		return nil, fmt.Errorf("oidc discovery missing jwks_uri")
	}
	oauthCfg := &oauth2.Config{
		ClientID:     cfg.OIDCClientID,
		ClientSecret: cfg.OIDCClientSecret,
		RedirectURL:  redirect,
		Scopes:       cfg.OIDCScopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  disc.AuthorizationEndpoint,
			TokenURL: disc.TokenEndpoint,
		},
	}
	return &OIDCProvider{
		cfg:      cfg,
		oauth:    oauthCfg,
		issuer:   issuer,
		authURL:  disc.AuthorizationEndpoint,
		tokenURL: disc.TokenEndpoint,
		jwksURL:  disc.JWKSURI,
		jwks:     jwksCache{url: disc.JWKSURI},
	}, nil
}

// LoginRedirect starts the OIDC authorization code flow (PKCE + nonce).
func (p *OIDCProvider) LoginRedirect(w http.ResponseWriter, r *http.Request) error {
	state, err := randomState()
	if err != nil {
		return err
	}
	nonce, err := randomState()
	if err != nil {
		return err
	}
	verifier := oauth2.GenerateVerifier()
	if err := setOIDCFlowCookie(w, oidcFlow{State: state, Nonce: nonce, Verifier: verifier}, p.cfg.SecureCookies); err != nil {
		return err
	}
	url := p.oauth.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("nonce", nonce),
	)
	http.Redirect(w, r, url, http.StatusFound)
	return nil
}

type idTokenClaims struct {
	Sub               string                 `json:"sub"`
	Email             string                 `json:"email"`
	PreferredUsername string                 `json:"preferred_username"`
	Name              string                 `json:"name"`
	Iss               string                 `json:"iss"`
	Aud               interface{}            `json:"aud"`
	Exp               float64                `json:"exp"`
	Nonce             string                 `json:"nonce"`
	Raw               map[string]interface{} `json:"-"`
}

// HandleCallback completes OIDC login and returns identity fields.
func (p *OIDCProvider) HandleCallback(w http.ResponseWriter, r *http.Request, svc *Service) (Identity, error) {
	q := r.URL.Query()
	if errMsg := q.Get("error"); errMsg != "" {
		return Identity{}, fmt.Errorf("oidc error: %s", errMsg)
	}
	code := q.Get("code")
	state := q.Get("state")
	if code == "" {
		return Identity{}, fmt.Errorf("missing code")
	}
	flow, err := readOIDCFlow(r)
	if err != nil || flow.State != state {
		return Identity{}, fmt.Errorf("invalid oidc state")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	tok, err := p.oauth.Exchange(ctx, code, oauth2.VerifierOption(flow.Verifier))
	if err != nil {
		return Identity{}, fmt.Errorf("token exchange: %w", err)
	}
	claims, err := p.verifyIDToken(ctx, tok.Extra("id_token"), flow.Nonce)
	if err != nil {
		return Identity{}, err
	}
	email := claims.Email
	if email == "" {
		email, _ = claims.Raw[p.cfg.OIDCEmailClaim].(string)
	}
	username := claims.PreferredUsername
	if username == "" {
		username = claims.Name
	}
	if username == "" {
		username = email
	}
	if username == "" {
		username = claims.Sub
	}
	groups := ExtractGroups(claims.Raw, p.cfg.OIDCGroupClaim)
	role, err := svc.MapGroupsToRole(ctx, groups)
	if err != nil {
		return Identity{}, err
	}
	u, err := svc.UpsertOIDCUser(ctx, claims.Sub, username, email, role)
	if err != nil {
		return Identity{}, err
	}
	sess, err := svc.Store.CreateSession(ctx, u.ID, svc.Config.SessionTTL)
	if err != nil {
		return Identity{}, err
	}
	svc.setSessionCookie(w, sess.ID)
	http.SetCookie(w, &http.Cookie{Name: oidcStateCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: p.cfg.SecureCookies})
	return Identity{UserID: u.ID, Username: u.Username, Role: u.Role, Source: "oidc"}, nil
}

// TestDiscovery validates issuer reachability.
func (p *OIDCProvider) TestDiscovery(ctx context.Context) error {
	u, err := url.Parse(p.issuer + "/.well-known/openid-configuration")
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("discovery returned %d", res.StatusCode)
	}
	return nil
}

// RandomState exported for tests.
func RandomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
