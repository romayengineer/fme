package app

import (
	"fmt"

	"github.com/romayengineer/fme/go/browser/driver"
	"github.com/romayengineer/fme/go/browser/facebook"
	"github.com/romayengineer/fme/go/capture"
	"github.com/romayengineer/fme/go/core/logging"
)

// Session is one authenticated browser context plus the capture handlers that
// must be drained before the context is closed.
type Session struct {
	Context driver.ContextWrapperInterface
	Waiter  Waiter
}

// Waiter lets a use case wait for in-flight response handlers without depending
// on the concrete capture handler type.
type Waiter interface {
	Wait()
}

// BrowserSessionFactory opens Playwright and restores a Facebook session from
// a persistent Chrome profile or saved credentials.
type BrowserSessionFactory struct {
	BlockImages bool
	Sink        capture.Sink
	// Headless overrides the browser mode when non-nil (from -headless).
	Headless *bool
}

// NewSessionFactory returns a factory that configures the browser for the
// current action. headless is the -headless override, or nil to let
// FME_HEADLESS and the available display decide. The .env file is read lazily
// in Open, so actions such as install_drivers work before credentials are
// configured.
func NewSessionFactory(blockImages bool, sink capture.Sink, headless *bool) *BrowserSessionFactory {
	return &BrowserSessionFactory{BlockImages: blockImages, Sink: sink, Headless: headless}
}

// Open implements SessionFactory.
func (f *BrowserSessionFactory) Open() (*Session, error) {
	config, err := LoadConfig()
	if err != nil {
		return nil, err
	}

	headless := ResolveHeadless(f.Headless, config.Headless)
	logging.LogInfo0("session", "browser mode", "headless", headless)

	playwrightWrapper, err := driver.NewPlaywrightWrapper()
	if err != nil {
		return nil, fmt.Errorf("start playwright: %w", err)
	}

	var ctx driver.ContextWrapperInterface
	if config.UserDataDir != "" {
		ctx, err = playwrightWrapper.LaunchPersistentContext(config.UserDataDir, config.BrowserChannel, config.ProfileDirectory, headless)
		if err != nil {
			return nil, fmt.Errorf("launch persistent context: %w", err)
		}
		if err := facebook.EnsureLoggedIn(ctx); err != nil {
			return nil, fmt.Errorf("ensure logged in: %w", err)
		}
	} else {
		browser, err := playwrightWrapper.NewBrowser(headless)
		if err != nil {
			return nil, fmt.Errorf("launch browser: %w", err)
		}

		facebookScrapper := facebook.NewFacebookScrapper(browser)
		ctx, err = facebookScrapper.Login(config.UserCredentials)
		if err != nil {
			return nil, fmt.Errorf("login: %w", err)
		}
	}

	handlers := capture.SetContextEventHandlers(ctx, f.BlockImages, f.Sink)

	return &Session{Context: ctx, Waiter: handlers}, nil
}
