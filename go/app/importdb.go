package app

import "errors"

// importBatchSize is how many products are upserted per transaction.
const importBatchSize = 500

// ImportDB saves every detail record into the product repository. It returns
// the number of products written and the first error encountered.
func (a *App) ImportDB() (int64, error) {
	if a.Details == nil {
		return 0, errors.New("save: detail store is not configured")
	}
	if a.Products == nil {
		return 0, errors.New("save: product repository is not configured")
	}

	batcher := &importBatcher{repo: a.Products, app: a}
	_, iterationErr := a.Details.ForEachDetail(batcher.add, true)
	batcher.flush()
	saveErr := firstError(batcher.saveErr, iterationErr)

	a.Logger.Info("save: finished", "productsStored", batcher.stored)
	return batcher.stored, saveErr
}

// importBatcher accumulates detail records and upserts them in batches.
type importBatcher struct {
	repo     ProductRepository
	app      *App
	products []map[string]any
	stored   int64
	saveErr  error
}

// add appends one detail record and flushes a full batch.
func (b *importBatcher) add(_ string, jsonData map[string]any) bool {
	b.products = append(b.products, jsonData)
	if len(b.products) >= importBatchSize {
		b.flush()
	}
	return true
}

// flush writes the pending records and remembers the first failure.
func (b *importBatcher) flush() {
	if len(b.products) == 0 {
		return
	}

	stored, err := b.repo.SaveDetailRecords(b.products)
	b.stored += int64(stored)
	if err != nil {
		b.app.Logger.Error("save: batch", "stored", stored, "error", err)
		b.saveErr = firstError(b.saveErr, err)
	}
	b.products = b.products[:0]
}
