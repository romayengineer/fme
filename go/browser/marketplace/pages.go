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

type Pages struct {
	Page           driver.PageWrapperInterface
	InputUsername  string
	InputPassword  string
	ButtonLogIn    string
	InputSearchBar string
}

// AuthPage reports whether the Facebook session is usable.
type AuthPage interface {
	IsInHomePage() bool
	WaitForHomePage(timeoutSeconds int) bool
}

// SearchPage drives the Marketplace search feed.
type SearchPage interface {
	MarketpaceSearch(searchKeyworkds string, scrollDown bool, scrollSeconds int) error
	MarketplaceHome() error
	ScrollDown(maxSeconds int) error
}

// ProductPage opens individual Marketplace listings.
type ProductPage interface {
	OpenFirstProduct() bool
	OpenProduct(index int) bool
	GoToProduct(id string) error
}

// PagesInterface is the full page-object surface. Consumers should accept the
// narrow interface they need instead.
type PagesInterface interface {
	AuthPage
	SearchPage
	ProductPage
}

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

func (pl *Pages) IsInHomePage() bool {
	if err := pl.Page.Locator(pl.InputSearchBar).WaitFor(30_000); err != nil {
		logging.LogDebug0("IsInHomePage", "search bar not visible, checking URL", "error", err)
	}
	return pl.isLoggedIn()
}

func (pl *Pages) isLoggedIn() bool {
	items, err := pl.Page.Locator(pl.InputSearchBar).All()
	if err != nil {
		logging.LogDebug0("isLoggedIn", "could not inspect search bar", "error", err)
		return false
	}
	if len(items) > 0 {
		return true
	}

	urlAny, err := pl.Page.Evaluate("location.href")
	if err != nil {
		return false
	}
	currentURL, ok := urlAny.(string)
	if !ok {
		return false
	}
	if !strings.Contains(currentURL, "facebook.com") {
		return false
	}
	return !strings.Contains(currentURL, "/login") && !strings.Contains(currentURL, "checkpoint")
}

func (pl *Pages) WaitForHomePage(timeoutSeconds int) bool {
	for i := 0; i < timeoutSeconds; i++ {
		if pl.isLoggedIn() {
			return true
		}
		time.Sleep(1 * time.Second)
	}
	return false
}

func (pl *Pages) ScrollDown(maxSeconds int) error {
	if maxSeconds <= 0 {
		logging.LogInfo0("ScrollDown", "scrolling disabled")
		return nil
	}

	maxScrollHeight := 0
	start := time.Now()
	lastIncrease := start
	for {
		// Stop only after the page height has failed to grow for maxSeconds.
		if time.Since(lastIncrease) >= time.Duration(maxSeconds)*time.Second {
			logging.LogInfo0("ScrollDown", "no height increase, stopping",
				"waitedSeconds", maxSeconds, "elapsedSeconds", int(time.Since(start).Seconds()), "maxScrollHeight", maxScrollHeight)
			return nil
		}

		scrollHeight, err := pl.Page.Evaluate("document.body.scrollHeight")
		if err != nil {
			return fmt.Errorf("read page scroll height: %w", err)
		}
		if intHeight, ok := toInt(scrollHeight); ok {
			if intHeight > maxScrollHeight {
				lastIncrease = time.Now()
				maxScrollHeight = intHeight
				logging.LogDebug0("ScrollDown", "maxScrollHeight", "height", maxScrollHeight)
			}
		} else {
			logging.LogDebug0("ScrollDown", "unexpected scrollHeight type", "type", fmt.Sprintf("%T", scrollHeight))
		}

		time.Sleep(1 * time.Second)
		if _, err := pl.Page.Evaluate("window.scrollTo(0, document.body.scrollHeight)"); err != nil {
			return fmt.Errorf("scroll page: %w", err)
		}
	}
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
	default:
		return 0, false
	}
}

func (pl *Pages) MarketpaceSearch(searchKeyworkds string, scrollDown bool, scrollSeconds int) error {
	params := url.Values{}
	params.Add("query", searchKeyworkds)
	baseUrl := "https://www.facebook.com/marketplace/category/search/?" + params.Encode()
	if err := pl.Page.Goto(baseUrl); err != nil {
		return fmt.Errorf("open marketplace search: %w", err)
	}
	if scrollDown {
		return pl.ScrollDown(scrollSeconds)
	}
	return nil
}

func (pl *Pages) MarketplaceHome() error {
	return pl.Page.Goto("https://www.facebook.com/marketplace/")
}

func (pl *Pages) GoToProduct(id string) error {
	baseUrl := model.ProductIDTolink(id)
	logging.LogInfo0("GoToProduct", "Goto", "url", baseUrl)
	return pl.Page.Goto(baseUrl)
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
