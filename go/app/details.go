package app

import (
	"errors"
	"fmt"
	"time"

	"github.com/romayengineer/fme/go/browser/driver"
	"github.com/romayengineer/fme/go/browser/marketplace"
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

	_, pageHelpers, err := newPageHelpers(session.Context, "get-details")
	if err != nil {
		return err
	}

	if err := a.visitDetails(pageHelpers, detailDelay(opts.Delay)); err != nil {
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
func (a *App) visitDetails(pageHelpers marketplace.PagesInterface, delay time.Duration) error {
	_, err := a.Details.ForEachDetail(func(filePath string, jsonData map[string]any) bool {
		return a.visitDetail(pageHelpers, delay, filePath, jsonData)
	}, false)
	if err != nil {
		return fmt.Errorf("get-details: iterate details: %w", err)
	}
	return nil
}

// visitDetail opens one listing, when it still needs a price.
func (a *App) visitDetail(pageHelpers marketplace.PagesInterface, delay time.Duration, filePath string, jsonData map[string]any) bool {
	if jsonutil.GetKey(jsonData, "PriceAmount") != nil {
		return true
	}

	productID, ok := jsonutil.GetKey(jsonData, "ID").(string)
	if !ok || productID == "" {
		a.Logger.Warn("get-details: skipping invalid product id", "file", filePath)
		return true
	}

	if err := pageHelpers.GoToProduct(productID); err != nil {
		a.Logger.Error("get-details: open product", "productID", productID, "error", err)
		return true
	}
	time.Sleep(delay)

	return true
}

// InstallDrivers downloads the Playwright driver and Chromium ahead of time.
func (a *App) InstallDrivers() error {
	fmt.Println("Installing Playwright driver and Chromium (this may take a while)...")

	wrapper, err := driver.NewPlaywrightWrapper()
	if err != nil {
		return fmt.Errorf("install playwright drivers: %w", err)
	}

	if err := wrapper.Stop(); err != nil {
		a.Logger.Error("install-drivers: stop playwright", "error", err)
	}

	a.Logger.Info("install-drivers: playwright driver and chromium are installed")
	return nil
}
