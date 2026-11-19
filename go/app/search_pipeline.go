package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// descriptionPollInterval is how often the pipeline looks for freshly captured
// search responses while the search runs.
const descriptionPollInterval = 500 * time.Millisecond

// descriptionPipeline pulls descriptions for listings discovered by a running
// search. A producer materializes new responses into detail records and pushes
// their ids into a bounded FIFO; one worker replays the PDP request for each id
// on a second page, so search scrolling cannot cancel an in-flight fetch.
type descriptionPipeline struct {
	app   *App
	queue *descriptionQueue

	page Page

	state    *pullState
	template PDPTemplate

	//nolint:containedctx // The pipeline owns its run context so the producer, worker and gate share cancellation.
	ctx        context.Context
	cancel     context.CancelFunc
	searchDone chan struct{}

	since    time.Time
	seen     map[string]struct{}
	enqueued map[string]struct{}

	producerDone chan struct{}
	workerDone   chan struct{}

	searchFinishedOnce sync.Once
	finishOnce         sync.Once
	result             SearchResult
}

// startDescriptionPipeline opens the worker page, captures the PDP template,
// and starts the producer and worker. It returns an error when inline pulling
// cannot run; Search then falls back to search-only mode.
func (a *App) startDescriptionPipeline(ctx context.Context, session *Session, opts SearchOptions) (*descriptionPipeline, error) {
	if err := a.validateInlineSearch(session); err != nil {
		return nil, err
	}

	workerPage, workerPages, err := session.NewPage()
	if err != nil {
		return nil, fmt.Errorf("open description page: %w", err)
	}

	template, err := a.preparePDPTemplate(workerPages, workerPage)
	if err != nil {
		return nil, err
	}

	pullOpts := pullOptionsFromSearch(opts)
	existing, unavailable := a.loadPullMaps(pullOpts)

	runCtx, cancel := context.WithCancel(ctx)
	pipeline := &descriptionPipeline{
		app:          a,
		queue:        newDescriptionQueue(opts.DescriptionQueueLimit),
		page:         workerPage,
		state:        newPullState(a, pullOpts, existing, unavailable),
		template:     template,
		ctx:          runCtx,
		cancel:       cancel,
		searchDone:   make(chan struct{}),
		since:        time.Now(),
		seen:         map[string]struct{}{},
		enqueued:     map[string]struct{}{},
		producerDone: make(chan struct{}),
		workerDone:   make(chan struct{}),
	}

	go pipeline.runProducer()
	go pipeline.runWorker()
	a.Logger.Info("search: inline descriptions enabled", "queueLimit", cap(pipeline.queue.ch))
	return pipeline, nil
}

// validateInlineSearch checks the dependencies the pipeline needs.
func (a *App) validateInlineSearch(session *Session) error {
	if a.Details == nil {
		return errors.New("detail store is not configured")
	}
	if a.PDP == nil {
		return errors.New("PDP fetcher is not configured")
	}
	if a.Sink == nil {
		return errors.New("response sink is not configured")
	}
	if session.NewPage == nil {
		return errors.New("session cannot open a description page")
	}
	return nil
}

// preparePDPTemplate navigates the worker page to Marketplace (so the replay
// fetch runs from a facebook.com page) and reuses a captured template or opens
// one listing to obtain a fresh one.
func (a *App) preparePDPTemplate(pages Pages, page Page) (PDPTemplate, error) {
	if err := pages.MarketplaceHome(); err != nil {
		return nil, fmt.Errorf("open marketplace home for description worker: %w", err)
	}

	if template, ok := a.PDP.Template(0); ok {
		return template, nil
	}

	template := a.capturePDPTemplate(pages, page)
	if template == nil {
		return nil, errors.New("did not capture the PDP request template (Facebook may be throttling this session)")
	}
	return template, nil
}

// pullOptionsFromSearch maps the search options to the description pass.
func pullOptionsFromSearch(opts SearchOptions) PullDescriptionsOptions {
	return PullDescriptionsOptions{
		DelayMs:                 opts.DelayMs,
		RateLimitWaitSeconds:    opts.RateLimitWaitSeconds,
		MaxRateLimitWaitSeconds: opts.MaxRateLimitWaitSeconds,
	}
}

// runProducer materializes newly captured responses and queues their ids. It
// blocks while the queue is full, which is the backpressure the scroll gate
// observes.
func (p *descriptionPipeline) runProducer() {
	defer close(p.producerDone)
	defer p.queue.close()

	for p.ctx.Err() == nil {
		if !p.produce() {
			return
		}
	}
}

// produce materializes one response file and enqueues its ids. It returns false
// when there is no more work or the pipeline is stopping.
func (p *descriptionPipeline) produce() bool {
	ids, processed, err := p.materialize()
	if err != nil {
		p.app.Logger.Error("search: materialize responses for descriptions", "error", err)
		p.cancel()
		return false
	}
	if !p.enqueue(ids) {
		return false
	}
	if processed > 0 {
		return true
	}
	return p.waitForWork()
}

// waitForWork sleeps until a new capture arrives or the search finishes.
func (p *descriptionPipeline) waitForWork() bool {
	select {
	case <-p.ctx.Done():
		return false
	case <-p.searchDone:
		return p.finishMaterializing()
	case <-time.After(descriptionPollInterval):
		return true
	}
}

// finishMaterializing processes one leftover response after the capture
// handlers stopped. It reports whether another file was found.
func (p *descriptionPipeline) finishMaterializing() bool {
	ids, processed, err := p.materialize()
	if err != nil {
		p.app.Logger.Error("search: final response materialization", "error", err)
		p.cancel()
		return false
	}
	if !p.enqueue(ids) {
		return false
	}
	return processed > 0
}

// materialize processes at most one new response file.
func (p *descriptionPipeline) materialize() ([]string, int, error) {
	return p.app.Details.MaterializeNewResponses(p.since, p.seen, 1)
}

// enqueue pushes ids into the queue, skipping ones already queued this run. It
// reports false when the pipeline is stopping.
func (p *descriptionPipeline) enqueue(ids []string) bool {
	for _, id := range ids {
		if _, exists := p.enqueued[id]; exists {
			continue
		}
		p.enqueued[id] = struct{}{}
		if !p.queue.push(p.ctx, id) {
			return false
		}
	}
	return true
}

// runWorker pulls one queued description at a time.
func (p *descriptionPipeline) runWorker() {
	defer close(p.workerDone)

	for {
		id, ok := p.queue.pop(p.ctx)
		if !ok {
			return
		}
		if !p.pull(id) {
			p.cancel()
			return
		}
	}
}

// pull fetches one description, skipping ids already fetched or tombstoned.
func (p *descriptionPipeline) pull(id string) bool {
	if p.state.alreadyHandled(id) {
		return true
	}
	return p.state.pullOne(p.page, p.template, id)
}

// gate pauses scrolling while the queue is full. It returns false when the
// pipeline stopped, which ends the search early.
func (p *descriptionPipeline) gate() bool {
	for {
		if p.ctx.Err() != nil {
			return false
		}
		if p.hasQueueSpace() {
			return true
		}
		if !p.waitForQueueSpace() {
			return false
		}
	}
}

// hasQueueSpace reports whether another listing id would fit.
func (p *descriptionPipeline) hasQueueSpace() bool {
	return p.queue.len() < cap(p.queue.ch)
}

// waitForQueueSpace sleeps briefly and reports whether the gate should retry.
func (p *descriptionPipeline) waitForQueueSpace() bool {
	select {
	case <-p.ctx.Done():
		return false
	case <-time.After(descriptionPollInterval):
		return true
	}
}

// searchFinished tells the producer that no more response files will appear.
func (p *descriptionPipeline) searchFinished() {
	p.searchFinishedOnce.Do(func() { close(p.searchDone) })
}

// finish waits for the producer to drain captured responses and the worker to
// pull every queued description, then returns the pipeline counters.
func (p *descriptionPipeline) finish() SearchResult {
	p.finishOnce.Do(func() {
		p.searchFinished()
		<-p.producerDone
		<-p.workerDone
		p.cancel()

		p.result = SearchResult{
			DescriptionsPulled:     p.state.pulled,
			DescriptionsPending:    p.queue.len(),
			DescriptionRateLimited: p.state.rateLimited,
		}
		p.logSummary()
	})
	return p.result
}

// logSummary reports what the inline pipeline did.
func (p *descriptionPipeline) logSummary() {
	p.app.Logger.Info("search: inline descriptions finished",
		"pulled", p.state.pulled,
		"skipped", p.state.skipped,
		"unavailable", p.state.unavailableCount,
		"pending", p.queue.len(),
		"rateLimited", p.state.rateLimited)

	if p.state.rateLimited {
		p.app.Logger.Warn("search: description pulling stopped on a rate limit; progress is saved, the scheduler will retry later")
	}
}
