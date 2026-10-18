// Copyright 2026, Jamf Software LLC

package client

import (
	"context"
	"fmt"
	"time"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	sdkpro "github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
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

// responseHeaderTimeout bounds how long any one request waits for the gateway to
// start answering. A var so tests can shorten it.
var responseHeaderTimeout = 60 * time.Second

// newClient builds the root SDK client every jamformer call site uses: the
// scope header, a response-header timeout, and no inter-request pacing.
//
// The timeout restores the 60s bound the SDK applied by default until v1.3.0.
// The SDK dropped it because it overrode a caller's context — a Terraform
// `timeouts { update = "10m" }` on a slow write was cut off client-side — and
// left the context as the only deadline. jamformer's contexts carry none:
// VerifyAuth runs on context.Background() and the pipeline steps on the Ctrl+C
// context. Without a bound a stalled gateway hangs the export indefinitely,
// which in CI means until the job is killed. jamformer only reads, so the slow
// write the SDK guards against does not arise, and a per-request bound (rather
// than a per-step context deadline) does not grow with the tenant's size.
//
// The pacing is off for the reason the provider defaults min_request_interval_ms
// to 0: the SDK's 100ms default gates request starts, so it is a throughput
// ceiling (10 req/s) rather than a concurrency limit, and buys nothing serially.
// The SDK's gateway rate-limit pacing (x-ratelimit-limit/-remaining, and holding
// requests after a 429) is always on and independent of this interval, so it is
// what keeps jamformer under the gateway's limit.
func newClient(baseURL, clientID, clientSecret string, scope Scope) *jamfplatform.Client {
	opts := append([]jamfplatform.Option{
		jamfplatform.WithMinRequestInterval(0),
		jamfplatform.WithResponseHeaderTimeout(responseHeaderTimeout),
	}, scope.options()...)
	return jamfplatform.NewClient(baseURL, clientID, clientSecret, opts...)
}

// VerifyAuth creates a Jamf Platform SDK client and validates the credentials
// using the SDK's built-in credential validation.
func VerifyAuth(baseURL, clientID, clientSecret string, scope Scope) error {
	c := newClient(baseURL, clientID, clientSecret, scope)

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
	return sdkpro.New(newClient(baseURL, clientID, clientSecret, scope))
}
