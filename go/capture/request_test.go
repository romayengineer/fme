package capture

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mxschmitt/playwright-go"
	"github.com/romayengineer/fme/go/browser/driver"
	"github.com/romayengineer/fme/go/core/orderedmap"
	"github.com/romayengineer/fme/go/store/filestore"
)

// fakePage returns a canned response body from Evaluate and satisfies the
// PageWrapperInterface so FetchPDP can run without a browser.
type fakePage struct {
	body string
}

func (f fakePage) Goto(string) error                             { return nil }
func (f fakePage) Locator(string) driver.LocatorWrapperInterface { return nil }
func (f fakePage) Close() error                                  { return nil }
func (f fakePage) GetByRole(playwright.AriaRole) driver.LocatorWrapperInterface {
	return nil
}
func (f fakePage) Evaluate(string, ...any) (any, error) { return f.body, nil }

func testTemplate() *PDPTemplate {
	om := orderedmap.NewOrderedMap()
	om.Set("variables", `{"targetId":"original"}`)
	return &PDPTemplate{
		URL:         "https://www.facebook.com/api/graphql/",
		Method:      "POST",
		Headers:     map[string]string{"x-fb": "1"},
		PostDataMap: om,
	}
}

func TestFetchPDPSuccess(t *testing.T) {
	t.Chdir(t.TempDir())

	body := `{"data":{"viewer":{"marketplace_product_details_page":{"target":{"id":"999","redacted_description":{"text":"hi"}}}}}}`
	outcome, err := FetchPDP(fakePage{body: body}, testTemplate(), "999")
	if err != nil {
		t.Fatalf("FetchPDP: %v", err)
	}
	if outcome != (PDPOutcome{}) {
		t.Fatalf("outcome = %+v, want success", outcome)
	}
	if _, err := os.Stat(filepath.Join(filestore.DataDir, "MarketplacePDPContainerQuery", "response_0_999.json")); err != nil {
		t.Errorf("expected response file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filestore.DataDir, "__Details", "detail_999.json")); err != nil {
		t.Errorf("expected merged detail file: %v", err)
	}
}

func TestFetchPDPGone(t *testing.T) {
	t.Chdir(t.TempDir())

	body := `{"data":{"viewer":{"marketplace_product_details_page":{"id":"page"}}}}`
	outcome, err := FetchPDP(fakePage{body: body}, testTemplate(), "555")
	if err != nil {
		t.Fatalf("FetchPDP: %v", err)
	}
	if !outcome.Gone {
		t.Fatalf("outcome = %+v, want Gone", outcome)
	}
	if _, err := os.Stat(filepath.Join(filestore.DataDir, "MarketplacePDPContainerQuery", "response_0_555.json")); !os.IsNotExist(err) {
		t.Error("gone listing must not write a response file")
	}
	if _, err := os.Stat(filepath.Join(filestore.DataDir, "MarketplacePDPContainerQuery", "debug", "miss_555.json")); err != nil {
		t.Errorf("expected debug miss dump: %v", err)
	}
}

func TestFetchPDPNoProduct(t *testing.T) {
	t.Chdir(t.TempDir())

	outcome, err := FetchPDP(fakePage{body: `{"data":{"unrelated":true}}`}, testTemplate(), "7")
	if err != nil {
		t.Fatalf("FetchPDP: %v", err)
	}
	if !outcome.NoProduct || outcome.Gone || outcome.RateLimited {
		t.Fatalf("outcome = %+v, want NoProduct", outcome)
	}
	if _, err := os.Stat(filepath.Join(filestore.DataDir, "MarketplacePDPContainerQuery", "debug", "miss_7.json")); err != nil {
		t.Errorf("expected debug miss dump: %v", err)
	}
}

func TestFetchPDPRateLimited(t *testing.T) {
	t.Chdir(t.TempDir())

	body := `{"errors":[{"message":"Rate limit exceeded","code":1675004}]}`
	outcome, err := FetchPDP(fakePage{body: body}, testTemplate(), "8")
	if err != nil {
		t.Fatalf("FetchPDP: %v", err)
	}
	if !outcome.RateLimited {
		t.Fatalf("outcome = %+v, want RateLimited", outcome)
	}
	entries, _ := os.ReadDir(filepath.Join(filestore.DataDir, "MarketplacePDPContainerQuery"))
	if len(entries) != 0 {
		t.Errorf("rate-limited fetch must not persist files, found %d", len(entries))
	}
}
