package source

import (
	"testing"

	"github.com/romayengineer/fme/go/analytics"
)

type sliceLocatedSource struct{ points []analytics.LocatedPoint }

func (s sliceLocatedSource) ForEachLocated(fn func(analytics.LocatedPoint) bool) int {
	n := 0
	for _, p := range s.points {
		if !fn(p) {
			break
		}
		n++
	}
	return n
}

func located(lat, lon float64, city string) analytics.LocatedPoint {
	return analytics.LocatedPoint{GeoPoint: analytics.GeoPoint{Lat: lat, Lon: lon}, City: city}
}

func collectPoints(t *testing.T, src analytics.PointSource) []analytics.GeoPoint {
	t.Helper()
	var got []analytics.GeoPoint
	src.ForEach(func(p analytics.GeoPoint) bool {
		got = append(got, p)
		return true
	})
	return got
}

func TestDropCityCentroidsDropsRepeatedWithinCity(t *testing.T) {
	src := sliceLocatedSource{points: []analytics.LocatedPoint{
		located(-34.60, -58.42, "caba"),
		located(-34.60, -58.42, "caba"),
		located(-34.60, -58.42, "caba"),
		located(-34.61, -58.43, "caba"),
		located(1, 2, ""),
	}}

	got := collectPoints(t, DropCityCentroids(src, 3))
	if len(got) != 2 {
		t.Fatalf("kept = %d points, want 2", len(got))
	}
	for _, p := range got {
		if p.Lat == -34.60 && p.Lon == -58.42 {
			t.Errorf("repeated city coordinate was not dropped: %+v", p)
		}
	}
}

func TestDropCityCentroidsKeepsUnknownCity(t *testing.T) {
	src := sliceLocatedSource{points: []analytics.LocatedPoint{
		located(1, 2, ""),
		located(1, 2, ""),
		located(1, 2, ""),
	}}

	got := collectPoints(t, DropCityCentroids(src, 2))
	if len(got) != 3 {
		t.Fatalf("kept = %d points, want 3 (unknown city never dropped)", len(got))
	}
}

func TestDropCityCentroidsDisabled(t *testing.T) {
	src := sliceLocatedSource{points: []analytics.LocatedPoint{
		located(-34.60, -58.42, "caba"),
		located(-34.60, -58.42, "caba"),
		located(-34.60, -58.42, "caba"),
	}}

	got := collectPoints(t, DropCityCentroids(src, 0))
	if len(got) != 3 {
		t.Fatalf("kept = %d points, want 3 (filter disabled)", len(got))
	}
}

func TestDropCityCentroidsKeepsDifferentCitySameCoordinate(t *testing.T) {
	src := sliceLocatedSource{points: []analytics.LocatedPoint{
		located(-34.60, -58.42, "caba"),
		located(-34.60, -58.42, "caba"),
		located(-34.60, -58.42, "laplata"),
	}}

	got := collectPoints(t, DropCityCentroids(src, 2))
	if len(got) != 1 {
		t.Fatalf("kept = %d points, want 1 (only caba pair dropped)", len(got))
	}
}

func TestDropCityCentroidsStats(t *testing.T) {
	src := sliceLocatedSource{points: []analytics.LocatedPoint{
		located(-34.60, -58.42, "caba"),
		located(-34.60, -58.42, "caba"),
		located(-34.60, -58.42, "caba"),
		located(-34.61, -58.43, "caba"),
		located(1, 2, ""),
	}}

	stats := DropCityCentroids(src, 3).(analytics.StatsSource).Stats()
	if stats.Scanned != 5 || stats.Valid != 2 || stats.Dropped != 3 || stats.Skipped != 0 {
		t.Errorf("stats = %+v, want {Scanned:5 Valid:2 Skipped:0 Dropped:3}", stats)
	}
}
