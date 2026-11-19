package main

import (
	"testing"

	"github.com/romayengineer/fme/go/app"
)

func TestActionRegistryCoversCLIActions(t *testing.T) {
	actions := []string{
		"search",
		"pull_description",
		"get_details",
		"install_drivers",
		"process_data",
		"dedupe_pdp",
		"fill_empty",
		"description_stats",
		"serve",
		"save",
		"heatmap",
		"heatmap_serve",
	}

	for _, action := range actions {
		if _, ok := actionRegistry[action]; !ok {
			t.Errorf("action %q is missing from actionRegistry", action)
		}
	}
}

func TestBlockImagesFor(t *testing.T) {
	tests := map[string]bool{
		"search":           true,
		"SEARCH":           true,
		"pull_description": true,
		"get_details":      false,
		"save":             false,
		"":                 false,
	}

	for action, want := range tests {
		if got := blockImagesFor(action); got != want {
			t.Errorf("blockImagesFor(%q) = %v, want %v", action, got, want)
		}
	}
}

// assertDepsWired checks that every use-case dependency was constructed.
func assertDepsWired(t *testing.T, application *app.App) {
	t.Helper()

	checks := map[string]bool{
		"Sessions": application.Sessions == nil,
		"Details":  application.Details == nil,
		"PDPState": application.PDPState == nil,
		"Sink":     application.Sink == nil,
		"PDP":      application.PDP == nil,
		"Products": application.Products == nil,
	}
	for name, missing := range checks {
		if missing {
			t.Errorf("%s is nil", name)
		}
	}
}

func TestNewApplicationIsWired(t *testing.T) {
	application := newApplication("search", Flags{})
	if application == nil {
		t.Fatal("newApplication returned nil")
	}
	assertDepsWired(t, application)
}
