package heatmap

import (
	"math"
	"sort"

	"github.com/romayengineer/fme/go/analytics"
)

// boundsAccumulator grows a bounding box as points are added.
type boundsAccumulator struct {
	bounds Bounds
	seen   bool
}

// add expands the box to include p.
func (b *boundsAccumulator) add(p analytics.GeoPoint) {
	if !b.seen {
		b.bounds = Bounds{MinLat: p.Lat, MaxLat: p.Lat, MinLon: p.Lon, MaxLon: p.Lon}
		b.seen = true
		return
	}

	b.bounds.MinLat = math.Min(b.bounds.MinLat, p.Lat)
	b.bounds.MaxLat = math.Max(b.bounds.MaxLat, p.Lat)
	b.bounds.MinLon = math.Min(b.bounds.MinLon, p.Lon)
	b.bounds.MaxLon = math.Max(b.bounds.MaxLon, p.Lon)
}

// value returns the accumulated box, or the zero box when no points were added.
func (b *boundsAccumulator) value() Bounds {
	if !b.seen {
		return Bounds{}
	}
	return b.bounds
}

// Aggregate bins every point from src into a fixed lat/lon grid.
//
// It performs a single pass over the points, accumulating counts into a map
// keyed by integer cell indices, then flattens and sorts the cells by count
// (descending, ties broken by cell position) so the output is deterministic.
func Aggregate(src analytics.PointSource, opts Options) Grid {
	opts = opts.withDefaults()
	cell := opts.CellDeg

	counts := make(map[CellKey]int)
	grid := Grid{Options: opts}
	bounds := &boundsAccumulator{}

	src.ForEach(func(p analytics.GeoPoint) bool {
		counts[cellKey(p, cell)]++
		grid.Total++
		bounds.add(p)
		return true
	})

	grid.Bounds = bounds.value()
	grid.Cells, grid.MaxCount = gridCells(counts, cell)

	return grid
}

// cellKey maps a point to its grid cell.
func cellKey(p analytics.GeoPoint, cell float64) CellKey {
	return CellKey{
		X: int32(math.Floor(p.Lon / cell)),
		Y: int32(math.Floor(p.Lat / cell)),
	}
}

// gridCells flattens the counts into sorted cells and returns the peak count.
func gridCells(counts map[CellKey]int, cell float64) ([]Cell, int) {
	cells := make([]Cell, 0, len(counts))
	maxCount := 0

	for key, count := range counts {
		minLon := float64(key.X) * cell
		minLat := float64(key.Y) * cell
		cells = append(cells, Cell{
			Key:    key,
			Count:  count,
			MinLat: minLat,
			MaxLat: minLat + cell,
			MinLon: minLon,
			MaxLon: minLon + cell,
		})
		if count > maxCount {
			maxCount = count
		}
	}

	sort.Slice(cells, func(i, j int) bool { return cellLess(cells[i], cells[j]) })
	return cells, maxCount
}

// cellLess orders cells by count descending, then by position.
func cellLess(a, b Cell) bool {
	if a.Count != b.Count {
		return a.Count > b.Count
	}
	if a.Key.X != b.Key.X {
		return a.Key.X < b.Key.X
	}
	return a.Key.Y < b.Key.Y
}

// Intensity returns the count normalized to [0,1] against the busiest cell.
func (g Grid) Intensity(c Cell) float64 {
	if g.MaxCount == 0 {
		return 0
	}
	return float64(c.Count) / float64(g.MaxCount)
}
