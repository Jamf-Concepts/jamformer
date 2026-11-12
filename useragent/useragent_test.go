// Copyright 2026, Jamf Software LLC

package useragent

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestString(t *testing.T) {
	SetVersion("1.2.3")
	if got := String(); got != "jamformer/1.2.3" {
		t.Fatalf("got %q", got)
	}
}

func TestTransportOverridesUserAgent(t *testing.T) {
	SetVersion("9.9.9")
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("User-Agent")
	}))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	req.Header.Add("User-Agent", "go-api-http-client-jamfpro-integration")
	resp, err := (&http.Client{Transport: Transport(nil)}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if got != "jamformer/9.9.9" {
		t.Fatalf("server saw %q", got)
	}
	if req.Header.Get("User-Agent") != "go-api-http-client-jamfpro-integration" {
		t.Fatal("caller's request was mutated")
	}
}
