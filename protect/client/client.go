// Copyright 2026, Jamf Software LLC

package client

import (
	"context"
	"fmt"

	"github.com/Jamf-Concepts/jamfprotect-go-sdk/jamfprotect"
)

// VerifyAuth creates a Jamf Protect SDK client and requests an access token
// to verify that the credentials are valid. Returns the authenticated client
// for reuse in pre-discovery checks.
func VerifyAuth(url, clientID, clientSecret string) (*jamfprotect.Client, error) {
	c := jamfprotect.NewClient(url, clientID, clientSecret)

	if _, err := c.AccessToken(context.Background()); err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	return c, nil
}

// IsDataForwardingConfigured checks whether any data forwarding service
// (S3, Sentinel, or Sentinel V2) is enabled on the Jamf Protect tenant.
func IsDataForwardingConfigured(ctx context.Context, c *jamfprotect.Client) (bool, error) {
	fwd, err := c.GetDataForwarding(ctx)
	if err != nil {
		return false, fmt.Errorf("checking data forwarding: %w", err)
	}
	return fwd.Forward.S3.Enabled || fwd.Forward.Sentinel.Enabled || fwd.Forward.SentinelV2.Enabled, nil
}
