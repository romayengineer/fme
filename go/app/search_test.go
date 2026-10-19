package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"
	"github.com/romayengineer/fme/go/browser/driver"
)

type fakePage struct {
	gotos []string
}

func (p *fakePage) Goto(url string) error {
	p.gotos = append(p.gotos, url)
	return nil
}
func (p *fakePage) Locator(string) driver.LocatorWrapperInterface { return nil }
func (p *fakePage) Close() error                                  { return nil }
func (p *fakePage) GetByRole(playwright.AriaRole) driver.LocatorWrapperInterface {
	return nil
}
func (p *fakePage) Evaluate(string, ...any) (any, error) { return nil, nil }

type fakeContext struct {
	page   driver.PageWrapperInterface
	closed bool
}

func (c *fakeContext) NewPage() (driver.PageWrapperInterface, error) { return c.page, nil }
func (c *fakeContext) Pages() ([]driver.PageWrapperInterface, error) { return nil, nil }
func (c *fakeContext) Close() error {
	c.closed = true
	return nil
}
func (c *fakeContext) StorageState() (*playwright.StorageState, error) { return nil, nil }
func (c *fakeContext) Fetch(any, ...playwright.APIRequestContextFetchOptions) (playwright.APIResponse, error) {
	return nil, nil
}
func (c *fakeContext) Route(any, driver.RouteHandler) error { return nil }
func (c *fakeContext) OnRequest(driver.RequestHandler)      {}
func (c *fakeContext) OnResponse(driver.ResponseHandler)    {}

type fakeWaiter struct {
	waits int
}

func (w *fakeWaiter) Wait() { w.waits++ }

type fakeSessions struct {
	session *Session
	err     error
}

func (f *fakeSessions) Open() (*Session, error) { return f.session, f.err }

type fakeDetails struct {
	processed int
	err       error
}

func (d *fakeDetails) ProcessResponses(int64) (int64, error) {
	d.processed++
	return 0, d.err
}
func (d *fakeDetails) ForEachDetail(func(string, map[string]any) bool, bool) (int, error) {
	return 0, nil
}
func (d *fakeDetails) PendingDescriptions(map[string]struct{}, map[string]struct{}) (int, int, int, error) {
	return 0, 0, 0, nil
}
func (d *fakeDetails) FillEmpty() error { return nil }
func (d *fakeDetails) DedupePDP()       {}

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

	if len(page.gotos) != 1 {
		t.Fatalf("got %d navigations, want 1", len(page.gotos))
	}
	if !strings.Contains(page.gotos[0], "query=macbook+m1") {
		t.Errorf("search URL = %q", page.gotos[0])
	}
	if details.processed != 1 {
		t.Errorf("ProcessResponses called %d times, want 1", details.processed)
	}
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
