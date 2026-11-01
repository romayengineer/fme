package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/romayengineer/fme/go/analytics/heatmap"
)

// writeDetail writes one detail fixture.
func writeDetail(t *testing.T, dir, name, body string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// mustRunHeatmap runs a heat map or fails the test.
func mustRunHeatmap(t *testing.T, opts Options) Result {
	t.Helper()

	res, err := RunFileHeatmap(opts)
	if err != nil {
		t.Fatalf("RunFileHeatmap: %v", err)
	}
	return res
}

// assertInt compares one integer value.
func assertInt(t *testing.T, name string, got, want int) {
	t.Helper()

	if got != want {
		t.Errorf("%s = %d, want %d", name, got, want)
	}
}

// assertJSONFile checks that a file contains a FeatureCollection.
func assertJSONFile(t *testing.T, path string) {
	t.Helper()

	geo, err := os.ReadFile(path) //nolint:gosec // temp test path.
	if err != nil {
		t.Fatalf("read geojson: %v", err)
	}

	var fc map[string]any
	if err := json.Unmarshal(geo, &fc); err != nil {
		t.Fatalf("geojson is not valid JSON: %v", err)
	}
	if fc["type"] != "FeatureCollection" {
		t.Errorf("geojson type = %v", fc["type"])
	}
}

// assertNonEmptyFile checks that a file exists and has content.
func assertNonEmptyFile(t *testing.T, path string) {
	t.Helper()

	body, err := os.ReadFile(path) //nolint:gosec // temp test path.
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if len(body) == 0 {
		t.Errorf("%s is empty", path)
	}
}

func TestRunFileHeatmapWritesArtifacts(t *testing.T) {
	dir := t.TempDir()
	writeDetail(t, dir, "detail_1.json", `{"LocationLatitud": -34.605, "LocationLongitude": -58.405}`)
	writeDetail(t, dir, "detail_2.json", `{"LocationLatitud": -34.603, "LocationLongitude": -58.403}`)
	writeDetail(t, dir, "detail_3.json", `{"LocationLatitud": null, "LocationLongitude": null}`)

	outBase := filepath.Join(t.TempDir(), "nested", "heatmap")
	res := mustRunHeatmap(t, Options{Dir: dir, CellDeg: 0.01, OutBase: outBase})

	assertInt(t, "Valid", res.Stats.Valid, 2)
	assertInt(t, "Skipped", res.Stats.Skipped, 1)
	if len(res.Grid.Cells) != 1 || res.Grid.Cells[0].Count != 2 {
		t.Errorf("grid cells = %+v, want one cell with 2", res.Grid.Cells)
	}
	if res.MapID != heatmap.DefaultMapID {
		t.Errorf("MapID = %q, want default %q", res.MapID, heatmap.DefaultMapID)
	}

	wantHTML := outBase + "." + heatmap.DefaultMapID + ".html"
	if res.HTMLPath != wantHTML {
		t.Errorf("HTMLPath = %q, want %q", res.HTMLPath, wantHTML)
	}

	assertJSONFile(t, res.GeoJSONPath)
	assertNonEmptyFile(t, res.HTMLPath)
}

func TestRunFileHeatmapAllMaps(t *testing.T) {
	dir := t.TempDir()
	writeDetail(t, dir, "detail_1.json", `{"LocationLatitud": -34.605, "LocationLongitude": -58.405}`)

	outBase := filepath.Join(t.TempDir(), "heatmap")
	res := mustRunHeatmap(t, Options{Dir: dir, OutBase: outBase, Map: MapAll})

	if res.MapID != MapAll {
		t.Errorf("MapID = %q, want %q", res.MapID, MapAll)
	}
	if len(res.HTMLPaths) != len(heatmap.MapIDs()) {
		t.Fatalf("HTMLPaths = %d, want %d", len(res.HTMLPaths), len(heatmap.MapIDs()))
	}
	for _, p := range res.HTMLPaths {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s to exist: %v", p, err)
		}
	}
}

func TestRunFileHeatmapDropsCityRepeats(t *testing.T) {
	dir := t.TempDir()
	for i := 1; i <= 5; i++ {
		writeDetail(t, dir, fmt.Sprintf("detail_%d.json", i),
			`{"LocationLatitud": -34.60, "LocationLongitude": -58.42, "LocationGeocodeCityID": "caba"}`)
	}
	writeDetail(t, dir, "detail_6.json", `{"LocationLatitud": -34.61, "LocationLongitude": -58.43, "LocationGeocodeCityID": "caba"}`)
	writeDetail(t, dir, "detail_7.json", `{"LocationLatitud": -34.70, "LocationLongitude": -58.50, "LocationGeocodeCityID": "caba"}`)

	outBase := filepath.Join(t.TempDir(), "heatmap")
	res := mustRunHeatmap(t, Options{Dir: dir, OutBase: outBase, MinRepeatCount: 5})

	assertInt(t, "Scanned", res.Stats.Scanned, 7)
	assertInt(t, "Dropped", res.Stats.Dropped, 5)
	assertInt(t, "Valid", res.Stats.Valid, 2)
	assertInt(t, "Total", res.Grid.Total, 2)
	assertInt(t, "MaxCount", res.Grid.MaxCount, 1)
}

func TestRunFileHeatmapUnknownMap(t *testing.T) {
	dir := t.TempDir()
	writeDetail(t, dir, "detail_1.json", `{"LocationLatitud": -34.605, "LocationLongitude": -58.405}`)

	_, err := RunFileHeatmap(Options{Dir: dir, OutBase: filepath.Join(t.TempDir(), "heatmap"), Map: "nope"})
	if err == nil {
		t.Fatal("expected error for unknown map, got nil")
	}
}
