// Package marketplace drives Facebook Marketplace pages through the browser
// driver.
package marketplace

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/romayengineer/fme/go/browser/driver"
	"github.com/romayengineer/fme/go/core/logging"
	"github.com/romayengineer/fme/go/marketplace/model"
)

// Pages is the page-object implementation bound to one browser page.
type Pages struct {
	Page           driver.PageWrapperInterface
	InputUsername  string
	InputPassword  string
	ButtonLogIn    string
	InputSearchBar string
}

// AuthPage reports whether the Facebook session is usable.
type AuthPage interface {
	// IsInHomePage reports whether the page is a logged-in Facebook page.
	IsInHomePage() bool
	// WaitForHomePage waits up to timeoutSeconds for a logged-in page.
	WaitForHomePage(timeoutSeconds int) bool
}

// SearchPage drives the Marketplace search feed.
type SearchPage interface {
	// MarketplaceSearch opens a Marketplace search for the keywords.
	MarketplaceSearch(searchKeywords string, scrollDown bool, scrollSeconds int) error
	// MarketplaceHome opens the Marketplace home page.
	MarketplaceHome() error
	// ScrollDown scrolls until the page height stops growing for maxSeconds.
	ScrollDown(maxSeconds int) error
}

// ProductPage opens individual Marketplace listings.
type ProductPage interface {
	// OpenFirstProduct clicks the first listing on the search page.
	OpenFirstProduct() bool
	// OpenProduct clicks the listing at index.
	OpenProduct(index int) bool
	// GoToProduct navigates directly to a listing URL.
	GoToProduct(id string) error
}

// PagesInterface is the full page-object surface. Consumers should accept the
// narrow interface they need instead.
type PagesInterface interface {
	AuthPage
	SearchPage
	ProductPage
}

// NewPages binds the page object to page and returns the full page surface.
func NewPages(page driver.PageWrapperInterface) (PagesInterface, error) {
	pages := Pages{
		Page:          page,
		InputUsername: "input[name=email]",
		InputPassword: "input[name=pass]",
		ButtonLogIn:   "span:has-text('Log in')",
		// Match the localized search box in the languages FME is used with.
		InputSearchBar: "input[placeholder='Buscar en Facebook'], input[aria-label='Search Facebook'], input[placeholder='Search Facebook']",
	}
	return &pages, nil
}

// IsInHomePage reports whether the page is a logged-in Facebook page.
func (pl *Pages) IsInHomePage() bool {
	if err := pl.Page.Locator(pl.InputSearchBar).WaitFor(30_000); err != nil {
		logging.LogDebug0("IsInHomePage", "search bar not visible, checking URL", "error", err)
	}
	return pl.isLoggedIn()
}

// isLoggedIn reports whether the session can reach Marketplace.
func (pl *Pages) isLoggedIn() bool {
	if pl.searchBarVisible() {
		return true
	}

	currentURL, ok := pl.currentURL()
	return ok && isLoggedInURL(currentURL)
}

// searchBarVisible reports whether the Marketplace search bar is on the page.
func (pl *Pages) searchBarVisible() bool {
	items, err := pl.Page.Locator(pl.InputSearchBar).All()
	if err != nil {
		logging.LogDebug0("isLoggedIn", "could not inspect search bar", "error", err)
		return false
	}
	return len(items) > 0
}

// currentURL returns the page's location, when it is a string.
func (pl *Pages) currentURL() (string, bool) {
	urlAny, err := pl.Page.Evaluate("location.href")
	if err != nil {
		return "", false
	}
	currentURL, ok := urlAny.(string)
	return currentURL, ok
}

// isLoggedInURL reports whether url is a Facebook page that is not a login or
// checkpoint wall.
func isLoggedInURL(currentURL string) bool {
	if !strings.Contains(currentURL, "facebook.com") {
		return false
	}
	return !strings.Contains(currentURL, "/login") && !strings.Contains(currentURL, "checkpoint")
}

// WaitForHomePage waits up to timeoutSeconds for a logged-in page.
func (pl *Pages) WaitForHomePage(timeoutSeconds int) bool {
	for range timeoutSeconds {
		if pl.isLoggedIn() {
			return true
		}
		time.Sleep(1 * time.Second)
	}
	return false
}

// ScrollDown scrolls until the page height stops growing for maxSeconds.
func (pl *Pages) ScrollDown(maxSeconds int) error {
	if maxSeconds <= 0 {
		logging.LogInfo0("ScrollDown", "scrolling disabled")
		return nil
	}
	return pl.scrollUntilStable(maxSeconds)
}

// scrollUntilStable scrolls until the page height is stable for maxSeconds.
func (pl *Pages) scrollUntilStable(maxSeconds int) error {
	start := time.Now()
	lastIncrease := start
	maxScrollHeight := 0

	for {
		// Stop only after the page height has failed to grow for maxSeconds.
		if time.Since(lastIncrease) >= time.Duration(maxSeconds)*time.Second {
			logging.LogInfo0("ScrollDown", "no height increase, stopping",
				"waitedSeconds", maxSeconds, "elapsedSeconds", int(time.Since(start).Seconds()), "maxScrollHeight", maxScrollHeight)
			return nil
		}

		height, err := pl.scrollOnce()
		if err != nil {
			return err
		}
		if height > maxScrollHeight {
			lastIncrease = time.Now()
			maxScrollHeight = height
			logging.LogDebug0("ScrollDown", "maxScrollHeight", "height", maxScrollHeight)
		}
	}
}

// scrollOnce scrolls to the bottom and returns the page height.
func (pl *Pages) scrollOnce() (int, error) {
	scrollHeight, err := pl.Page.Evaluate("document.body.scrollHeight")
	if err != nil {
		return 0, fmt.Errorf("read page scroll height: %w", err)
	}

	height, ok := toInt(scrollHeight)
	if !ok {
		logging.LogDebug0("ScrollDown", "unexpected scrollHeight type", "type", fmt.Sprintf("%T", scrollHeight))
	}

	time.Sleep(1 * time.Second)
	if _, err := pl.Page.Evaluate("window.scrollTo(0, document.body.scrollHeight)"); err != nil {
		return 0, fmt.Errorf("scroll page: %w", err)
	}
	return height, nil
}

// toInt converts a JavaScript number value returned by Playwright. JS numbers
// decode as float64, so an int assertion alone would silently never match.
func toInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case float32:
		return int(v), true
	}
	return 0, false
}

// MarketplaceSearch opens a Marketplace search for the keywords.
func (pl *Pages) MarketplaceSearch(searchKeywords string, scrollDown bool, scrollSeconds int) error {
	params := url.Values{}
	params.Add("query", searchKeywords)
	searchURL := "https://www.facebook.com/marketplace/category/search/?" + params.Encode()
	if err := pl.Page.Goto(searchURL); err != nil {
		return fmt.Errorf("open marketplace search: %w", err)
	}
	if scrollDown {
		return pl.ScrollDown(scrollSeconds)
	}
	return nil
}

// MarketplaceHome opens the Marketplace home page.
func (pl *Pages) MarketplaceHome() error {
	return pl.Page.Goto("https://www.facebook.com/marketplace/")
}

// GoToProduct navigates directly to a listing URL.
func (pl *Pages) GoToProduct(id string) error {
	productURL := model.ProductIDToLink(id)
	logging.LogInfo0("GoToProduct", "Goto", "url", productURL)
	return pl.Page.Goto(productURL)
}

// OpenFirstProduct clicks the first Marketplace listing on the search page.
//
// This is required to obtain `redacted_description`: Facebook only sends the
// MarketplacePDPContainerQuery request (which contains the description) when a
// listing is opened through a client-side click. A direct Goto does not reliably
// trigger it, so we click the link instead. See docs/redacted_description.md.
func (pl *Pages) OpenFirstProduct() bool {
	link := pl.Page.Locator(`a[href*="/marketplace/item/"]`).Nth(0)
	if err := link.Click(); err != nil {
		logging.LogWarn0("OpenFirstProduct", "click failed, falling back to direct navigation", "error", err)
		return pl.openFirstProductByHref()
	}
	logging.LogInfo0("OpenFirstProduct", "clicked first product link")
	return true
}

// OpenProduct clicks the listing link at the given index. Used when capturing the PDP
// template: if one listing fails to load (e.g. Facebook serves its error page), we try
// the next one.
func (pl *Pages) OpenProduct(index int) bool {
	if index == 0 {
		return pl.OpenFirstProduct()
	}
	link := pl.Page.Locator(`a[href*="/marketplace/item/"]`).Nth(index)
	if err := link.Click(); err != nil {
		logging.LogWarn0("OpenProduct", "click failed", "index", index, "error", err)
		return false
	}
	logging.LogInfo0("OpenProduct", "clicked product link", "index", index)
	return true
}

func (pl *Pages) openFirstProductByHref() bool {
	result, err := pl.Page.Evaluate(`(() => { const a = document.querySelector('a[href*="/marketplace/item/"]'); return a ? a.href : ""; })()`)
	if err != nil {
		logging.LogError0("OpenFirstProduct", "Error evaluating selector", "error", err)
		return false
	}

	href, ok := result.(string)
	if !ok || href == "" {
		logging.LogWarn0("OpenFirstProduct", "no marketplace item link found on page")
		return false
	}

	logging.LogInfo0("OpenFirstProduct", "opening", "url", href)
	if err := pl.Page.Goto(href); err != nil {
		logging.LogError0("OpenFirstProduct", "Error Goto", "url", href, "error", err)
		return false
	}
	return true
}
