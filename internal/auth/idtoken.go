package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Typ string `json:"typ"`
}

type jwksDoc struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwksCache struct {
	mu      sync.Mutex
	url     string
	fetched time.Time
	keys    map[string]*rsa.PublicKey
}

func (c *jwksCache) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.fetched) > 10*time.Minute || len(c.keys) == 0 {
		if err := c.refresh(ctx); err != nil {
			if len(c.keys) == 0 {
				return nil, err
			}
		}
	}
	pub, ok := c.keys[kid]
	if !ok {
		if err := c.refresh(ctx); err != nil {
			return nil, err
		}
		pub, ok = c.keys[kid]
	}
	if !ok {
		return nil, fmt.Errorf("jwks: unknown kid %q", kid)
	}
	return pub, nil
}

func (c *jwksCache) refresh(ctx context.Context) error {
	if c.url == "" {
		return fmt.Errorf("jwks: empty url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("jwks GET: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks status %d", res.StatusCode)
	}
	var doc jwksDoc
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&doc); err != nil {
		return err
	}
	keys := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		if !strings.EqualFold(k.Kty, "RSA") {
			continue
		}
		pub, err := rsaPublicKey(k.N, k.E)
		if err != nil {
			continue
		}
		if k.Kid != "" {
			keys[k.Kid] = pub
		}
	}
	if len(keys) == 0 {
		return fmt.Errorf("jwks: no RSA keys")
	}
	c.keys = keys
	c.fetched = time.Now()
	return nil
}

func rsaPublicKey(nB64, eB64 string) (*rsa.PublicKey, error) {
	nb, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, err
	}
	eb, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, err
	}
	if len(nb) == 0 || len(eb) == 0 {
		return nil, fmt.Errorf("invalid rsa jwk")
	}
	e := 0
	for _, b := range eb {
		e = e<<8 | int(b)
	}
	if e <= 0 {
		return nil, fmt.Errorf("invalid rsa exponent")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: e}, nil
}

func (p *OIDCProvider) verifyIDToken(ctx context.Context, raw interface{}, nonce string) (idTokenClaims, error) {
	s, ok := raw.(string)
	if !ok || s == "" {
		return idTokenClaims{}, fmt.Errorf("missing id_token")
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return idTokenClaims{}, fmt.Errorf("invalid id_token")
	}
	hdrJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return idTokenClaims{}, fmt.Errorf("id_token header: %w", err)
	}
	var hdr jwtHeader
	if err := json.Unmarshal(hdrJSON, &hdr); err != nil {
		return idTokenClaims{}, err
	}
	if !strings.EqualFold(hdr.Alg, "RS256") {
		return idTokenClaims{}, fmt.Errorf("id_token alg %q not allowed", hdr.Alg)
	}
	pub, err := p.jwks.key(ctx, hdr.Kid)
	if err != nil {
		return idTokenClaims{}, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return idTokenClaims{}, err
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig); err != nil {
		return idTokenClaims{}, fmt.Errorf("id_token signature: %w", err)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return idTokenClaims{}, err
	}
	claims, err := claimsFromPayload(payload)
	if err != nil {
		return idTokenClaims{}, err
	}
	if err := validateIDTokenClaims(claims, p.issuer, p.cfg.OIDCClientID, nonce); err != nil {
		return idTokenClaims{}, err
	}
	return claims, nil
}

func claimsFromPayload(payload []byte) (idTokenClaims, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return idTokenClaims{}, err
	}
	out := idTokenClaims{Raw: raw}
	if v, ok := raw["sub"].(string); ok {
		out.Sub = v
	}
	if v, ok := raw["email"].(string); ok {
		out.Email = v
	}
	if v, ok := raw["preferred_username"].(string); ok {
		out.PreferredUsername = v
	}
	if v, ok := raw["name"].(string); ok {
		out.Name = v
	}
	if v, ok := raw["iss"].(string); ok {
		out.Iss = v
	}
	if v, ok := raw["nonce"].(string); ok {
		out.Nonce = v
	}
	out.Aud = raw["aud"]
	switch v := raw["exp"].(type) {
	case float64:
		out.Exp = v
	case json.Number:
		f, _ := v.Float64()
		out.Exp = f
	}
	return out, nil
}

func validateIDTokenClaims(c idTokenClaims, issuer, clientID, nonce string) error {
	if c.Sub == "" {
		return fmt.Errorf("missing sub claim")
	}
	iss := strings.TrimRight(c.Iss, "/")
	wantIss := strings.TrimRight(issuer, "/")
	if iss == "" || !strings.EqualFold(iss, wantIss) {
		return fmt.Errorf("id_token issuer mismatch")
	}
	if !audienceHas(c.Aud, clientID) {
		return fmt.Errorf("id_token audience mismatch")
	}
	if c.Exp <= 0 || time.Now().Unix() >= int64(c.Exp) {
		return fmt.Errorf("id_token expired")
	}
	if nonce != "" && c.Nonce != nonce {
		return fmt.Errorf("id_token nonce mismatch")
	}
	return nil
}

func audienceHas(aud interface{}, clientID string) bool {
	switch v := aud.(type) {
	case string:
		return v == clientID
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok && s == clientID {
				return true
			}
		}
	case []string:
		for _, s := range v {
			if s == clientID {
				return true
			}
		}
	}
	return false
}
