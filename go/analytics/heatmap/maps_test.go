package heatmap

import (
	"bytes"
	"strings"
	"testing"

	"github.com/romayengineer/fme/go/analytics"
)

func testGrid(t *testing.T) Grid {
	t.Helper()
	src := sliceSource{points: []analytics.GeoPoint{{Lat: -34.6, Lon: -58.4}}}
	return Aggregate(src, Options{CellDeg: 0.01})
}

func TestMapsRegistry(t *testing.T) {
	maps := Maps()
	if len(maps) == 0 {
		t.Fatal("no maps registered")
	}

	ids := MapIDs()
	if _, ok := MapByID(DefaultMapID); !ok {
		t.Fatalf("default map %q not registered", DefaultMapID)
	}
	if _, ok := MapByID("does-not-exist"); ok {
		t.Error("unexpected map lookup success")
	}
	for i, m := range maps {
		if m.ID != ids[i] {
			t.Errorf("Maps() order diverges from MapIDs() at %d", i)
		}
		if m.ID == "" || m.Title == "" {
			t.Errorf("map %d has empty id/title: %+v", i, m)
		}
	}
}

func TestRenderEveryMap(t *testing.T) {
	grid := testGrid(t)

	// Each map must reference its own basemap host.
	wantHost := map[string]string{
		"leaflet-esri": "server.arcgisonline.com",
	}

	for _, m := range Maps() {
		html, err := grid.RenderHTML(m.ID, MapOptions{Title: "test"})
		if err != nil {
			t.Errorf("RenderHTML(%s): %v", m.ID, err)
			continue
		}
		if !bytes.Contains(html, []byte("FeatureCollection")) {
			t.Errorf("%s: missing embedded GeoJSON", m.ID)
		}
		host, ok := wantHost[m.ID]
		if !ok {
			t.Errorf("%s: no expected host in test", m.ID)
			continue
		}
		if !bytes.Contains(html, []byte(host)) {
			t.Errorf("%s: output does not reference %s", m.ID, host)
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
