package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/romayengineer/fme/go/browser/driver"
)

type fakeLocator struct{}

func (l *fakeLocator) Fill(string) error                                            { return nil }
func (l *fakeLocator) Click() error                                                 { return nil }
func (l *fakeLocator) Locator(string) driver.LocatorWrapperInterface                { return l }
func (l *fakeLocator) WaitFor(float64) error                                        { return nil }
func (l *fakeLocator) GetByRole(playwright.AriaRole) driver.LocatorWrapperInterface { return l }
func (l *fakeLocator) Nth(int) driver.LocatorWrapperInterface                       { return l }
func (l *fakeLocator) All() ([]driver.LocatorWrapperInterface, error) {
	return []driver.LocatorWrapperInterface{l}, nil
}

type fakePage struct {
	gotos []string
	body  string
}

func (p *fakePage) Goto(url string) error {
	p.gotos = append(p.gotos, url)
	return nil
}
func (p *fakePage) Locator(string) driver.LocatorWrapperInterface { return &fakeLocator{} }
func (p *fakePage) Close() error                                  { return nil }
func (p *fakePage) GetByRole(playwright.AriaRole) driver.LocatorWrapperInterface {
	return &fakeLocator{}
}

func (p *fakePage) Evaluate(string, ...any) (any, error) {
	if p.body != "" {
		return p.body, nil
	}
	return nil, nil //nolint:nilnil // fake page has no body.
}

type fakeContext struct {
	page   driver.PageWrapperInterface
	closed bool
}

func (c *fakeContext) NewPage() (driver.PageWrapperInterface, error) { return c.page, nil }
func (c *fakeContext) Pages() ([]driver.PageWrapperInterface, error) {
	return nil, nil
}

func (c *fakeContext) Close() error {
	c.closed = true
	return nil
}

func (c *fakeContext) StorageState() (*playwright.StorageState, error) {
	return nil, nil //nolint:nilnil // fake context has no stored session.
}

func (c *fakeContext) Fetch(any, ...playwright.APIRequestContextFetchOptions) (playwright.APIResponse, error) {
	return nil, nil //nolint:nilnil // fake context cannot fetch.
}
func (c *fakeContext) Route(any, driver.RouteHandler) error { return nil }
func (c *fakeContext) OnRequest(driver.RequestHandler)      {}
func (c *fakeContext) OnResponse(driver.ResponseHandler)    {}

type fakeWaiter struct {
	waits int
}

func (w *fakeWaiter) Wait() { w.waits++ }

type fakeSessions struct {
	session   *Session
	err       error
	openCalls int
}

func (f *fakeSessions) Open() (*Session, error) {
	f.openCalls++
	return f.session, f.err
}

type fakeDetails struct {
	processed   int
	err         error
	pending     int
	details     []map[string]any
	pdpIDs      map[string]struct{}
	fillErr     error
	dedupeCalls int
	stats       DescriptionStats
}

func (d *fakeDetails) ProcessResponses(int64) (int64, error) {
	d.processed++
	return 0, d.err
}

func (d *fakeDetails) ForEachDetail(process func(string, map[string]any) bool, _ bool) (int, error) {
	for i, detail := range d.details {
		if !process("detail_"+string(rune('0'+i))+".json", detail) {
			break
		}
	}
	return len(d.details), nil
}

func (d *fakeDetails) LoadPDPProductIDs() map[string]struct{} {
	if d.pdpIDs == nil {
		return map[string]struct{}{}
	}
	return d.pdpIDs
}

func (d *fakeDetails) PendingDescriptions(map[string]struct{}, map[string]struct{}) (int, int, int, error) {
	return d.pending, 0, 0, nil
}
func (d *fakeDetails) FillEmpty() error { return d.fillErr }
func (d *fakeDetails) DedupePDP()       { d.dedupeCalls++ }
func (d *fakeDetails) DescriptionStats() (DescriptionStats, error) {
	return d.stats, nil
}

type fakeResponseSink struct {
	responses  int
	byID       int
	mergeCalls int
	mergeCount int
	debugMiss  int
}

func (s *fakeResponseSink) SaveResponse([]any, string) (int, error) {
	s.responses++
	return 1, nil
}

func (s *fakeResponseSink) SaveResponseByID([]any, string, string) (int, error) {
	s.byID++
	return 1, nil
}

func (s *fakeResponseSink) MergeProducts([]any) int {
	s.mergeCalls++
	return s.mergeCount
}

func (s *fakeResponseSink) WriteDebugMiss(string, []byte) error {
	s.debugMiss++
	return nil
}

type fakePDPState struct {
	marked []string
}

func (s *fakePDPState) MarkUnavailable(productID string) error {
	s.marked = append(s.marked, productID)
	return nil
}
func (s *fakePDPState) ClearUnavailable(string) error { return nil }
func (s *fakePDPState) LoadUnavailableProductIDs() map[string]struct{} {
	return map[string]struct{}{}
}
func (s *fakePDPState) RecordPDPFailure(string, int) (bool, int) { return false, 1 }
func (s *fakePDPState) ClearPDPFailure(string)                   {}

func TestSearchRunsSearchAndProcessesResponses(t *testing.T) {
	page := &fakePage{}
	ctx := &fakeContext{page: page}
	waiter := &fakeWaiter{}
	details := &fakeDetails{}
	paused := false

	app := New(Deps{
		Sessions: &fakeSessions{session: &Session{Context: ctx, Waiter: waiter}},
		Details:  details,
	})

	err := app.Search(SearchOptions{
		Keywords: "macbook m1",
		Pause:    func() { paused = true },
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	assertSearchRun(t, page, details)
	assertSearchShutdown(t, waiter, paused, ctx)
}

// assertSearchRun checks the search navigation and response processing.
func assertSearchRun(t *testing.T, page *fakePage, details *fakeDetails) {
	t.Helper()

	if len(page.gotos) != 1 {
		t.Fatalf("got %d navigations, want 1", len(page.gotos))
	}
	if !strings.Contains(page.gotos[0], "query=macbook+m1") {
		t.Errorf("search URL = %q", page.gotos[0])
	}
	assertInt(t, "ProcessResponses calls", details.processed, 1)
}

// assertSearchShutdown checks the waiter, pause, and context shutdown.
func assertSearchShutdown(t *testing.T, waiter *fakeWaiter, paused bool, ctx *fakeContext) {
	t.Helper()

	if waiter.waits < 1 {
		t.Errorf("Waiter.Wait called %d times, want at least 1", waiter.waits)
	}
	if !paused {
		t.Error("Pause was not called")
	}
	if !ctx.closed {
		t.Error("context was not closed")
	}
}

func TestSearchReturnsProcessError(t *testing.T) {
	details := &fakeDetails{err: errors.New("disk full")}
	app := New(Deps{
		Sessions: &fakeSessions{session: &Session{Context: &fakeContext{page: &fakePage{}}, Waiter: &fakeWaiter{}}},
		Details:  details,
	})

	if err := app.Search(SearchOptions{Keywords: "x"}); err == nil {
		t.Fatal("Search returned no error")
	}
}

func TestSearchReturnsSessionError(t *testing.T) {
	app := New(Deps{
		Sessions: &fakeSessions{err: errors.New("login failed")},
	})

	if err := app.Search(SearchOptions{Keywords: "x"}); err == nil {
		t.Fatal("Search returned no error")
	}
}

func TestSearchRequiresSessionFactory(t *testing.T) {
	app := New(Deps{})

	if err := app.Search(SearchOptions{Keywords: "x"}); err == nil {
		t.Fatal("Search returned no error without a session factory")
	}
}
