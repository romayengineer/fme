package main

import (
	"fmt"
	"os"

	"github.com/romayengineer/fme/go/app"
	"github.com/romayengineer/fme/go/browser/facebook"
	"github.com/romayengineer/fme/go/core/logging"
)

type ScrapperImpl struct {
	BlockImages             bool
	ScrollDownOnSearch      bool
	ScrollSeconds           int
	DelayMs                 int
	RateLimitWaitSeconds    int
	MaxRateLimitWaitSeconds int
	RetryUnavailable        bool
	SearchKewords           string
	StartTimeToProcess      int64
	application             *app.App
}

func NewScrapper(flags Flags) ScrapperImpl {
	var blockImages bool
	var ScrollDownOnSearch bool
	switch flags.action {
	case "search":
		blockImages = true
		ScrollDownOnSearch = true
	case "pull_description":
		blockImages = true
	default:
		blockImages = false
		ScrollDownOnSearch = false
	}
	sink := app.NewFileResponseSink()

	return ScrapperImpl{
		BlockImages:             blockImages,
		ScrollDownOnSearch:      ScrollDownOnSearch,
		ScrollSeconds:           flags.scrollSeconds,
		DelayMs:                 flags.delayMs,
		RateLimitWaitSeconds:    flags.rateLimitWaitSeconds,
		MaxRateLimitWaitSeconds: flags.maxRateLimitWaitSeconds,
		RetryUnavailable:        flags.retryUnavailable,
		StartTimeToProcess:      0,
		SearchKewords:           flags.keywords,
		application: app.New(app.Deps{
			Sessions: app.NewSessionFactory(blockImages, sink),
			Details:  app.NewFileDetailStore(),
			PDPState: app.NewFilePDPState(),
			Sink:     sink,
		}),
	}
}

func (s *ScrapperImpl) SearchProducts() error {
	if s.application == nil {
		return fmt.Errorf("search: application is not configured")
	}

	return s.application.Search(app.SearchOptions{
		Keywords:      s.SearchKewords,
		ScrollDown:    s.ScrollDownOnSearch,
		ScrollSeconds: s.ScrollSeconds,
		Pause:         facebook.WaitingForInput,
	})
}

// PullDescriptions fetches the description (redacted_description) for every detail
// file that is missing one. It only needs to open a listing once, to capture the
// MarketplacePDPContainerQuery request template; the rest is replayed via ctx.Fetch.
// See docs/redacted_description.md.
func (s *ScrapperImpl) PullDescriptions() error {
	if s.application == nil {
		return fmt.Errorf("pull-description: application is not configured")
	}

	return s.application.PullDescriptions(app.PullDescriptionsOptions{
		RetryUnavailable:        s.RetryUnavailable,
		DelayMs:                 s.DelayMs,
		RateLimitWaitSeconds:    s.RateLimitWaitSeconds,
		MaxRateLimitWaitSeconds: s.MaxRateLimitWaitSeconds,
	})
}

// InstallDrivers downloads the Playwright driver and Chromium ahead of time, so the
// (large, one-off) download happens during `just setup` instead of inside the first
// scrape. It reuses the exact install step Begin performs and is safe to re-run:
// already-downloaded artifacts are kept. It needs neither a .env nor a Facebook
// session, so it can run right after cloning.
func (s *ScrapperImpl) InstallDrivers() error {
	if s.application == nil {
		return fmt.Errorf("install-drivers: application is not configured")
	}
	return s.application.InstallDrivers()
}

func (s *ScrapperImpl) GetDetails() error {
	if s.application == nil {
		return fmt.Errorf("get-details: application is not configured")
	}
	return s.application.GetDetails(app.GetDetailsOptions{
		Pause: facebook.WaitingForInput,
	})
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

	scrapper := NewScrapper(flags)

	switch flags.action {
	case "search":
		return scrapper.SearchProducts()
	case "install_drivers":
		return scrapper.InstallDrivers()
	case "pull_description":
		return scrapper.PullDescriptions()
	case "get_details":
		return scrapper.GetDetails()
	case "process_data":
		return scrapper.application.ProcessResponses()
	case "dedupe_pdp":
		scrapper.application.DedupePDP()
		return nil
	case "serve":
		return scrapper.application.Serve()
	case "save":
		_, err := scrapper.application.ImportDB()
		return err
	case "fill_empty":
		return scrapper.application.FillEmpty()
	case "description_stats":
		stats, err := scrapper.application.DescriptionStats()
		if err != nil {
			return err
		}
		printDescriptionStats(stats)
		return nil
	case "heatmap":
		return runHeatmap(flags)
	case "heatmap_serve":
		return runHeatmapServe(flags)
	default:
		return fmt.Errorf("unknown action %q", flags.action)
	}
}
