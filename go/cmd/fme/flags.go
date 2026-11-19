package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/romayengineer/fme/go/app"
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
	termsFile               string
	interval                time.Duration
	perTermDelay            time.Duration
	once                    bool
	archiveAfter            time.Duration
	keepArchived            time.Duration
	scheduleDelayMs         int
	logFile                 string
}

// optionalBool distinguishes an absent boolean flag from an explicit false.
type optionalBool struct {
	value bool
	set   bool
}

func (b *optionalBool) String() string { return strconv.FormatBool(b.value) }

func (b *optionalBool) Set(raw string) error {
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return err
	}
	b.value = value
	b.set = true
	return nil
}

// IsBoolFlag lets -headless work without a value.
func (b *optionalBool) IsBoolFlag() bool { return true }

// ptr returns the override, or nil when the flag was absent.
func (b *optionalBool) ptr() *bool {
	if !b.set {
		return nil
	}
	return &b.value
}

// registerFlags declares the command-line flags directly in flags and returns
// the tri-state headless flag.
func registerFlags(flags *Flags) *optionalBool {
	headless := &optionalBool{}
	flag.StringVar(&flags.action, "action", "search", "Action to perform: search, pull_description, get_details, save, process_data, dedupe_pdp, fill_empty, description_stats, archive_responses, schedule, status, heatmap, heatmap_serve, install_drivers, serve")
	flag.StringVar(&flags.keywords, "keywords", "", "Keywords to search for (required when -action is search)")
	flag.StringVar(&flags.logLevel, "log-level", "debug", "Log level: debug, info, warn, error")
	flag.IntVar(&flags.scrollSeconds, "scroll-seconds", 240, "Max seconds to keep scrolling while the results stop growing (action=search, schedule)")
	flag.IntVar(&flags.delayMs, "delay-ms", 1000, "Delay in milliseconds between replayed description requests (action=pull_description)")
	flag.IntVar(&flags.rateLimitWaitSeconds, "rate-limit-wait", 300, "Seconds to wait before retrying a rate-limited request (action=pull_description; 0 disables waiting)")
	flag.IntVar(&flags.maxRateLimitWaitSeconds, "max-rate-limit-wait", 1800, "Max total seconds to keep waiting for the rate limit to clear within a run (action=pull_description)")
	flag.Float64Var(&flags.cellDeg, "cell", 0.01, "Heat map grid cell size in degrees (action=heatmap)")
	flag.StringVar(&flags.out, "out", "", "Heat map output base path without extension (action=heatmap; default $FME_DATA_DIR/analytics/heatmap)")
	flag.StringVar(&flags.dir, "dir", "", "Detail files directory (action=heatmap; default $FME_DATA_DIR/__Details)")
	flag.StringVar(&flags.addr, "addr", "127.0.0.1:8799", "HTTP listen address for action=heatmap_serve")
	flag.StringVar(&flags.mapID, "map", "leaflet-esri", "Heat map renderer id, or \"all\" (action=heatmap; see -list-maps)")
	flag.StringVar(&flags.mapToken, "map-token", "", "API token for map providers that need one (falls back to the provider env var)")
	flag.BoolVar(&flags.listMaps, "list-maps", false, "List available heat map renderers and exit")
	flag.IntVar(&flags.minRepeat, "min-repeat", 5, "Heat map: drop coordinates repeated >= N times within one city (action=heatmap; 0 disables)")
	flag.BoolVar(&flags.retryUnavailable, "retry-unavailable", false, "pull_description: retry listings previously marked as no longer existing")
	flag.Var(headless, "headless", "Launch Chromium without a window; default is headed when a display is available (also FME_HEADLESS)")
	registerScheduleFlags(flags)
	return headless
}

// registerScheduleFlags declares the unattended-scheduler and logging flags.
func registerScheduleFlags(flags *Flags) {
	flag.StringVar(&flags.termsFile, "terms", "search-terms.txt", "Search terms file, one query per line (action=schedule)")
	flag.DurationVar(&flags.interval, "interval", app.DefaultScheduleInterval, "Pause between scheduler cycles (action=schedule)")
	flag.DurationVar(&flags.perTermDelay, "per-term-delay", app.DefaultPerTermDelay, "Pause between consecutive searches (action=schedule; 0 disables)")
	flag.BoolVar(&flags.once, "once", false, "Run one scheduler cycle now and exit (a current rate-limit block is still honoured)")
	flag.DurationVar(&flags.archiveAfter, "archive-after", app.DefaultArchiveAfter, "Age after which processed search responses are archived (action=schedule, archive_responses)")
	flag.DurationVar(&flags.keepArchived, "keep-archived", app.DefaultKeepArchived, "Age after which archived responses are deleted (action=schedule, archive_responses)")
	flag.IntVar(&flags.scheduleDelayMs, "schedule-delay-ms", app.DefaultScheduleDelayMs, "Delay in milliseconds between description requests in a scheduler cycle (action=schedule)")
	flag.StringVar(&flags.logFile, "log-file", "", "Append logs to this file in addition to stderr (all actions)")
}

// NewFlags parses the command-line flags, validates them, and configures logging.
func NewFlags() (Flags, error) {
	var flags Flags
	headless := registerFlags(&flags)
	flag.Parse()

	if err := validateFlags(&flags); err != nil {
		return Flags{}, err
	}
	if err := setLogLevel(flags.logLevel, flags.logFile); err != nil {
		return Flags{}, err
	}
	flags.headless = headless.ptr()

	return flags, nil
}

// validateFlags rejects flag combinations that cannot run.
func validateFlags(flags *Flags) error {
	if strings.ToLower(flags.action) == "search" && flags.keywords == "" {
		return errors.New("-keywords is required when -action=search")
	}
	return nil
}

// setLogLevel configures the default slog logger, optionally teeing to a file.
func setLogLevel(name, file string) error {
	writer, err := logWriter(file)
	if err != nil {
		return err
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(writer, &slog.HandlerOptions{
		Level: parseLogLevel(name),
	})))
	return nil
}

// logWriter returns stderr, or stderr plus an appended log file when configured.
func logWriter(file string) (io.Writer, error) {
	if file == "" {
		return os.Stderr, nil
	}

	if err := os.MkdirAll(filepath.Dir(file), 0o750); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	logFile, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // the log path is user-configured.
	if err != nil {
		return nil, fmt.Errorf("open log file %s: %w", file, err)
	}
	return io.MultiWriter(os.Stderr, logFile), nil
}

// parseLogLevel maps a flag value to an slog level, defaulting to debug.
func parseLogLevel(name string) slog.Level {
	if level, ok := logLevels[strings.ToLower(name)]; ok {
		return level
	}
	return slog.LevelDebug
}
