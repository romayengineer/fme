package app

import (
	"errors"

	"github.com/romayengineer/fme/go/browser/marketplace"
	"github.com/romayengineer/fme/go/store/filestore"
)

// FileDetailStore adapts the JSON detail store to the DetailStore port.
type FileDetailStore struct{}

// NewFileDetailStore returns a DetailStore backed by data/__Details.
func NewFileDetailStore() *FileDetailStore {
	return &FileDetailStore{}
}

// ProcessResponses materializes captured responses into data/__Details.
func (s *FileDetailStore) ProcessResponses(startTime int64) (int64, error) {
	return filestore.ProcessData(startTime)
}

// ForEachDetail calls process for every detail record.
func (s *FileDetailStore) ForEachDetail(process func(filePath string, jsonData map[string]any) bool, sortit bool) (int, error) {
	return filestore.ForEachDetail(process, sortit)
}

// LoadPDPProductIDs returns the ids that already have a PDP response.
func (s *FileDetailStore) LoadPDPProductIDs() map[string]struct{} {
	return filestore.LoadPDPProductIDs()
}

// PendingDescriptions counts records still missing a description.
func (s *FileDetailStore) PendingDescriptions(existing, unavailable map[string]struct{}) (pending, alreadyFetched, unavailableCount int, err error) {
	return filestore.PendingDescriptionCount(existing, unavailable)
}

// FillEmpty backfills missing URLs.
func (s *FileDetailStore) FillEmpty() error {
	return filestore.FillEmpty()
}

// DedupePDP removes duplicate PDP response files.
func (s *FileDetailStore) DedupePDP() {
	filestore.DedupePDP()
}

// DescriptionStats returns description coverage for the store.
func (s *FileDetailStore) DescriptionStats() (DescriptionStats, error) {
	stats, err := filestore.ComputeDescriptionStats()
	if err != nil {
		return DescriptionStats{}, err
	}
	return DescriptionStats{
		Total:           stats.Total,
		WithDescription: stats.WithDescription,
		Unavailable:     stats.Unavailable,
		Empty:           stats.Empty,
	}, nil
}

// SearchOptions configures a Marketplace search run.
type SearchOptions struct {
	Keywords      string
	ScrollDown    bool
	ScrollSeconds int
	Pause         func()
}

// Search opens a session, searches Marketplace, and materializes the captured
// responses into data/__Details. It returns the first operational error while
// still processing whatever responses were captured.
func (a *App) Search(opts SearchOptions) error {
	if a.Sessions == nil {
		return errors.New("search: session factory is not configured")
	}

	session, err := a.openAuthenticatedSession("search")
	if err != nil {
		return err
	}
	defer a.closeAuthenticatedSession("search", session)

	_, pageHelpers, err := newPageHelpers(session.Context, "search")
	if err != nil {
		return err
	}

	runErr := a.runSearch(pageHelpers, opts)
	runErr = a.finishCapture(session, "search", runErr)
	a.pause(opts.Pause)

	return runErr
}

// runSearch drives the Marketplace search, logging rather than returning the
// search error so captured responses are still materialized.
func (a *App) runSearch(pageHelpers marketplace.PagesInterface, opts SearchOptions) error {
	if err := pageHelpers.MarketplaceSearch(opts.Keywords, opts.ScrollDown, opts.ScrollSeconds); err != nil {
		a.Logger.Error("search: run marketplace search", "error", err)
		return err
	}
	return nil
}
