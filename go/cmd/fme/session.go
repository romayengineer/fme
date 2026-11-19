package main

import (
	"fmt"
	"sync"

	"github.com/romayengineer/fme/go/app"
	"github.com/romayengineer/fme/go/browser/driver"
	"github.com/romayengineer/fme/go/browser/facebook"
	"github.com/romayengineer/fme/go/browser/marketplace"
	"github.com/romayengineer/fme/go/capture"
	"github.com/romayengineer/fme/go/core/logging"
)

// browserSessionFactory opens Playwright and restores a Facebook session from a
// persistent Chrome profile or saved credentials.
type browserSessionFactory struct {
	blockImages bool
	sink        capture.Sink
	headless    *bool

	// mu guards playwright so concurrent opens share one runtime.
	mu sync.Mutex
	// playwright is the lazily started runtime, reused across sessions. A
	// long-running scheduler must not start (and leak) one driver per cycle.
	playwright driver.PlaywrightWrapperInterface
	// newPlaywright starts a runtime; tests replace it with a fake.
	newPlaywright func() (driver.PlaywrightWrapperInterface, error)
}

// newBrowserSessionFactory configures the browser for the current action.
func newBrowserSessionFactory(blockImages bool, sink capture.Sink, headless *bool) *browserSessionFactory {
	return &browserSessionFactory{
		blockImages:   blockImages,
		sink:          sink,
		headless:      headless,
		newPlaywright: driver.NewPlaywrightWrapper,
	}
}

// Open implements app.SessionFactory.
func (f *browserSessionFactory) Open() (*app.Session, error) {
	config, err := app.LoadConfig()
	if err != nil {
		return nil, err
	}

	headless := app.ResolveHeadless(f.headless, config.Headless)
	logging.LogInfo0("session", "browser mode", "headless", headless)

	playwrightWrapper, err := f.runtime()
	if err != nil {
		return nil, fmt.Errorf("start playwright: %w", err)
	}

	ctx, err := openContext(playwrightWrapper, config, headless)
	if err != nil {
		// Drop the runtime so the next scheduler cycle starts a fresh driver
		// instead of reusing one that may be wedged.
		_ = f.Close()
		return nil, err
	}

	return f.newSession(ctx)
}

// runtime returns the shared Playwright runtime, starting it on first use.
func (f *browserSessionFactory) runtime() (driver.PlaywrightWrapperInterface, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.playwright != nil {
		return f.playwright, nil
	}

	wrapper, err := f.newPlaywright()
	if err != nil {
		return nil, err
	}
	f.playwright = wrapper
	return wrapper, nil
}

// Close stops the shared Playwright runtime. It is safe to call repeatedly and
// is a no-op before the first Open.
func (f *browserSessionFactory) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.playwright == nil {
		return nil
	}

	err := f.playwright.Stop()
	f.playwright = nil
	return err
}

// newSession builds the app session surface from an authenticated context.
func (f *browserSessionFactory) newSession(ctx driver.ContextWrapperInterface) (*app.Session, error) {
	handlers := capture.SetContextEventHandlers(ctx, f.blockImages, f.sink)
	page, pages, err := newSessionPage(ctx)
	if err != nil {
		_ = ctx.Close()
		return nil, err
	}

	return &app.Session{
		Page:  page,
		Pages: pages,
		NewPage: func() (app.Page, app.Pages, error) {
			return newSessionPage(ctx)
		},
		Waiter:  handlers,
		Context: ctx,
	}, nil
}

// newSessionPage opens one authenticated page and its page objects.
func newSessionPage(ctx driver.ContextWrapperInterface) (app.Page, app.Pages, error) {
	page, err := ctx.NewPage()
	if err != nil {
		return nil, nil, fmt.Errorf("create page: %w", err)
	}
	pages, err := marketplace.NewPages(page)
	if err != nil {
		_ = page.Close()
		return nil, nil, fmt.Errorf("create page helpers: %w", err)
	}
	return page, pages, nil
}

// openContext restores the configured profile or logs in with credentials.
func openContext(wrapper driver.PlaywrightWrapperInterface, config *app.Config, headless bool) (driver.ContextWrapperInterface, error) {
	if config.UserDataDir != "" {
		return openPersistentContext(wrapper, config, headless)
	}
	return openBrowserSession(wrapper, config, headless)
}

// openPersistentContext launches the configured Chrome profile, then waits for
// the existing Facebook session.
func openPersistentContext(wrapper driver.PlaywrightWrapperInterface, config *app.Config, headless bool) (driver.ContextWrapperInterface, error) {
	ctx, err := wrapper.LaunchPersistentContext(config.UserDataDir, config.BrowserChannel, config.ProfileDirectory, headless)
	if err != nil {
		return nil, fmt.Errorf("launch persistent context: %w", err)
	}
	if err := facebook.EnsureLoggedIn(ctx); err != nil {
		return nil, fmt.Errorf("ensure logged in: %w", err)
	}
	return ctx, nil
}

// openBrowserSession launches a fresh browser and logs in with credentials.
func openBrowserSession(wrapper driver.PlaywrightWrapperInterface, config *app.Config, headless bool) (driver.ContextWrapperInterface, error) {
	browser, err := wrapper.NewBrowser(headless)
	if err != nil {
		return nil, fmt.Errorf("launch browser: %w", err)
	}

	credentials := facebook.UserCredentials{
		Username: config.UserCredentials.Username,
		Password: config.UserCredentials.Password,
	}
	ctx, err := facebook.NewScrapper(browser).Login(credentials)
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	return ctx, nil
}
