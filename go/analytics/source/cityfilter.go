package source

import (
	"sync"

	"github.com/romayengineer/fme/go/analytics"
)

// DropCityCentroids wraps a LocatedSource and removes coordinates that repeat
// across many listings within the same city. Facebook returns a single
// representative coordinate for listings that specify only a city (no street or
// neighborhood), so those points pile into one grid cell and swamp the heat map.
//
// A coordinate is dropped when at least minCount listings of the same city share
// it. Listings with no city information are never dropped, and minCount <= 0
// disables filtering entirely.
func DropCityCentroids(inner analytics.LocatedSource, minCount int) analytics.PointSource {
	return &cityCentroidFilter{inner: inner, minCount: minCount}
}

// locatedKey identifies a coordinate within a city. Both fields are comparable,
// so the struct can be used directly as a map key.
type locatedKey struct {
	point analytics.GeoPoint
	city  string
}

type cityCentroidFilter struct {
	inner    analytics.LocatedSource
	minCount int

	once sync.Once
	pts  []analytics.LocatedPoint
	drop map[locatedKey]bool
}

// ensure materializes the inner source once and computes the drop set. Repeated
// calls (ForEach, then Stats) reuse the cached result.
func (f *cityCentroidFilter) ensure() {
	f.once.Do(func() {
		f.pts = collect(f.inner)
		if f.minCount <= 0 {
			return
		}

		counts := make(map[locatedKey]int)
		for _, p := range f.pts {
			if p.City == "" {
				continue
			}
			counts[locatedKey{point: p.GeoPoint, city: p.City}]++
		}

		f.drop = make(map[locatedKey]bool, len(counts))
		for key, n := range counts {
			if n >= f.minCount {
				f.drop[key] = true
			}
		}
	})
}

// ForEach implements analytics.PointSource, skipping dropped coordinates.
func (f *cityCentroidFilter) ForEach(fn func(analytics.GeoPoint) bool) int {
	f.ensure()
	emitted := 0
	for _, p := range f.pts {
		if f.drop[locatedKey{point: p.GeoPoint, city: p.City}] {
			continue
		}
		if !fn(p.GeoPoint) {
			break
		}
		emitted++
	}
	return emitted
}

// Stats implements analytics.StatsSource, reporting how many points the filter
// removed on top of the inner source's scan/skip counters.
func (f *cityCentroidFilter) Stats() analytics.Stats {
	f.ensure()

	kept := 0
	for _, p := range f.pts {
		if !f.drop[locatedKey{point: p.GeoPoint, city: p.City}] {
			kept++
		}
	}
	dropped := len(f.pts) - kept

	if s, ok := f.inner.(analytics.StatsSource); ok {
		stats := s.Stats()
		stats.Valid = kept
		stats.Dropped = dropped
		return stats
	}
	return analytics.Stats{Scanned: len(f.pts), Valid: kept, Dropped: dropped}
}

// collect drains every located point from src into a slice.
func collect(src analytics.LocatedSource) []analytics.LocatedPoint {
	var points []analytics.LocatedPoint
	src.ForEachLocated(func(p analytics.LocatedPoint) bool {
		points = append(points, p)
		return true
	})
	return points
}
