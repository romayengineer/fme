package app

import (
	"fmt"

	"github.com/romayengineer/fme/go/browser/driver"
	"github.com/romayengineer/fme/go/browser/facebook"
	"github.com/romayengineer/fme/go/capture"
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
}

// NewSessionFactory returns a factory that configures the browser for the
// current action. The .env file is read lazily in Open, so actions such as
// install_drivers work before credentials are configured.
func NewSessionFactory(blockImages bool, sink capture.Sink) *BrowserSessionFactory {
	return &BrowserSessionFactory{BlockImages: blockImages, Sink: sink}
}

// Open implements SessionFactory.
func (f *BrowserSessionFactory) Open() (*Session, error) {
	config, err := LoadConfig()
	if err != nil {
		return nil, err
	}

	playwrightWrapper, err := driver.NewPlaywrightWrapper()
	if err != nil {
		return nil, fmt.Errorf("start playwright: %w", err)
	}

	var ctx driver.ContextWrapperInterface
	if config.UserDataDir != "" {
		ctx, err = playwrightWrapper.LaunchPersistentContext(config.UserDataDir, config.BrowserChannel, config.ProfileDirectory, false)
		if err != nil {
			return nil, fmt.Errorf("launch persistent context: %w", err)
		}
		if err := facebook.EnsureLoggedIn(ctx); err != nil {
			return nil, fmt.Errorf("ensure logged in: %w", err)
		}
	} else {
		browser, err := playwrightWrapper.NewBrowser(false)
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
