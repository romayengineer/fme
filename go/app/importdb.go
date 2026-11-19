package app

import (
	"errors"
	"fmt"
	"os"

	"github.com/romayengineer/fme/go/marketplace/model"
	"github.com/romayengineer/fme/go/store/db"
)

// importBatchSize is how many products are upserted per transaction.
const importBatchSize = 500

// ImportDB saves every detail record into PocketBase. It returns the number of
// products written and the first error encountered during the pass.
func (a *App) ImportDB() (int64, error) {
	if a.Details == nil {
		return 0, errors.New("save: detail store is not configured")
	}

	database, err := openDatabase("save")
	if err != nil {
		return 0, err
	}

	batcher := &importBatcher{database: database, app: a}
	_, iterationErr := a.Details.ForEachDetail(batcher.add, true)
	batcher.flush()
	saveErr := firstError(batcher.saveErr, iterationErr)

	a.Logger.Info("save: finished", "productsStored", batcher.stored)
	return batcher.stored, saveErr
}

// importBatcher accumulates detail records and upserts them in batches.
type importBatcher struct {
	database *db.PocketBaseDB
	app      *App
	products []model.MarketplaceItemDetails
	stored   int64
	saveErr  error
}

// add appends one detail record and flushes a full batch.
func (b *importBatcher) add(_ string, jsonData map[string]any) bool {
	b.products = append(b.products, model.ToMarketplaceItemDetails(jsonData))
	if len(b.products) >= importBatchSize {
		b.flush()
	}
	return true
}

// flush writes the pending products and remembers the first failure.
func (b *importBatcher) flush() {
	if len(b.products) == 0 {
		return
	}

	stored, err := b.database.SaveProducts(b.products)
	b.stored += int64(stored)
	if err != nil {
		b.app.Logger.Error("save: batch", "stored", stored, "error", err)
		b.saveErr = firstError(b.saveErr, err)
	}
	b.products = b.products[:0]
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

// Serve starts the PocketBase admin/web UI. PocketBase parses os.Args, so the
// command is rewritten to "serve" before starting.
func (a *App) Serve() error {
	database, err := openDatabase("serve")
	if err != nil {
		return err
	}

	os.Args = os.Args[:1]
	os.Args = append(os.Args, "serve")

	return database.GetApp().Start()
}
