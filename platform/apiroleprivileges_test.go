// Copyright 2026, Jamf Software LLC

package platform

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdkpro "github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
)

type fakePrivLister struct{ privs []string }

func (f fakePrivLister) ListApiRolePrivilegesV1(context.Context) (*sdkpro.ApiRolePrivileges, error) {
	return &sdkpro.ApiRolePrivileges{Privileges: f.privs}, nil
}

func TestFilterApiRolePrivileges(t *testing.T) {
	dir := t.TempDir()
	content := `resource "jamfplatform_pro_api_role" "role" {
  display_name = "R"
  privileges   = ["Read Buildings", "Read Parent App Profile", "Read Categories"]
}

resource "jamfplatform_pro_api_role" "clean" {
  display_name = "C"
  privileges   = ["Read Buildings"]
}
`
	path := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// "Read Parent App Profile" is absent from the instance catalog → dropped.
	lister := fakePrivLister{privs: []string{"Read Buildings", "Read Categories", "Read Computers"}}
	n, err := FilterApiRolePrivileges(context.Background(), lister, path)
	if err != nil {
		t.Fatalf("FilterApiRolePrivileges: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 dropped, got %d", n)
	}

	out, _ := os.ReadFile(path)
	s := string(out)
	if strings.Contains(s, "Read Parent App Profile") {
		t.Error("invalid privilege should have been dropped")
	}
	if !strings.Contains(s, "Read Buildings") || !strings.Contains(s, "Read Categories") {
		t.Error("valid privileges should be retained")
	}
}

func TestFilterApiRolePrivilegesEmptyCatalog(t *testing.T) {
	dir := t.TempDir()
	content := `resource "jamfplatform_pro_api_role" "role" {
  privileges = ["Read Buildings"]
}
`
	path := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	// An empty catalog means we can't validate — leave the config untouched.
	n, err := FilterApiRolePrivileges(context.Background(), fakePrivLister{}, path)
	if err != nil {
		t.Fatalf("FilterApiRolePrivileges: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 dropped with empty catalog, got %d", n)
	}
	out, _ := os.ReadFile(path)
	if !strings.Contains(string(out), "Read Buildings") {
		t.Error("config must be untouched when catalog is empty")
	}
}
