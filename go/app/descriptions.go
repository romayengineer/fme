package app

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/romayengineer/fme/go/core/jsonutil"
)

// defaultMissThreshold is how many ambiguous "no product" PDP fetches a listing
// may accumulate before the pass writes it off as no-longer-available.
const defaultMissThreshold = 3

// PullDescriptionsOptions configures the description fill pass.
type PullDescriptionsOptions struct {
	RetryUnavailable        bool
	DelayMs                 int
	RateLimitWaitSeconds    int
	MaxRateLimitWaitSeconds int
}

// PullResult summarizes one description pass.
type PullResult struct {
	// Pending is how many listings were missing a description at the start.
	Pending int
	// Pulled is how many descriptions were fetched in this pass.
	Pulled int
	// Skipped counts listings that already had a PDP response.
	Skipped int
	// Unavailable counts tombstoned listings.
	Unavailable int
	// WaitedSeconds is the total time spent waiting on rate limits.
	WaitedSeconds int
	// RateLimited reports that the pass stopped because Facebook throttled it.
	RateLimited bool
}

// pullPlan is the pre-browser snapshot of which listings still need work.
type pullPlan struct {
	existing    map[string]struct{}
	unavailable map[string]struct{}
	pending     int
}

// pullState accumulates the counters and lookups for one description pass.
type pullState struct {
	app              *App
	opts             PullDescriptionsOptions
	existing         map[string]struct{}
	unavailable      map[string]struct{}
	pulled           int
	skipped          int
	unavailableCount int
	waitedSeconds    int
	rateLimited      bool
}

// PullDescriptions fetches redacted_description for every detail record whose
// Description is null. It opens one listing to capture the PDP request
// template, then replays that request for the remaining listings.
func (a *App) PullDescriptions(opts PullDescriptionsOptions) (PullResult, error) {
	if err := a.validatePullConfig(); err != nil {
		return PullResult{}, err
	}

	runErr := a.processResponses("pull-description", nil)

	plan, done, err := a.planPull(opts)
	if err != nil {
		return PullResult{}, err
	}
	if done {
		return PullResult{}, runErr
	}

	session, err := a.startPullSession()
	if err != nil {
		return PullResult{}, err
	}
	defer a.closeAuthenticatedSession("pull-description", session)

	state := newPullState(a, opts, plan.existing, plan.unavailable)
	runErr = a.pullAll(state, session.Page, session.Pages, runErr)
	a.logPullSummary(state)

	return state.result(plan.pending), a.finishCapture(session, "pull-description", runErr)
}

// validatePullConfig checks the dependencies the description pass needs.
func (a *App) validatePullConfig() error {
	if a.Details == nil {
		return errors.New("pull-description: detail store is not configured")
	}
	if a.Sessions == nil {
		return errors.New("pull-description: session factory is not configured")
	}
	if a.Sink == nil {
		return errors.New("pull-description: response sink is not configured")
	}
	return nil
}

// planPull reports which listings still need a description.
func (a *App) planPull(opts PullDescriptionsOptions) (*pullPlan, bool, error) {
	existing, unavailable := a.loadPullMaps(opts)
	a.Logger.Info("pull-description: loaded existing PDP responses", "count", len(existing))

	pending, alreadyFetched, unavailableSkipped, err := a.Details.PendingDescriptions(existing, unavailable)
	if err != nil {
		return nil, false, fmt.Errorf("pull-description: scan pending descriptions: %w", err)
	}
	if pending == 0 {
		fmt.Printf("nothing to pull: %d already fetched, %d unavailable (deleted listings)\n", alreadyFetched, unavailableSkipped)
		a.Logger.Info("pull-description: nothing to pull", "alreadyFetched", alreadyFetched, "unavailable", unavailableSkipped)
		return nil, true, nil
	}

	a.Logger.Info("pull-description: pending descriptions", "pending", pending)
	return &pullPlan{existing: existing, unavailable: unavailable, pending: pending}, false, nil
}

// loadPullMaps returns the ids that already have a PDP response and the ones
// tombstoned as unavailable.
func (a *App) loadPullMaps(opts PullDescriptionsOptions) (existing, unavailable map[string]struct{}) {
	existing = a.Details.LoadPDPProductIDs()
	unavailable = a.loadUnavailable(opts)
	return existing, unavailable
}

// newPullState builds the counters shared by a full description pass and the
// inline worker started by Search.
func newPullState(a *App, opts PullDescriptionsOptions, existing, unavailable map[string]struct{}) *pullState {
	if existing == nil {
		existing = map[string]struct{}{}
	}
	if unavailable == nil {
		unavailable = map[string]struct{}{}
	}
	return &pullState{
		app:         a,
		opts:        opts,
		existing:    existing,
		unavailable: unavailable,
	}
}

// loadUnavailable returns the tombstoned ids, unless a retry was requested.
func (a *App) loadUnavailable(opts PullDescriptionsOptions) map[string]struct{} {
	if opts.RetryUnavailable {
		a.Logger.Info("pull-description: retrying previously unavailable listings")
		return map[string]struct{}{}
	}
	if a.PDPState != nil {
		return a.PDPState.LoadUnavailableProductIDs()
	}
	return map[string]struct{}{}
}

// startPullSession opens the session and its page surfaces.
func (a *App) startPullSession() (*Session, error) {
	session, err := a.openAuthenticatedSession("pull-description")
	if err != nil {
		return nil, err
	}

	if err := session.Pages.MarketplaceHome(); err != nil {
		a.closeAuthenticatedSession("pull-description", session)
		return nil, fmt.Errorf("pull-description: open marketplace home: %w", err)
	}

	return session, nil
}

// pullAll captures the PDP template and replays it for every pending listing.
func (a *App) pullAll(state *pullState, page Page, pages Pages, runErr error) error {
	fmt.Println("Opening a listing to capture the PDP request template...")
	template := a.capturePDPTemplate(pages, page)
	if template == nil {
		return errors.New("pull-description: did not capture the PDP request template: Facebook is likely throttling this session (error 1675004); wait and retry later")
	}

	if _, err := a.Details.ForEachDetail(state.handleDetail(page, template), false); err != nil {
		a.Logger.Error("pull-description: iterate detail files", "error", err)
		return firstError(runErr, err)
	}
	return runErr
}

// handleDetail returns the ForEachDetail callback for the pass.
func (s *pullState) handleDetail(page Page, template PDPTemplate) func(string, map[string]any) bool {
	return func(_ string, jsonData map[string]any) bool {
		return s.processDetail(page, template, jsonData)
	}
}

// processDetail handles one detail record.
func (s *pullState) processDetail(page Page, template PDPTemplate, jsonData map[string]any) bool {
	productID, ok := s.pendingProductID(jsonData)
	if !ok {
		return true
	}
	if s.alreadyHandled(productID) {
		return true
	}
	return s.pullOne(page, template, productID)
}

// pendingProductID returns the listing id when the record still needs a
// description. A present Description (including an empty string) counts as done.
func (s *pullState) pendingProductID(jsonData map[string]any) (string, bool) {
	if jsonutil.GetKey(jsonData, "Description") != nil {
		return "", false
	}

	productID, ok := jsonutil.GetKey(jsonData, "ID").(string)
	return productID, ok && productID != ""
}

// alreadyHandled records a listing that is already fetched or unavailable.
func (s *pullState) alreadyHandled(productID string) bool {
	if _, exists := s.existing[productID]; exists {
		s.skipped++
		return true
	}
	if _, exists := s.unavailable[productID]; exists {
		s.unavailableCount++
		return true
	}
	return false
}

// pullOne replays the PDP request for one listing and records the outcome.
func (s *pullState) pullOne(page Page, template PDPTemplate, productID string) bool {
	outcome, waited, err := s.app.fetchPDPWithBackoff(page, template, productID, s.opts)
	s.waitedSeconds += waited

	if err != nil {
		s.app.Logger.Error("pull-description: fetch PDP", "productID", productID, "error", err)
		return true
	}
	if outcome.RateLimited {
		s.app.Logger.Warn("pull-description: rate limit did not clear, stopping", "pulled", s.pulled, "waitedSec", s.waitedSeconds)
		s.rateLimited = true
		return false
	}
	if outcome.Gone {
		s.handleGone(productID)
		return true
	}
	if outcome.NoProduct {
		s.handleNoProduct(productID)
		return true
	}

	s.handleSuccess(productID)
	return true
}

// handleGone tombstones a listing that Facebook reports as removed.
func (s *pullState) handleGone(productID string) {
	if s.app.PDPState != nil {
		if err := s.app.PDPState.MarkUnavailable(productID); err != nil {
			s.app.Logger.Error("pull-description: mark unavailable", "productID", productID, "error", err)
		}
	}

	s.unavailable[productID] = struct{}{}
	s.unavailableCount++
	s.app.Logger.Warn("pull-description: listing no longer exists, skipping future runs", "productID", productID)
}

// handleNoProduct counts an ambiguous failure and tombstones repeated ones.
func (s *pullState) handleNoProduct(productID string) {
	if s.app.PDPState == nil {
		return
	}

	became, count := s.app.PDPState.RecordPDPFailure(productID, defaultMissThreshold)
	if became {
		s.unavailable[productID] = struct{}{}
		s.unavailableCount++
		s.app.Logger.Warn("pull-description: no listing after repeated attempts, marking unavailable",
			"productID", productID, "misses", count)
		return
	}

	s.app.Logger.Warn("pull-description: PDP response had no listing, will retry",
		"productID", productID, "misses", count, "threshold", defaultMissThreshold)
}

// handleSuccess clears failure state and records the new response.
func (s *pullState) handleSuccess(productID string) {
	s.clearFailureState(productID)

	s.existing[productID] = struct{}{}
	s.pulled++
	if s.pulled%50 == 0 {
		s.app.Logger.Info("pull-description: progress", "pulled", s.pulled)
	}
	if s.opts.DelayMs > 0 {
		time.Sleep(time.Duration(s.opts.DelayMs) * time.Millisecond)
	}
}

// clearFailureState resets the failure bookkeeping after a successful fetch.
func (s *pullState) clearFailureState(productID string) {
	if s.app.PDPState == nil {
		return
	}

	s.app.PDPState.ClearPDPFailure(productID)
	if s.opts.RetryUnavailable {
		if err := s.app.PDPState.ClearUnavailable(productID); err != nil {
			s.app.Logger.Error("pull-description: clear unavailable marker", "productID", productID, "error", err)
		}
	}
}

// result snapshots the pass counters for the caller.
func (s *pullState) result(pending int) PullResult {
	return PullResult{
		Pending:       pending,
		Pulled:        s.pulled,
		Skipped:       s.skipped,
		Unavailable:   s.unavailableCount,
		WaitedSeconds: s.waitedSeconds,
		RateLimited:   s.rateLimited,
	}
}

// logPullSummary reports the pass counters.
func (a *App) logPullSummary(state *pullState) {
	a.Logger.Info("pull-description: finished",
		"pulled", state.pulled,
		"skipped", state.skipped,
		"unavailable", state.unavailableCount,
		"rateLimited", state.rateLimited)

	if state.rateLimited {
		a.Logger.Warn("pull-description: stopped early on rate limit; progress is saved, re-run later to continue (optionally -delay-ms=N)")
	}
}

// fetchPDPWithBackoff fetches a PDP and, if Facebook rate-limits the request
// (error code 1675004), waits and retries until the limit clears or the per-run
// waiting budget is exhausted.
func (a *App) fetchPDPWithBackoff(page Page, template PDPTemplate, productID string, opts PullDescriptionsOptions) (PDPOutcome, int, error) {
	if a.PDP == nil {
		return PDPOutcome{}, 0, errors.New("pull-description: PDP fetcher is not configured")
	}

	return a.fetchWithRetry(page, template, productID, opts)
}

// fetchWithRetry replays the PDP until it succeeds or the wait budget is spent.
func (a *App) fetchWithRetry(page Page, template PDPTemplate, productID string, opts PullDescriptionsOptions) (PDPOutcome, int, error) {
	waited := 0
	wait := opts.RateLimitWaitSeconds
	for {
		outcome, err := a.PDP.Fetch(page, template, productID)
		if err != nil {
			return outcome, waited, err
		}
		if !outcome.RateLimited {
			return outcome, waited, nil
		}
		if !canWait(opts, waited, wait) {
			return outcome, waited, nil
		}

		waited += a.sleepBackoff(productID, wait, waited)
		wait = nextWait(wait)
	}
}

// canWait reports whether the per-run rate-limit budget allows another wait.
func canWait(opts PullDescriptionsOptions, waited, wait int) bool {
	return opts.RateLimitWaitSeconds > 0 && waited+wait <= opts.MaxRateLimitWaitSeconds
}

// sleepBackoff sleeps the exponential backoff with jitter and returns the sleep
// duration so the caller can track its budget.
func (a *App) sleepBackoff(productID string, wait, waited int) int {
	jitter := rand.IntN(wait/4 + 1) //nolint:gosec // retry jitter does not need cryptographic randomness.
	sleepFor := wait + jitter

	a.Logger.Warn("pull-description: rate limited, waiting before retry", "productID", productID, "waitSec", sleepFor, "waitedSec", waited)
	time.Sleep(time.Duration(sleepFor) * time.Second)
	return sleepFor
}

// nextWait doubles the backoff, capped at ten minutes.
func nextWait(wait int) int {
	wait *= 2
	if wait > 600 {
		return 600
	}
	return wait
}

// pdpPage is the narrow page surface needed to capture a PDP request template.
type pdpPage interface {
	MarketplaceHome() error
	OpenProduct(index int) bool
}

// capturePDPTemplate clicks listings until the browser issues a
// MarketplacePDPContainerQuery, which provides the reusable request template.
func (a *App) capturePDPTemplate(pages pdpPage, page Page) PDPTemplate {
	if a.PDP == nil {
		return nil
	}

	const maxAttempts = 6
	for attempt := range maxAttempts {
		if template, ok := a.tryTemplate(pages, page, attempt); ok {
			return template
		}
	}
	return nil
}

// tryTemplate opens one listing and waits for its PDP request.
func (a *App) tryTemplate(pages pdpPage, page Page, attempt int) (PDPTemplate, bool) {
	if attempt > 0 {
		a.returnToMarketplace(pages, attempt)
	}

	if !pages.OpenProduct(attempt) {
		a.Logger.Warn("pull-description: could not open a listing for the template", "attempt", attempt)
		return nil, false
	}
	if template, ok := a.PDP.Template(15); ok {
		return template, true
	}

	a.logThrottleHint(page)
	a.Logger.Warn("pull-description: no template captured, trying another listing", "attempt", attempt)
	return nil, false
}

// returnToMarketplace reopens the Marketplace home page between attempts.
func (a *App) returnToMarketplace(pages pdpPage, attempt int) {
	if err := pages.MarketplaceHome(); err != nil {
		a.Logger.Warn("pull-description: could not return to marketplace", "attempt", attempt, "error", err)
	}
	time.Sleep(2 * time.Second)
}

// logThrottleHint reports Facebook's error page, a sign of throttling.
func (a *App) logThrottleHint(page Page) {
	throttled, err := page.Evaluate(`document.body.innerText.includes("This page isn't available right now")`)
	if err != nil {
		return
	}
	if isThrottled, ok := throttled.(bool); ok && isThrottled {
		a.Logger.Warn("pull-description: Facebook is serving its error page for this session (likely throttled, code 1675004)")
	}
}
