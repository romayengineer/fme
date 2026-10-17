package main

import (
	"flag"
	"log/slog"
	"os"
	"strings"

	"github.com/romayengineer/fme/go/core/logging"
)

type Flags struct {
	action                  string
	keywords                string
	logLevel                string
	scrollSeconds           int
	delayMs                 int
	rateLimitWaitSeconds    int
	maxRateLimitWaitSeconds int
	cellDeg                 float64
	out                     string
	dir                     string
	addr                    string
	mapID                   string
	mapToken                string
	listMaps                bool
	minRepeat               int
}

func NewFlags() Flags {
	action := flag.String("action", "search", "Action to perform: search, pull_description, get_details, save, process_data, dedupe_pdp, fill_empty, description_stats, heatmap, heatmap_serve, install_drivers, serve")
	keywords := flag.String("keywords", "", "Keywords to search for (required when -action is search)")
	logLevel := flag.String("log-level", "debug", "Log level: debug, info, warn, error")
	scrollSeconds := flag.Int("scroll-seconds", 60, "Max seconds to scroll search results before opening a product")
	delayMs := flag.Int("delay-ms", 1000, "Delay in milliseconds between replayed description requests (action=pull_description)")
	rateLimitWaitSeconds := flag.Int("rate-limit-wait", 300, "Seconds to wait before retrying a rate-limited request (action=pull_description; 0 disables waiting)")
	maxRateLimitWaitSeconds := flag.Int("max-rate-limit-wait", 1800, "Max total seconds to keep waiting for the rate limit to clear within a run (action=pull_description)")
	cellDeg := flag.Float64("cell", 0.01, "Heat map grid cell size in degrees (action=heatmap)")
	out := flag.String("out", "data/analytics/heatmap", "Heat map output base path without extension (action=heatmap)")
	dir := flag.String("dir", "", "Detail files directory (action=heatmap; default data/__Details)")
	addr := flag.String("addr", "127.0.0.1:8799", "HTTP listen address for action=heatmap_serve")
	mapID := flag.String("map", "leaflet-carto", "Heat map renderer id, or \"all\" (action=heatmap; see -list-maps)")
	mapToken := flag.String("map-token", "", "API token for map providers that need one (falls back to the provider env var)")
	listMaps := flag.Bool("list-maps", false, "List available heat map renderers and exit")
	minRepeat := flag.Int("min-repeat", 5, "Heat map: drop coordinates repeated >= N times within one city (action=heatmap; 0 disables)")
	flag.Parse()

	if strings.ToLower(*action) == "search" && *keywords == "" {
		logging.LogFatal("keywords flag is required when action is search")
	}

	var level slog.Level
	switch strings.ToLower(*logLevel) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelDebug
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})))

	return Flags{
		action:                  *action,
		keywords:                *keywords,
		logLevel:                *logLevel,
		scrollSeconds:           *scrollSeconds,
		delayMs:                 *delayMs,
		rateLimitWaitSeconds:    *rateLimitWaitSeconds,
		maxRateLimitWaitSeconds: *maxRateLimitWaitSeconds,
		cellDeg:                 *cellDeg,
		out:                     *out,
		dir:                     *dir,
		addr:                    *addr,
		mapID:                   *mapID,
		mapToken:                *mapToken,
		listMaps:                *listMaps,
		minRepeat:               *minRepeat,
	}
}
