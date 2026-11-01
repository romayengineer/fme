package facebook

import (
	"errors"
	"fmt"

	"github.com/romayengineer/fme/go/browser/driver"
	"github.com/romayengineer/fme/go/browser/marketplace"
	"github.com/romayengineer/fme/go/browser/session"
	"github.com/romayengineer/fme/go/core/logging"
)

const facebookURL = "https://www.facebook.com"

type UserCredentials struct {
	Username string
	Password string
}

type FacebookScrapperInterface interface {
	Login(userCredentials UserCredentials) (driver.ContextWrapperInterface, error)
}

type FacebookScrapper struct {
	Browser  driver.BrowserWrapperInterface
	Sessions session.Store
}

func NewFacebookScrapper(browser driver.BrowserWrapperInterface) FacebookScrapperInterface {
	return &FacebookScrapper{
		Browser:  browser,
		Sessions: session.FileStore{},
	}
}

// NewFacebookScrapperWithStore allows tests to provide a fake session store.
func NewFacebookScrapperWithStore(browser driver.BrowserWrapperInterface, store session.Store) *FacebookScrapper {
	return &FacebookScrapper{Browser: browser, Sessions: store}
}

func EnsureLoggedIn(ctx driver.ContextWrapperInterface) error {
	ctxPages, err := ctx.Pages()
	if err != nil {
		return fmt.Errorf("list pages: %w", err)
	}

	var page driver.PageWrapperInterface
	if len(ctxPages) > 0 {
		page = ctxPages[0]
	} else {
		page, err = ctx.NewPage()
		if err != nil {
			return fmt.Errorf("create page: %w", err)
		}
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

func (fs *FacebookScrapper) Login(userCredentials UserCredentials) (driver.ContextWrapperInterface, error) {
	if ctx, ok := fs.restoreSession(); ok {
		return ctx, nil
	}
	return fs.loginWithCredentials(userCredentials)
}

// restoreSession tries the stored cookies. It returns ok=false on a missing,
// corrupt or expired session so the caller can fall back to a credential login.
func (fs *FacebookScrapper) restoreSession() (driver.ContextWrapperInterface, bool) {
	savedSession, err := fs.Sessions.Load()
	if err != nil {
		logging.LogWarn0("Login", "could not load saved session", "error", err)
		_ = fs.Sessions.Delete()
		return nil, false
	}
	if savedSession == nil {
		return nil, false
	}

	fmt.Println("Loading existing session...")
	ctx, err := fs.Browser.NewContext(savedSession, false)
	if err != nil {
		logging.LogWarn0("Login", "could not restore session context", "error", err)
		_ = fs.Sessions.Delete()
		return nil, false
	}

	page, err := ctx.NewPage()
	if err != nil {
		logging.LogWarn0("Login", "could not open session page", "error", err)
		_ = ctx.Close()
		_ = fs.Sessions.Delete()
		return nil, false
	}

	valid := false
	if err := page.Goto(facebookURL); err == nil {
		if pageHelpers, err := marketplace.NewPages(page); err == nil && pageHelpers.IsInHomePage() {
			valid = true
		}
	}
	if err := page.Close(); err != nil {
		logging.LogWarn0("Login", "could not close session page", "error", err)
	}

	if valid {
		fmt.Println("Session restored successfully")
		return ctx, true
	}

	if err := ctx.Close(); err != nil {
		logging.LogWarn0("Login", "could not close expired session context", "error", err)
	}
	fmt.Println("session expired, deleting")
	if err := fs.Sessions.Delete(); err != nil {
		logging.LogWarn0("Login", "could not delete expired session", "error", err)
	}
	return nil, false
}

func (fs *FacebookScrapper) loginWithCredentials(userCredentials UserCredentials) (driver.ContextWrapperInterface, error) {
	fmt.Println("creating new session")

	ctx, err := fs.Browser.NewContext(nil, false)
	if err != nil {
		return nil, fmt.Errorf("create browser context: %w", err)
	}

	page, err := ctx.NewPage()
	if err != nil {
		return nil, fmt.Errorf("create page: %w", err)
	}
	defer func() {
		if err := page.Close(); err != nil {
			logging.LogWarn0("Login", "could not close login page", "error", err)
		}
	}()

	if err := page.Goto(facebookURL); err != nil {
		return nil, fmt.Errorf("open Facebook: %w", err)
	}

	fmt.Printf("Successfully opened %s in Chromium\n", facebookURL)

	emailInput := page.Locator("input[name=email]")
	if err := emailInput.Fill(userCredentials.Username); err != nil {
		return nil, fmt.Errorf("fill email: %w", err)
	}

	passwordInput := page.Locator("input[name=pass]")
	if err := passwordInput.Fill(userCredentials.Password); err != nil {
		return nil, fmt.Errorf("fill password: %w", err)
	}

	loginButtons, err := page.Locator("span:has-text('Log in')").All()
	if err != nil {
		return nil, fmt.Errorf("find login buttons: %w", err)
	}
	if len(loginButtons) != 4 {
		return nil, fmt.Errorf("log in button must be 4, found %d", len(loginButtons))
	}
	loginButton := page.Locator("span:has-text('Log in')").Nth(1)
	if err := loginButton.Click(); err != nil {
		return nil, fmt.Errorf("click Log In button: %w", err)
	}

	pageHelpers, err := marketplace.NewPages(page)
	if err != nil {
		return nil, fmt.Errorf("create page helpers: %w", err)
	}
	fmt.Println("Waiting for login to complete (solve any captcha in the browser)...")
	if !pageHelpers.WaitForHomePage(600) {
		return nil, errors.New("not in home page")
	}

	storageState, err := ctx.StorageState()
	if err != nil {
		return nil, fmt.Errorf("get session storage state: %w", err)
	}

	if err := fs.Sessions.Save(storageState); err != nil {
		return nil, fmt.Errorf("save session: %w", err)
	}

	fmt.Println("Session saved successfully")

	return ctx, nil
}
