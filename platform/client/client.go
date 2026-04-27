// Copyright 2026, Jamf Software LLC

package client

import (
	"context"
	"fmt"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
)

// VerifyAuth creates a Jamf Platform SDK client and validates the credentials
// using the SDK's built-in credential validation.
func VerifyAuth(baseURL, clientID, clientSecret, tenantID string) error {
	var opts []jamfplatform.Option
	if tenantID != "" {
		opts = append(opts, jamfplatform.WithTenantID(tenantID))
	}
	c := jamfplatform.NewClient(baseURL, clientID, clientSecret, opts...)

	if err := c.ValidateCredentials(context.Background()); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	return nil
}
