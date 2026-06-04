package auth

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sim4gh/nikte-cli/internal/config"
)

// Cognito configuration - hardcoded values from Node.js CLI
const (
	CognitoDomain = "nikte-fcf57b8c.auth.us-west-2.amazoncognito.com"
	ClientID      = "2385ict6amoluilmqns4jf0n73"
	TokenEndpoint = "https://" + CognitoDomain + "/oauth2/token"
)

// TokenResponse represents the response from Cognito token endpoint
type TokenResponse struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// TokenErrorResponse represents an error response from Cognito
type TokenErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// RefreshTokens exchanges the refresh token for new access/id tokens
func RefreshTokens() (*TokenResponse, error) {
	cfg := config.Get()
	if cfg == nil || cfg.RefreshToken == "" {
		return nil, errors.New("no refresh token available. Please run \"nk auth login\" again")
	}

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("client_id", ClientID)
	data.Set("refresh_token", cfg.RefreshToken)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("POST", TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		var errResp TokenErrorResponse
		if err := json.Unmarshal(body, &errResp); err != nil {
			return nil, errors.New("failed to refresh tokens: " + string(body))
		}
		if errResp.ErrorDescription != "" {
			return nil, errors.New(errResp.ErrorDescription)
		}
		if errResp.Error != "" {
			return nil, errors.New(errResp.Error)
		}
		return nil, errors.New("failed to refresh tokens")
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, err
	}

	// Update stored tokens
	cfg.IDToken = tokenResp.IDToken
	cfg.AccessToken = tokenResp.AccessToken
	if tokenResp.RefreshToken != "" {
		cfg.RefreshToken = tokenResp.RefreshToken
	}

	if err := config.SetConfig(cfg); err != nil {
		return nil, err
	}

	return &tokenResp, nil
}

// EnsureValidToken checks if the token is valid and refreshes if needed
func EnsureValidToken() (string, error) {
	cfg := config.Get()
	if cfg == nil || cfg.IDToken == "" {
		return "", errors.New("not authenticated. Please run \"nk auth login\" first")
	}

	if !IsTokenExpired(cfg.IDToken) {
		return cfg.IDToken, nil
	}

	// Token is expired, try to refresh
	tokens, err := RefreshTokens()
	if err != nil {
		return "", errors.New("authentication expired: " + err.Error())
	}

	return tokens.IDToken, nil
}
