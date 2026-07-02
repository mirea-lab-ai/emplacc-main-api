// Package keycloak is the outbound adapter for the Keycloak identity provider.
// It implements ports.IdentityProvider; the gocloak driver and the net/http
// token-exchange call stay confined to this adapter and never reach the services.
package keycloak

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"emplacc-api/internal/ports"

	"github.com/Nerzal/gocloak/v13"
)

type client struct {
	gc           *gocloak.GoCloak
	realm        string
	clientID     string
	clientSecret string
	keycloakURL  string
}

// New builds the Keycloak adapter from KEYCLOAK_* env vars.
func New() ports.IdentityProvider {
	return &client{
		gc:           gocloak.NewClient(os.Getenv("KEYCLOAK_URL")),
		realm:        os.Getenv("KEYCLOAK_REALM"),
		clientID:     os.Getenv("KEYCLOAK_CLIENT_ID"),
		clientSecret: os.Getenv("KEYCLOAK_CLIENT_SECRET"),
		keycloakURL:  os.Getenv("KEYCLOAK_URL"),
	}
}

func (c *client) Login(ctx context.Context, email, password string) (*ports.IdentityToken, error) {
	jwt, err := c.gc.Login(ctx, c.clientID, c.clientSecret, c.realm, email, password)
	if err != nil {
		return nil, err
	}
	return jwtToToken(jwt), nil
}

func (c *client) Logout(ctx context.Context, refreshToken string) error {
	return c.gc.Logout(ctx, c.clientID, c.clientSecret, c.realm, refreshToken)
}

func (c *client) RefreshToken(ctx context.Context, refreshToken string) (*ports.IdentityToken, error) {
	jwt, err := c.gc.RefreshToken(ctx, refreshToken, c.clientID, c.clientSecret, c.realm)
	if err != nil {
		return nil, err
	}
	return jwtToToken(jwt), nil
}

// GetUserInfo fetches the profile, falling back to a token exchange (Flutter tokens).
func (c *client) GetUserInfo(ctx context.Context, accessToken string) (*ports.UserInfo, error) {
	userInfo, err := c.gc.GetUserInfo(ctx, accessToken, c.realm)
	if err != nil {
		exchanged, exErr := c.exchangeToken(ctx, accessToken)
		if exErr != nil {
			log.Printf("Me token exchange failed: %v / original err: %v", exErr, err)
			return nil, exErr
		}
		userInfo, err = c.gc.GetUserInfo(ctx, exchanged.AccessToken, c.realm)
		if err != nil {
			log.Printf("Me failed after exchange: %v", err)
			return nil, err
		}
	}
	return userInfoToDomain(userInfo), nil
}

// VerifyToken проверяет подпись по JWKS realm (gocloak кэширует сертификаты).
func (c *client) VerifyToken(ctx context.Context, accessToken string) error {
	decoded, _, err := c.gc.DecodeAccessToken(ctx, accessToken, c.realm)
	if err != nil {
		return fmt.Errorf("token signature invalid: %w", err)
	}
	if decoded == nil || !decoded.Valid {
		return fmt.Errorf("token invalid")
	}
	return nil
}

// exchangeToken — OAuth token-exchange через прямой HTTP-вызов Keycloak.
func (c *client) exchangeToken(ctx context.Context, subjectToken string) (*ports.IdentityToken, error) {
	endpoint := strings.TrimRight(c.keycloakURL, "/") +
		"/realms/" + c.realm + "/protocol/openid-connect/token"

	data := url.Values{}
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:token-exchange")
	data.Set("subject_token", subjectToken)
	data.Set("subject_token_type", "urn:ietf:params:oauth:token-type:access_token")
	data.Set("client_id", c.clientID)
	data.Set("client_secret", c.clientSecret)
	data.Set("scope", "openid")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed: %s", body)
	}

	var tok struct {
		AccessToken      string `json:"access_token"`
		RefreshToken     string `json:"refresh_token"`
		ExpiresIn        int    `json:"expires_in"`
		RefreshExpiresIn int    `json:"refresh_expires_in"`
		TokenType        string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, fmt.Errorf("error parsing token response: %v", err)
	}
	return &ports.IdentityToken{
		AccessToken:      tok.AccessToken,
		RefreshToken:     tok.RefreshToken,
		ExpiresIn:        tok.ExpiresIn,
		RefreshExpiresIn: tok.RefreshExpiresIn,
		TokenType:        tok.TokenType,
	}, nil
}

func jwtToToken(j *gocloak.JWT) *ports.IdentityToken {
	if j == nil {
		return nil
	}
	return &ports.IdentityToken{
		AccessToken:      j.AccessToken,
		RefreshToken:     j.RefreshToken,
		ExpiresIn:        j.ExpiresIn,
		RefreshExpiresIn: j.RefreshExpiresIn,
		TokenType:        j.TokenType,
	}
}

func userInfoToDomain(u *gocloak.UserInfo) *ports.UserInfo {
	if u == nil {
		return nil
	}
	return &ports.UserInfo{
		Sub:               u.Sub,
		Name:              u.Name,
		PreferredUsername: u.PreferredUsername,
		GivenName:         u.GivenName,
		FamilyName:        u.FamilyName,
		Email:             u.Email,
		EmailVerified:     u.EmailVerified,
	}
}
