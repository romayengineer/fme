package heatmap

import (
	"bytes"
	"testing"

	"github.com/romayengineer/fme/go/analytics"
)

func TestHTMLUsesKeylessEsriBasemap(t *testing.T) {
	src := sliceSource{points: []analytics.GeoPoint{{Lat: -34.6, Lon: -58.4}}}
	grid := Aggregate(src, Options{CellDeg: 0.01})

	html, err := grid.HTML("test")
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}

	if !bytes.Contains(html, []byte("server.arcgisonline.com")) {
		t.Error("expected Esri basemap tiles")
	}
	if bytes.Contains(html, []byte("basemaps.cartocdn.com")) {
		t.Error("must not use CARTO (requires an API key)")
	}
	if bytes.Contains(html, []byte("tile.openstreetmap.org")) {
		t.Error("must not use OSM (requires an API key)")
	}
	if !bytes.Contains(html, []byte("FeatureCollection")) {
		t.Error("expected embedded GeoJSON")
	}
}
