package facebook

import (
	"bufio"
	"fmt"
	"os"

	"github.com/romayengineer/fme/go/browser/driver"
	"github.com/romayengineer/fme/go/browser/pages"
	"github.com/romayengineer/fme/go/browser/session"
	"github.com/romayengineer/fme/go/core/logging"
)

type UserCredentials struct {
	Username string
	Password string
}

type FacebookScrapperInterface interface {
	Login(userCredentials UserCredentials) (driver.ContextWrapperInterface, error)
}

type FacebookScrapper struct {
	Browser driver.BrowserWrapperInterface
}

func NewFacebookScrapper(browser driver.BrowserWrapperInterface) FacebookScrapperInterface {
	facebookScrapper := FacebookScrapper{
		Browser: browser,
	}
	return &facebookScrapper
}

func WaitingForInput() {
	fmt.Printf("waiting for input: ")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
}

func EnsureLoggedIn(ctx driver.ContextWrapperInterface) error {
	ctxPages, err := ctx.Pages()
	if err != nil {
		return fmt.Errorf("error Pages: %v", err)
	}

	var page driver.PageWrapperInterface
	if len(ctxPages) > 0 {
		page = ctxPages[0]
	} else {
		page, err = ctx.NewPage()
		if err != nil {
			return fmt.Errorf("error NewPage: %v", err)
		}
	}

	if err := page.Goto("https://www.facebook.com"); err != nil {
		return fmt.Errorf("error Goto: %v", err)
	}

	pages, err := pages.NewPages(page)
	if err != nil {
		return fmt.Errorf("create page helpers: %w", err)
	}
	fmt.Println("Waiting for Facebook session...")
	if !pages.WaitForHomePage(600) {
		urlAny, _ := page.Evaluate("location.href")
		return fmt.Errorf("not logged in (url: %v)", urlAny)
	}
	fmt.Println("Session restored successfully")

	return nil
}

func (fs *FacebookScrapper) Login(userCredentials UserCredentials) (driver.ContextWrapperInterface, error) {

	savedSession, err := session.LoadSession()
	if err != nil {
		logging.LogWarn0("Login", "could not load saved session", "error", err)
	}
	if savedSession != nil {
		fmt.Println("Loading existing session...")
		ctx, err := fs.Browser.NewContext(savedSession, false)
		if err == nil {
			page, err := ctx.NewPage()
			if err == nil {
				// Verify session is still valid
				err = page.Goto("https://www.facebook.com")
				if err == nil {
					pageHelpers, err := pages.NewPages(page)
					if err == nil && pageHelpers.IsInHomePage() {
						fmt.Println("Session restored successfully")
						if err := page.Close(); err != nil {
							logging.LogWarn0("Login", "could not close session page", "error", err)
						}
						return ctx, nil
					}
				}
				if err := page.Close(); err != nil {
					logging.LogWarn0("Login", "could not close session page", "error", err)
				}
			}
			if err := ctx.Close(); err != nil {
				logging.LogWarn0("Login", "could not close expired session context", "error", err)
			}
		}
		fmt.Println("session expired, deleting")
		if err := session.DeleteSession(); err != nil {
			logging.LogWarn0("Login", "could not delete expired session", "error", err)
		}
	}

	fmt.Println("creating new session")

	// Create new context without saved session
	ctx, err := fs.Browser.NewContext(nil, false)
	if err != nil {
		return nil, fmt.Errorf("error NewContext: %v", err)
	}

	page, err := ctx.NewPage()
	if err != nil {
		return nil, fmt.Errorf("error NewPage: %v", err)
	}
	defer func() {
		if err := page.Close(); err != nil {
			logging.LogWarn0("Login", "could not close login page", "error", err)
		}
	}()

	url := "https://www.facebook.com"

	// Navigate to Facebook
	if err = page.Goto(url); err != nil {
		return nil, fmt.Errorf("error Goto: %v", err)
	}

	fmt.Printf("Successfully opened %s in Chromium\n", url)

	emailInput := page.Locator("input[name=email]")
	if err := emailInput.Fill(userCredentials.Username); err != nil {
		return nil, fmt.Errorf("fill email: %w", err)
	}

	passwordInput := page.Locator("input[name=pass]")
	if err := passwordInput.Fill(userCredentials.Password); err != nil {
		return nil, fmt.Errorf("fill password: %w", err)
	}

	// Find and click the Log In button using getByRole
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

	pages, err := pages.NewPages(page)
	if err != nil {
		return nil, fmt.Errorf("create page helpers: %w", err)
	}
	fmt.Println("Waiting for login to complete (solve any captcha in the browser)...")
	if !pages.WaitForHomePage(600) {
		return nil, fmt.Errorf("not in home page")
	}

	// Save the session for future use
	storageState, err := ctx.StorageState()
	if err != nil {
		return nil, fmt.Errorf("get session storage state: %w", err)
	}

	if err := session.SaveSession(storageState); err != nil {
		return nil, fmt.Errorf("save session: %w", err)
	}

	fmt.Println("Session saved successfully")

	return ctx, nil
}
