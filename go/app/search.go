package app

import "errors"

// SearchOptions configures a Marketplace search run.
type SearchOptions struct {
	Keywords      string
	ScrollDown    bool
	ScrollSeconds int
	Pause         func()
}

// SearchResult summarizes one Marketplace search run.
type SearchResult struct {
	// Throttled reports that Facebook served its error page for the search.
	Throttled bool
}

// Search opens a session, searches Marketplace, and materializes the captured
// responses into data/__Details. It returns the first operational error while
// still processing whatever responses were captured.
func (a *App) Search(opts SearchOptions) (SearchResult, error) {
	if a.Sessions == nil {
		return SearchResult{}, errors.New("search: session factory is not configured")
	}

	session, err := a.openAuthenticatedSession("search")
	if err != nil {
		return SearchResult{}, err
	}
	defer a.closeAuthenticatedSession("search", session)

	result := SearchResult{}
	runErr := a.runSearch(session.Pages, opts)
	if runErr == nil {
		result.Throttled = session.Pages.IsThrottled()
		a.logSearchThrottle(result)
	}

	runErr = a.finishCapture(session, "search", runErr)
	a.pause(opts.Pause)

	return result, runErr
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
