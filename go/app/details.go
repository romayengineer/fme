package app

import (
	"errors"
	"fmt"
	"time"

	"github.com/romayengineer/fme/go/core/jsonutil"
)

// GetDetailsOptions configures the listing-detail visit pass.
type GetDetailsOptions struct {
	// Delay between opening listings. Defaults to 3 seconds.
	Delay time.Duration
	Pause func()
}

// GetDetails opens every detail record that has no PriceAmount yet. The browser
// visit triggers the PDP response that capture persists.
func (a *App) GetDetails(opts GetDetailsOptions) error {
	if err := a.validateDetails(); err != nil {
		return err
	}

	session, err := a.openAuthenticatedSession("get-details")
	if err != nil {
		return err
	}
	defer a.closeAuthenticatedSession("get-details", session)

	if err := a.visitDetails(session.Pages, detailDelay(opts.Delay)); err != nil {
		return err
	}

	a.pause(opts.Pause)
	return nil
}

// validateDetails checks the dependencies GetDetails needs.
func (a *App) validateDetails() error {
	if a.Sessions == nil {
		return errors.New("get-details: session factory is not configured")
	}
	if a.Details == nil {
		return errors.New("get-details: detail store is not configured")
	}
	return nil
}

// detailDelay returns the configured delay or the three-second default.
func detailDelay(delay time.Duration) time.Duration {
	if delay == 0 {
		return 3 * time.Second
	}
	return delay
}

// visitDetails opens each detail record that is missing a price.
func (a *App) visitDetails(pages Pages, delay time.Duration) error {
	_, err := a.Details.ForEachDetail(func(filePath string, jsonData map[string]any) bool {
		return a.visitDetail(pages, delay, filePath, jsonData)
	}, false)
	if err != nil {
		return fmt.Errorf("get-details: iterate details: %w", err)
	}
	return nil
}

// visitDetail opens one listing, when it still needs a price.
func (a *App) visitDetail(pages Pages, delay time.Duration, filePath string, jsonData map[string]any) bool {
	if jsonutil.GetKey(jsonData, "PriceAmount") != nil {
		return true
	}

	productID, ok := jsonutil.GetKey(jsonData, "ID").(string)
	if !ok || productID == "" {
		a.Logger.Warn("get-details: skipping invalid product id", "file", filePath)
		return true
	}

	if err := pages.GoToProduct(productID); err != nil {
		a.Logger.Error("get-details: open product", "productID", productID, "error", err)
		return true
	}
	time.Sleep(delay)

	return true
}
