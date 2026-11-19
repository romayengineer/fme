package app

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// fakePage is the page surface the description flow uses.
type fakePage struct {
	body string
}

func (p *fakePage) Evaluate(string, ...any) (any, error) {
	if p.body != "" {
		return p.body, nil
	}
	return nil, nil //nolint:nilnil // fake page has no body.
}

// fakePages records Marketplace navigation.
type fakePages struct {
	searches  []string
	gotos     []string
	opened    int
	throttled bool
}

func (p *fakePages) MarketplaceSearch(keywords string, _ bool, _ int) error {
	p.searches = append(p.searches, keywords)
	return nil
}

func (p *fakePages) MarketplaceHome() error { return nil }

func (p *fakePages) GoToProduct(id string) error {
	p.gotos = append(p.gotos, "https://www.facebook.com/marketplace/item/"+id+"/")
	return nil
}

func (p *fakePages) OpenProduct(int) bool {
	p.opened++
	return true
}

func (p *fakePages) IsThrottled() bool { return p.throttled }

// fakeCloser records context shutdown.
type fakeCloser struct {
	closed bool
}

func (c *fakeCloser) Close() error {
	c.closed = true
	return nil
}

// fakeWaiter records capture drains.
type fakeWaiter struct {
	waits int
}

func (w *fakeWaiter) Wait() { w.waits++ }

// fakeSessions returns a prepared session.
type fakeSessions struct {
	session    *Session
	err        error
	openCalls  int
	closeCalls int
	closeErr   error
}

func (f *fakeSessions) Open() (*Session, error) {
	f.openCalls++
	return f.session, f.err
}

func (f *fakeSessions) Close() error {
	f.closeCalls++
	return f.closeErr
}

// testSession builds a session with the standard fakes.
func testSession(page *fakePage, pages *fakePages, closer *fakeCloser, waiter *fakeWaiter) *Session {
	return &Session{Page: page, Pages: pages, Context: closer, Waiter: waiter}
}

// fakePDPFetcher returns canned PDP outcomes.
type fakePDPFetcher struct {
	template PDPTemplate
	outcome  PDPOutcome
	err      error
	fetches  int
}

func (f *fakePDPFetcher) Template(int) (PDPTemplate, bool) {
	return f.template, f.template != nil
}

func (f *fakePDPFetcher) Fetch(Page, PDPTemplate, string) (PDPOutcome, error) {
	f.fetches++
	return f.outcome, f.err
}

type fakeDetails struct {
	processed   int
	err         error
	pending     int
	details     []map[string]any
	pdpIDs      map[string]struct{}
	fillErr     error
	dedupeCalls int
	archiveCall int
	archived    int
	pruned      int
	archiveErr  error
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
func (d *fakeDetails) ArchiveSearchResponses(time.Time, time.Time) (int, int, error) {
	d.archiveCall++
	return d.archived, d.pruned, d.archiveErr
}
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
	pages := &fakePages{}
	closer := &fakeCloser{}
	waiter := &fakeWaiter{}
	details := &fakeDetails{}
	paused := false

	app := New(Deps{
		Sessions: &fakeSessions{session: testSession(&fakePage{}, pages, closer, waiter)},
		Details:  details,
	})

	_, err := app.Search(SearchOptions{
		Keywords: "macbook m1",
		Pause:    func() { paused = true },
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	assertSearchRun(t, pages, details)
	assertSearchShutdown(t, waiter, paused, closer)
}

// assertSearchRun checks the search navigation and response processing.
func assertSearchRun(t *testing.T, pages *fakePages, details *fakeDetails) {
	t.Helper()

	if len(pages.searches) != 1 || !strings.Contains(pages.searches[0], "macbook m1") {
		t.Fatalf("searches = %v, want one macbook m1 search", pages.searches)
	}
	assertInt(t, "ProcessResponses calls", details.processed, 1)
}

// assertSearchShutdown checks the waiter, pause, and context shutdown.
func assertSearchShutdown(t *testing.T, waiter *fakeWaiter, paused bool, closer *fakeCloser) {
	t.Helper()

	if waiter.waits < 1 {
		t.Errorf("Waiter.Wait called %d times, want at least 1", waiter.waits)
	}
	if !paused {
		t.Error("Pause was not called")
	}
	if !closer.closed {
		t.Error("context was not closed")
	}
}

func TestSearchReportsThrottle(t *testing.T) {
	pages := &fakePages{throttled: true}
	app := New(Deps{
		Sessions: &fakeSessions{session: testSession(&fakePage{}, pages, &fakeCloser{}, &fakeWaiter{})},
		Details:  &fakeDetails{},
	})

	result, err := app.Search(SearchOptions{Keywords: "x"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !result.Throttled {
		t.Error("SearchResult.Throttled = false, want true")
	}
}

func TestSearchReturnsProcessError(t *testing.T) {
	details := &fakeDetails{err: errors.New("disk full")}
	app := New(Deps{
		Sessions: &fakeSessions{session: testSession(&fakePage{}, &fakePages{}, &fakeCloser{}, &fakeWaiter{})},
		Details:  details,
	})

	if _, err := app.Search(SearchOptions{Keywords: "x"}); err == nil {
		t.Fatal("Search returned no error")
	}
}

func TestSearchReturnsSessionError(t *testing.T) {	app := New(Deps{
		Sessions: &fakeSessions{err: errors.New("login failed")},
	})

	if _, err := app.Search(SearchOptions{Keywords: "x"}); err == nil {
		t.Fatal("Search returned no error")
	}
}

func TestSearchRequiresSessionFactory(t *testing.T) {
	app := New(Deps{})

	if _, err := app.Search(SearchOptions{Keywords: "x"}); err == nil {
		t.Fatal("Search returned no error without a session factory")
	}
}
