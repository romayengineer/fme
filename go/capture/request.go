package capture

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/mxschmitt/playwright-go"

	"github.com/romayengineer/fme/go/browser/driver"
	"github.com/romayengineer/fme/go/core/jsonutil"
	"github.com/romayengineer/fme/go/core/logging"
	"github.com/romayengineer/fme/go/marketplace/extract"
)

// GetHeaders returns the request headers. When simple is true it uses the
// cheap subset Playwright tracks; otherwise it fetches every header and drops
// the HTTP/2 pseudo-headers.
func GetHeaders(request playwright.Request, simple bool) (map[string]string, error) {
	if simple {
		return request.Headers(), nil
	}
	return allHeaders(request)
}

// allHeaders returns every request header except the pseudo-headers.
func allHeaders(request playwright.Request) (map[string]string, error) {
	headersDirty, err := request.AllHeaders()
	if err != nil {
		return nil, err
	}

	headers := make(map[string]string, len(headersDirty))
	for h, v := range headersDirty {
		if strings.HasPrefix(h, ":") {
			continue
		}
		headers[h] = v
	}
	return headers, nil
}

// RunRequestWithData replays request with data as the body.
func RunRequestWithData(ctx driver.ContextWrapperInterface, pwRequest playwright.Request, data string, headersSimple bool) (playwright.APIResponse, error) {
	url := pwRequest.URL()
	method := pwRequest.Method()

	headers, err := GetHeaders(pwRequest, headersSimple)
	if err != nil {
		return nil, fmt.Errorf("get request headers: %w", err)
	}

	logging.LogDebug0("RunRequest", "request", "url", url, "method", method)
	logging.LogDebug0("RunRequest", "request headers", "count", len(headers))
	for h, v := range headers {
		logging.LogDebug0("RunRequest", "header", "key", h, "value", v)
	}

	response, err := ctx.Fetch(url,
		playwright.APIRequestContextFetchOptions{
			Method:  &method,
			Headers: headers,
			Data:    data,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("execute api request: %w", err)
	}

	responseHeaders := response.Headers()
	logging.LogDebug0("RunRequest", "response headers", "count", len(responseHeaders))
	for h, v := range responseHeaders {
		logging.LogDebug0("RunRequest", "response header", "key", h, "value", v)
	}

	return response, nil
}

// RunRequest replays request with its captured post data.
func RunRequest(ctx driver.ContextWrapperInterface, pwRequest playwright.Request, headersSimple bool) (playwright.APIResponse, error) {
	data, err := pwRequest.PostData()
	if err != nil {
		return nil, fmt.Errorf("read request post data: %w", err)
	}
	return RunRequestWithData(ctx, pwRequest, data, headersSimple)
}

// headerReplaySkip lists headers captured from the original request that must not be
// replayed: their values are tied to the original body/session and cause the request
// to be rejected (e.g. a stale content-length produces a small error response).
var headerReplaySkip = map[string]struct{}{
	"content-length":    {},
	"content-encoding":  {},
	"host":              {},
	"connection":        {},
	"transfer-encoding": {},
	"accept-encoding":   {},
}

// ReplayHeaders copies headers, dropping the ones tied to the original
// body or connection.
func ReplayHeaders(headers map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range headers {
		if _, skip := headerReplaySkip[strings.ToLower(key)]; skip {
			continue
		}
		out[key] = value
	}
	return out
}

func mapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// PDPOutcome reports the result of a FetchPDP replay.
type PDPOutcome struct {
	RateLimited bool // Facebook rate-limited the request
	Gone        bool // the listing no longer exists (definitive)
	NoProduct   bool // fetched but no listing was found (ambiguous failure)
}

// FetchPDP replays the captured MarketplacePDPContainerQuery template for a single
// product by replacing variables.targetId and running the request inside the
// logged-in page (so it uses the browser's own cookies, origin and CSRF tokens).
// It persists the response and reports whether it was rate-limited, whether the
// listing is gone, or whether the reply carried no product.
//
// The request must run in the page: Facebook rejects the replayed GraphQL request
// with error 1357054 when it is issued outside the page context (e.g. via an
// APIRequestContext), even though the body and headers are identical.
func FetchPDP(page driver.PageWrapperInterface, template *PDPTemplate, productID string, sink Sink) (PDPOutcome, error) {
	if sink == nil {
		return PDPOutcome{}, errors.New("response sink is not configured")
	}

	body, err := replayPDP(page, template, productID)
	if err != nil {
		return PDPOutcome{}, err
	}

	jsonDatas, err := jsonutil.ExtractJSONFromBody(body)
	if err != nil {
		return PDPOutcome{}, fmt.Errorf("error ExtractJSONFromBody: %w", err)
	}

	return classifyPDPResponse(jsonDatas, body, productID, sink)
}

// replayPDP runs the PDP query in the page and returns the response body.
func replayPDP(page driver.PageWrapperInterface, template *PDPTemplate, productID string) ([]byte, error) {
	postData, err := buildPDPPostData(template, productID)
	if err != nil {
		return nil, err
	}

	result, err := page.Evaluate(`async (opts) => {
		const response = await fetch(opts.url, {
			method: opts.method,
			headers: opts.headers,
			body: opts.body,
			credentials: "include",
		});
		return await response.text();
	}`, map[string]any{
		"url":     template.URL,
		"method":  template.Method,
		"headers": replayHeaderMap(template.Headers),
		"body":    postData,
	})
	if err != nil {
		return nil, fmt.Errorf("error evaluating fetch: %w", err)
	}

	bodyStr, ok := result.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected fetch result type %T", result)
	}
	return []byte(strings.TrimPrefix(bodyStr, "for (;;);")), nil
}

// buildPDPPostData clones the template body and points it at productID.
func buildPDPPostData(template *PDPTemplate, productID string) (string, error) {
	postDataMap := template.PostDataMap.Clone()
	if err := postDataMap.SetJSONString("variables", "targetId", productID); err != nil {
		return "", fmt.Errorf("error setting targetId: %w", err)
	}

	postData, err := GetPostData(postDataMap)
	if err != nil {
		return "", fmt.Errorf("error building post data: %w", err)
	}
	return postData, nil
}

// replayHeaderMap builds the JS options map from the replay headers.
func replayHeaderMap(headers map[string]string) map[string]any {
	out := make(map[string]any, len(headers))
	for key, value := range ReplayHeaders(headers) {
		out[key] = value
	}
	return out
}

// classifyPDPResponse persists the replayed response and reports its outcome.
func classifyPDPResponse(jsonDatas []any, body []byte, productID string, sink Sink) (PDPOutcome, error) {
	if slices.ContainsFunc(jsonDatas, extract.IsErrorRateLimit) {
		return PDPOutcome{RateLimited: true}, nil
	}

	// A PDP container with no listing inside means the item was removed. Do not
	// write a response file (ProcessData would just delete it); save the raw body
	// for inspection and let the caller tombstone the id.
	if reason, gone := goneProduct(jsonDatas); gone {
		logging.LogWarn0("FetchPDP", "listing no longer exists", "productID", productID, "reason", reason)
		_ = sink.WriteDebugMiss(productID, body)
		return PDPOutcome{Gone: true}, nil
	}

	// Merge the description into data/__Details immediately so it is filled as soon
	// as the response arrives, even if the run is interrupted before ProcessData.
	if saved := sink.MergeProducts(jsonDatas); saved == 0 {
		// No product and not a rate limit: an ambiguous failure. Save the raw body
		// and report it so the caller can count it and eventually skip the listing.
		_ = sink.WriteDebugMiss(productID, body)
		return PDPOutcome{NoProduct: true}, nil
	}

	if _, err := sink.SaveResponseByID(jsonDatas, "MarketplacePDPContainerQuery", productID); err != nil {
		return PDPOutcome{}, fmt.Errorf("save PDP response: %w", err)
	}

	return PDPOutcome{}, nil
}

// goneProduct reports the first payload that proves its listing was removed.
func goneProduct(jsonDatas []any) (string, bool) {
	for _, jsonData := range jsonDatas {
		if reason, gone := extract.ProductGone(jsonData); gone {
			return reason, true
		}
	}
	return "", false
}
