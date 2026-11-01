package filestore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/romayengineer/fme/go/marketplace/model"
)

func TestSaveProductsFromMergesDescription(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	detailDir := filepath.Join(DataDir, "__Details")
	if err := os.MkdirAll(detailDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	existing := map[string]any{"ID": "123", "Title": "a listing", "Description": nil}
	body, err := json.Marshal(existing)
	if err != nil {
		t.Fatalf("marshal existing: %v", err)
	}
	if err := os.WriteFile(filepath.Join(detailDir, "detail_123.json"), body, 0o644); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	pdp := map[string]any{
		"data": map[string]any{
			"viewer": map[string]any{
				"marketplace_product_details_page": map[string]any{
					"target": map[string]any{
						"id":                   "123",
						"redacted_description": map[string]any{"text": "hello"},
					},
				},
			},
		},
	}

	if saved := SaveProductsFrom([]any{pdp}); saved != 1 {
		t.Fatalf("SaveProductsFrom = %d, want 1", saved)
	}

	got, err := os.ReadFile(filepath.Join(detailDir, "detail_123.json"))
	if err != nil {
		t.Fatalf("read detail: %v", err)
	}

	var record map[string]any
	if err := json.Unmarshal(got, &record); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}
	if record["Description"] != "hello" {
		t.Errorf("Description = %v, want hello", record["Description"])
	}
	if record["Title"] != "a listing" {
		t.Errorf("Title = %v, want a listing (existing field preserved)", record["Title"])
	}
}

func TestSaveProductsFromNoProduct(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	if saved := SaveProductsFrom([]any{map[string]any{"unrelated": true}}); saved != 0 {
		t.Errorf("SaveProductsFrom = %d, want 0", saved)
	}
}

func TestDetailMergePreservesPersistedKeysAndFilename(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	detailDir := filepath.Join(DataDir, "__Details")
	if err := os.MkdirAll(detailDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	fixture := map[string]any{
		"ID":                       "123",
		"TaxonomiPathJoined":       "vehicles,cars",
		"Title":                    "a listing",
		"Description":              nil,
		"PriceAmount":              nil,
		"LocationLatitud":          -34.6,
		"LocationGeocodeCityID":    "city-1",
		"LocationGeocodeCityName1": "Buenos Aires",
		"IsLive":                   true,
	}
	body, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(detailDir, "detail_123.json"), body, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	pdp := map[string]any{
		"data": map[string]any{
			"viewer": map[string]any{
				"marketplace_product_details_page": map[string]any{
					"target": map[string]any{
						"id":                   "123",
						"redacted_description": map[string]any{"text": "hello"},
					},
				},
			},
		},
	}

	if saved := SaveProductsFrom([]any{pdp}); saved != 1 {
		t.Fatalf("SaveProductsFrom = %d, want 1", saved)
	}

	gotBody, err := os.ReadFile(filepath.Join(detailDir, "detail_123.json"))
	if err != nil {
		t.Fatalf("read detail: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(gotBody, &got); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}

	for key, want := range fixture {
		if key == "Description" {
			if got[key] != "hello" {
				t.Errorf("Description = %v, want hello", got[key])
			}
			continue
		}
		if !reflect.DeepEqual(got[key], want) {
			t.Errorf("%s = %#v, want %#v", key, got[key], want)
		}
	}
}

func TestWriteFileAndDirsWritesAtomically(t *testing.T) {
	tmpDir := t.TempDir()
	name := filepath.Join(tmpDir, "nested", "detail_1.json")

	if err := WriteFileAndDirs(name, []byte(`{"first":true}`), 0o600); err != nil {
		t.Fatalf("first write: %v", err)
	}

	info, err := os.Stat(name)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %o, want 600", got)
	}

	if err := WriteFileAndDirs(name, []byte(`{"second":true}`), 0o644); err != nil {
		t.Fatalf("second write: %v", err)
	}

	body, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(body) != `{"second":true}` {
		t.Errorf("content = %s, want the replacement payload", body)
	}

	entries, err := os.ReadDir(filepath.Dir(name))
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("got %d entries, want 1 (no temp files left behind)", len(entries))
	}
}

func TestForEachDetailReturnsDataDirectoryErrors(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	_, err := ForEachDetail(func(string, map[string]any) bool { return true }, false)
	if err == nil {
		t.Fatal("ForEachDetail returned nil error for a missing data directory")
	}
}

func TestSaveProductsIfAnySkipsInvalidIDs(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	products := []model.MarketplaceItemDetails{
		{ID: nil, Title: "missing id"},
		{ID: 123, Title: "numeric id"},
		{ID: "valid-id", Title: "valid"},
	}

	if saved := SaveProductsIfAny(products); !saved {
		t.Fatal("SaveProductsIfAny = false, want true")
	}

	entries, err := os.ReadDir(filepath.Join(DataDir, "__Details"))
	if err != nil {
		t.Fatalf("read details dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d detail files, want 1", len(entries))
	}
	if got := entries[0].Name(); got != "detail_valid-id.json" {
		t.Errorf("detail file = %q, want detail_valid-id.json", got)
	}
}
