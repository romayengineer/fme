package app

import "testing"

// testTemplate is an opaque stand-in for a captured PDP request.
func testTemplate() PDPTemplate {
	return "template"
}

func TestPullDescriptionsNothingToPullSkipsSession(t *testing.T) {
	sessions := &fakeSessions{}
	details := &fakeDetails{pending: 0}

	app := New(Deps{
		Sessions: sessions,
		Details:  details,
		PDPState: &fakePDPState{},
		Sink:     &fakeResponseSink{},
	})

	if _, err := app.PullDescriptions(PullDescriptionsOptions{}); err != nil {
		t.Fatalf("PullDescriptions: %v", err)
	}
	if sessions.openCalls != 0 {
		t.Errorf("session opened %d times, want 0", sessions.openCalls)
	}
}

func TestPullDescriptionsStopsOnRateLimit(t *testing.T) {
	page := &fakePage{body: `{"errors":[{"message":"Rate limit exceeded"}]}`}
	pages := &fakePages{}
	closer := &fakeCloser{}
	state := &fakePDPState{}
	details := &fakeDetails{
		pending: 1,
		details: []map[string]any{{"ID": "123", "Description": nil}},
	}

	app := New(Deps{
		Sessions: &fakeSessions{session: testSession(page, pages, closer, &fakeWaiter{})},
		Details:  details,
		PDPState: state,
		PDP: &fakePDPFetcher{
			template: testTemplate(),
			outcome:  PDPOutcome{RateLimited: true},
		},
		Sink: &fakeResponseSink{},
	})

	result, err := app.PullDescriptions(PullDescriptionsOptions{RateLimitWaitSeconds: 0})
	if err != nil {
		t.Fatalf("PullDescriptions: %v", err)
	}
	if !result.RateLimited {
		t.Errorf("result = %+v, want RateLimited", result)
	}
	if len(state.marked) != 0 {
		t.Errorf("rate-limited listing was marked unavailable: %v", state.marked)
	}
	if !closer.closed {
		t.Error("context was not closed")
	}
}

func TestFetchPDPWithBackoffExhaustedBudget(t *testing.T) {
	fetcher := &fakePDPFetcher{outcome: PDPOutcome{RateLimited: true}}
	app := New(Deps{PDP: fetcher})

	outcome, waited, err := app.fetchPDPWithBackoff(
		&fakePage{},
		testTemplate(),
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
