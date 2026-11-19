package main

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/romayengineer/fme/go/app"
	"github.com/romayengineer/fme/go/browser/driver"
	"github.com/romayengineer/fme/go/core/logging"
	"github.com/romayengineer/fme/go/marketplace/model"
	"github.com/romayengineer/fme/go/store/db"
	"github.com/romayengineer/fme/go/store/filestore"
	"github.com/romayengineer/fme/go/store/pdpstate"
)

// fileDetailStore adapts the JSON detail store to app.DetailStore.
type fileDetailStore struct{}

func newFileDetailStore() *fileDetailStore { return &fileDetailStore{} }

func (s *fileDetailStore) ProcessResponses(startTime int64) (int64, error) {
	return filestore.ProcessData(startTime)
}

func (s *fileDetailStore) ForEachDetail(process func(filePath string, jsonData map[string]any) bool, sortit bool) (int, error) {
	return filestore.ForEachDetail(process, sortit)
}

func (s *fileDetailStore) LoadPDPProductIDs() map[string]struct{} {
	return filestore.LoadPDPProductIDs()
}

func (s *fileDetailStore) PendingDescriptions(existing, unavailable map[string]struct{}) (pending, alreadyFetched, unavailableCount int, err error) {
	return filestore.PendingDescriptionCount(existing, unavailable)
}

func (s *fileDetailStore) FillEmpty() error { return filestore.FillEmpty() }

func (s *fileDetailStore) DedupePDP() { filestore.DedupePDP() }

// ArchiveSearchResponses moves processed search responses into data/archived/.
func (s *fileDetailStore) ArchiveSearchResponses(cutoff, pruneBefore time.Time) (int, int, error) {
	return filestore.ArchiveSearchResponses(cutoff, pruneBefore)
}

func (s *fileDetailStore) DescriptionStats() (app.DescriptionStats, error) {
	stats, err := filestore.ComputeDescriptionStats()
	if err != nil {
		return app.DescriptionStats{}, err
	}
	return app.DescriptionStats{
		Total:           stats.Total,
		WithDescription: stats.WithDescription,
		Unavailable:     stats.Unavailable,
		Empty:           stats.Empty,
	}, nil
}

// fileResponseSink adapts the JSON store to app.ResponseSink and capture.Sink.
type fileResponseSink struct{}

func newFileResponseSink() *fileResponseSink { return &fileResponseSink{} }

func (s *fileResponseSink) SaveResponse(jsonDatas []any, friendlyName string) (int, error) {
	return filestore.WriteJSONResponse(jsonDatas, friendlyName)
}

func (s *fileResponseSink) SaveResponseByID(jsonDatas []any, friendlyName, id string) (int, error) {
	return filestore.WriteJSONResponseByID(jsonDatas, friendlyName, id)
}

func (s *fileResponseSink) MergeProducts(jsonDatas []any) int {
	return filestore.SaveProductsFrom(jsonDatas)
}

func (s *fileResponseSink) WriteDebugMiss(productID string, body []byte) error {
	return pdpstate.WriteDebugMiss(productID, body)
}

// filePDPState adapts the PDP tombstone/miss files to app.PDPState.
type filePDPState struct{}

func newFilePDPState() *filePDPState { return &filePDPState{} }

func (s *filePDPState) MarkUnavailable(productID string) error {
	return pdpstate.MarkUnavailable(productID)
}

func (s *filePDPState) ClearUnavailable(productID string) error {
	return pdpstate.ClearUnavailable(productID)
}

func (s *filePDPState) LoadUnavailableProductIDs() map[string]struct{} {
	return pdpstate.LoadUnavailableProductIDs()
}

func (s *filePDPState) RecordPDPFailure(productID string, threshold int) (bool, int) {
	return pdpstate.RecordPDPFailure(productID, threshold)
}

func (s *filePDPState) ClearPDPFailure(productID string) {
	pdpstate.ClearPDPFailure(productID)
}

// pocketBaseProducts adapts PocketBase to app.ProductRepository. It opens the
// database lazily so actions that never save do not touch pb_data.
type pocketBaseProducts struct {
	once     sync.Once
	database *db.PocketBaseDB
	err      error
}

func newProductRepository() *pocketBaseProducts { return &pocketBaseProducts{} }

func (p *pocketBaseProducts) open() (*db.PocketBaseDB, error) {
	p.once.Do(func() {
		p.database, p.err = openDatabase("save")
	})
	return p.database, p.err
}

func (p *pocketBaseProducts) SaveDetailRecords(records []map[string]any) (int, error) {
	database, err := p.open()
	if err != nil {
		return 0, err
	}

	products := make([]model.MarketplaceItemDetails, 0, len(records))
	for _, record := range records {
		products = append(products, model.ToMarketplaceItemDetails(record))
	}
	return database.SaveProducts(products)
}

// openDatabase opens and starts the PocketBase database.
func openDatabase(action string) (*db.PocketBaseDB, error) {
	database, err := db.GetPocketBaseDB()
	if err != nil {
		return nil, fmt.Errorf("%s: open database: %w", action, err)
	}
	if err := database.Start(); err != nil {
		return nil, fmt.Errorf("%s: start database: %w", action, err)
	}
	return database, nil
}

// runServe starts the PocketBase admin/web UI. PocketBase parses os.Args, so
// the command is rewritten to "serve" before starting.
func runServe() error {
	database, err := openDatabase("serve")
	if err != nil {
		return err
	}

	os.Args = []string{os.Args[0], "serve"}
	return database.GetApp().Start()
}

// runInstallDrivers downloads the Playwright driver and Chromium ahead of time.
func runInstallDrivers() error {
	fmt.Println("Installing Playwright driver and Chromium (this may take a while)...")

	wrapper, err := driver.NewPlaywrightWrapper()
	if err != nil {
		return fmt.Errorf("install playwright drivers: %w", err)
	}
	if err := wrapper.Stop(); err != nil {
		logging.LogError0("install-drivers", "stop playwright", "error", err)
	}

	logging.LogInfo0("install-drivers", "playwright driver and chromium are installed")
	return nil
}

// ensure the sink satisfies both ports at compile time.
var (
	_ app.DetailStore       = (*fileDetailStore)(nil)
	_ app.ResponseSink      = (*fileResponseSink)(nil)
	_ app.ProductRepository = (*pocketBaseProducts)(nil)
	_ app.PDPState          = (*filePDPState)(nil)
)
