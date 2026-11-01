package app

import (
	"strings"
	"testing"
	"time"
)

func TestGetDetailsOpensListingsWithoutPrice(t *testing.T) {
	page := &fakePage{}
	ctx := &fakeContext{page: page}
	details := &fakeDetails{
		details: []map[string]any{
			{"ID": "123"},
			{"ID": "456", "PriceAmount": 10},
			{"ID": 789},
		},
	}
	paused := false

	app := New(Deps{
		Sessions: &fakeSessions{session: &Session{Context: ctx, Waiter: &fakeWaiter{}}},
		Details:  details,
	})

	err := app.GetDetails(GetDetailsOptions{
		Delay: time.Millisecond,
		Pause: func() { paused = true },
	})
	if err != nil {
		t.Fatalf("GetDetails: %v", err)
	}

	assertDetailsVisit(t, page, paused, ctx)
}

// assertDetailsVisit checks the navigation, pause, and context shutdown.
func assertDetailsVisit(t *testing.T, page *fakePage, paused bool, ctx *fakeContext) {
	t.Helper()

	if len(page.gotos) != 1 {
		t.Fatalf("got %d navigations, want 1", len(page.gotos))
	}
	if !strings.Contains(page.gotos[0], "/marketplace/item/123/") {
		t.Errorf("navigation = %q, want listing 123", page.gotos[0])
	}
	if !paused {
		t.Error("Pause was not called")
	}
	if !ctx.closed {
		t.Error("context was not closed")
	}
}

func TestGetDetailsRequiresSessionFactory(t *testing.T) {
	app := New(Deps{Details: &fakeDetails{}})

	if err := app.GetDetails(GetDetailsOptions{}); err == nil {
		t.Fatal("GetDetails returned no error without a session factory")
	}
}
