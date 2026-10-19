package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/romayengineer/fme/go/app"
	"github.com/romayengineer/fme/go/browser/facebook"
	"github.com/romayengineer/fme/go/core/logging"
)

type actionFunc func(*app.App, Flags) error

// actionRegistry maps a CLI action to its use case. Adding an action means
// adding one entry here and, if needed, one options struct in go/app.
var actionRegistry = map[string]actionFunc{
	"search": func(a *app.App, flags Flags) error {
		return a.Search(app.SearchOptions{
			Keywords:      flags.keywords,
			ScrollDown:    true,
			ScrollSeconds: flags.scrollSeconds,
			Pause:         facebook.WaitingForInput,
		})
	},
	"pull_description": func(a *app.App, flags Flags) error {
		return a.PullDescriptions(app.PullDescriptionsOptions{
			RetryUnavailable:        flags.retryUnavailable,
			DelayMs:                 flags.delayMs,
			RateLimitWaitSeconds:    flags.rateLimitWaitSeconds,
			MaxRateLimitWaitSeconds: flags.maxRateLimitWaitSeconds,
		})
	},
	"get_details": func(a *app.App, _ Flags) error {
		return a.GetDetails(app.GetDetailsOptions{Pause: facebook.WaitingForInput})
	},
	"install_drivers": func(a *app.App, _ Flags) error {
		return a.InstallDrivers()
	},
	"process_data": func(a *app.App, _ Flags) error {
		return a.ProcessResponses()
	},
	"dedupe_pdp": func(a *app.App, _ Flags) error {
		a.DedupePDP()
		return nil
	},
	"fill_empty": func(a *app.App, _ Flags) error {
		return a.FillEmpty()
	},
	"description_stats": func(a *app.App, _ Flags) error {
		stats, err := a.DescriptionStats()
		if err != nil {
			return err
		}
		printDescriptionStats(stats)
		return nil
	},
	"serve": func(a *app.App, _ Flags) error {
		return a.Serve()
	},
	"save": func(a *app.App, _ Flags) error {
		_, err := a.ImportDB()
		return err
	},
	"heatmap": func(_ *app.App, flags Flags) error {
		return runHeatmap(flags)
	},
	"heatmap_serve": func(_ *app.App, flags Flags) error {
		return runHeatmapServe(flags)
	},
}

func main() {
	if err := run(); err != nil {
		logging.LogError0("main", "action failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	flags, err := NewFlags()
	if err != nil {
		return err
	}
	logging.LogInfo0("main", "flags", "action", flags.action)

	if flags.listMaps {
		printMaps()
		return nil
	}

	action := strings.ToLower(flags.action)
	runner, ok := actionRegistry[action]
	if !ok {
		return fmt.Errorf("unknown action %q", flags.action)
	}

	application := newApplication(action)
	return runner(application, flags)
}

func newApplication(action string) *app.App {
	sink := app.NewFileResponseSink()

	return app.New(app.Deps{
		Sessions: app.NewSessionFactory(blockImagesFor(action), sink),
		Details:  app.NewFileDetailStore(),
		PDPState: app.NewFilePDPState(),
		Sink:     sink,
	})
}

func blockImagesFor(action string) bool {
	switch strings.ToLower(action) {
	case "search", "pull_description":
		return true
	default:
		return false
	}
}
