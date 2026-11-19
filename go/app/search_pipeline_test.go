package app

import (
	"context"
	"testing"
	"time"
)

// newInlineTestApp builds an app whose session can open a second page.
func newInlineTestApp(details *fakeDetails, fetcher *fakePDPFetcher, pages *fakePages) *App {
	session := testSession(&fakePage{}, pages, &fakeCloser{}, &fakeWaiter{})
	session.NewPage = func() (Page, Pages, error) {
		return &fakePage{}, &fakePages{}, nil
	}

	return New(Deps{
		Sessions: &fakeSessions{session: session},
		Details:  details,
		PDPState: &fakePDPState{},
		PDP:      fetcher,
		Sink:     &fakeResponseSink{},
	})
}

func TestSearchInlinePullsQueuedDescriptions(t *testing.T) {
	details := &fakeDetails{materializeBatches: [][]string{{"101", "102"}}}
	fetcher := &fakePDPFetcher{template: testTemplate()}
	app := newInlineTestApp(details, fetcher, &fakePages{})

	result, err := app.Search(SearchOptions{
		Keywords:              "macbook m1",
		InlineDescriptions:    true,
		DescriptionQueueLimit: 2,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	assertInlineResult(t, result, 2, 0, false)
	assertFetched(t, fetcher, []string{"101", "102"})
	assertInt(t, "ProcessResponses calls", details.processed, 1)
}

func TestSearchInlineStopsOnRateLimit(t *testing.T) {
	details := &fakeDetails{materializeBatches: [][]string{{"1"}, {"2"}}}
	fetcher := &fakePDPFetcher{
		template: testTemplate(),
		outcome:  PDPOutcome{RateLimited: true},
	}
	app := newInlineTestApp(details, fetcher, &fakePages{})

	result, err := app.Search(SearchOptions{
		Keywords:              "macbook m1",
		InlineDescriptions:    true,
		DescriptionQueueLimit: 1,
		RateLimitWaitSeconds:  0,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if !result.DescriptionRateLimited {
		t.Errorf("result = %+v, want DescriptionRateLimited", result)
	}
	if result.DescriptionsPulled != 0 {
		t.Errorf("DescriptionsPulled = %d, want 0", result.DescriptionsPulled)
	}
	if details.processed != 1 {
		t.Errorf("ProcessResponses calls = %d, want 1", details.processed)
	}
}

func TestSearchInlineFallsBackWithoutWorkerPage(t *testing.T) {
	details := &fakeDetails{}
	fetcher := &fakePDPFetcher{template: testTemplate()}
	session := testSession(&fakePage{}, &fakePages{}, &fakeCloser{}, &fakeWaiter{})

	app := New(Deps{
		Sessions: &fakeSessions{session: session},
		Details:  details,
		PDPState: &fakePDPState{},
		PDP:      fetcher,
		Sink:     &fakeResponseSink{},
	})

	result, err := app.Search(SearchOptions{
		Keywords:           "macbook m1",
		InlineDescriptions: true,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if fetcher.fetches != 0 {
		t.Errorf("Fetches = %d, want 0 without a worker page", fetcher.fetches)
	}
	if result.DescriptionsPulled != 0 {
		t.Errorf("DescriptionsPulled = %d, want 0", result.DescriptionsPulled)
	}
	if details.processed != 1 {
		t.Errorf("ProcessResponses calls = %d, want 1", details.processed)
	}
}

func TestDescriptionPipelineGateWaitsForQueueSpace(t *testing.T) {
	queue := newDescriptionQueue(1)
	pipeline := &descriptionPipeline{queue: queue, ctx: context.Background()}
	if !queue.push(context.Background(), "1") {
		t.Fatal("push = false")
	}

	allowed := make(chan bool, 1)
	go func() { allowed <- pipeline.gate() }()

	assertGateBlocked(t, allowed)
	queue.pop(context.Background())
	assertGateAllowed(t, allowed)
}

func TestDescriptionPipelineGateStopsOnCancel(t *testing.T) {
	queue := newDescriptionQueue(1)
	ctx, cancel := context.WithCancel(context.Background())
	pipeline := &descriptionPipeline{queue: queue, ctx: ctx}
	if !queue.push(context.Background(), "1") {
		t.Fatal("push = false")
	}
	cancel()

	if pipeline.gate() {
		t.Fatal("gate = true after cancellation")
	}
}

// assertInlineResult checks the pipeline counters copied into the search result.
func assertInlineResult(t *testing.T, result SearchResult, pulled, pending int, rateLimited bool) {
	t.Helper()

	if result.DescriptionsPulled != pulled {
		t.Errorf("DescriptionsPulled = %d, want %d", result.DescriptionsPulled, pulled)
	}
	if result.DescriptionsPending != pending {
		t.Errorf("DescriptionsPending = %d, want %d", result.DescriptionsPending, pending)
	}
	if result.DescriptionRateLimited != rateLimited {
		t.Errorf("DescriptionRateLimited = %v, want %v", result.DescriptionRateLimited, rateLimited)
	}
}

// assertFetched checks the order of fetched description ids.
func assertFetched(t *testing.T, fetcher *fakePDPFetcher, want []string) {
	t.Helper()

	if len(fetcher.fetched) != len(want) {
		t.Fatalf("fetched = %v, want %v", fetcher.fetched, want)
	}
	for i := range want {
		if fetcher.fetched[i] != want[i] {
			t.Errorf("fetch[%d] = %q, want %q", i, fetcher.fetched[i], want[i])
		}
	}
}

// assertGateBlocked checks that the gate has not returned.
func assertGateBlocked(t *testing.T, allowed <-chan bool) {
	t.Helper()

	select {
	case <-allowed:
		t.Fatal("gate returned while the queue was full")
	case <-time.After(50 * time.Millisecond):
	}
}

// assertGateAllowed checks that the gate resumed successfully.
func assertGateAllowed(t *testing.T, allowed <-chan bool) {
	t.Helper()

	select {
	case ok := <-allowed:
		if !ok {
			t.Fatal("gate = false after space opened")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("gate did not resume")
	}
}
