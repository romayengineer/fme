// Package facebook restores or creates the authenticated Facebook session.
package facebook

import (
	"errors"
	"fmt"

	"github.com/mxschmitt/playwright-go"

	"github.com/romayengineer/fme/go/browser/driver"
	"github.com/romayengineer/fme/go/browser/marketplace"
	"github.com/romayengineer/fme/go/browser/session"
	"github.com/romayengineer/fme/go/core/logging"
)

const facebookURL = "https://www.facebook.com"

// UserCredentials is the username/password pair for a credential login.
type UserCredentials struct {
	Username string
	Password string
}

// ScrapperInterface obtains an authenticated browser context.
type ScrapperInterface interface {
	// Login restores a stored session or logs in with the given credentials.
	Login(userCredentials UserCredentials) (driver.ContextWrapperInterface, error)
}

// Scrapper restores or creates the Facebook session in a browser.
type Scrapper struct {
	Browser  driver.BrowserWrapperInterface
	Sessions session.Store
}

// NewScrapper returns a scrapper that persists the session on disk.
func NewScrapper(browser driver.BrowserWrapperInterface) ScrapperInterface {
	return &Scrapper{
		Browser:  browser,
		Sessions: session.FileStore{},
	}
}

// NewScrapperWithStore allows tests to provide a fake session store.
func NewScrapperWithStore(browser driver.BrowserWrapperInterface, store session.Store) *Scrapper {
	return &Scrapper{Browser: browser, Sessions: store}
}

// EnsureLoggedIn waits until ctx reaches a logged-in Facebook page.
func EnsureLoggedIn(ctx driver.ContextWrapperInterface) error {
	page, err := existingOrNewPage(ctx)
	if err != nil {
		return err
	}

	if err := page.Goto(facebookURL); err != nil {
		return fmt.Errorf("open Facebook: %w", err)
	}

	pageHelpers, err := marketplace.NewPages(page)
	if err != nil {
		return fmt.Errorf("create page helpers: %w", err)
	}
	fmt.Println("Waiting for Facebook session...")
	if !pageHelpers.WaitForHomePage(600) {
		urlAny, _ := page.Evaluate("location.href")
		return fmt.Errorf("not logged in (url: %v)", urlAny)
	}
	fmt.Println("Session restored successfully")

	return nil
}

// existingOrNewPage returns the context's first page, opening one when empty.
func existingOrNewPage(ctx driver.ContextWrapperInterface) (driver.PageWrapperInterface, error) {
	ctxPages, err := ctx.Pages()
	if err != nil {
		return nil, fmt.Errorf("list pages: %w", err)
	}
	if len(ctxPages) > 0 {
		return ctxPages[0], nil
	}

	page, err := ctx.NewPage()
	if err != nil {
		return nil, fmt.Errorf("create page: %w", err)
	}
	return page, nil
}

// Login restores a stored session or logs in with the given credentials.
func (fs *Scrapper) Login(userCredentials UserCredentials) (driver.ContextWrapperInterface, error) {
	if ctx, ok := fs.restoreSession(); ok {
		return ctx, nil
	}
	return fs.loginWithCredentials(userCredentials)
}

// restoreSession tries the stored cookies. It returns ok=false on a missing,
// corrupt or expired session so the caller can fall back to a credential login.
func (fs *Scrapper) restoreSession() (driver.ContextWrapperInterface, bool) {
	savedSession, ok := fs.loadSession()
	if !ok {
		return nil, false
	}

	fmt.Println("Loading existing session...")
	ctx, err := fs.Browser.NewContext(savedSession, false)
	if err != nil {
		logging.LogWarn0("Login", "could not restore session context", "error", err)
		fs.deleteSession()
		return nil, false
	}

	if !sessionContextValid(ctx) {
		fs.discardExpiredSession(ctx)
		return nil, false
	}

	fmt.Println("Session restored successfully")
	return ctx, true
}

// loadSession returns the stored session, or false when there is none or it
// cannot be read.
func (fs *Scrapper) loadSession() (*playwright.StorageState, bool) {
	savedSession, err := fs.Sessions.Load()
	if errors.Is(err, session.ErrNoSession) {
		return nil, false
	}
	if err != nil {
		logging.LogWarn0("Login", "could not load saved session", "error", err)
		fs.deleteSession()
		return nil, false
	}
	if savedSession == nil {
		return nil, false
	}
	return savedSession, true
}

// deleteSession removes the stored session, logging failures.
func (fs *Scrapper) deleteSession() {
	if err := fs.Sessions.Delete(); err != nil {
		logging.LogWarn0("Login", "could not delete saved session", "error", err)
	}
}

// sessionContextValid reports whether the restored context reaches Facebook.
func sessionContextValid(ctx driver.ContextWrapperInterface) bool {
	page, err := ctx.NewPage()
	if err != nil {
		logging.LogWarn0("Login", "could not open session page", "error", err)
		_ = ctx.Close()
		return false
	}

	valid := sessionPageValid(page)
	closePage(page)
	return valid
}

// sessionPageValid reports whether page is a logged-in Facebook page.
func sessionPageValid(page driver.PageWrapperInterface) bool {
	if err := page.Goto(facebookURL); err != nil {
		return false
	}

	pageHelpers, err := marketplace.NewPages(page)
	if err != nil {
		return false
	}
	return pageHelpers.IsInHomePage()
}

// discardExpiredSession closes the context and deletes the expired session.
func (fs *Scrapper) discardExpiredSession(ctx driver.ContextWrapperInterface) {
	if err := ctx.Close(); err != nil {
		logging.LogWarn0("Login", "could not close expired session context", "error", err)
	}
	fmt.Println("session expired, deleting")
	fs.deleteSession()
}

// loginWithCredentials opens the login form and stores the resulting session.
func (fs *Scrapper) loginWithCredentials(creds UserCredentials) (driver.ContextWrapperInterface, error) {
	fmt.Println("creating new session")

	ctx, page, err := fs.newLoginPage()
	if err != nil {
		return nil, err
	}
	defer closePage(page)

	if err := fillAndSubmitLogin(page, creds); err != nil {
		return nil, err
	}
	if err := waitForLoggedIn(page); err != nil {
		return nil, err
	}
	if err := fs.saveSession(ctx); err != nil {
		return nil, err
	}

	fmt.Println("Session saved successfully")
	return ctx, nil
}

// newLoginPage opens a fresh context and one page for the credential login.
func (fs *Scrapper) newLoginPage() (driver.ContextWrapperInterface, driver.PageWrapperInterface, error) {
	ctx, err := fs.Browser.NewContext(nil, false)
	if err != nil {
		return nil, nil, fmt.Errorf("create browser context: %w", err)
	}

	page, err := ctx.NewPage()
	if err != nil {
		return nil, nil, fmt.Errorf("create page: %w", err)
	}
	return ctx, page, nil
}

// fillAndSubmitLogin opens the Facebook login page and submits the credentials.
func fillAndSubmitLogin(page driver.PageWrapperInterface, creds UserCredentials) error {
	if err := openLoginPage(page); err != nil {
		return err
	}
	if err := fillCredentials(page, creds); err != nil {
		return err
	}
	return submitLogin(page)
}

// openLoginPage navigates to Facebook.
func openLoginPage(page driver.PageWrapperInterface) error {
	if err := page.Goto(facebookURL); err != nil {
		return fmt.Errorf("open Facebook: %w", err)
	}
	fmt.Printf("Successfully opened %s in Chromium\n", facebookURL)
	return nil
}

// fillCredentials fills the email and password fields.
func fillCredentials(page driver.PageWrapperInterface, creds UserCredentials) error {
	if err := page.Locator("input[name=email]").Fill(creds.Username); err != nil {
		return fmt.Errorf("fill email: %w", err)
	}
	if err := page.Locator("input[name=pass]").Fill(creds.Password); err != nil {
		return fmt.Errorf("fill password: %w", err)
	}
	return nil
}

// submitLogin clicks the login button.
func submitLogin(page driver.PageWrapperInterface) error {
	loginButtons, err := page.Locator("span:has-text('Log in')").All()
	if err != nil {
		return fmt.Errorf("find login buttons: %w", err)
	}
	if len(loginButtons) != 4 {
		return fmt.Errorf("log in button must be 4, found %d", len(loginButtons))
	}
	if err := page.Locator("span:has-text('Log in')").Nth(1).Click(); err != nil {
		return fmt.Errorf("click Log In button: %w", err)
	}
	return nil
}

// waitForLoggedIn waits for Facebook to reach the home page after login.
func waitForLoggedIn(page driver.PageWrapperInterface) error {
	pageHelpers, err := marketplace.NewPages(page)
	if err != nil {
		return fmt.Errorf("create page helpers: %w", err)
	}
	fmt.Println("Waiting for login to complete (solve any captcha in the browser)...")
	if !pageHelpers.WaitForHomePage(600) {
		return errors.New("not in home page")
	}
	return nil
}

// saveSession persists the context's cookies and storage state.
func (fs *Scrapper) saveSession(ctx driver.ContextWrapperInterface) error {
	storageState, err := ctx.StorageState()
	if err != nil {
		return fmt.Errorf("get session storage state: %w", err)
	}
	if err := fs.Sessions.Save(storageState); err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

// closePage closes page, logging failures.
func closePage(page driver.PageWrapperInterface) {
	if err := page.Close(); err != nil {
		logging.LogWarn0("Login", "could not close page", "error", err)
	}
}
