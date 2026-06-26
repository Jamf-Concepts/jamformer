// Copyright 2026, Jamf Software LLC

package client

import (
	"context"
	"fmt"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	sdkpro "github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
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

// NewProClient builds a Jamf Platform "pro" SDK client for the federated Jamf
// Pro surface (e.g. package downloads via the Jamf Cloud Distribution Service).
// A tenant ID is required: pro endpoints are tenant-scoped, so without it the
// SDK builds malformed URLs.
func NewProClient(baseURL, clientID, clientSecret, tenantID string) *sdkpro.Client {
	var opts []jamfplatform.Option
	if tenantID != "" {
		opts = append(opts, jamfplatform.WithTenantID(tenantID))
	}
	root := jamfplatform.NewClient(baseURL, clientID, clientSecret, opts...)
	return sdkpro.New(root)
}
