package main

import (
	"fmt"

	"github.com/romayengineer/fme/go/analytics/heatmap"
	"github.com/romayengineer/fme/go/analytics/pipeline"
	"github.com/romayengineer/fme/go/core/logging"
)

// runHeatmap aggregates product locations from data/__Details into a grid and
// writes <out>.geojson plus one <out>.<map>.html per selected map. See
// `-action=heatmap -map=<id> -cell=<deg> -out=<base> -dir=<details dir>`.
func runHeatmap(flags Flags) {
	result, err := pipeline.RunFileHeatmap(pipeline.Options{
		Dir:            flags.dir,
		CellDeg:        flags.cellDeg,
		OutBase:        flags.out,
		Map:            flags.mapID,
		MapToken:       flags.mapToken,
		MinRepeatCount: flags.minRepeat,
	})
	if err != nil {
		logging.LogError0("runHeatmap", "heat map failed", "error", err)
		return
	}

	fmt.Printf("heat map written (map: %s)\n", result.MapID)
	fmt.Printf("  geoJSON:   %s\n", result.GeoJSONPath)
	for _, p := range result.HTMLPaths {
		fmt.Printf("  html:      %s\n", p)
	}
	fmt.Printf("  products:  %d (of %d scanned, %d skipped: no/!valid coords, %d dropped: city-level)\n",
		result.Stats.Valid, result.Stats.Scanned, result.Stats.Skipped, result.Stats.Dropped)
	fmt.Printf("  cells:     %d (peak %d, cell %.4f deg)\n",
		len(result.Grid.Cells), result.Grid.MaxCount, result.Grid.Options.CellDeg)
	fmt.Printf("\ntip: switch basemap with -map=<id> (%s), or open the .html directly\n", joinMapIDs())

	logging.LogInfo0("runHeatmap", "heat map written",
		"geojson", result.GeoJSONPath, "html", result.HTMLPaths, "map", result.MapID,
		"products", result.Stats.Valid, "cells", len(result.Grid.Cells))
}

// printMaps lists the available map renderers for -list-maps.
func printMaps() {
	fmt.Println("available maps (-map=<id>):")
	for _, m := range heatmap.Maps() {
		marker := " "
		if m.ID == heatmap.DefaultMapID {
			marker = "*"
		}
		token := ""
		if m.RequiresToken {
			token = fmt.Sprintf("  [token: -map-token or $%s]", m.TokenEnv)
		}
		fmt.Printf(" %s %-16s %s%s\n", marker, m.ID, m.Description, token)
	}
	fmt.Println("also: -map=all")
	fmt.Printf("\n(* = default)\n")
}

func joinMapIDs() string {
	ids := heatmap.MapIDs()
	out := ""
	for i, id := range ids {
		if i > 0 {
			out += ", "
		}
		out += id
	}
	return out
}
