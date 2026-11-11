// Copyright 2026, Jamf Software LLC

package client

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Jamf-Concepts/jamformer/useragent"
)

func TestVerifyAuthSendsJamformerUserAgent(t *testing.T) {
	useragent.SetVersion("test")
	var mu sync.Mutex
	var agents []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		agents = append(agents, r.Header.Get("User-Agent"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"x","token_type":"Bearer","expires_in":3600}`))
	}))
	defer srv.Close()

	_ = VerifyAuth(srv.URL, "id", "secret", Scope{})

	mu.Lock()
	defer mu.Unlock()
	if len(agents) == 0 {
		t.Fatal("no requests reached the server")
	}
	for _, a := range agents {
		if a != "jamformer/test" {
			t.Errorf("User-Agent = %q, want jamformer/test", a)
		}
	}
}
