package app

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/romayengineer/fme/go/browser/driver"
	"github.com/romayengineer/fme/go/browser/pages"
	"github.com/romayengineer/fme/go/capture"
	"github.com/romayengineer/fme/go/core/jsonutil"
	"github.com/romayengineer/fme/go/store/pdpstate"
)

// PullDescriptionsOptions configures the description fill pass.
type PullDescriptionsOptions struct {
	RetryUnavailable        bool
	DelayMs                 int
	RateLimitWaitSeconds    int
	MaxRateLimitWaitSeconds int
}

// PullDescriptions fetches redacted_description for every detail record whose
// Description is null. It opens one listing to capture the PDP request
// template, then replays that request for the remaining listings.
func (a *App) PullDescriptions(opts PullDescriptionsOptions) error {
	if a.Details == nil {
		return fmt.Errorf("pull-description: detail store is not configured")
	}
	if a.Sessions == nil {
		return fmt.Errorf("pull-description: session factory is not configured")
	}
	if a.Sink == nil {
		return fmt.Errorf("pull-description: response sink is not configured")
	}

	var runErr error

	// Materialize unprocessed search responses first; no browser is needed.
	if _, err := a.Details.ProcessResponses(0); err != nil {
		a.Logger.Error("pull-description: process responses", "error", err)
		runErr = err
	}

	existingPDPIds := a.Details.LoadPDPProductIDs()
	a.Logger.Info("pull-description: loaded existing PDP responses", "count", len(existingPDPIds))

	unavailableIds := map[string]struct{}{}
	if opts.RetryUnavailable {
		a.Logger.Info("pull-description: retrying previously unavailable listings")
	} else if a.PDPState != nil {
		unavailableIds = a.PDPState.LoadUnavailableProductIDs()
	}
	a.Logger.Info("pull-description: loaded unavailable listings", "count", len(unavailableIds))

	pending, alreadyFetched, unavailableSkipped, err := a.Details.PendingDescriptions(existingPDPIds, unavailableIds)
	if err != nil {
		return fmt.Errorf("pull-description: scan pending descriptions: %w", err)
	}
	if pending == 0 {
		fmt.Printf("nothing to pull: %d already fetched, %d unavailable (deleted listings)\n", alreadyFetched, unavailableSkipped)
		a.Logger.Info("pull-description: nothing to pull", "alreadyFetched", alreadyFetched, "unavailable", unavailableSkipped)
		return runErr
	}
	a.Logger.Info("pull-description: pending descriptions", "pending", pending)

	session, err := a.Sessions.Open()
	if err != nil {
		return fmt.Errorf("pull-description: open session: %w", err)
	}
	defer func() {
		if session.Waiter != nil {
			session.Waiter.Wait()
		}
		if err := session.Context.Close(); err != nil {
			a.Logger.Error("pull-description: close browser context", "error", err)
		}
	}()

	page, err := session.Context.NewPage()
	if err != nil {
		return fmt.Errorf("pull-description: create page: %w", err)
	}
	pageHelpers, err := pages.NewPages(page)
	if err != nil {
		return fmt.Errorf("pull-description: create page helpers: %w", err)
	}

	if err := pageHelpers.MarketplaceHome(); err != nil {
		return fmt.Errorf("pull-description: open marketplace home: %w", err)
	}

	fmt.Println("Opening a listing to capture the PDP request template...")
	template := a.capturePDPTemplate(pageHelpers, page)
	if template == nil {
		return fmt.Errorf("pull-description: did not capture the PDP request template: Facebook is likely throttling this session (error 1675004); wait and retry later")
	}

	pulled := 0
	skipped := 0
	unavailable := 0
	waitedSeconds := 0
	rateLimited := false

	_, err = a.Details.ForEachDetail(func(filePath string, jsonData map[string]any) bool {
		description := jsonutil.GetKey(jsonData, "Description")
		// A present Description (including an empty string) means the PDP was already
		// fetched; only null/absent descriptions still need pulling.
		if description != nil {
			return true
		}

		productId, ok := jsonutil.GetKey(jsonData, "ID").(string)
		if !ok || productId == "" {
			return true
		}

		if _, exists := existingPDPIds[productId]; exists {
			skipped++
			return true
		}

		if _, exists := unavailableIds[productId]; exists {
			unavailable++
			return true
		}

		outcome, waited, err := a.fetchPDPWithBackoff(page, template, productId, opts)
		waitedSeconds += waited
		if err != nil {
			a.Logger.Error("pull-description: fetch PDP", "productId", productId, "error", err)
			return true
		}
		if outcome.RateLimited {
			a.Logger.Warn("pull-description: rate limit did not clear, stopping", "pulled", pulled, "waitedSec", waitedSeconds)
			rateLimited = true
			return false
		}
		if outcome.Gone {
			if a.PDPState != nil {
				if err := a.PDPState.MarkUnavailable(productId); err != nil {
					a.Logger.Error("pull-description: mark unavailable", "productId", productId, "error", err)
				}
			}
			unavailableIds[productId] = struct{}{}
			unavailable++
			a.Logger.Warn("pull-description: listing no longer exists, skipping future runs", "productId", productId)
			return true
		}
		if outcome.NoProduct {
			if a.PDPState == nil {
				return true
			}
			if became, count := a.PDPState.RecordPDPFailure(productId, pdpstate.DefaultMissThreshold); became {
				unavailableIds[productId] = struct{}{}
				unavailable++
				a.Logger.Warn("pull-description: no listing after repeated attempts, marking unavailable",
					"productId", productId, "misses", count)
			} else {
				a.Logger.Warn("pull-description: PDP response had no listing, will retry",
					"productId", productId, "misses", count, "threshold", pdpstate.DefaultMissThreshold)
			}
			return true
		}

		// Success: clear any failure state and remember the response.
		if a.PDPState != nil {
			a.PDPState.ClearPDPFailure(productId)
			if opts.RetryUnavailable {
				if err := a.PDPState.ClearUnavailable(productId); err != nil {
					a.Logger.Error("pull-description: clear unavailable marker", "productId", productId, "error", err)
				}
			}
		}
		existingPDPIds[productId] = struct{}{}
		pulled++
		if pulled%50 == 0 {
			a.Logger.Info("pull-description: progress", "pulled", pulled)
		}
		if opts.DelayMs > 0 {
			time.Sleep(time.Duration(opts.DelayMs) * time.Millisecond)
		}
		return true
	}, false)
	if err != nil {
		a.Logger.Error("pull-description: iterate detail files", "error", err)
		runErr = err
	}

	a.Logger.Info("pull-description: finished", "pulled", pulled, "skipped", skipped, "unavailable", unavailable, "rateLimited", rateLimited)
	if rateLimited {
		a.Logger.Warn("pull-description: stopped early on rate limit; progress is saved, re-run later to continue (optionally -delay-ms=N)")
	}

	if session.Waiter != nil {
		session.Waiter.Wait()
	}
	if _, err := a.Details.ProcessResponses(0); err != nil {
		a.Logger.Error("pull-description: process responses", "error", err)
		if runErr == nil {
			runErr = err
		}
	}

	return runErr
}

// fetchPDPWithBackoff fetches a PDP and, if Facebook rate-limits the request
// (error code 1675004), waits and retries until the limit clears or the per-run
// waiting budget is exhausted.
func (a *App) fetchPDPWithBackoff(page driver.PageWrapperInterface, template *capture.PDPTemplate, productId string, opts PullDescriptionsOptions) (capture.PDPOutcome, int, error) {
	waited := 0
	wait := opts.RateLimitWaitSeconds
	for {
		outcome, err := capture.FetchPDP(page, template, productId, a.Sink)
		if err != nil {
			return outcome, waited, err
		}
		if !outcome.RateLimited {
			return outcome, waited, nil
		}

		if opts.RateLimitWaitSeconds <= 0 {
			return outcome, waited, nil
		}
		if waited+wait > opts.MaxRateLimitWaitSeconds {
			return outcome, waited, nil
		}

		// Exponential backoff with jitter so frequent retries do not keep the
		// throttle alive.
		jitter := rand.Intn(wait/4 + 1)
		sleepFor := wait + jitter
		a.Logger.Warn("pull-description: rate limited, waiting before retry", "productId", productId, "waitSec", sleepFor, "waitedSec", waited)
		time.Sleep(time.Duration(sleepFor) * time.Second)
		waited += sleepFor

		wait *= 2
		if wait > 600 {
			wait = 600
		}
	}
}

// capturePDPTemplate clicks listings until the browser issues a
// MarketplacePDPContainerQuery, which provides the reusable request template.
func (a *App) capturePDPTemplate(pageHelpers pages.PagesInterface, page driver.PageWrapperInterface) *capture.PDPTemplate {
	const maxAttempts = 6
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			if err := pageHelpers.MarketplaceHome(); err != nil {
				a.Logger.Warn("pull-description: could not return to marketplace", "attempt", attempt, "error", err)
			}
			time.Sleep(2 * time.Second)
		}

		if !pageHelpers.OpenProduct(attempt) {
			a.Logger.Warn("pull-description: could not open a listing for the template", "attempt", attempt)
			continue
		}

		if template := waitForPDPTemplate(15); template != nil {
			return template
		}

		if throttled, err := page.Evaluate(`document.body.innerText.includes("This page isn't available right now")`); err == nil {
			if isThrottled, ok := throttled.(bool); ok && isThrottled {
				a.Logger.Warn("pull-description: Facebook is serving its error page for this session (likely throttled, code 1675004)")
			}
		}
		a.Logger.Warn("pull-description: no PDP request captured, trying another listing", "attempt", attempt)
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
