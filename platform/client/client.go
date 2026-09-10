// Copyright 2026, Jamf Software LLC

package client

import (
	"context"
	"fmt"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	sdkpro "github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
)

// Scope carries the API integration's scope to the SDK. It mirrors
// platform.Scope without importing it, so the client package stays a leaf.
// Exactly one of the two IDs is set; both empty means organization scope, where
// the gateway resolves the organization from the access token and no scope
// header is sent at all.
type Scope struct {
	EnvironmentID string
	TenantID      string
}

// options turns a Scope into the SDK options that set the scope header.
// Organization scope contributes none, which is how the SDK selects it.
func (s Scope) options() []jamfplatform.Option {
	var opts []jamfplatform.Option
	switch {
	case s.EnvironmentID != "":
		opts = append(opts, jamfplatform.WithEnvironmentID(s.EnvironmentID))
	case s.TenantID != "":
		opts = append(opts, jamfplatform.WithTenantID(s.TenantID))
	}
	return opts
}

// VerifyAuth creates a Jamf Platform SDK client and validates the credentials
// using the SDK's built-in credential validation.
func VerifyAuth(baseURL, clientID, clientSecret string, scope Scope) error {
	c := jamfplatform.NewClient(baseURL, clientID, clientSecret, scope.options()...)

	if err := c.ValidateCredentials(context.Background()); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	return nil
}

// NewProClient builds a Jamf Platform "pro" SDK client for the federated Jamf
// Pro surface (e.g. package downloads via the Jamf Cloud Distribution Service).
//
// The pro endpoints are published at both environment and tenant scope, so
// either works. Organization scope does not reach them at all — callers gate on
// platform.Scope.ReachesPro before getting here.
func NewProClient(baseURL, clientID, clientSecret string, scope Scope) *sdkpro.Client {
	root := jamfplatform.NewClient(baseURL, clientID, clientSecret, scope.options()...)
	return sdkpro.New(root)
}
