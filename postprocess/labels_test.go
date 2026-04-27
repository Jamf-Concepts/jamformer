// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func defaultNameAttr(_ string) string { return "name" }

func TestRenameLabels_BasicRename(t *testing.T) {
	src := `resource "example_thing" "all_0" {
  name = "My Widget"
}

resource "example_thing" "all_1" {
  name = "Other Widget"
}

import {
  identity = {
    id = "1"
  }
  to = example_thing.all_0
}

import {
  identity = {
    id = "2"
  }
  to = example_thing.all_1
}
`
	dir := t.TempDir()
	genFile := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	if err := RenameLabels(genFile, defaultNameAttr); err != nil {
		t.Fatalf("RenameLabels: %v", err)
	}

	result, _ := os.ReadFile(genFile)
	body := string(result)

	if !strings.Contains(body, `"example_thing" "my_widget"`) {
		t.Errorf("expected resource renamed to my_widget, got:\n%s", body)
	}
	if !strings.Contains(body, `"example_thing" "other_widget"`) {
		t.Errorf("expected resource renamed to other_widget, got:\n%s", body)
	}
	if !strings.Contains(body, "example_thing.my_widget") {
		t.Error("expected import block updated to my_widget")
	}
	if !strings.Contains(body, "example_thing.other_widget") {
		t.Error("expected import block updated to other_widget")
	}
	if strings.Contains(body, "all_0") || strings.Contains(body, "all_1") {
		t.Error("old labels should not appear in output")
	}
}

func TestRenameLabels_CustomNameAttr(t *testing.T) {
	src := `resource "example_user" "all_0" {
  email = "alice@example.com"
}
`
	dir := t.TempDir()
	genFile := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	nameAttrFn := func(resourceType string) string {
		if resourceType == "example_user" {
			return "email"
		}
		return "name"
	}

	if err := RenameLabels(genFile, nameAttrFn); err != nil {
		t.Fatalf("RenameLabels: %v", err)
	}

	result, _ := os.ReadFile(genFile)
	if !strings.Contains(string(result), `"example_user" "alice_example_com"`) {
		t.Errorf("expected label derived from email, got:\n%s", string(result))
	}
}

func TestRenameLabels_DuplicateNames(t *testing.T) {
	src := `resource "example_thing" "all_0" {
  name = "Widget"
}

resource "example_thing" "all_1" {
  name = "Widget"
}
`
	dir := t.TempDir()
	genFile := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	if err := RenameLabels(genFile, defaultNameAttr); err != nil {
		t.Fatalf("RenameLabels: %v", err)
	}

	result, _ := os.ReadFile(genFile)
	body := string(result)

	if !strings.Contains(body, `"example_thing" "widget"`) {
		t.Errorf("expected first resource renamed to widget, got:\n%s", body)
	}
	if !strings.Contains(body, `"example_thing" "widget_2"`) {
		t.Errorf("expected second resource renamed to widget_2, got:\n%s", body)
	}
}

func TestRenameLabels_NoNameAttribute(t *testing.T) {
	src := `resource "example_thing" "all_0" {
  description = "no name here"
}
`
	dir := t.TempDir()
	genFile := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	if err := RenameLabels(genFile, defaultNameAttr); err != nil {
		t.Fatalf("RenameLabels: %v", err)
	}

	result, _ := os.ReadFile(genFile)
	if !strings.Contains(string(result), `"example_thing" "all_0"`) {
		t.Error("expected original label preserved when no name attribute")
	}
}

func TestRenameLabels_NoRenamesNeeded(t *testing.T) {
	src := `resource "example_thing" "widget" {
  name = "widget"
}
`
	dir := t.TempDir()
	genFile := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	if err := RenameLabels(genFile, defaultNameAttr); err != nil {
		t.Fatalf("RenameLabels: %v", err)
	}

	result, _ := os.ReadFile(genFile)
	if !strings.Contains(string(result), `"example_thing" "widget"`) {
		t.Error("expected label unchanged")
	}
}
