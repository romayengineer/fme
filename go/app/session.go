package app

// Session is one authenticated browser context plus the page surfaces and the
// capture handlers that must be drained before it is closed.
type Session struct {
	Page  Page
	Pages Pages
	// NewPage opens an additional page in the same authenticated context. Search
	// uses it for the description worker so scrolling cannot cancel a fetch.
	NewPage func() (Page, Pages, error)
	Waiter  Waiter
	Context Closer
}

// Closer closes the underlying browser context.
type Closer interface {
	// Close closes the underlying browser context.
	Close() error
}

// Waiter lets a use case wait for in-flight response handlers without depending
// on the concrete capture handler type.
type Waiter interface {
	// Wait blocks until in-flight response handlers finish.
	Wait()
}

// SessionFactory opens an authenticated browser session. Implementations live
// in the composition root; tests provide a fake.
type SessionFactory interface {
	// Open returns an authenticated browser session.
	Open() (*Session, error)
	// Close stops the shared browser runtime, if one was started.
	Close() error
}
