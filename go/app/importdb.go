package app

import (
	"fmt"
	"os"

	"github.com/romayengineer/fme/go/marketplace/model"
	"github.com/romayengineer/fme/go/store/db"
)

// ImportDB saves every detail record into PocketBase. It returns the number of
// products written and the first error that stopped the pass.
func (a *App) ImportDB() (int64, error) {
	if a.Details == nil {
		return 0, fmt.Errorf("save: detail store is not configured")
	}

	database, err := db.GetPocketBaseDB()
	if err != nil {
		return 0, fmt.Errorf("save: open database: %w", err)
	}
	if err := database.Start(); err != nil {
		return 0, fmt.Errorf("save: start database: %w", err)
	}

	var filesProcessed int64
	var firstErr error

	_, err = a.Details.ForEachDetail(func(filePath string, jsonData map[string]any) bool {
		product := model.ToMarketplaceItemDetails(jsonData)
		productID, err := database.SaveProduct(product)
		if err != nil {
			a.Logger.Error("save: save product", "file", filePath, "facebook_id", product.ID, "error", err)
			if firstErr == nil {
				firstErr = err
			}
			return false
		}

		a.Logger.Info("save: product stored", "productId", productID, "facebook_id", product.ID)
		filesProcessed++
		return true
	}, true)
	if err != nil && firstErr == nil {
		firstErr = err
	}

	a.Logger.Info("save: finished", "filesProcessed", filesProcessed)
	return filesProcessed, firstErr
}

// Serve starts the PocketBase admin/web UI. PocketBase parses os.Args, so the
// command is rewritten to "serve" before starting.
func (a *App) Serve() error {
	database, err := db.GetPocketBaseDB()
	if err != nil {
		return fmt.Errorf("serve: open database: %w", err)
	}
	if err := database.Start(); err != nil {
		return fmt.Errorf("serve: start database: %w", err)
	}

	os.Args = os.Args[:1]
	os.Args = append(os.Args, "serve")

	return database.GetApp().Start()
}
