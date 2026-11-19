package app

import (
	"context"
	"errors"
)

// SearchOptions configures a Marketplace search run.
type SearchOptions struct {
	Keywords      string
	ScrollDown    bool
	ScrollSeconds int
	Pause         func()

	// InlineDescriptions pulls descriptions for discovered listings while the
	// search runs. Search falls back to search-only mode when the pipeline
	// dependencies are missing.
	InlineDescriptions bool
	// DescriptionQueueLimit caps how many listing ids may wait for a pull.
	// Zero uses DefaultDescriptionQueueLimit.
	DescriptionQueueLimit int
	// DelayMs is the pause between replayed PDP requests.
	DelayMs int
	// RateLimitWaitSeconds and MaxRateLimitWaitSeconds control PDP backoff.
	RateLimitWaitSeconds    int
	MaxRateLimitWaitSeconds int
}

// SearchResult summarizes one Marketplace search run.
type SearchResult struct {
	// Throttled reports that Facebook served its error page for the search.
	Throttled bool
	// DescriptionsPulled is how many descriptions the inline pipeline fetched.
	DescriptionsPulled int
	// DescriptionsPending is how many ids were left in the queue when the run
	// ended (normally zero, non-zero after a rate limit).
	DescriptionsPending int
	// DescriptionRateLimited reports that the inline pipeline stopped because
	// Facebook throttled the PDP request.
	DescriptionRateLimited bool
}

// Search runs a Marketplace search with a background context.
func (a *App) Search(opts SearchOptions) (SearchResult, error) {
	return a.SearchContext(context.Background(), opts)
}

// SearchContext opens a session, searches Marketplace, and materializes the
// captured responses into data/__Details. When enabled, it also pulls
// descriptions for newly discovered listings through a bounded queue while the
// search runs. It returns the first operational error while still processing
// whatever responses were captured.
func (a *App) SearchContext(ctx context.Context, opts SearchOptions) (SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return SearchResult{}, err
	}
	if a.Sessions == nil {
		return SearchResult{}, errors.New("search: session factory is not configured")
	}

	session, err := a.openAuthenticatedSession("search")
	if err != nil {
		return SearchResult{}, err
	}
	defer a.closeAuthenticatedSession("search", session)

	return a.runSessionSearch(ctx, session, opts)
}

// runSessionSearch runs the search and its description pipeline, then drains
// capture handlers and materializes any leftover responses.
func (a *App) runSessionSearch(ctx context.Context, session *Session, opts SearchOptions) (SearchResult, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	pipeline := a.startInlinePipeline(ctx, session, opts)
	result, runErr := a.searchWithPipeline(session, opts, pipeline)

	// Stop accepting capture handlers before the producer's final sweep.
	if session.Waiter != nil {
		session.Waiter.Wait()
	}
	if pipeline != nil {
		applyInlineResult(&result, pipeline.finish())
	}

	runErr = a.processResponses("search", runErr)
	a.pause(opts.Pause)
	return result, runErr
}

// searchWithPipeline installs the scroll gate, runs the search, and reports
// whether Facebook throttled it.
func (a *App) searchWithPipeline(session *Session, opts SearchOptions, pipeline *descriptionPipeline) (SearchResult, error) {
	if pipeline != nil {
		session.Pages.SetScrollGate(pipeline.gate)
		defer session.Pages.SetScrollGate(nil)
	}

	result := SearchResult{}
	runErr := a.runSearch(session.Pages, opts)
	if runErr == nil {
		result.Throttled = session.Pages.IsThrottled()
		a.logSearchThrottle(result)
	}
	return result, runErr
}

// applyInlineResult copies the pipeline counters into the search result.
func applyInlineResult(result *SearchResult, inline SearchResult) {
	result.DescriptionsPulled = inline.DescriptionsPulled
	result.DescriptionsPending = inline.DescriptionsPending
	result.DescriptionRateLimited = inline.DescriptionRateLimited
}

// startInlinePipeline starts the search+pull pipeline when it is enabled and
// its dependencies are available. A failure is logged and search continues.
func (a *App) startInlinePipeline(ctx context.Context, session *Session, opts SearchOptions) *descriptionPipeline {
	if !opts.InlineDescriptions {
		return nil
	}

	pipeline, err := a.startDescriptionPipeline(ctx, session, opts)
	if err != nil {
		a.Logger.Warn("search: inline descriptions disabled", "error", err)
		return nil
	}
	return pipeline
}

// logSearchThrottle reports a throttled search so unattended runs are visible.
func (a *App) logSearchThrottle(result SearchResult) {
	if result.Throttled {
		a.Logger.Warn("search: Facebook is serving its throttle page (likely error 1675004)")
	}
}

// runSearch drives the Marketplace search, logging rather than returning the
// search error so captured responses are still materialized.
func (a *App) runSearch(pages Pages, opts SearchOptions) error {
	if err := pages.MarketplaceSearch(opts.Keywords, opts.ScrollDown, opts.ScrollSeconds); err != nil {
		a.Logger.Error("search: run marketplace search", "error", err)
		return err
	}
	return nil
}
