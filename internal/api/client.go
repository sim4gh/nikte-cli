package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sim4gh/nikte-cli/internal/auth"
	"github.com/sim4gh/nikte-cli/internal/config"
)

// Response represents an API response
type Response struct {
	StatusCode int
	Headers    http.Header
	Body       json.RawMessage
}

// RequestOptions configures an API request
type RequestOptions struct {
	Method      string
	Body        interface{}
	Headers     map[string]string
	RequireAuth bool
}

// DefaultClient is a pre-configured HTTP client
var DefaultClient = &http.Client{
	Timeout: 60 * time.Second,
}

// DefaultBaseURL is the default API base URL
const DefaultBaseURL = "https://auth.nikte.co"

// Request makes an authenticated API request
func Request(path string, opts *RequestOptions) (*Response, error) {
	if opts == nil {
		opts = &RequestOptions{}
	}

	// Default to GET if not specified
	method := opts.Method
	if method == "" {
		method = "GET"
	}

	// Default to requiring auth unless explicitly set to false
	requireAuth := true
	if opts.Method != "" || opts.Body != nil || opts.Headers != nil {
		requireAuth = opts.RequireAuth
	}

	cfg := config.Get()
	baseURL := DefaultBaseURL
	if cfg != nil && cfg.BaseURL != "" {
		baseURL = cfg.BaseURL
	} else if requireAuth {
		return nil, errors.New("not configured. Please run \"nk auth login\" first")
	}

	// Get valid token if auth is required
	var idToken string
	if requireAuth {
		if cfg == nil || cfg.IDToken == "" {
			return nil, errors.New("not authenticated. Please run \"nk auth login\" first")
		}

		// Check if token needs refresh
		if auth.IsTokenExpired(cfg.IDToken) {
			tokens, err := auth.RefreshTokens()
			if err != nil {
				return nil, fmt.Errorf("authentication expired: %w", err)
			}
			idToken = tokens.IDToken
		} else {
			idToken = cfg.IDToken
		}
	}

	// Build URL
	url := baseURL + path

	// Prepare request body
	var bodyReader io.Reader
	if opts.Body != nil {
		bodyBytes, err := json.Marshal(opts.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	// Create request
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set default headers
	req.Header.Set("Content-Type", "application/json")

	// Set custom headers
	for k, v := range opts.Headers {
		req.Header.Set(k, v)
	}

	// Set authorization header
	if idToken != "" {
		req.Header.Set("Authorization", "Bearer "+idToken)
	}

	// Execute request
	resp, err := DefaultClient.Do(req)
	if err != nil {
		if err.Error() == "connection refused" || err.Error() == "dial tcp" {
			return nil, fmt.Errorf("unable to connect to API at %s", baseURL)
		}
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       body,
	}, nil
}

// Get makes a GET request
func Get(path string) (*Response, error) {
	return Request(path, &RequestOptions{Method: "GET", RequireAuth: true})
}

// Post makes a POST request
func Post(path string, body interface{}) (*Response, error) {
	return Request(path, &RequestOptions{Method: "POST", Body: body, RequireAuth: true})
}

// Put makes a PUT request
func Put(path string, body interface{}) (*Response, error) {
	return Request(path, &RequestOptions{Method: "PUT", Body: body, RequireAuth: true})
}

// Patch makes a PATCH request
func Patch(path string, body interface{}) (*Response, error) {
	return Request(path, &RequestOptions{Method: "PATCH", Body: body, RequireAuth: true})
}

// Delete makes a DELETE request
func Delete(path string) (*Response, error) {
	return Request(path, &RequestOptions{Method: "DELETE", RequireAuth: true})
}

// GetNoAuth makes an unauthenticated GET request
func GetNoAuth(path string) (*Response, error) {
	return Request(path, &RequestOptions{Method: "GET", RequireAuth: false})
}

// PostNoAuth makes an unauthenticated POST request
func PostNoAuth(path string, body interface{}) (*Response, error) {
	return Request(path, &RequestOptions{Method: "POST", Body: body, RequireAuth: false})
}

// PutNoAuth makes an unauthenticated PUT request
func PutNoAuth(path string, body interface{}) (*Response, error) {
	return Request(path, &RequestOptions{Method: "PUT", Body: body, RequireAuth: false})
}

// Unmarshal unmarshals the response body into the given interface
func (r *Response) Unmarshal(v interface{}) error {
	return json.Unmarshal(r.Body, v)
}

// GetString returns a string field from the response body
func (r *Response) GetString(key string) string {
	var m map[string]interface{}
	if err := json.Unmarshal(r.Body, &m); err != nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// GetInt returns an int field from the response body
func (r *Response) GetInt(key string) int {
	var m map[string]interface{}
	if err := json.Unmarshal(r.Body, &m); err != nil {
		return 0
	}
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}
