package filestore

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/romayengineer/fme/go/marketplace/model"
)

// pdpPayload builds a Marketplace PDP response for one listing.
func pdpPayload(id, description string) map[string]any {
	return map[string]any{
		"data": map[string]any{
			"viewer": map[string]any{
				"marketplace_product_details_page": map[string]any{
					"target": map[string]any{
						"id":                   id,
						"redacted_description": map[string]any{"text": description},
					},
				},
			},
		},
	}
}

func TestSaveProductsFromMergesDescription(t *testing.T) {
	detailDir := newDetailDir(t)
	writeDetailRecord(t, detailDir, "123", map[string]any{"ID": "123", "Title": "a listing", "Description": nil})

	if saved := SaveProductsFrom([]any{pdpPayload("123", "hello")}); saved != 1 {
		t.Fatalf("SaveProductsFrom = %d, want 1", saved)
	}

	record := readDetailRecord(t, detailDir, "123")
	if record["Description"] != "hello" {
		t.Errorf("Description = %v, want hello", record["Description"])
	}
	if record["Title"] != "a listing" {
		t.Errorf("Title = %v, want a listing (existing field preserved)", record["Title"])
	}
}

func TestSaveProductsFromNoProduct(t *testing.T) {
	t.Chdir(t.TempDir())

	if saved := SaveProductsFrom([]any{map[string]any{"unrelated": true}}); saved != 0 {
		t.Errorf("SaveProductsFrom = %d, want 0", saved)
	}
}

func TestDetailMergePreservesPersistedKeysAndFilename(t *testing.T) {
	detailDir := newDetailDir(t)

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
	writeDetailRecord(t, detailDir, "123", fixture)

	if saved := SaveProductsFrom([]any{pdpPayload("123", "hello")}); saved != 1 {
		t.Fatalf("SaveProductsFrom = %d, want 1", saved)
	}

	got := readDetailRecord(t, detailDir, "123")
	for key, want := range fixture {
		assertDetailField(t, got, key, want)
	}
}

// assertDetailField checks one persisted field, treating Description as the
// field the PDP merge is expected to replace.
func assertDetailField(t *testing.T, got map[string]any, key string, want any) {
	t.Helper()

	if key == "Description" {
		if got[key] != "hello" {
			t.Errorf("Description = %v, want hello", got[key])
		}
		return
	}
	if !reflect.DeepEqual(got[key], want) {
		t.Errorf("%s = %#v, want %#v", key, got[key], want)
	}
}

func TestWriteFileAndDirsWritesAtomically(t *testing.T) {
	name := filepath.Join(t.TempDir(), "nested", "detail_1.json")

	t.Run("creates the file with the requested mode", func(t *testing.T) {
		writeWithMode(t, name, `{"first":true}`, 0o600)
	})

	t.Run("replaces the previous content", func(t *testing.T) {
		assertReplacedContent(t, name)
	})

	t.Run("leaves no temp files behind", func(t *testing.T) {
		entries, err := os.ReadDir(filepath.Dir(name))
		if err != nil {
			t.Fatalf("read dir: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("got %d entries, want 1", len(entries))
		}
	})
}

// assertReplacedContent overwrites the file and checks the new payload.
func assertReplacedContent(t *testing.T, name string) {
	t.Helper()

	if err := WriteFileAndDirs(name, []byte(`{"second":true}`), 0o600); err != nil {
		t.Fatalf("second write: %v", err)
	}
	body, err := os.ReadFile(name) //nolint:gosec // temp test path.
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(body) != `{"second":true}` {
		t.Errorf("content = %s, want the replacement payload", body)
	}
}

// writeWithMode writes content and checks the resulting file mode.
func writeWithMode(t *testing.T, name, content string, perm os.FileMode) {
	t.Helper()

	if err := WriteFileAndDirs(name, []byte(content), perm); err != nil {
		t.Fatalf("write: %v", err)
	}
	info, err := os.Stat(name)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != perm {
		t.Errorf("mode = %o, want %o", got, perm)
	}
}

func TestForEachDetailReturnsDataDirectoryErrors(t *testing.T) {
	t.Chdir(t.TempDir())

	_, err := ForEachDetail(func(string, map[string]any) bool { return true }, false)
	if err == nil {
		t.Fatal("ForEachDetail returned nil error for a missing data directory")
	}
}

func TestSaveProductsIfAnySkipsInvalidIDs(t *testing.T) {
	t.Chdir(t.TempDir())

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
