package app

import (
	"fmt"

	"github.com/romayengineer/fme/go/browser/pages"
	"github.com/romayengineer/fme/go/store/filestore"
)

// FileDetailStore adapts the JSON detail store to the DetailStore port.
type FileDetailStore struct{}

// NewFileDetailStore returns a DetailStore backed by data/__Details.
func NewFileDetailStore() *FileDetailStore {
	return &FileDetailStore{}
}

func (s *FileDetailStore) ProcessResponses(startTime int64) (int64, error) {
	return filestore.ProcessData(startTime)
}

func (s *FileDetailStore) ForEachDetail(process func(filePath string, jsonData map[string]any) bool, sortit bool) (int, error) {
	return filestore.ForEachDetail(process, sortit)
}

func (s *FileDetailStore) PendingDescriptions(existing, unavailable map[string]struct{}) (pending, alreadyFetched, unavailableCount int, err error) {
	return filestore.PendingDescriptionCount(existing, unavailable)
}

func (s *FileDetailStore) FillEmpty() error {
	return filestore.FillEmpty()
}

func (s *FileDetailStore) DedupePDP() {
	filestore.DedupePDP()
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
		return fmt.Errorf("search: session factory is not configured")
	}

	session, err := a.Sessions.Open()
	if err != nil {
		return fmt.Errorf("search: open session: %w", err)
	}
	defer func() {
		if session.Waiter != nil {
			session.Waiter.Wait()
		}
		if err := session.Context.Close(); err != nil {
			a.Logger.Error("search: close browser context", "error", err)
		}
	}()

	page, err := session.Context.NewPage()
	if err != nil {
		return fmt.Errorf("search: create page: %w", err)
	}

	pageHelpers, err := pages.NewPages(page)
	if err != nil {
		return fmt.Errorf("search: create page helpers: %w", err)
	}

	var runErr error
	if err := pageHelpers.MarketpaceSearch(opts.Keywords, opts.ScrollDown, opts.ScrollSeconds); err != nil {
		a.Logger.Error("search: run marketplace search", "error", err)
		runErr = err
	}

	// Wait for intercepted responses before materializing them.
	if session.Waiter != nil {
		session.Waiter.Wait()
	}

	if a.Details == nil {
		return fmt.Errorf("search: detail store is not configured")
	}
	if _, err := a.Details.ProcessResponses(0); err != nil {
		a.Logger.Error("search: process responses", "error", err)
		if runErr == nil {
			runErr = err
		}
	}

	if opts.Pause != nil {
		opts.Pause()
	}

	return runErr
}
