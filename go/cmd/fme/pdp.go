package main

import (
	"errors"
	"time"

	"github.com/romayengineer/fme/go/app"
	"github.com/romayengineer/fme/go/browser/driver"
	"github.com/romayengineer/fme/go/capture"
)

// pdpFetcher adapts the capture package's PDP template/replay API to app.
type pdpFetcher struct {
	sink capture.Sink
}

func newPDPFetcher(sink capture.Sink) *pdpFetcher {
	return &pdpFetcher{sink: sink}
}

// Template implements app.PDPFetcher.
func (f *pdpFetcher) Template(timeoutSeconds int) (app.PDPTemplate, bool) {
	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	for time.Now().Before(deadline) {
		if template, ok := capture.GetPDPTemplate(); ok {
			return template, true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return nil, false
}

// Fetch implements app.PDPFetcher.
func (f *pdpFetcher) Fetch(page app.Page, template app.PDPTemplate, productID string) (app.PDPOutcome, error) {
	captured, ok := template.(*capture.PDPTemplate)
	if !ok || captured == nil {
		return app.PDPOutcome{}, errors.New("invalid PDP template")
	}

	driverPage, ok := page.(driver.PageWrapperInterface)
	if !ok {
		return app.PDPOutcome{}, errors.New("page does not support PDP replay")
	}

	outcome, err := capture.FetchPDP(driverPage, captured, productID, f.sink)
	return app.PDPOutcome{
		RateLimited: outcome.RateLimited,
		Gone:        outcome.Gone,
		NoProduct:   outcome.NoProduct,
	}, err
}
