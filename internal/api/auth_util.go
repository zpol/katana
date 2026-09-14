package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func hashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func testIssuerDiscovery(r *http.Request, issuer string) error {
	issuer = strings.TrimRight(strings.TrimSpace(issuer), "/")
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, issuer+"/.well-known/openid-configuration", nil)
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
	var disc map[string]interface{}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&disc); err != nil {
		return err
	}
	return nil
}
