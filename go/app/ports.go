// Package app contains FME use cases and the ports they depend on.
//
// It is the composition layer between the CLI and the feature modules. The CLI
// parses flags and calls one use case; the use case coordinates browser,
// capture and storage implementations through the interfaces below.
package app

import (
	"log/slog"

	"github.com/romayengineer/fme/go/marketplace/model"
)

// SessionFactory opens an authenticated browser context. Implementations live
// in the browser module; tests provide a fake context.
type SessionFactory interface {
	// Open returns an authenticated browser session.
	Open() (*Session, error)
}

// DetailStore is the application view of the data/__Details store.
type DetailStore interface {
	// ProcessResponses materializes captured responses newer than startTime.
	ProcessResponses(startTime int64) (int64, error)
	// ForEachDetail calls process for every detail record.
	ForEachDetail(process func(filePath string, jsonData map[string]any) bool, sortit bool) (int, error)
	// LoadPDPProductIDs returns the ids that already have a PDP response.
	LoadPDPProductIDs() map[string]struct{}
	// PendingDescriptions counts records still missing a description.
	PendingDescriptions(existing, unavailable map[string]struct{}) (pending, alreadyFetched, unavailableCount int, err error)
	// FillEmpty backfills missing URLs.
	FillEmpty() error
	// DedupePDP removes duplicate PDP response files.
	DedupePDP()
	// DescriptionStats returns description coverage for the store.
	DescriptionStats() (DescriptionStats, error)
}

// PDPState tracks listings that are gone or repeatedly fail to return a product.
type PDPState interface {
	// MarkUnavailable tombstones a listing as no longer available.
	MarkUnavailable(productID string) error
	// ClearUnavailable removes a listing's tombstone.
	ClearUnavailable(productID string) error
	// LoadUnavailableProductIDs returns the tombstoned product ids.
	LoadUnavailableProductIDs() map[string]struct{}
	// RecordPDPFailure counts an ambiguous failure and reports whether the
	// listing just became unavailable, plus the current miss count.
	RecordPDPFailure(productID string, threshold int) (bool, int)
	// ClearPDPFailure resets a listing's miss counter.
	ClearPDPFailure(productID string)
}

// ResponseSink persists intercepted responses and the products extracted from
// them. Capture depends on this interface instead of the store package.
type ResponseSink interface {
	// SaveResponse stores the response payloads under a generated filename.
	SaveResponse(jsonDatas []any, friendlyName string) (int, error)
	// SaveResponseByID stores the response payloads under an id-keyed filename.
	SaveResponseByID(jsonDatas []any, friendlyName, id string) (int, error)
	// MergeProducts merges the payloads' products into their detail records.
	MergeProducts(jsonDatas []any) int
	// WriteDebugMiss saves a raw non-product payload for later inspection.
	WriteDebugMiss(productID string, body []byte) error
}

// ProductRepository persists Marketplace listings in the database.
type ProductRepository interface {
	// SaveProduct upserts one product and returns its row id.
	SaveProduct(product model.MarketplaceItemDetails) (string, error)
	// SaveProducts upserts a batch and returns how many were stored.
	SaveProducts(products []model.MarketplaceItemDetails) (int, error)
}

// Deps is the set of implementations a use case needs. Construct it once in the
// CLI and pass the same value to all use cases.
type Deps struct {
	Sessions SessionFactory
	Details  DetailStore
	PDPState PDPState
	Sink     ResponseSink
	Products ProductRepository
	Logger   *slog.Logger
}

// App holds the dependencies for the use cases. Use cases are added in the
// following Phase 1 tickets.
type App struct {
	Deps
}

// New returns an App with a default logger when none is supplied.
func New(deps Deps) *App {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	return &App{Deps: deps}
}
