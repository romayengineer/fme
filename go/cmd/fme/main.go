package main

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/romayengineer/fme/go/app"
	"github.com/romayengineer/fme/go/browser/driver"
	"github.com/romayengineer/fme/go/browser/facebook"
	"github.com/romayengineer/fme/go/browser/pages"
	"github.com/romayengineer/fme/go/capture"
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
	// Materialize any as-yet-unprocessed search responses into data/__Details first
	// (no browser needed), so listings found by a recent `search` run are included
	// in this fill pass.
	var runErr error
	if _, err := filestore.ProcessData(0); err != nil {
		logging.LogError0("PullDescriptions", "Error in ProcessData", "error", err)
		runErr = err
	}

	existingPDPIds := filestore.LoadPDPProductIds()
	logging.LogInfo0("PullDescriptions", "loaded existing PDP responses", "count", len(existingPDPIds))

	// Listings known to be gone are skipped unless the caller explicitly asks to
	// retry them (e.g. after a re-post).
	unavailableIds := map[string]struct{}{}
	if s.RetryUnavailable {
		logging.LogInfo0("PullDescriptions", "retrying previously unavailable listings", "flag", "-retry-unavailable")
	} else {
		unavailableIds = filestore.LoadUnavailableProductIds()
	}
	logging.LogInfo0("PullDescriptions", "loaded unavailable listings", "count", len(unavailableIds))

	// Stop before launching the browser when every remaining listing with a null
	// description is already fetched or known to be deleted.
	pending, alreadyFetched, unavailableSkipped, err := filestore.PendingDescriptionCount(existingPDPIds, unavailableIds)
	if err != nil {
		return fmt.Errorf("scan pending descriptions: %w", err)
	}
	if pending == 0 {
		fmt.Printf("nothing to pull: %d already fetched, %d unavailable (deleted listings)\n", alreadyFetched, unavailableSkipped)
		logging.LogInfo0("PullDescriptions", "nothing to pull", "alreadyFetched", alreadyFetched, "unavailable", unavailableSkipped)
		return runErr
	}
	logging.LogInfo0("PullDescriptions", "pending descriptions", "pending", pending)

	ctx, err := s.Begin()
	if err != nil {
		return fmt.Errorf("begin pull descriptions: %w", err)
	}
	defer func() {
		s.waitForCapture()
		if err := ctx.Close(); err != nil {
			logging.LogError0("PullDescriptions", "could not close browser context", "error", err)
		}
	}()

	page, err := ctx.NewPage()
	if err != nil {
		return fmt.Errorf("create pull-description page: %w", err)
	}
	pgs, err := pages.NewPages(page)
	if err != nil {
		return fmt.Errorf("create page helpers: %w", err)
	}

	if err := pgs.MarketplaceHome(); err != nil {
		return fmt.Errorf("open marketplace home: %w", err)
	}

	fmt.Println("Opening a listing to capture the PDP request template...")
	template := s.capturePDPTemplate(pgs, page)
	if template == nil {
		return fmt.Errorf("did not capture the PDP request template: Facebook is likely throttling this session (error 1675004); wait and retry later")
	}

	pulled := 0
	skipped := 0
	unavailable := 0
	waitedSeconds := 0
	rateLimited := false
	_, err = filestore.ForEachDetail(func(filePath string, jsonData map[string]any) bool {
		description := jsonutil.GetKey(jsonData, "Description")
		// A present Description (including an empty string) means the PDP was already
		// fetched; only null/absent descriptions still need pulling.
		if description != nil {
			return true
		}

		productId := jsonutil.GetKey(jsonData, "ID")
		if productId == nil {
			return true
		}
		productIdStr, ok := productId.(string)
		if !ok {
			return true
		}

		// Already captured in a previous run: don't fetch (or write) again.
		if _, exists := existingPDPIds[productIdStr]; exists {
			skipped++
			return true
		}

		// Known to no longer exist: skip instead of failing every run.
		if _, exists := unavailableIds[productIdStr]; exists {
			unavailable++
			return true
		}

		outcome, waited, err := s.fetchPDPWithBackoff(page, template, productIdStr)
		waitedSeconds += waited
		if err != nil {
			logging.LogError0("PullDescriptions", "Error fetching PDP", "productId", productIdStr, "error", err)
			return true
		}
		if outcome.RateLimited {
			logging.LogWarn0("PullDescriptions", "rate limit did not clear, stopping", "pulled", pulled, "waitedSec", waitedSeconds)
			rateLimited = true
			return false
		}
		if outcome.Gone {
			if err := filestore.MarkUnavailable(productIdStr); err != nil {
				logging.LogError0("PullDescriptions", "could not mark unavailable", "productId", productIdStr, "error", err)
			}
			unavailableIds[productIdStr] = struct{}{}
			unavailable++
			logging.LogWarn0("PullDescriptions", "listing no longer exists, skipping future runs", "productId", productIdStr)
			return true
		}
		if outcome.NoProduct {
			if became, count := filestore.RecordPDPFailure(productIdStr, filestore.DefaultMissThreshold); became {
				unavailableIds[productIdStr] = struct{}{}
				unavailable++
				logging.LogWarn0("PullDescriptions", "no listing after repeated attempts, marking unavailable",
					"productId", productIdStr, "misses", count)
			} else {
				logging.LogWarn0("PullDescriptions", "PDP response had no listing, will retry",
					"productId", productIdStr, "misses", count, "threshold", filestore.DefaultMissThreshold)
			}
			return true
		}

		// Success: clear any failure state and remember the response.
		filestore.ClearPDPFailure(productIdStr)
		if s.RetryUnavailable {
			if err := filestore.ClearUnavailable(productIdStr); err != nil {
				logging.LogError0("PullDescriptions", "could not clear unavailable marker", "productId", productIdStr, "error", err)
			}
		}
		existingPDPIds[productIdStr] = struct{}{}
		pulled++
		if pulled%50 == 0 {
			logging.LogInfo0("PullDescriptions", "progress", "pulled", pulled)
		}
		if s.DelayMs > 0 {
			time.Sleep(time.Duration(s.DelayMs) * time.Millisecond)
		}
		return true
	}, false)
	if err != nil {
		logging.LogError0("PullDescriptions", "error iterating detail files", "error", err)
		runErr = err
	}

	logging.LogInfo0("PullDescriptions", "finished", "pulled", pulled, "skipped", skipped, "unavailable", unavailable, "rateLimited", rateLimited)
	if rateLimited {
		logging.LogWarn0("PullDescriptions", "stopped early on rate limit; progress is saved, re-run later to continue (optionally -delay-ms=N)")
	}

	s.waitForCapture()
	if _, err := filestore.ProcessData(0); err != nil {
		logging.LogError0("PullDescriptions", "error processing responses", "error", err)
		if runErr == nil {
			runErr = err
		}
	}

	return runErr
}

// fetchPDPWithBackoff fetches a PDP and, if Facebook rate-limits the request (error
// code 1675004), waits and retries the same product until the limit clears or the
// per-run waiting budget (MaxRateLimitWaitSeconds) is exhausted. This makes a
// transient throttle pause the run instead of aborting it, while still saving all
// progress if the limit does not clear. Returns (outcome, waitedSeconds, err).
func (s *ScrapperImpl) fetchPDPWithBackoff(page driver.PageWrapperInterface, template *capture.PDPTemplate, productId string) (capture.PDPOutcome, int, error) {
	waited := 0
	wait := s.RateLimitWaitSeconds
	for {
		outcome, err := capture.FetchPDP(page, template, productId)
		if err != nil {
			return outcome, waited, err
		}
		if !outcome.RateLimited {
			return outcome, waited, nil
		}

		if s.RateLimitWaitSeconds <= 0 {
			return outcome, waited, nil
		}
		if waited+wait > s.MaxRateLimitWaitSeconds {
			return outcome, waited, nil
		}

		// Exponential backoff (capped) so frequent retries do not keep the throttle
		// alive. A little jitter avoids retrying in lockstep.
		jitter := rand.Intn(wait/4 + 1)
		sleepFor := wait + jitter
		logging.LogWarn0("PullDescriptions", "rate limited, waiting before retry", "productId", productId, "waitSec", sleepFor, "waitedSec", waited)
		time.Sleep(time.Duration(sleepFor) * time.Second)
		waited += sleepFor

		wait *= 2
		if wait > 600 {
			wait = 600
		}
	}
}

// capturePDPTemplate clicks listings until the browser issues a
// MarketplacePDPContainerQuery, which provides the reusable request template. If one
// listing does not produce the request (e.g. Facebook throttles it and serves the
// error page), it returns to the Marketplace home and tries the next listing.
func (s *ScrapperImpl) capturePDPTemplate(pgs pages.PagesInterface, page driver.PageWrapperInterface) *capture.PDPTemplate {
	const maxAttempts = 6
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			if err := pgs.MarketplaceHome(); err != nil {
				logging.LogWarn0("PullDescriptions", "could not return to marketplace", "attempt", attempt, "error", err)
			}
			time.Sleep(2 * time.Second)
		}

		if !pgs.OpenProduct(attempt) {
			logging.LogWarn0("PullDescriptions", "could not open a listing for the template", "attempt", attempt)
			continue
		}

		if template := waitForPDPTemplate(15); template != nil {
			return template
		}

		if throttled, err := page.Evaluate(`document.body.innerText.includes("This page isn't available right now")`); err == nil {
			if isThrottled, ok := throttled.(bool); ok && isThrottled {
				logging.LogWarn0("PullDescriptions", "Facebook is serving its error page for this session (likely throttled, code 1675004)")
			}
		}
		logging.LogWarn0("PullDescriptions", "no PDP request captured, trying another listing", "attempt", attempt)
	}
	return nil
}

func waitForPDPTemplate(timeoutSeconds int) *capture.PDPTemplate {
	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	for time.Now().Before(deadline) {
		if template, ok := capture.GetPDPTemplate(); ok {
			return template
		}
		time.Sleep(250 * time.Millisecond)
	}
	return nil
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
