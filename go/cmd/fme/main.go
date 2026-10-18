package main

import (
	"fmt"
	"math/rand"
	"os"
	"time"

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
	}
}

func (s *ScrapperImpl) Begin() (driver.ContextWrapperInterface, error) {
	config, err := NewConfig()
	if err != nil {
		return nil, fmt.Errorf("error NewConfig: %v", err)
	}

	playwrightWrapper, err := driver.NewPlaywrightWrapper()
	if err != nil {
		return nil, fmt.Errorf("error NewPlaywrightWrapper: %v", err)
	}

	var ctx driver.ContextWrapperInterface
	if config.UserDataDir != "" {
		ctx, err = playwrightWrapper.LaunchPersistentContext(config.UserDataDir, config.BrowserChannel, config.ProfileDirectory, false)
		if err != nil {
			return nil, fmt.Errorf("error LaunchPersistentContext: %v", err)
		}
		if err := facebook.EnsureLoggedIn(ctx); err != nil {
			return nil, fmt.Errorf("error EnsureLoggedIn: %v", err)
		}
	} else {
		browser, err := playwrightWrapper.NewBrowser(false)
		if err != nil {
			return nil, fmt.Errorf("error NewBrowser: %v", err)
		}

		facebookScrapper := facebook.NewFacebookScrapper(browser)

		ctx, err = facebookScrapper.Login(config.UserCredentials)
		if err != nil {
			return nil, fmt.Errorf("error Login: %v", err)
		}
	}

	capture.SetContextEventHandlers(ctx, s.BlockImages)

	return ctx, nil
}

func (s *ScrapperImpl) SearchProducts() {
	ctx, err := s.Begin()
	if err != nil {
		logging.LogError0("SearchProducts", "Error in Begin", "error", err)
		os.Exit(1)
	}
	defer ctx.Close()

	page, _ := ctx.NewPage()
	pgs, _ := pages.NewPages(page)
	if err := pgs.MarketpaceSearch(s.SearchKewords, s.ScrollDownOnSearch, s.ScrollSeconds); err != nil {
		logging.LogError0("SearchProducts", "Error in MarketpaceSearch", "error", err)
	}

	// Extract the listings from the search responses into data/__Details now, so the
	// search step materializes new products (with an empty Description) on its own.
	// Descriptions are still filled afterwards by pull_description.
	if _, err := filestore.ProcessData(0); err != nil {
		logging.LogError0("SearchProducts", "Error in ProcessData", "error", err)
	}

	facebook.WaitingForInput()
}

// PullDescriptions fetches the description (redacted_description) for every detail
// file that is missing one. It only needs to open a listing once, to capture the
// MarketplacePDPContainerQuery request template; the rest is replayed via ctx.Fetch.
// See docs/redacted_description.md.
func (s *ScrapperImpl) PullDescriptions() {
	ctx, err := s.Begin()
	if err != nil {
		logging.LogError0("PullDescriptions", "Error in Begin", "error", err)
		os.Exit(1)
	}
	defer ctx.Close()

	page, _ := ctx.NewPage()
	pgs, _ := pages.NewPages(page)

	// Materialize any as-yet-unprocessed search responses into data/__Details first,
	// so listings found by a recent `search` run are included in this fill pass.
	if _, err := filestore.ProcessData(0); err != nil {
		logging.LogError0("PullDescriptions", "Error in ProcessData", "error", err)
	}

	if err := pgs.MarketplaceHome(); err != nil {
		logging.LogError0("PullDescriptions", "Error opening marketplace", "error", err)
		return
	}

	fmt.Println("Opening a listing to capture the PDP request template...")
	template := s.capturePDPTemplate(pgs, page)
	if template == nil {
		logging.LogError0("PullDescriptions", "did not capture the PDP request template", "hint", "Facebook is likely throttling this session (error 1675004); wait and retry later")
		return
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

	pulled := 0
	skipped := 0
	unavailable := 0
	waitedSeconds := 0
	rateLimited := false
	filestore.ForEachDetail(func(filePath string, jsonData map[string]any) bool {
		description := jsonutil.GetKey(jsonData, "Description")
		if description != nil && description != "" {
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

	logging.LogInfo0("PullDescriptions", "finished", "pulled", pulled, "skipped", skipped, "unavailable", unavailable, "rateLimited", rateLimited)
	if rateLimited {
		logging.LogWarn0("PullDescriptions", "stopped early on rate limit; progress is saved, re-run later to continue (optionally -delay-ms=N)")
	}

	filestore.ProcessData(0)
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

		if throttled, _ := page.Evaluate(`document.body.innerText.includes("This page isn't available right now")`); throttled == true {
			logging.LogWarn0("PullDescriptions", "Facebook is serving its error page for this session (likely throttled, code 1675004)")
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
func (s *ScrapperImpl) InstallDrivers() {
	fmt.Println("Installing Playwright driver and Chromium (this may take a while)...")

	wrapper, err := driver.NewPlaywrightWrapper()
	if err != nil {
		logging.LogFatal(fmt.Sprintf("could not install playwright drivers: %v", err))
	}

	if err := wrapper.Stop(); err != nil {
		logging.LogError0("InstallDrivers", "could not stop playwright", "error", err)
	}

	logging.LogInfo0("InstallDrivers", "playwright driver and chromium are installed")
}

func (s *ScrapperImpl) GetDetails() {
	ctx, err := s.Begin()
	if err != nil {
		logging.LogError0("GetDetails", "Error in Begin", "error", err)
		os.Exit(1)
	}
	defer ctx.Close()

	page, _ := ctx.NewPage()
	pgs, _ := pages.NewPages(page)

	filestore.ForEachDetail(func(filePath string, jsonData map[string]any) bool {
		PriceAmount := jsonutil.GetKey(jsonData, "PriceAmount")
		if PriceAmount != nil {
			return true
		}

		productId := jsonutil.GetKey(jsonData, "ID")
		if productId == nil {
			return true
		}

		pgs.GoToProduct(productId.(string))
		time.Sleep(3 * time.Second)

		return true
	}, false)

	facebook.WaitingForInput()
}

func main() {
	flags := NewFlags()
	logging.LogInfo0("main", "flags", "action", flags.action)

	if flags.listMaps {
		printMaps()
		return
	}

	scrapper := NewScrapper(flags)

	switch flags.action {
	case "search":
		scrapper.SearchProducts()
	case "install_drivers":
		scrapper.InstallDrivers()
	case "pull_description":
		scrapper.PullDescriptions()
	case "get_details":
		scrapper.GetDetails()
	case "process_data":
		filestore.ProcessData(0)
	case "dedupe_pdp":
		filestore.DedupePDP()
	case "serve":
		db.Serve()
	case "save":
		_, err := db.ProcessDataInDB()
		if err != nil {
			logging.LogFatal(err)
		}
	case "fill_empty":
		filestore.FillEmpty()
	case "description_stats":
		filestore.PrintDescriptionStats(filestore.ComputeDescriptionStats())
	case "heatmap":
		runHeatmap(flags)
	case "heatmap_serve":
		runHeatmapServe(flags)
	default:
		logging.LogError0("main", "Unknown action", "action", flags.action)
		os.Exit(1)
	}
}
