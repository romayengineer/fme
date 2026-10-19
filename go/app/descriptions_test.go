package app

import (
	"testing"

	"github.com/romayengineer/fme/go/capture"
	"github.com/romayengineer/fme/go/core/orderedmap"
)

func testPDPTemplate() *capture.PDPTemplate {
	om := orderedmap.NewOrderedMap()
	om.Set("variables", `{"targetId":"original"}`)
	return &capture.PDPTemplate{
		URL:         "https://www.facebook.com/api/graphql/",
		Method:      "POST",
		Headers:     map[string]string{"x-fb": "1"},
		PostDataMap: om,
	}
}

func TestPullDescriptionsNothingToPullSkipsSession(t *testing.T) {
	sessions := &fakeSessions{}
	details := &fakeDetails{pending: 0}

	app := New(Deps{
		Sessions: sessions,
		Details:  details,
		PDPState: &fakePDPState{},
	})

	if err := app.PullDescriptions(PullDescriptionsOptions{}); err != nil {
		t.Fatalf("PullDescriptions: %v", err)
	}
	if sessions.openCalls != 0 {
		t.Errorf("session opened %d times, want 0", sessions.openCalls)
	}
}

func TestPullDescriptionsStopsOnRateLimit(t *testing.T) {
	t.Chdir(t.TempDir())
	capture.SetPDPTemplate(*testPDPTemplate())

	page := &fakePage{body: `{"errors":[{"message":"Rate limit exceeded"}]}`}
	ctx := &fakeContext{page: page}
	state := &fakePDPState{}
	details := &fakeDetails{
		pending: 1,
		details: []map[string]any{{"ID": "123", "Description": nil}},
	}

	app := New(Deps{
		Sessions: &fakeSessions{session: &Session{Context: ctx, Waiter: &fakeWaiter{}}},
		Details:  details,
		PDPState: state,
	})

	if err := app.PullDescriptions(PullDescriptionsOptions{RateLimitWaitSeconds: 0}); err != nil {
		t.Fatalf("PullDescriptions: %v", err)
	}
	if len(state.marked) != 0 {
		t.Errorf("rate-limited listing was marked unavailable: %v", state.marked)
	}
	if !ctx.closed {
		t.Error("context was not closed")
	}
}

func TestFetchPDPWithBackoffExhaustedBudget(t *testing.T) {
	template := testPDPTemplate()
	app := New(Deps{})

	outcome, waited, err := app.fetchPDPWithBackoff(
		&fakePage{body: `{"errors":[{"message":"Rate limit exceeded"}]}`},
		template,
		"123",
		PullDescriptionsOptions{RateLimitWaitSeconds: 1, MaxRateLimitWaitSeconds: 0},
	)
	if err != nil {
		t.Fatalf("fetchPDPWithBackoff: %v", err)
	}
	if !outcome.RateLimited {
		t.Errorf("outcome = %+v, want RateLimited", outcome)
	}
	if waited != 0 {
		t.Errorf("waited = %d, want 0 (budget exhausted immediately)", waited)
	}
}
