// Package pipeline wires a PointSource to the heatmap aggregation and writers.
package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/romayengineer/fme/go/analytics"
	"github.com/romayengineer/fme/go/analytics/heatmap"
	"github.com/romayengineer/fme/go/analytics/source"
)

// DefaultOutBase is the output path prefix; files are named
// <OutBase>.<map>.html and <OutBase>.geojson.
const DefaultOutBase = "data/analytics/heatmap"

// MapAll selects every registered map.
const MapAll = "all"

// Options configures a heatmap run.
type Options struct {
	Dir            string  // detail files dir; defaults to data/__Details
	CellDeg        float64 // grid cell size in degrees; defaults to heatmap.DefaultCellDeg
	OutBase        string  // output path without extension; defaults to DefaultOutBase
	Title          string  // map/page title
	Workers        int     // file read concurrency; defaults to GOMAXPROCS
	Map            string  // map id, "all", or "" for the default map
	MapToken       string  // API token; falls back to the map's TokenEnv
	MinRepeatCount int     // drop coordinates repeated >= N times in one city; 0 disables
}

// Result describes what a run produced.
type Result struct {
	MapID       string // selected map, or "all"
	GeoJSONPath string
	HTMLPath    string   // first generated page (convenience)
	HTMLPaths   []string // every generated page
	Grid        heatmap.Grid
	Stats       analytics.Stats
}

// RunFileHeatmap reads product locations from detail JSON files, aggregates them
// into a grid, and writes <OutBase>.geojson plus one <OutBase>.<map>.html per
// selected map.
func RunFileHeatmap(opts Options) (Result, error) {
	if opts.OutBase == "" {
		opts.OutBase = DefaultOutBase
	}
	if opts.Title == "" {
		opts.Title = "Marketplace product heat map"
	}

	result, mapIDs, geojson, err := buildFileHeatmap(opts)
	if err != nil {
		return Result{}, err
	}
	if err := writeFileHeatmap(opts, &result, mapIDs, geojson); err != nil {
		return Result{}, err
	}

	return result, nil
}

// buildFileHeatmap loads points, aggregates the grid and renders GeoJSON. It
// performs no writes.
func buildFileHeatmap(opts Options) (Result, []string, []byte, error) {
	mapIDs, err := resolveMaps(opts.Map)
	if err != nil {
		return Result{}, nil, nil, err
	}

	src := &source.FileSource{Dir: opts.Dir, Workers: opts.Workers}
	var points analytics.PointSource = src
	if opts.MinRepeatCount > 0 {
		points = source.DropCityCentroids(src, opts.MinRepeatCount)
	}
	grid := heatmap.Aggregate(points, heatmap.Options{CellDeg: opts.CellDeg})

	if err := src.Err(); err != nil {
		return Result{}, nil, nil, fmt.Errorf("read detail files: %w", err)
	}

	geojson, err := grid.GeoJSON()
	if err != nil {
		return Result{}, nil, nil, fmt.Errorf("build geojson: %w", err)
	}

	result := Result{
		MapID: opts.Map,
		Grid:  grid,
		Stats: sourceStats(points),
	}
	if result.MapID == "" {
		result.MapID = heatmap.DefaultMapID
	}

	return result, mapIDs, geojson, nil
}

// writeFileHeatmap writes the GeoJSON and one HTML page per selected map.
func writeFileHeatmap(opts Options, result *Result, mapIDs []string, geojson []byte) error {
	if dir := filepath.Dir(opts.OutBase); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create output dir: %w", err)
		}
	}

	geojsonPath := opts.OutBase + ".geojson"
	if err := os.WriteFile(geojsonPath, geojson, 0o644); err != nil {
		return fmt.Errorf("write geojson: %w", err)
	}
	result.GeoJSONPath = geojsonPath

	for _, mapID := range mapIDs {
		html, err := result.Grid.RenderHTML(mapID, heatmap.MapOptions{
			Title: opts.Title,
			Token: tokenFor(mapID, opts.MapToken),
		})
		if err != nil {
			return fmt.Errorf("build html for %s: %w", mapID, err)
		}

		path := fmt.Sprintf("%s.%s.html", opts.OutBase, mapID)
		if err := os.WriteFile(path, html, 0o644); err != nil {
			return fmt.Errorf("write html for %s: %w", mapID, err)
		}
		result.HTMLPaths = append(result.HTMLPaths, path)
	}

	if len(result.HTMLPaths) > 0 {
		result.HTMLPath = result.HTMLPaths[0]
	}

	return nil
}

// sourceStats reports coverage from a source that implements StatsSource.
func sourceStats(src analytics.PointSource) analytics.Stats {
	if s, ok := src.(analytics.StatsSource); ok {
		return s.Stats()
	}
	return analytics.Stats{}
}

// resolveMaps turns the -map selector into a concrete, ordered list of map ids.
func resolveMaps(selector string) ([]string, error) {
	switch selector {
	case "", heatmap.DefaultMapID:
		return []string{heatmap.DefaultMapID}, nil
	case MapAll:
		return heatmap.MapIDs(), nil
	}
	if _, ok := heatmap.MapByID(selector); !ok {
		return nil, fmt.Errorf("unknown map %q (valid: %s, %s)", selector, MapAll, strings.Join(heatmap.MapIDs(), ", "))
	}
	return []string{selector}, nil
}

// tokenFor returns the explicit token if set, otherwise the map's env var.
func tokenFor(mapID, explicit string) string {
	if explicit != "" {
		return explicit
	}
	info, ok := heatmap.MapByID(mapID)
	if !ok || !info.RequiresToken || info.TokenEnv == "" {
		return ""
	}
	return os.Getenv(info.TokenEnv)
}
