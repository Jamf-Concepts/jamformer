// Copyright 2026, Jamf Software LLC

package registry

import "testing"

func TestRegisterAndResolve(t *testing.T) {
	reg := New()
	reg.Register("jamfpro_script", "42", "jamfpro_script.disable_bluetooth")

	addr, ok := reg.Resolve("jamfpro_script", "42")
	if !ok {
		t.Fatal("expected Resolve to find registered resource")
	}
	if addr != "jamfpro_script.disable_bluetooth" {
		t.Errorf("got %q, want %q", addr, "jamfpro_script.disable_bluetooth")
	}
}

func TestResolveNotFound(t *testing.T) {
	reg := New()
	reg.Register("jamfpro_script", "42", "jamfpro_script.disable_bluetooth")

	_, ok := reg.Resolve("jamfpro_script", "999")
	if ok {
		t.Error("expected Resolve to return false for unknown ID")
	}

	_, ok = reg.Resolve("jamfpro_category", "42")
	if ok {
		t.Error("expected Resolve to return false for unknown resource type")
	}
}

func TestResolveAny(t *testing.T) {
	reg := New()
	reg.Register("jamfpro_smart_computer_group_v2", "10", "jamfpro_smart_computer_group_v2.staff")
	reg.Register("jamfpro_static_computer_group", "20", "jamfpro_static_computer_group.lab_macs")

	// Should find smart group first
	addr, ok := reg.ResolveAny("10", "jamfpro_smart_computer_group_v2", "jamfpro_static_computer_group")
	if !ok || addr != "jamfpro_smart_computer_group_v2.staff" {
		t.Errorf("got (%q, %v), want (%q, true)", addr, ok, "jamfpro_smart_computer_group_v2.staff")
	}

	// Should find static group
	addr, ok = reg.ResolveAny("20", "jamfpro_smart_computer_group_v2", "jamfpro_static_computer_group")
	if !ok || addr != "jamfpro_static_computer_group.lab_macs" {
		t.Errorf("got (%q, %v), want (%q, true)", addr, ok, "jamfpro_static_computer_group.lab_macs")
	}

	// Should not find unknown ID
	_, ok = reg.ResolveAny("999", "jamfpro_smart_computer_group_v2", "jamfpro_static_computer_group")
	if ok {
		t.Error("expected ResolveAny to return false for unknown ID")
	}
}

func TestAttrReference(t *testing.T) {
	reg := New()
	reg.Register("jamfpro_category", "5", "jamfpro_category.productivity")

	ref, ok := reg.AttrReference("jamfpro_category", "5", "id")
	if !ok {
		t.Fatal("expected AttrReference to find registered resource")
	}
	if ref != "jamfpro_category.productivity.id" {
		t.Errorf("got %q, want %q", ref, "jamfpro_category.productivity.id")
	}

	_, ok = reg.AttrReference("jamfpro_category", "999", "id")
	if ok {
		t.Error("expected AttrReference to return false for unknown ID")
	}
}

func TestRegisterOverwrite(t *testing.T) {
	reg := New()
	reg.Register("jamfpro_script", "42", "jamfpro_script.old_label")
	reg.Register("jamfpro_script", "42", "jamfpro_script.new_label")

	addr, ok := reg.Resolve("jamfpro_script", "42")
	if !ok {
		t.Fatal("expected Resolve to find resource after overwrite")
	}
	if addr != "jamfpro_script.new_label" {
		t.Errorf("got %q, want %q", addr, "jamfpro_script.new_label")
	}
}
