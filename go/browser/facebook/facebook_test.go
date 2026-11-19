package facebook

import (
	"errors"
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/romayengineer/fme/go/browser/driver"
)

type fakeLocator struct {
	clickErr error
	clicks   int
}

func (l *fakeLocator) Fill(string) error                             { return nil }
func (l *fakeLocator) Click() error                                  { l.clicks++; return l.clickErr }
func (l *fakeLocator) Locator(string) driver.LocatorWrapperInterface { return l }
func (l *fakeLocator) WaitFor(float64) error                         { return nil }
func (l *fakeLocator) GetByRole(playwright.AriaRole) driver.LocatorWrapperInterface {
	return l
}
func (l *fakeLocator) Nth(int) driver.LocatorWrapperInterface { return l }
func (l *fakeLocator) All() ([]driver.LocatorWrapperInterface, error) {
	return []driver.LocatorWrapperInterface{l}, nil
}

type fakePage struct {
	locator   *fakeLocator
	selectors []string
}

func (p *fakePage) Goto(string) error { return nil }
func (p *fakePage) Locator(selector string) driver.LocatorWrapperInterface {
	p.selectors = append(p.selectors, selector)
	return p.locator
}
func (p *fakePage) Close() error { return nil }
func (p *fakePage) GetByRole(playwright.AriaRole) driver.LocatorWrapperInterface {
	return p.locator
}
func (p *fakePage) Evaluate(string, ...any) (any, error) { return nil, nil } //nolint:nilnil // fake page.

func TestSubmitLoginUsesFirstWorkingSelector(t *testing.T) {
	page := &fakePage{locator: &fakeLocator{}}

	if err := submitLogin(page); err != nil {
		t.Fatalf("submitLogin: %v", err)
	}
	if len(page.selectors) != 1 || page.selectors[0] != loginButtonSelectors[0] {
		t.Errorf("selectors = %v, want only %q", page.selectors, loginButtonSelectors[0])
	}
}

func TestSubmitLoginTriesEverySelector(t *testing.T) {
	page := &fakePage{locator: &fakeLocator{clickErr: errors.New("not found")}}

	if err := submitLogin(page); err == nil {
		t.Fatal("submitLogin returned no error")
	}
	if len(page.selectors) != len(loginButtonSelectors) {
		t.Errorf("tried %d selectors, want %d", len(page.selectors), len(loginButtonSelectors))
	}
}
