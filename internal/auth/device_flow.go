package auth

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// BaseURL is the API base URL
const BaseURL = "https://auth.nikte.co"

// DeviceAuthResponse represents the response from device authorization endpoint
type DeviceAuthResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// DeviceTokenResponse represents the response from token endpoint during device flow
type DeviceTokenResponse struct {
	IDToken      string `json:"id_token,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	Error        string `json:"error,omitempty"`
	ErrorCode    string `json:"error_code,omitempty"`
}

// InitiateDeviceAuth starts the device authorization flow
func InitiateDeviceAuth() (*DeviceAuthResponse, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest("POST", BaseURL+"/device_authorization", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "nikte")

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
		return nil, errors.New("failed to initiate device authorization: " + string(body))
	}

	var authResp DeviceAuthResponse
	if err := json.Unmarshal(body, &authResp); err != nil {
		return nil, err
	}

	return &authResp, nil
}

// PollForToken polls the token endpoint until authentication is complete
func PollForToken(deviceCode string, interval int) (*DeviceTokenResponse, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	data := url.Values{}
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	data.Set("device_code", deviceCode)

	pollInterval := time.Duration(interval) * time.Second
	if pollInterval < time.Second {
		pollInterval = 2 * time.Second
	}

	for {
		time.Sleep(pollInterval)

		req, err := http.NewRequest("POST", BaseURL+"/token", strings.NewReader(data.Encode()))
		if err != nil {
			return nil, err
		}

		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("User-Agent", "nikte")

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		var tokenResp DeviceTokenResponse
		if err := json.Unmarshal(body, &tokenResp); err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusOK {
			return &tokenResp, nil
		}

		// Check for authorization_pending - continue polling
		if tokenResp.Error == "authorization_pending" || tokenResp.ErrorCode == "authorization_pending" {
			continue
		}

		// Check for slow_down - increase interval
		if tokenResp.Error == "slow_down" {
			pollInterval += 5 * time.Second
			continue
		}

		// Other errors should stop the flow
		errMsg := tokenResp.Error
		if errMsg == "" {
			errMsg = tokenResp.ErrorCode
		}
		if errMsg == "" {
			errMsg = "unknown error"
		}
		return nil, errors.New("login failed: " + errMsg)
	}
}
