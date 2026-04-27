// Copyright 2026, Jamf Software LLC

package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeTokenLifetime(t *testing.T) {
	tests := []struct {
		name     string
		handler  http.HandlerFunc
		wantLife int
		wantErr  bool
	}{
		{
			name: "success with expires_in",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"expires_in": 3600, "access_token": "tok"})
			},
			wantLife: 3600,
		},
		{
			name: "short-lived token",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"expires_in": 60})
			},
			wantLife: 60,
		},
		{
			name: "HTTP error status",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			wantLife: 0,
			wantErr:  true,
		},
		{
			name: "server error",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantLife: 0,
			wantErr:  true,
		},
		{
			name: "malformed JSON",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{not valid json`))
			},
			wantLife: 0,
			wantErr:  true,
		},
		{
			name: "missing expires_in field",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok"})
			},
			wantLife: 0,
			wantErr:  false, // no error, just zero value
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			got, err := probeTokenLifetime(srv.URL, "test-client", "test-secret")
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if got != tc.wantLife {
				t.Errorf("probeTokenLifetime() = %d, want %d", got, tc.wantLife)
			}
		})
	}
}

func TestProbeTokenLifetimeSendsCorrectRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify it's a POST
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		// Verify path
		if r.URL.Path != "/api/oauth/token" {
			t.Errorf("expected /api/oauth/token, got %s", r.URL.Path)
		}

		// Verify form values
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parsing form: %v", err)
		}
		if got := r.FormValue("client_id"); got != "my-id" {
			t.Errorf("client_id = %q, want %q", got, "my-id")
		}
		if got := r.FormValue("client_secret"); got != "my-secret" {
			t.Errorf("client_secret = %q, want %q", got, "my-secret")
		}
		if got := r.FormValue("grant_type"); got != "client_credentials" {
			t.Errorf("grant_type = %q, want %q", got, "client_credentials")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"expires_in": 1800})
	}))
	defer srv.Close()

	got, err := probeTokenLifetime(srv.URL, "my-id", "my-secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 1800 {
		t.Errorf("expected 1800, got %d", got)
	}
}

func TestProbeTokenLifetimeUnreachableServer(t *testing.T) {
	_, err := probeTokenLifetime("http://127.0.0.1:1", "id", "secret")
	if err == nil {
		t.Error("expected error for unreachable server, got nil")
	}
}
