// Copyright 2026, Jamf Software LLC

package protect

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseQueryEvents(t *testing.T) {
	stream := `{"@level":"info","@message":"Starting","type":"version"}
{"@level":"info","@message":"Result","type":"list_resource_found","list_resource_found":{"address":"list.jamfprotect_analytic_managed.example","display_name":"SuspiciousJavaActivity","identity":{"id":"c94af094-5ea1-11ec-be1c-0660d8e6ab1f"},"resource_type":"jamfprotect_analytic_managed"}}
{"@level":"info","@message":"Result","type":"list_resource_found","list_resource_found":{"address":"list.jamfprotect_role.all","display_name":"Full Admin","identity":{"id":"abc"},"resource_type":"jamfprotect_role"}}
{"type":"list_resource_found","list_resource_found":{"address":"list.jamfprotect_analytic_managed.example","display_name":"","identity":{"id":"no-name"},"resource_type":"jamfprotect_analytic_managed"}}
not-json-line-should-be-skipped
`
	dir := t.TempDir()
	path := filepath.Join(dir, "events.json")
	if err := os.WriteFile(path, []byte(stream), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := ParseQueryEvents(path)
	if err != nil {
		t.Fatalf("ParseQueryEvents: %v", err)
	}

	managed, ok := got["jamfprotect_analytic_managed"]
	if !ok {
		t.Fatalf("missing jamfprotect_analytic_managed entries: %#v", got)
	}
	if managed["c94af094-5ea1-11ec-be1c-0660d8e6ab1f"] != "SuspiciousJavaActivity" {
		t.Errorf("unexpected mapping for managed analytic: %#v", managed)
	}
	if _, present := managed["no-name"]; present {
		t.Error("entries with empty display_name should be skipped")
	}

	roles, ok := got["jamfprotect_role"]
	if !ok || roles["abc"] != "Full Admin" {
		t.Errorf("unexpected mapping for roles: %#v", roles)
	}
}
