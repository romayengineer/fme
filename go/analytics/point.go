// Package analytics provides spatial analytics over scraped Marketplace listings.
//
// The domain is intentionally dependency-free: it knows only about GeoPoint and
// the PointSource abstraction, so the same aggregation can be fed from detail
// JSON files today and from the PocketBase database later.
package analytics

// GeoPoint is a single product location in WGS84 decimal degrees.
type GeoPoint struct {
	Lat float64
	Lon float64
}

// LocatedPoint is a GeoPoint annotated with the city Facebook geocoded it to.
// City is a stable grouping key (the geocode city id, falling back to its name)
// and is empty when the listing carries no city information.
type LocatedPoint struct {
	GeoPoint
	City string
}

// PointSource yields the usable product locations from some backing store.
type PointSource interface {
	// ForEach calls fn for every point with valid coordinates (null and
	// out-of-range locations are already filtered out). Iteration stops early
	// when fn returns false. It returns the number of points emitted.
	ForEach(fn func(GeoPoint) bool) int
}

// LocatedSource yields product locations together with their city. It is the
// richer counterpart to PointSource, used by filters that must reason about
// which coordinates are city-level defaults.
type LocatedSource interface {
	// ForEachLocated calls fn for every point with valid coordinates. Iteration
	// stops early when fn returns false. It returns the number of points emitted.
	ForEachLocated(fn func(LocatedPoint) bool) int
}

// Stats describes how many records a source scanned and how many it kept.
type Stats struct {
	Scanned int // records inspected
	Valid   int // records with usable coordinates
	Skipped int // records dropped (null, malformed or out-of-range coordinates)
	Dropped int // records removed by a filter (e.g. repeated city-level coordinates)
}

// StatsSource is an optional interface a PointSource may implement so callers can
// report coverage without coupling to a concrete implementation.
type StatsSource interface {
	Stats() Stats
}
