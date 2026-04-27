// Copyright 2026, Jamf Software LLC

package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

// lastBufferPeriod stores the dynamically-determined token refresh buffer period
// (in seconds) from the most recent OAuth2 client initialization. Zero means
// no probing was performed (basic auth or probe failed).
var lastBufferPeriod int

// TokenRefreshBufferPeriod returns the token refresh buffer period (in seconds)
// determined during the most recent OAuth2 client initialization. Returns 0
// if no probing was performed.
func TokenRefreshBufferPeriod() int {
	return lastBufferPeriod
}

// AuthConfig holds the authentication configuration for the Jamf Pro client.
type AuthConfig struct {
	URL          string
	AuthMethod   string // "basic" or "oauth2"
	Username     string // basic auth
	Password     string // basic auth
	ClientID     string // oauth2
	ClientSecret string // oauth2
}

// VerifyAuth creates a client and makes a lightweight API call to verify
// that the credentials are valid. Returns the client on success.
func VerifyAuth(auth *AuthConfig) (*jamfpro.Client, error) {
	client, err := New(auth)
	if err != nil {
		return nil, err
	}

	if _, err := client.GetJamfProInformation(); err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	return client, nil
}

// New creates a new Jamf Pro API client using the provided auth config.
func New(auth *AuthConfig) (*jamfpro.Client, error) {
	// Ensure URL has scheme and no trailing slash
	instanceURL := strings.TrimRight(auth.URL, "/")
	if !strings.HasPrefix(instanceURL, "https://") && !strings.HasPrefix(instanceURL, "http://") {
		instanceURL = "https://" + instanceURL
	}

	// For OAuth2, probe the token endpoint to determine the token lifetime
	// and set the buffer period accordingly. Some API integrations have very
	// short token lifetimes (e.g. 60s) that are shorter than the SDK default
	// buffer period (300s), which causes the SDK to reject the token.
	bufferPeriod := 300
	if auth.AuthMethod == "oauth2" {
		if lifetime, err := probeTokenLifetime(instanceURL, auth.ClientID, auth.ClientSecret); err == nil && lifetime > 0 {
			bufferPeriod = max(lifetime/2, 5)
		}
		lastBufferPeriod = bufferPeriod
	}

	config := &jamfpro.ConfigContainer{
		InstanceDomain:              instanceURL,
		AuthMethod:                  auth.AuthMethod,
		Username:                    auth.Username,
		Password:                    auth.Password,
		ClientID:                    auth.ClientID,
		ClientSecret:                auth.ClientSecret,
		LogLevel:                    "error",
		HideSensitiveData:           true,
		MaxRetryAttempts:            3,
		MaxConcurrentRequests:       1,
		EnableDynamicRateLimiting:   false,
		CustomTimeout:               120,
		TokenRefreshBufferPeriod:    bufferPeriod,
		TotalRetryDuration:          120,
		FollowRedirects:             true,
		MaxRedirects:                5,
		EnableConcurrencyManagement: true,
		RetryEligiableRequests:      true,
	}

	client, err := jamfpro.BuildClient(config)
	if err != nil {
		return nil, fmt.Errorf("building jamf pro client: %w", err)
	}

	return client, nil
}

// probeTokenLifetime makes a lightweight OAuth2 token request to determine
// the token's expires_in value (in seconds). Returns 0 if it can't be determined.
func probeTokenLifetime(instanceURL, clientID, clientSecret string) (int, error) {
	data := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"grant_type":    {"client_credentials"},
	}

	resp, err := http.PostForm(instanceURL+"/api/oauth/token", data)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return 0, fmt.Errorf("token probe returned status %d", resp.StatusCode)
	}

	var tokenResp struct {
		ExpiresIn int `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return 0, err
	}

	return tokenResp.ExpiresIn, nil
}
