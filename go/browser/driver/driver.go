// Package driver wraps the Playwright API in small interfaces so browser code
// can be exercised with fakes.
package driver

import (
	"errors"
	"fmt"

	"github.com/mxschmitt/playwright-go"
)

// LocatorWrapperInterface is the subset of a Playwright locator used by the app.
type LocatorWrapperInterface interface {
	// Fill clears the element and types value into it.
	Fill(value string) error
	// Click clicks the element.
	Click() error
	// Locator returns a locator nested under this one.
	Locator(selector string) LocatorWrapperInterface
	// WaitFor waits up to timeout for the element to become visible.
	WaitFor(timeout float64) error
	// GetByRole returns a locator matching an ARIA role.
	GetByRole(role playwright.AriaRole) LocatorWrapperInterface
	// Nth returns the nth matching locator.
	Nth(index int) LocatorWrapperInterface
	// All returns every matching locator.
	All() ([]LocatorWrapperInterface, error)
}

type (
	// RouteHandler handles an intercepted network route.
	RouteHandler func(playwright.Route)
	// ResponseHandler receives an intercepted response.
	ResponseHandler func(playwright.Response)
	// RequestHandler receives an intercepted request.
	RequestHandler func(playwright.Request)
)

// ContextWrapperInterface is the subset of a browser context used by the app.
type ContextWrapperInterface interface {
	// NewPage opens a new page in the context.
	NewPage() (PageWrapperInterface, error)
	// Pages returns every open page.
	Pages() ([]PageWrapperInterface, error)
	// Close closes the context and its pages.
	Close() error
	// StorageState returns the cookies and storage state to persist.
	StorageState() (*playwright.StorageState, error)
	// Fetch sends a request through the context.
	Fetch(urlOrRequest any, options ...playwright.APIRequestContextFetchOptions) (playwright.APIResponse, error)
	// Route registers a handler for matching requests.
	Route(url any, handler RouteHandler) error
	// OnRequest registers a request listener.
	OnRequest(fn RequestHandler)
	// OnResponse registers a response listener.
	OnResponse(fn ResponseHandler)
}

// PageWrapperInterface is the subset of a Playwright page used by the app.
type PageWrapperInterface interface {
	// Goto navigates the page to url.
	Goto(url string) error
	// Locator returns a locator for the selector.
	Locator(selector string) LocatorWrapperInterface
	// Close closes the page.
	Close() error
	// GetByRole returns a locator matching an ARIA role.
	GetByRole(role playwright.AriaRole) LocatorWrapperInterface
	// Evaluate runs a JavaScript expression in the page.
	Evaluate(expression string, arg ...any) (any, error)
}

// BrowserWrapperInterface is the subset of a Playwright browser used by the app.
type BrowserWrapperInterface interface {
	// NewContext creates a browser context, optionally restoring a session.
	NewContext(storageState *playwright.StorageState, isMobile bool) (ContextWrapperInterface, error)
	// Close closes the browser.
	Close() error
}

// PlaywrightWrapperInterface owns the Playwright runtime lifecycle.
type PlaywrightWrapperInterface interface {
	// Run installs the browser drivers and starts Playwright.
	Run() error
	// NewBrowser launches a fresh browser.
	NewBrowser(headless bool) (BrowserWrapperInterface, error)
	// LaunchPersistentContext launches a browser backed by a Chrome profile.
	LaunchPersistentContext(userDataDir string, channel string, profileDirectory string, headless bool) (ContextWrapperInterface, error)
	// Stop shuts the Playwright runtime down.
	Stop() error
}

// LocatorWrapper adapts playwright.Locator to LocatorWrapperInterface.
type LocatorWrapper struct {
	locator playwright.Locator
}

// ContextWrapper adapts playwright.BrowserContext to ContextWrapperInterface.
type ContextWrapper struct {
	Context playwright.BrowserContext
}

// PageWrapper adapts playwright.Page to PageWrapperInterface.
type PageWrapper struct {
	Page playwright.Page
}

// BrowserWrapper adapts playwright.Browser to BrowserWrapperInterface.
type BrowserWrapper struct {
	Browser    playwright.Browser
	Playwright *playwright.Playwright
}

// PlaywrightWrapper is the project's handle on the Playwright runtime.
type PlaywrightWrapper struct {
	Playwright *playwright.Playwright
}

// NewLocatorWrapper wraps a Playwright locator.
func NewLocatorWrapper(locator playwright.Locator) LocatorWrapperInterface {
	locatorWrapper := LocatorWrapper{
		locator: locator,
	}
	return &locatorWrapper
}

// WaitFor waits for the element to become visible.
func (lw *LocatorWrapper) WaitFor(timeout float64) error {
	err := lw.locator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: &timeout,
	})
	return err
}

// GetByRole returns a locator matching an ARIA role.
func (lw *LocatorWrapper) GetByRole(role playwright.AriaRole) LocatorWrapperInterface {
	locator := lw.locator.GetByRole(role)
	locatorWrapper := NewLocatorWrapper(locator)
	return locatorWrapper
}

// Locator returns a locator nested under this one.
func (lw *LocatorWrapper) Locator(selector string) LocatorWrapperInterface {
	locator := lw.locator.Locator(selector)
	locatorWrapper := NewLocatorWrapper(locator)
	return locatorWrapper
}

// Nth returns the nth matching locator.
func (lw *LocatorWrapper) Nth(index int) LocatorWrapperInterface {
	locator := lw.locator.Nth(index)
	locatorWrapper := NewLocatorWrapper(locator)
	return locatorWrapper
}

// All returns every matching locator.
func (lw *LocatorWrapper) All() ([]LocatorWrapperInterface, error) {
	locators, err := lw.locator.All()
	locatorsWrapper := make([]LocatorWrapperInterface, 0, len(locators))
	for _, locator := range locators {
		locatorsWrapper = append(locatorsWrapper, NewLocatorWrapper(locator))
	}
	return locatorsWrapper, err
}

// Fill clears the element and types value into it.
func (lw *LocatorWrapper) Fill(value string) error {
	err := lw.locator.Fill(value)
	if err != nil {
		return fmt.Errorf("error on fill: %w", err)
	}
	return nil
}

// Click clicks the element.
func (lw *LocatorWrapper) Click() error {
	err := lw.locator.Click()
	if err != nil {
		return fmt.Errorf("error on click: %w", err)
	}
	return nil
}

// NewPageWrapper wraps a Playwright page.
func NewPageWrapper(page playwright.Page) PageWrapperInterface {
	pageWrapper := PageWrapper{
		Page: page,
	}
	return &pageWrapper
}

// Evaluate runs a JavaScript expression in the page.
func (pw *PageWrapper) Evaluate(expression string, arg ...any) (any, error) {
	response, err := pw.Page.Evaluate(expression, arg...)
	return response, err
}

// GetByRole returns a locator matching an ARIA role.
func (pw *PageWrapper) GetByRole(role playwright.AriaRole) LocatorWrapperInterface {
	locator := pw.Page.GetByRole(role)
	return NewLocatorWrapper(locator)
}

// Goto navigates the page to url.
func (pw *PageWrapper) Goto(url string) error {
	_, err := pw.Page.Goto(url)
	if err != nil {
		return fmt.Errorf("could not goto %s: %w", url, err)
	}
	return nil
}

// Locator returns a locator for the selector.
func (pw *PageWrapper) Locator(selector string) LocatorWrapperInterface {
	locator := pw.Page.Locator(selector)
	locatorWrapper := NewLocatorWrapper(locator)
	return locatorWrapper
}

// Close closes the page.
func (pw *PageWrapper) Close() error {
	return pw.Page.Close()
}

// NewBrowserWrapper wraps a Playwright browser.
func NewBrowserWrapper(browser playwright.Browser, pw *playwright.Playwright) (BrowserWrapperInterface, error) {
	browserWrapper := BrowserWrapper{
		Browser:    browser,
		Playwright: pw,
	}
	return &browserWrapper, nil
}

// NewContext creates a browser context, optionally restoring a session.
func (ww *BrowserWrapper) NewContext(storageState *playwright.StorageState, isMobile bool) (ContextWrapperInterface, error) {
	opts := playwright.BrowserNewContextOptions{}
	if storageState != nil {
		opts.StorageState = storageState.ToOptionalStorageState()
	}
	if isMobile && ww.Playwright != nil {
		device := ww.Playwright.Devices["Pixel 5"]
		opts.Viewport = device.Viewport
		opts.UserAgent = playwright.String(device.UserAgent)
		opts.DeviceScaleFactor = playwright.Float(device.DeviceScaleFactor)
		opts.IsMobile = playwright.Bool(device.IsMobile)
		opts.HasTouch = playwright.Bool(device.HasTouch)
	}
	context, err := ww.Browser.NewContext(opts)
	if err != nil {
		return nil, fmt.Errorf("could not create context: %w", err)
	}
	return NewContextWrapper(context), nil
}

// Close closes the browser.
func (ww *BrowserWrapper) Close() error {
	return ww.Browser.Close()
}

// NewContextWrapper wraps a Playwright browser context.
func NewContextWrapper(context playwright.BrowserContext) ContextWrapperInterface {
	contextWrapper := ContextWrapper{Context: context}
	return &contextWrapper
}

// OnRequest registers a request listener.
func (cw *ContextWrapper) OnRequest(handler RequestHandler) {
	cw.Context.OnRequest(handler)
}

// OnResponse registers a response listener.
func (cw *ContextWrapper) OnResponse(handler ResponseHandler) {
	cw.Context.OnResponse(handler)
}

// Route registers a handler for matching requests.
func (cw *ContextWrapper) Route(url any, handler RouteHandler) error {
	return cw.Context.Route(url, handler)
}

// NewPage opens a new page in the context.
func (cw *ContextWrapper) NewPage() (PageWrapperInterface, error) {
	page, err := cw.Context.NewPage()
	if err != nil {
		return nil, fmt.Errorf("could not create page: %w", err)
	}
	return NewPageWrapper(page), nil
}

// Pages returns every open page.
func (cw *ContextWrapper) Pages() ([]PageWrapperInterface, error) {
	pages := cw.Context.Pages()
	wrapped := make([]PageWrapperInterface, 0, len(pages))
	for _, page := range pages {
		wrapped = append(wrapped, NewPageWrapper(page))
	}
	return wrapped, nil
}

// Close closes the context and its pages.
func (cw *ContextWrapper) Close() error {
	return cw.Context.Close()
}

// Fetch sends a request through the context.
func (cw *ContextWrapper) Fetch(urlOrRequest any, options ...playwright.APIRequestContextFetchOptions) (playwright.APIResponse, error) {
	return cw.Context.Request().Fetch(urlOrRequest, options...)
}

// StorageState returns the cookies and storage state to persist.
func (cw *ContextWrapper) StorageState() (*playwright.StorageState, error) {
	storageState, err := cw.Context.StorageState()
	if err != nil {
		return nil, fmt.Errorf("could not get storage state: %w", err)
	}
	return storageState, nil
}

// NewPlaywrightWrapper starts Playwright and returns the project's handle on it.
func NewPlaywrightWrapper() (PlaywrightWrapperInterface, error) {
	playwrightWrapper := PlaywrightWrapper{}
	err := playwrightWrapper.Run()
	if err != nil {
		return &playwrightWrapper, fmt.Errorf("could not create PlaywrightWrapper: %w", err)
	}
	return &playwrightWrapper, nil
}

// Stop shuts the Playwright runtime down.
func (pw *PlaywrightWrapper) Stop() error {
	return pw.Playwright.Stop()
}

// Run installs the browser drivers and starts Playwright.
func (pw *PlaywrightWrapper) Run() error {
	if pw.Playwright != nil {
		return errors.New("playwright already running")
	}

	// Install only Chromium browser
	err := playwright.Install(&playwright.RunOptions{
		Browsers: []string{"chromium"},
	})
	if err != nil {
		return fmt.Errorf("could not install browser drivers: %w", err)
	}

	playwright, err := playwright.Run()
	pw.Playwright = playwright

	if err != nil {
		return fmt.Errorf("could not start playwright: %w", err)
	}

	return err
}

// LaunchPersistentContext launches a browser backed by a Chrome profile.
func (pw *PlaywrightWrapper) LaunchPersistentContext(userDataDir string, channel string, profileDirectory string, headless bool) (ContextWrapperInterface, error) {
	opts := playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless: playwright.Bool(headless),
		Timeout:  playwright.Float(60000),
		// Use the real OS keychain so cookies copied from a Chrome profile
		// can be decrypted (Playwright defaults to a mock keychain).
		IgnoreDefaultArgs: []string{"--use-mock-keychain", "--password-store=basic"},
	}
	if channel != "" {
		opts.Channel = playwright.String(channel)
	}
	if profileDirectory != "" {
		opts.Args = append(opts.Args, "--profile-directory="+profileDirectory)
	}
	context, err := pw.Playwright.Chromium.LaunchPersistentContext(userDataDir, opts)
	if err != nil {
		return nil, fmt.Errorf("could not launch persistent context: %w", err)
	}
	return NewContextWrapper(context), nil
}

// NewBrowser launches a fresh browser.
func (pw *PlaywrightWrapper) NewBrowser(headless bool) (BrowserWrapperInterface, error) {
	browser, err := pw.Playwright.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: &headless,
	})
	if err != nil {
		return nil, fmt.Errorf("could not launch chromium: %w", err)
	}
	browserWrapper, err := NewBrowserWrapper(browser, pw.Playwright)
	if err != nil {
		return browserWrapper, fmt.Errorf("could not create BrowserWrapper: %w", err)
	}
	return browserWrapper, nil
}
