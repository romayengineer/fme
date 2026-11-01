package app

import (
	"fmt"

	"github.com/romayengineer/fme/go/browser/driver"
	"github.com/romayengineer/fme/go/browser/marketplace"
)

// firstError returns current when it is set, otherwise next.
func firstError(current, next error) error {
	if current != nil {
		return current
	}
	return next
}

// pause calls fn when one was supplied.
func (a *App) pause(fn func()) {
	if fn != nil {
		fn()
	}
}

// openAuthenticatedSession opens a browser session for action.
func (a *App) openAuthenticatedSession(action string) (*Session, error) {
	session, err := a.Sessions.Open()
	if err != nil {
		return nil, fmt.Errorf("%s: open session: %w", action, err)
	}
	return session, nil
}

// closeAuthenticatedSession waits for in-flight response handlers and closes
// the browser context.
func (a *App) closeAuthenticatedSession(action string, session *Session) {
	if session.Waiter != nil {
		session.Waiter.Wait()
	}
	if err := session.Context.Close(); err != nil {
		a.Logger.Error(action+": close browser context", "error", err)
	}
}

// newPageHelpers opens a page in ctx and returns it plus the Marketplace helpers.
func newPageHelpers(ctx driver.ContextWrapperInterface, action string) (driver.PageWrapperInterface, marketplace.PagesInterface, error) {
	page, err := ctx.NewPage()
	if err != nil {
		return nil, nil, fmt.Errorf("%s: create page: %w", action, err)
	}

	pageHelpers, err := marketplace.NewPages(page)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: create page helpers: %w", action, err)
	}
	return page, pageHelpers, nil
}

// processResponses materializes captured responses, preserving an earlier error.
func (a *App) processResponses(action string, runErr error) error {
	if a.Details == nil {
		return fmt.Errorf("%s: detail store is not configured", action)
	}

	if _, err := a.Details.ProcessResponses(0); err != nil {
		a.Logger.Error(action+": process responses", "error", err)
		return firstError(runErr, err)
	}
	return runErr
}

// finishCapture waits for the captured responses and materializes them.
func (a *App) finishCapture(session *Session, action string, runErr error) error {
	if session.Waiter != nil {
		session.Waiter.Wait()
	}
	return a.processResponses(action, runErr)
}
