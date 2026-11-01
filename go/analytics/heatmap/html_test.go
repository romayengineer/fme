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

	markers := []struct {
		needle string
		want   bool
	}{
		{needle: "server.arcgisonline.com", want: true},
		{needle: "basemaps.cartocdn.com", want: false},
		{needle: "tile.openstreetmap.org", want: false},
		{needle: "FeatureCollection", want: true},
	}
	for _, m := range markers {
		if got := bytes.Contains(html, []byte(m.needle)); got != m.want {
			t.Errorf("%s present = %v, want %v", m.needle, got, m.want)
		}
	}
}
