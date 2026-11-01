package heatmap

import (
	"encoding/json"
	"testing"

	"github.com/romayengineer/fme/go/analytics"
)

type decodedFeatureCollection struct {
	Type     string           `json:"type"`
	BBox     []float64        `json:"bbox"`
	Features []decodedFeature `json:"features"`
}

type decodedFeature struct {
	Type       string          `json:"type"`
	Geometry   decodedGeometry `json:"geometry"`
	Properties map[string]any  `json:"properties"`
}

type decodedGeometry struct {
	Type        string         `json:"type"`
	Coordinates [][][2]float64 `json:"coordinates"`
}

// decodeGeoJSON renders and decodes a grid's GeoJSON document.
func decodeGeoJSON(t *testing.T, grid Grid) decodedFeatureCollection {
	t.Helper()

	raw, err := grid.GeoJSON()
	if err != nil {
		t.Fatalf("GeoJSON: %v", err)
	}

	var fc decodedFeatureCollection
	if err := json.Unmarshal(raw, &fc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return fc
}

// assertPolygon checks the feature's polygon ring.
func assertPolygon(t *testing.T, geometry decodedGeometry) {
	t.Helper()

	if geometry.Type != "Polygon" {
		t.Errorf("geometry type = %q, want Polygon", geometry.Type)
	}
	ring := geometry.Coordinates[0]
	if len(ring) != 5 {
		t.Fatalf("ring points = %d, want 5 (closed)", len(ring))
	}
	if ring[0] != ring[4] {
		t.Errorf("ring is not closed: %v vs %v", ring[0], ring[4])
	}
}

// assertProperties checks the feature's count and intensity.
func assertProperties(t *testing.T, properties map[string]any) {
	t.Helper()

	if count := properties["count"].(float64); count != 2 {
		t.Errorf("count = %v, want 2", count)
	}
	if intensity := properties["intensity"].(float64); intensity != 1 {
		t.Errorf("intensity = %v, want 1", intensity)
	}
}

func TestGeoJSONShape(t *testing.T) {
	src := sliceSource{points: []analytics.GeoPoint{
		{Lat: -34.605, Lon: -58.405},
		{Lat: -34.603, Lon: -58.403},
	}}
	fc := decodeGeoJSON(t, Aggregate(src, Options{CellDeg: 0.01}))

	if fc.Type != "FeatureCollection" {
		t.Errorf("type = %q, want FeatureCollection", fc.Type)
	}

	if len(fc.Features) != 1 {
		t.Fatalf("features = %d, want 1", len(fc.Features))
	}
	assertPolygon(t, fc.Features[0].Geometry)
	assertProperties(t, fc.Features[0].Properties)
}
