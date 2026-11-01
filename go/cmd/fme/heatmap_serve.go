package main

import (
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/romayengineer/fme/go/analytics/pipeline"
	"github.com/romayengineer/fme/go/core/logging"
)

// runHeatmapServe builds the heat map and serves the output directory over HTTP.
//
// Opening heatmap.html directly as a file:// URL often makes the browser send no
// (or a null) origin, and tile servers such as OpenStreetMap reject those
// requests, so the basemap shows "Access blocked". Serving the same page from
// 127.0.0.1 gives the browser a real origin and the tiles load normally.
func runHeatmapServe(flags Flags) error {
	result, err := pipeline.RunFileHeatmap(pipeline.Options{
		Dir:            flags.dir,
		CellDeg:        flags.cellDeg,
		OutBase:        flags.out,
		Map:            flags.mapID,
		MapToken:       flags.mapToken,
		MinRepeatCount: flags.minRepeat,
	})
	if err != nil {
		logging.LogError0("runHeatmapServe", "heat map failed", "error", err)
		return err
	}

	fmt.Printf("heat map written (map: %s)\n", result.MapID)
	fmt.Printf("  products:  %d (of %d scanned, %d skipped, %d dropped: city-level)\n",
		result.Stats.Valid, result.Stats.Scanned, result.Stats.Skipped, result.Stats.Dropped)
	fmt.Printf("  cells:     %d (peak %d, cell %.4f deg)\n",
		len(result.Grid.Cells), result.Grid.MaxCount, result.Grid.Options.CellDeg)

	dir := filepath.Dir(result.HTMLPath)
	fmt.Printf("\nserving %s at:\n", dir)
	for _, p := range result.HTMLPaths {
		fmt.Printf("  http://%s/%s\n", flags.addr, filepath.Base(p))
	}
	fmt.Printf("press Ctrl+C to stop\n")

	logging.LogInfo0("runHeatmapServe", "serving heat map", "dir", dir, "addr", flags.addr, "html", result.HTMLPaths)

	server := &http.Server{
		Addr:              flags.addr,
		Handler:           noCache(http.FileServer(http.Dir(dir))),
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		logging.LogError0("runHeatmapServe", "http server stopped", "error", err)
		return err
	}

	return nil
}

// noCache stops the browser from serving a stale heat map after a re-run.
func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
