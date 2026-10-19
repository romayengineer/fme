package app

import (
	"fmt"
	"time"

	"github.com/romayengineer/fme/go/browser/driver"
	"github.com/romayengineer/fme/go/browser/pages"
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
	if a.Sessions == nil {
		return fmt.Errorf("get-details: session factory is not configured")
	}
	if a.Details == nil {
		return fmt.Errorf("get-details: detail store is not configured")
	}

	delay := opts.Delay
	if delay == 0 {
		delay = 3 * time.Second
	}

	session, err := a.Sessions.Open()
	if err != nil {
		return fmt.Errorf("get-details: open session: %w", err)
	}
	defer func() {
		if session.Waiter != nil {
			session.Waiter.Wait()
		}
		if err := session.Context.Close(); err != nil {
			a.Logger.Error("get-details: close browser context", "error", err)
		}
	}()

	page, err := session.Context.NewPage()
	if err != nil {
		return fmt.Errorf("get-details: create page: %w", err)
	}
	pageHelpers, err := pages.NewPages(page)
	if err != nil {
		return fmt.Errorf("get-details: create page helpers: %w", err)
	}

	_, err = a.Details.ForEachDetail(func(filePath string, jsonData map[string]any) bool {
		if jsonutil.GetKey(jsonData, "PriceAmount") != nil {
			return true
		}

		productID, ok := jsonutil.GetKey(jsonData, "ID").(string)
		if !ok || productID == "" {
			a.Logger.Warn("get-details: skipping invalid product id", "file", filePath)
			return true
		}

		if err := pageHelpers.GoToProduct(productID); err != nil {
			a.Logger.Error("get-details: open product", "productId", productID, "error", err)
			return true
		}
		time.Sleep(delay)

		return true
	}, false)
	if err != nil {
		return fmt.Errorf("get-details: iterate details: %w", err)
	}

	if opts.Pause != nil {
		opts.Pause()
	}

	return nil
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
