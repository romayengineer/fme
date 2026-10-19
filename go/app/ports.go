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
	Open() (*Session, error)
}

// DetailStore is the application view of the data/__Details store.
type DetailStore interface {
	ProcessResponses(startTime int64) (int64, error)
	ForEachDetail(process func(filePath string, jsonData map[string]any) bool, sortit bool) (int, error)
	LoadPDPProductIDs() map[string]struct{}
	PendingDescriptions(existing, unavailable map[string]struct{}) (pending, alreadyFetched, unavailableCount int, err error)
	FillEmpty() error
	DedupePDP()
	DescriptionStats() (DescriptionStats, error)
}

// PDPState tracks listings that are gone or repeatedly fail to return a product.
type PDPState interface {
	MarkUnavailable(productID string) error
	ClearUnavailable(productID string) error
	LoadUnavailableProductIDs() map[string]struct{}
	RecordPDPFailure(productID string, threshold int) (bool, int)
	ClearPDPFailure(productID string)
}

// ResponseSink persists intercepted responses and the products extracted from
// them. Capture depends on this interface instead of the store package.
type ResponseSink interface {
	SaveResponse(jsonDatas []any, friendlyName string) (int, error)
	SaveResponseByID(jsonDatas []any, friendlyName, id string) (int, error)
	MergeProducts(jsonDatas []any) int
	WriteDebugMiss(productID string, body []byte) error
}

// ProductRepository persists Marketplace listings in the database.
type ProductRepository interface {
	SaveProduct(product model.MarketplaceItemDetails) (string, error)
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
