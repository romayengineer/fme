package app

import (
	"errors"
	"fmt"
	"os"

	"github.com/romayengineer/fme/go/marketplace/model"
	"github.com/romayengineer/fme/go/store/db"
)

// ImportDB saves every detail record into PocketBase. It returns the number of
// products written and the first error that stopped the pass.
func (a *App) ImportDB() (int64, error) {
	if a.Details == nil {
		return 0, errors.New("save: detail store is not configured")
	}

	database, err := openDatabase("save")
	if err != nil {
		return 0, err
	}

	var processed int64
	var saveErr error
	_, iterationErr := a.Details.ForEachDetail(a.importProduct(database, &processed, &saveErr), true)
	saveErr = firstError(saveErr, iterationErr)

	a.Logger.Info("save: finished", "filesProcessed", processed)
	return processed, saveErr
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

// importProduct stores one detail record and remembers the first failure.
func (a *App) importProduct(database *db.PocketBaseDB, processed *int64, saveErr *error) func(string, map[string]any) bool {
	return func(filePath string, jsonData map[string]any) bool {
		product := model.ToMarketplaceItemDetails(jsonData)
		productID, err := database.SaveProduct(product)
		if err != nil {
			a.Logger.Error("save: save product", "file", filePath, "facebook_id", product.ID, "error", err)
			*saveErr = firstError(*saveErr, err)
			return false
		}

		a.Logger.Info("save: product stored", "productID", productID, "facebook_id", product.ID)
		*processed++
		return true
	}
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
