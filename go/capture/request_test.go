package capture

import (
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/romayengineer/fme/go/browser/driver"
	"github.com/romayengineer/fme/go/core/orderedmap"
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

type fakeSink struct {
	responses   int
	byID        int
	lastID      string
	mergeCalls  int
	mergeResult int
	debugMisses int
}

func (s *fakeSink) SaveResponse([]any, string) (int, error) {
	s.responses++
	return 1, nil
}

func (s *fakeSink) SaveResponseByID(_ []any, _ string, id string) (int, error) {
	s.byID++
	s.lastID = id
	return 1, nil
}

func (s *fakeSink) MergeProducts([]any) int {
	s.mergeCalls++
	return s.mergeResult
}

func (s *fakeSink) WriteDebugMiss(string, []byte) error {
	s.debugMisses++
	return nil
}

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
	body := `{"data":{"viewer":{"marketplace_product_details_page":{"target":{"id":"999","redacted_description":{"text":"hi"}}}}}}`
	sink := &fakeSink{mergeResult: 1}

	outcome, err := FetchPDP(fakePage{body: body}, testTemplate(), "999", sink)
	if err != nil {
		t.Fatalf("FetchPDP: %v", err)
	}
	if outcome != (PDPOutcome{}) {
		t.Fatalf("outcome = %+v, want success", outcome)
	}
	if sink.byID != 1 || sink.lastID != "999" {
		t.Errorf("SaveResponseByID calls = %d, lastID = %q, want 1/999", sink.byID, sink.lastID)
	}
	if sink.mergeCalls != 1 {
		t.Errorf("MergeProducts calls = %d, want 1", sink.mergeCalls)
	}
}

func TestFetchPDPGone(t *testing.T) {
	body := `{"data":{"viewer":{"marketplace_product_details_page":{"id":"page"}}}}`
	sink := &fakeSink{}

	outcome, err := FetchPDP(fakePage{body: body}, testTemplate(), "555", sink)
	if err != nil {
		t.Fatalf("FetchPDP: %v", err)
	}
	if !outcome.Gone {
		t.Fatalf("outcome = %+v, want Gone", outcome)
	}
	if sink.byID != 0 {
		t.Errorf("gone listing wrote a response (byID=%d)", sink.byID)
	}
	if sink.debugMisses != 1 {
		t.Errorf("debug misses = %d, want 1", sink.debugMisses)
	}
}

func TestFetchPDPNoProduct(t *testing.T) {
	sink := &fakeSink{mergeResult: 0}

	outcome, err := FetchPDP(fakePage{body: `{"data":{"unrelated":true}}`}, testTemplate(), "7", sink)
	if err != nil {
		t.Fatalf("FetchPDP: %v", err)
	}
	if !outcome.NoProduct || outcome.Gone || outcome.RateLimited {
		t.Fatalf("outcome = %+v, want NoProduct", outcome)
	}
	if sink.debugMisses != 1 {
		t.Errorf("debug misses = %d, want 1", sink.debugMisses)
	}
}

func TestFetchPDPRateLimited(t *testing.T) {
	body := `{"errors":[{"message":"Rate limit exceeded","code":1675004}]}`
	sink := &fakeSink{}

	outcome, err := FetchPDP(fakePage{body: body}, testTemplate(), "8", sink)
	if err != nil {
		t.Fatalf("FetchPDP: %v", err)
	}
	if !outcome.RateLimited {
		t.Fatalf("outcome = %+v, want RateLimited", outcome)
	}
	if sink.responses != 0 || sink.byID != 0 || sink.mergeCalls != 0 || sink.debugMisses != 0 {
		t.Errorf("rate-limited fetch touched the sink: %+v", sink)
	}
}

func TestFetchPDPRequiresSink(t *testing.T) {
	if _, err := FetchPDP(fakePage{}, testTemplate(), "1", nil); err == nil {
		t.Fatal("FetchPDP returned no error for a nil sink")
	}
}
