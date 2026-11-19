package app

import (
	"strings"
	"testing"
	"time"
)

func TestGetDetailsOpensListingsWithoutPrice(t *testing.T) {
	pages := &fakePages{}
	closer := &fakeCloser{}
	details := &fakeDetails{
		details: []map[string]any{
			{"ID": "123"},
			{"ID": "456", "PriceAmount": 10},
			{"ID": 789},
		},
	}
	paused := false

	app := New(Deps{
		Sessions: &fakeSessions{session: testSession(&fakePage{}, pages, closer, &fakeWaiter{})},
		Details:  details,
	})

	err := app.GetDetails(GetDetailsOptions{
		Delay: time.Millisecond,
		Pause: func() { paused = true },
	})
	if err != nil {
		t.Fatalf("GetDetails: %v", err)
	}

	assertDetailsVisit(t, pages, paused, closer)
}

// assertDetailsVisit checks the navigation, pause, and context shutdown.
func assertDetailsVisit(t *testing.T, pages *fakePages, paused bool, closer *fakeCloser) {
	t.Helper()

	if len(pages.gotos) != 1 {
		t.Fatalf("got %d navigations, want 1", len(pages.gotos))
	}
	if !strings.Contains(pages.gotos[0], "/marketplace/item/123/") {
		t.Errorf("navigation = %q, want listing 123", pages.gotos[0])
	}
	if !paused {
		t.Error("Pause was not called")
	}
	if !closer.closed {
		t.Error("context was not closed")
	}
}

func TestGetDetailsRequiresSessionFactory(t *testing.T) {
	app := New(Deps{Details: &fakeDetails{}})

	if err := app.GetDetails(GetDetailsOptions{}); err == nil {
		t.Fatal("GetDetails returned no error without a session factory")
	}
}
