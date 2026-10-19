package main

import (
	"fmt"
	"os"
	"time"

	"github.com/romayengineer/fme/go/app"
	"github.com/romayengineer/fme/go/browser/driver"
	"github.com/romayengineer/fme/go/browser/facebook"
	"github.com/romayengineer/fme/go/browser/pages"
	"github.com/romayengineer/fme/go/core/jsonutil"
	"github.com/romayengineer/fme/go/core/logging"
	"github.com/romayengineer/fme/go/store/db"
	"github.com/romayengineer/fme/go/store/filestore"
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
	sessions                app.SessionFactory
	waiter                  app.Waiter
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
		sessions:                app.NewSessionFactory(blockImages),
		application: app.New(app.Deps{
			Sessions: app.NewSessionFactory(blockImages),
			Details:  app.NewFileDetailStore(),
			PDPState: app.NewFilePDPState(),
		}),
	}
}

func (s *ScrapperImpl) Begin() (driver.ContextWrapperInterface, error) {
	session, err := s.sessions.Open()
	if err != nil {
		return nil, fmt.Errorf("open browser session: %w", err)
	}

	s.waiter = session.Waiter
	return session.Context, nil
}

// waitForCapture waits until in-flight response handlers finish writing. It is
// called before closing the context or reading the files they produce.
func (s *ScrapperImpl) waitForCapture() {
	if s.waiter != nil {
		s.waiter.Wait()
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
	fmt.Println("Installing Playwright driver and Chromium (this may take a while)...")

	wrapper, err := driver.NewPlaywrightWrapper()
	if err != nil {
		return fmt.Errorf("install playwright drivers: %w", err)
	}

	if err := wrapper.Stop(); err != nil {
		logging.LogError0("InstallDrivers", "could not stop playwright", "error", err)
	}

	logging.LogInfo0("InstallDrivers", "playwright driver and chromium are installed")
	return nil
}

func (s *ScrapperImpl) GetDetails() error {
	ctx, err := s.Begin()
	if err != nil {
		return fmt.Errorf("begin get details: %w", err)
	}
	defer func() {
		s.waitForCapture()
		if err := ctx.Close(); err != nil {
			logging.LogError0("GetDetails", "could not close browser context", "error", err)
		}
	}()

	page, err := ctx.NewPage()
	if err != nil {
		return fmt.Errorf("create get-details page: %w", err)
	}
	pgs, err := pages.NewPages(page)
	if err != nil {
		return fmt.Errorf("create page helpers: %w", err)
	}

	_, err = filestore.ForEachDetail(func(filePath string, jsonData map[string]any) bool {
		PriceAmount := jsonutil.GetKey(jsonData, "PriceAmount")
		if PriceAmount != nil {
			return true
		}

		productId := jsonutil.GetKey(jsonData, "ID")
		if productId == nil {
			return true
		}

		productID, ok := productId.(string)
		if !ok {
			logging.LogWarn0("GetDetails", "skipping invalid product id", "id", productId)
			return true
		}
		if err := pgs.GoToProduct(productID); err != nil {
			logging.LogError0("GetDetails", "could not open product", "productId", productID, "error", err)
			return true
		}
		time.Sleep(3 * time.Second)

		return true
	}, false)
	if err != nil {
		logging.LogError0("GetDetails", "error iterating detail files", "error", err)
	}

	facebook.WaitingForInput()

	return err
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
		_, err := filestore.ProcessData(0)
		return err
	case "dedupe_pdp":
		filestore.DedupePDP()
		return nil
	case "serve":
		_, err := db.Serve()
		return err
	case "save":
		_, err := db.ProcessDataInDB()
		return err
	case "fill_empty":
		return filestore.FillEmpty()
	case "description_stats":
		stats, err := filestore.ComputeDescriptionStats()
		if err != nil {
			return err
		}
		filestore.PrintDescriptionStats(stats)
		return nil
	case "heatmap":
		return runHeatmap(flags)
	case "heatmap_serve":
		return runHeatmapServe(flags)
	default:
		return fmt.Errorf("unknown action %q", flags.action)
	}
}
