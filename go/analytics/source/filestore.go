// Package source provides PointSource adapters over the different places FME
// stores scraped listings.
package source

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/romayengineer/fme/go/analytics"
)

// DefaultDir is where product detail files live.
const DefaultDir = "data/__Details"

// FileSource reads product locations from data/__Details/detail_*.json, the same
// files the rest of FME materializes, so heat maps work without the PocketBase
// database. File reads are parallelized across workers; results are replayed in
// filename order so aggregation stays deterministic.
//
// The located points are read once and cached, so filters that need a first pass
// over the data (like the city-centroid filter) do not re-read the directory.
type FileSource struct {
	Dir     string // defaults to DefaultDir
	Workers int    // defaults to GOMAXPROCS

	mu     sync.Mutex
	stats  analytics.Stats
	points []analytics.LocatedPoint
	loaded bool
}

// detailRecord decodes only the fields we need. The coordinate fields are
// pointers so JSON null and malformed values are simply skipped.
type detailRecord struct {
	LocationLatitud         *float64 `json:"LocationLatitud"`
	LocationLongitude       *float64 `json:"LocationLongitude"`
	LocationGeocodeCityID   *string  `json:"LocationGeocodeCityID"`
	LocationGeocodeCityName *string  `json:"LocationGeocodeCityName2"`
}

// Stats returns coverage counters for the loaded points.
func (s *FileSource) Stats() analytics.Stats {
	s.locate()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats
}

// ForEach implements analytics.PointSource.
func (s *FileSource) ForEach(fn func(analytics.GeoPoint) bool) int {
	emitted := 0
	for _, p := range s.locate() {
		if !fn(p.GeoPoint) {
			break
		}
		emitted++
	}
	return emitted
}

// ForEachLocated implements analytics.LocatedSource.
func (s *FileSource) ForEachLocated(fn func(analytics.LocatedPoint) bool) int {
	emitted := 0
	for _, p := range s.locate() {
		if !fn(p) {
			break
		}
		emitted++
	}
	return emitted
}

// locate reads and caches the located points exactly once.
func (s *FileSource) locate() []analytics.LocatedPoint {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return s.points
	}

	dir := s.Dir
	if dir == "" {
		dir = DefaultDir
	}
	workers := s.Workers
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}

	s.points, s.stats = readAll(detailFiles(dir), workers)
	s.loaded = true
	return s.points
}

// readAll reads the given files concurrently and returns the valid points (with
// their city) in the same order as paths, plus coverage stats.
func readAll(paths []string, workers int) ([]analytics.LocatedPoint, analytics.Stats) {
	results := make([][]analytics.LocatedPoint, len(paths))
	scanned := make([]bool, len(paths))

	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results[i] = readPoints(paths[i])
				scanned[i] = true
			}
		}()
	}
	for i := range paths {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	var stats analytics.Stats
	points := make([]analytics.LocatedPoint, 0, len(paths))
	for i := range paths {
		if !scanned[i] {
			continue
		}
		stats.Scanned++
		points = append(points, results[i]...)
	}
	stats.Valid = len(points)
	stats.Skipped = stats.Scanned - stats.Valid

	return points, stats
}

// detailFiles lists data/__Details entries in deterministic (sorted) order.
func detailFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	paths := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "detail_") || !strings.HasSuffix(name, ".json") {
			continue
		}
		paths = append(paths, filepath.Join(dir, name))
	}
	sort.Strings(paths)
	return paths
}

// readPoints parses one detail file and returns its point, if the coordinates
// are usable. A missing/invalid file yields no points.
func readPoints(path string) []analytics.LocatedPoint {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var rec detailRecord
	if err := json.Unmarshal(body, &rec); err != nil {
		return nil
	}
	if rec.LocationLatitud == nil || rec.LocationLongitude == nil {
		return nil
	}
	lat, lon := *rec.LocationLatitud, *rec.LocationLongitude
	if !validCoord(lat, lon) {
		return nil
	}
	return []analytics.LocatedPoint{{
		GeoPoint: analytics.GeoPoint{Lat: lat, Lon: lon},
		City:     cityKey(rec.LocationGeocodeCityID, rec.LocationGeocodeCityName),
	}}
}

// cityKey builds the stable grouping key for a listing's city. The geocode id is
// preferred because it is language- and format-stable; the display name is the
// fallback when the id is absent. An empty key means the city is unknown.
func cityKey(id, name *string) string {
	if id != nil && *id != "" {
		return *id
	}
	if name != nil && *name != "" {
		return *name
	}
	return ""
}

// validCoord rejects out-of-range values and Facebook's (0,0) "unknown"
// placeholder, which would otherwise pile every missing location onto the map at
// the Gulf of Guinea.
func validCoord(lat, lon float64) bool {
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return false
	}
	return lat != 0 || lon != 0
}
