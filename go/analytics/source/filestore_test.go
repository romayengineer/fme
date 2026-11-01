package source

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/romayengineer/fme/go/analytics"
)

// writeDetail writes one detail fixture.
func writeDetail(t *testing.T, dir, name, body string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// assertInt compares one integer value.
func assertInt(t *testing.T, name string, got, want int) {
	t.Helper()

	if got != want {
		t.Errorf("%s = %d, want %d", name, got, want)
	}
}

func TestFileSourceSkipsInvalidCoordinates(t *testing.T) {
	dir := t.TempDir()
	writeDetail(t, dir, "detail_1.json", `{"LocationLatitud": -34.60, "LocationLongitude": -58.40}`)
	writeDetail(t, dir, "detail_2.json", `{"LocationLatitud": null, "LocationLongitude": null}`)
	writeDetail(t, dir, "detail_3.json", `{"LocationLatitud": 0, "LocationLongitude": 0}`)
	writeDetail(t, dir, "detail_4.json", `{"LocationLatitud": 200, "LocationLongitude": 10}`)
	writeDetail(t, dir, "detail_5.json", `not json`)
	writeDetail(t, dir, "ignored.json", `{"LocationLatitud": 1, "LocationLongitude": 1}`)

	src := &FileSource{Dir: dir, Workers: 4}

	var got []analytics.GeoPoint
	n := src.ForEach(func(p analytics.GeoPoint) bool {
		got = append(got, p)
		return true
	})

	assertInt(t, "points", n, 1)
	if len(got) != 1 {
		t.Fatalf("points = %d, want 1", len(got))
	}
	if want := (analytics.GeoPoint{Lat: -34.60, Lon: -58.40}); got[0] != want {
		t.Errorf("point = %+v, want %+v", got[0], want)
	}

	stats := src.Stats()
	assertInt(t, "Scanned", stats.Scanned, 5)
	assertInt(t, "Valid", stats.Valid, 1)
	assertInt(t, "Skipped", stats.Skipped, 4)
}

func TestFileSourceEmptyDir(t *testing.T) {
	src := &FileSource{Dir: t.TempDir()}
	if n := src.ForEach(func(analytics.GeoPoint) bool { return true }); n != 0 {
		t.Errorf("points = %d, want 0", n)
	}
	if stats := src.Stats(); stats.Valid != 0 || stats.Skipped != 0 {
		t.Errorf("stats = %+v, want zero", stats)
	}
}

func TestFileSourceMissingDirReturnsError(t *testing.T) {
	src := &FileSource{Dir: filepath.Join(t.TempDir(), "does-not-exist")}
	if n := src.ForEach(func(analytics.GeoPoint) bool { return true }); n != 0 {
		t.Errorf("points = %d, want 0", n)
	}
	if err := src.Err(); err == nil {
		t.Error("Err returned nil for a missing directory")
	}
}
