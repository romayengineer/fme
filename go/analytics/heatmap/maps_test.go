package heatmap

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/romayengineer/fme/go/analytics"
)

// wantHost lists the basemap host each built-in map must reference.
var wantHost = map[string]string{
	"leaflet-esri": "server.arcgisonline.com",
}

func testGrid(t *testing.T) Grid {
	t.Helper()

	src := sliceSource{points: []analytics.GeoPoint{{Lat: -34.6, Lon: -58.4}}}
	return Aggregate(src, Options{CellDeg: 0.01})
}

// assertMapOrder checks that Maps() and MapIDs() agree and are non-empty.
func assertMapOrder(t *testing.T, maps []MapInfo, ids []string) {
	t.Helper()

	if len(maps) == 0 {
		t.Fatal("no maps registered")
	}
	for i, m := range maps {
		if m.ID != ids[i] {
			t.Errorf("Maps() order diverges from MapIDs() at %d", i)
		}
		assertMapFields(t, i, m)
	}
}

// assertMapFields checks that a map entry is fully described.
func assertMapFields(t *testing.T, index int, m MapInfo) {
	t.Helper()

	if m.ID == "" || m.Title == "" {
		t.Errorf("map %d has empty id/title: %+v", index, m)
	}
}

func TestMapsRegistry(t *testing.T) {
	if _, ok := MapByID(DefaultMapID); !ok {
		t.Fatalf("default map %q not registered", DefaultMapID)
	}
	if _, ok := MapByID("does-not-exist"); ok {
		t.Error("unexpected map lookup success")
	}
	assertMapOrder(t, Maps(), MapIDs())
}

// renderMap renders one map and checks its basemap host and payload.
func renderMap(t *testing.T, grid Grid, info MapInfo) error {
	t.Helper()

	html, err := grid.RenderHTML(info.ID, MapOptions{Title: "test"})
	if err != nil {
		return fmt.Errorf("RenderHTML(%s): %w", info.ID, err)
	}
	if !bytes.Contains(html, []byte("FeatureCollection")) {
		return fmt.Errorf("%s: missing embedded GeoJSON", info.ID)
	}

	host, ok := wantHost[info.ID]
	if !ok {
		return fmt.Errorf("%s: no expected host in test", info.ID)
	}
	if !bytes.Contains(html, []byte(host)) {
		return fmt.Errorf("%s: output does not reference %s", info.ID, host)
	}
	return nil
}

func TestRenderEveryMap(t *testing.T) {
	grid := testGrid(t)

	for _, m := range Maps() {
		if err := renderMap(t, grid, m); err != nil {
			t.Error(err)
		}
	}
}

func TestRenderUnknownMapErrors(t *testing.T) {
	grid := testGrid(t)

	_, err := grid.RenderHTML("nope", MapOptions{})
	if err == nil {
		t.Fatal("expected error for unknown map")
	}
	if !strings.Contains(err.Error(), DefaultMapID) {
		t.Errorf("error should list valid ids, got: %v", err)
	}
}

func TestDefaultWrapperUsesDefaultMap(t *testing.T) {
	grid := testGrid(t)

	wrapper, err := grid.HTML("x")
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	direct, err := grid.RenderHTML(DefaultMapID, MapOptions{Title: "x"})
	if err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	if !bytes.Equal(wrapper, direct) {
		t.Error("HTML wrapper output differs from RenderHTML(default)")
	}
}
