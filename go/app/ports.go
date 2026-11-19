// Package app contains FME use cases and the ports they depend on.
//
// It is the composition layer between the CLI and the feature modules. The CLI
// parses flags and calls one use case; the use case coordinates browser,
// capture and storage implementations through the interfaces below.
package app

import (
	"log/slog"
	"time"
)

// Page is the page surface the description flow needs.
type Page interface {
	// Evaluate runs a JavaScript expression in the page.
	Evaluate(expression string, arg ...any) (any, error)
}

// Pages drives Marketplace navigation. Implementations live in the browser
// module; tests provide a fake.
type Pages interface {
	// MarketplaceSearch opens a Marketplace search for the keywords.
	MarketplaceSearch(searchKeywords string, scrollDown bool, scrollSeconds int) error
	// MarketplaceHome opens the Marketplace home page.
	MarketplaceHome() error
	// GoToProduct navigates directly to a listing URL.
	GoToProduct(id string) error
	// OpenProduct clicks the listing at index.
	OpenProduct(index int) bool
	// IsThrottled reports whether the last navigation hit Facebook's
	// throttling page.
	IsThrottled() bool
}

// PDPTemplate is an opaque request template captured from Facebook. The
// composition root supplies the concrete capture type.
type PDPTemplate any

// PDPOutcome reports the result of a PDP replay.
type PDPOutcome struct {
	RateLimited bool // Facebook rate-limited the request
	Gone        bool // the listing no longer exists (definitive)
	NoProduct   bool // fetched but no listing was found (ambiguous failure)
}

// PDPFetcher captures and replays Marketplace PDP requests.
type PDPFetcher interface {
	// Template waits up to timeoutSeconds for a captured PDP request.
	Template(timeoutSeconds int) (PDPTemplate, bool)
	// Fetch replays template for productID and returns the outcome.
	Fetch(page Page, template PDPTemplate, productID string) (PDPOutcome, error)
}

// DetailStore is the application view of the data/__Details store.
type DetailStore interface {
	// ProcessResponses materializes every captured response.
	ProcessResponses() error
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
	// ArchiveSearchResponses moves processed search responses older than
	// cutoff into data/archived/ and prunes archives older than pruneBefore.
	ArchiveSearchResponses(cutoff, pruneBefore time.Time) (archived, pruned int, err error)
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

// ProductRepository persists raw detail records in the database. The adapter
// owns the model mapping, so app does not depend on the storage model.
type ProductRepository interface {
	// SaveDetailRecords upserts a batch and returns how many were stored.
	SaveDetailRecords(records []map[string]any) (int, error)
}

// Deps is the set of implementations a use case needs. Construct it once in the
// CLI and pass the same value to all use cases.
type Deps struct {
	Sessions SessionFactory
	Details  DetailStore
	PDPState PDPState
	PDP      PDPFetcher
	Sink     ResponseSink
	Products ProductRepository
	Logger   *slog.Logger
}

// App holds the dependencies for the use cases.
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
