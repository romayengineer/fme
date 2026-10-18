package capture

import (
	"fmt"
	"strings"

	"github.com/mxschmitt/playwright-go"
	"github.com/romayengineer/fme/go/browser/driver"
	"github.com/romayengineer/fme/go/core/jsonutil"
	"github.com/romayengineer/fme/go/core/logging"
	"github.com/romayengineer/fme/go/marketplace/model"
	"github.com/romayengineer/fme/go/store/filestore"
)

func GetHeaders(request playwright.Request, simple bool) (map[string]string, error) {

	headers := map[string]string{}

	if simple {

		headers = request.Headers()

	} else {

		headersDirty, err := request.AllHeaders()
		if err != nil {
			return nil, err
		}

		for h, v := range headersDirty {
			if strings.HasPrefix(h, ":") {
				continue
			}
			headers[h] = v
		}
	}

	return headers, nil
}

func RunRequestWithData(ctx driver.ContextWrapperInterface, pwRequest playwright.Request, data string, headersSimple bool) (playwright.APIResponse, error) {
	url := pwRequest.URL()
	method := pwRequest.Method()

	headers, err := GetHeaders(pwRequest, headersSimple)
	if err != nil {
		return nil, fmt.Errorf("Error in GetHeaders: %w\n", err)
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
		return nil, fmt.Errorf("Error executing apiRequest: %w\n", err)
	}

	responseHeaders := response.Headers()
	logging.LogDebug0("RunRequest", "response headers", "count", len(responseHeaders))
	for h, v := range responseHeaders {
		logging.LogDebug0("RunRequest", "response header", "key", h, "value", v)
	}

	return response, nil
}

func RunRequest(ctx driver.ContextWrapperInterface, pwRequest playwright.Request, headersSimple bool) (playwright.APIResponse, error) {
	data, err := pwRequest.PostData()
	if err != nil {
		return nil, fmt.Errorf("Error in PostData: %w\n", err)
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
func FetchPDP(page driver.PageWrapperInterface, template *PDPTemplate, productId string) (PDPOutcome, error) {
	postDataMap := template.PostDataMap.Clone()
	if err := postDataMap.SetJsonString("variables", "targetId", productId); err != nil {
		return PDPOutcome{}, fmt.Errorf("error setting targetId: %w", err)
	}

	postData, err := GetPostData(postDataMap)
	if err != nil {
		return PDPOutcome{}, fmt.Errorf("error building post data: %w", err)
	}

	headers := map[string]any{}
	for key, value := range ReplayHeaders(template.Headers) {
		headers[key] = value
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
		"headers": headers,
		"body":    postData,
	})
	if err != nil {
		return PDPOutcome{}, fmt.Errorf("error evaluating fetch: %w", err)
	}

	bodyStr, ok := result.(string)
	if !ok {
		return PDPOutcome{}, fmt.Errorf("unexpected fetch result type %T", result)
	}

	body := []byte(strings.TrimPrefix(bodyStr, "for (;;);"))
	jsonDatas, err := jsonutil.ExtractJsonFromBody(body)
	if err != nil {
		return PDPOutcome{}, fmt.Errorf("error ExtractJsonFromBody: %w", err)
	}

	for _, jsonData := range jsonDatas {
		if model.IsErrorRateLimit(jsonData) {
			return PDPOutcome{RateLimited: true}, nil
		}
	}

	// A PDP container with no listing inside means the item was removed. Do not
	// write a response file (ProcessData would just delete it); save the raw body
	// for inspection and let the caller tombstone the id.
	for _, jsonData := range jsonDatas {
		if reason, gone := model.ProductGone(jsonData); gone {
			logging.LogWarn0("FetchPDP", "listing no longer exists", "productId", productId, "reason", reason)
			_ = filestore.WriteDebugMiss(productId, body)
			return PDPOutcome{Gone: true}, nil
		}
	}

	// Merge the description into data/__Details immediately so it is filled as soon
	// as the response arrives, even if the run is interrupted before ProcessData.
	if saved := filestore.SaveProductsFrom(jsonDatas); saved == 0 {
		// No product and not a rate limit: an ambiguous failure. Save the raw body
		// and report it so the caller can count it and eventually skip the listing.
		_ = filestore.WriteDebugMiss(productId, body)
		return PDPOutcome{NoProduct: true}, nil
	}

	if _, err := filestore.WriteJsonResponseById(jsonDatas, "MarketplacePDPContainerQuery", productId); err != nil {
		return PDPOutcome{}, fmt.Errorf("error WriteJsonResponseById: %w", err)
	}

	return PDPOutcome{}, nil
}
