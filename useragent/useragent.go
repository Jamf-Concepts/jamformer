// Copyright 2026, Jamf Software LLC

// Package useragent holds the User-Agent jamformer identifies itself with on
// every HTTP request it makes, so tenant and gateway logs can tell an export
// run from other users of the same SDKs.
package useragent

import (
	"net/http"
	"sync"
	"sync/atomic"
)

var version atomic.Value // string

// SetVersion records the build version. Call it once at startup, before any
// client is built.
func SetVersion(v string) {
	version.Store(v)
}

// String returns the User-Agent value, e.g. "jamformer/1.4.0" or
// "jamformer/dev" for an unversioned local build.
func String() string {
	v, _ := version.Load().(string)
	if v == "" {
		v = "dev"
	}
	return "jamformer/" + v
}

// Transport returns a RoundTripper that overwrites the User-Agent on every
// request with String() before passing it to base (http.DefaultTransport if
// nil). The request is cloned, never mutated.
func Transport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &transport{base: base}
}

type transport struct{ base http.RoundTripper }

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.Header.Set("User-Agent", String())
	return t.base.RoundTrip(r)
}

var installOnce sync.Once

// InstallDefault wraps http.DefaultTransport so that clients which cannot be
// given a User-Agent or transport (the Jamf Pro community SDK builds its own
// http.Client with a hardcoded UA) still send jamformer's. It is idempotent.
func InstallDefault() {
	installOnce.Do(func() {
		http.DefaultTransport = Transport(http.DefaultTransport)
	})
}
