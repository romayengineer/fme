package main

import (
	"errors"
	"flag"
	"log/slog"
	"os"
	"strings"
)

// logLevels maps the -log-level values to slog levels.
var logLevels = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

// Flags holds the parsed command-line options.
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
	retryUnavailable        bool
	headless                *bool
}

// rawFlags holds the flag pointers before they are validated and copied.
type rawFlags struct {
	action                  *string
	keywords                *string
	logLevel                *string
	scrollSeconds           *int
	delayMs                 *int
	rateLimitWaitSeconds    *int
	maxRateLimitWaitSeconds *int
	cellDeg                 *float64
	out                     *string
	dir                     *string
	addr                    *string
	mapID                   *string
	mapToken                *string
	listMaps                *bool
	minRepeat               *int
	retryUnavailable        *bool
	headless                *bool
}

// registerFlags declares the command-line flags.
func registerFlags() *rawFlags {
	return &rawFlags{
		action:                  flag.String("action", "search", "Action to perform: search, pull_description, get_details, save, process_data, dedupe_pdp, fill_empty, description_stats, heatmap, heatmap_serve, install_drivers, serve"),
		keywords:                flag.String("keywords", "", "Keywords to search for (required when -action is search)"),
		logLevel:                flag.String("log-level", "debug", "Log level: debug, info, warn, error"),
		scrollSeconds:           flag.Int("scroll-seconds", 240, "Max seconds to keep scrolling while the results stop growing (action=search)"),
		delayMs:                 flag.Int("delay-ms", 1000, "Delay in milliseconds between replayed description requests (action=pull_description)"),
		rateLimitWaitSeconds:    flag.Int("rate-limit-wait", 300, "Seconds to wait before retrying a rate-limited request (action=pull_description; 0 disables waiting)"),
		maxRateLimitWaitSeconds: flag.Int("max-rate-limit-wait", 1800, "Max total seconds to keep waiting for the rate limit to clear within a run (action=pull_description)"),
		cellDeg:                 flag.Float64("cell", 0.01, "Heat map grid cell size in degrees (action=heatmap)"),
		out:                     flag.String("out", "data/analytics/heatmap", "Heat map output base path without extension (action=heatmap)"),
		dir:                     flag.String("dir", "", "Detail files directory (action=heatmap; default data/__Details)"),
		addr:                    flag.String("addr", "127.0.0.1:8799", "HTTP listen address for action=heatmap_serve"),
		mapID:                   flag.String("map", "leaflet-esri", "Heat map renderer id, or \"all\" (action=heatmap; see -list-maps)"),
		mapToken:                flag.String("map-token", "", "API token for map providers that need one (falls back to the provider env var)"),
		listMaps:                flag.Bool("list-maps", false, "List available heat map renderers and exit"),
		minRepeat:               flag.Int("min-repeat", 5, "Heat map: drop coordinates repeated >= N times within one city (action=heatmap; 0 disables)"),
		retryUnavailable:        flag.Bool("retry-unavailable", false, "pull_description: retry listings previously marked as no longer existing"),
		headless:                flag.Bool("headless", false, "Launch Chromium without a window; default is headed when a display is available (also FME_HEADLESS)"),
	}
}

// NewFlags parses the command-line flags, validates them, and configures logging.
func NewFlags() (Flags, error) {
	raw := registerFlags()
	flag.Parse()

	if err := validateFlags(raw); err != nil {
		return Flags{}, err
	}
	setLogLevel(*raw.logLevel)

	return Flags{
		action:                  *raw.action,
		keywords:                *raw.keywords,
		logLevel:                *raw.logLevel,
		scrollSeconds:           *raw.scrollSeconds,
		delayMs:                 *raw.delayMs,
		rateLimitWaitSeconds:    *raw.rateLimitWaitSeconds,
		maxRateLimitWaitSeconds: *raw.maxRateLimitWaitSeconds,
		cellDeg:                 *raw.cellDeg,
		out:                     *raw.out,
		dir:                     *raw.dir,
		addr:                    *raw.addr,
		mapID:                   *raw.mapID,
		mapToken:                *raw.mapToken,
		listMaps:                *raw.listMaps,
		minRepeat:               *raw.minRepeat,
		retryUnavailable:        *raw.retryUnavailable,
		headless:                headlessOverride(raw.headless),
	}, nil
}

// validateFlags rejects flag combinations that cannot run.
func validateFlags(raw *rawFlags) error {
	if strings.ToLower(*raw.action) == "search" && *raw.keywords == "" {
		return errors.New("-keywords is required when -action=search")
	}
	return nil
}

// headlessOverride returns the flag value only when -headless was given. A
// plain bool cannot distinguish "-headless=false" from an absent flag.
func headlessOverride(headless *bool) *bool {
	var override *bool
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "headless" {
			override = headless
		}
	})
	return override
}

// setLogLevel configures the default slog logger.
func setLogLevel(name string) {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: parseLogLevel(name),
	})))
}

// parseLogLevel maps a flag value to an slog level, defaulting to debug.
func parseLogLevel(name string) slog.Level {
	if level, ok := logLevels[strings.ToLower(name)]; ok {
		return level
	}
	return slog.LevelDebug
}
