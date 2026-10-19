package main

import "testing"

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

func TestNewApplicationIsWired(t *testing.T) {
	application := newApplication("search")
	if application == nil {
		t.Fatal("newApplication returned nil")
	}
	if application.Sessions == nil || application.Details == nil || application.PDPState == nil || application.Sink == nil {
		t.Fatalf("application dependencies are incomplete: %+v", application.Deps)
	}
}
