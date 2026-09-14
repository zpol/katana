package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestValidateIDTokenClaims(t *testing.T) {
	exp := float64(time.Now().Add(5 * time.Minute).Unix())
	c := idTokenClaims{
		Sub:   "user-1",
		Iss:   "https://issuer.example/as",
		Aud:   "katana",
		Exp:   exp,
		Nonce: "n1",
	}
	if err := validateIDTokenClaims(c, "https://issuer.example/as", "katana", "n1"); err != nil {
		t.Fatal(err)
	}
	if err := validateIDTokenClaims(c, "https://other.example", "katana", "n1"); err == nil {
		t.Fatal("expected issuer mismatch")
	}
	if err := validateIDTokenClaims(c, "https://issuer.example/as", "other", "n1"); err == nil {
		t.Fatal("expected audience mismatch")
	}
	if err := validateIDTokenClaims(c, "https://issuer.example/as", "katana", "other"); err == nil {
		t.Fatal("expected nonce mismatch")
	}
	c.Exp = float64(time.Now().Add(-time.Minute).Unix())
	if err := validateIDTokenClaims(c, "https://issuer.example/as", "katana", "n1"); err == nil {
		t.Fatal("expected expired")
	}
}

func TestVerifyRS256RoundTrip(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	header, _ := json.Marshal(jwtHeader{Alg: "RS256", Kid: "k1"})
	payload, _ := json.Marshal(map[string]interface{}{
		"sub":   "u",
		"iss":   "https://issuer.example",
		"aud":   "katana",
		"exp":   time.Now().Add(time.Minute).Unix(),
		"nonce": "abc",
	})
	h := base64.RawURLEncoding.EncodeToString(header)
	p := base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(h + "." + p))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	token := h + "." + p + "." + base64.RawURLEncoding.EncodeToString(sig)

	prov := &OIDCProvider{
		cfg:    Config{OIDCClientID: "katana"},
		issuer: "https://issuer.example",
		jwks:   jwksCache{keys: map[string]*rsa.PublicKey{"k1": &key.PublicKey}, fetched: time.Now()},
	}
	ctx := context.Background()
	claims, err := prov.verifyIDToken(ctx, token, "abc")
	if err != nil {
		t.Fatal(err)
	}
	if claims.Sub != "u" {
		t.Fatalf("sub %q", claims.Sub)
	}

	tampered := token[:len(token)-2] + "AA"
	if _, err := prov.verifyIDToken(ctx, tampered, "abc"); err == nil {
		t.Fatal("expected signature failure")
	}

	noneHdr, _ := json.Marshal(jwtHeader{Alg: "none", Kid: "k1"})
	noneTok := base64.RawURLEncoding.EncodeToString(noneHdr) + "." + p + "."
	if _, err := prov.verifyIDToken(ctx, noneTok, "abc"); err == nil {
		t.Fatal("expected alg none rejected")
	}
}
