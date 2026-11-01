// Package capture intercepts Facebook GraphQL responses and replays the PDP
// request to fill in listing descriptions.
package capture

import (
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/mxschmitt/playwright-go"

	"github.com/romayengineer/fme/go/browser/driver"
	"github.com/romayengineer/fme/go/core/jsonutil"
	"github.com/romayengineer/fme/go/core/logging"
	"github.com/romayengineer/fme/go/core/orderedmap"
	"github.com/romayengineer/fme/go/marketplace/extract"
)

var friendlyNamesToSkipSet = map[string]struct{}{
	"CometActorGatewayHandlerQuery":                                              {},
	"CometClassicHomeLeftRailContainerQuery":                                     {},
	"CometFeedInlineComposerQuery":                                               {},
	"CometHomeContactChannelsContainerQuery":                                     {},
	"CometHomeContactCommunityChatsContainerQuery":                               {},
	"CometHomeContactGroupsContainerQuery":                                       {},
	"CometHomeContactsContainerQuery":                                            {},
	"CometHomeCreateMenuContentQuery":                                            {},
	"CometHomeMegaMenuAllProductsQuery":                                          {},
	"CometHovercardQueryRendererQuery":                                           {},
	"CometLinkSharingInlineDisclosureCountQuery":                                 {},
	"CometMarketplaceSetProductItemSeenStateMutation":                            {},
	"CometMegaphoneRootQuery":                                                    {},
	"CometMessagingJewelDropdownEBUpsellContainerQuery":                          {},
	"CometMessagingJewelDropdownOnboardingUpsellQuery":                           {},
	"CometModernHomeFeedQuery":                                                   {},
	"CometNotificationsDropdownQuery":                                            {},
	"CometRightSideHeaderCardsQuery":                                             {},
	"CometSearchBootstrapKeywordsDataSourceQuery":                                {},
	"CometUnifiedShareSheetDialogQuery":                                          {},
	"FBScreenTimeLogger_syncMutation":                                            {},
	"FBYRPTimeLimitsEnforcementQuery":                                            {},
	"fetchMWChatVideoAutoplaySettingQuery":                                       {},
	"MarketplaceNotificationsUpdateSeenStateMutation":                            {},
	"MarketplacePDPRightColumnAdsQuery":                                          {},
	"MAWFetchXMAData_fetchXmaPreviewDataQuery":                                   {},
	"MAWVerifyThreadCutover_ContactCapabilities2Query":                           {},
	"OhaiWebClientMessengerConfigsQuery":                                         {},
	"RTWebCallBlockSettingHooksQuery":                                            {},
	"StoriesTrayRectangularRootQuery":                                            {},
	"UnifiedShareSheetMessengerSectionQuery":                                     {},
	"useCIXLogMutation":                                                          {},
	"useFeedComposerCometMentionsBootloadDataSourceQuery":                        {},
	"useFeedComposerCometMentionsBootloadDataSourceWithTaggingTransparencyQuery": {},
	"useMWEBDismissUpsellsOptOutEBHardblockReleaseMutation":                      {},
	"useMWEBOnboardingLogHardblockImpressionMutation":                            {},
	"useMWEncryptedBackupsFetchBackupIdsV2Query":                                 {},
	"usePseudoBlockedUserInterstitialF3Query":                                    {},
	"useRainbowNativeSurveyDialogPlatformIntegrationPointQuery":                  {},
	// "CometMarketplaceSearchContentPaginationQuery":                            {},
	// "MarketplaceCometBrowseFeedLightPaginationQuery":                          {},
	// "MarketplacePDPC2CMediaViewerWithImagesQuery":                             {},
	// "MarketplacePDPContainerQuery":                                            {},
}

// ContextEventHandlers persists the responses captured from one browser context.
type ContextEventHandlers struct {
	ctx              driver.ContextWrapperInterface
	extensionsToSkip map[string]struct{}
	sink             Sink

	mu     sync.Mutex
	closed bool
	wg     sync.WaitGroup
}

// ShouldProcess describes whether and how one request should be persisted.
type ShouldProcess struct {
	skip         bool
	postDataMap  orderedmap.OrderedMap
	friendlyName string
}

// PDPTemplate is the reusable request captured from a real MarketplacePDPContainerQuery.
// It is used to replay the query for other listings (with variables.targetId replaced).
type PDPTemplate struct {
	URL         string
	Method      string
	Headers     map[string]string
	PostDataMap orderedmap.OrderedMap
}

var (
	pdpTemplateMu sync.Mutex
	pdpTemplate   *PDPTemplate
)

// SetPDPTemplate stores the first captured PDP request, ignoring later ones.
func SetPDPTemplate(template PDPTemplate) {
	pdpTemplateMu.Lock()
	defer pdpTemplateMu.Unlock()
	if pdpTemplate == nil {
		pdpTemplate = &template
		logging.LogInfo0("SetPDPTemplate", "captured PDP request template", "url", template.URL, "headerKeys", mapKeys(template.Headers), "postKeys", template.PostDataMap.Keys())
	}
}

// GetPDPTemplate returns the captured PDP request and whether one exists.
func GetPDPTemplate() (*PDPTemplate, bool) {
	pdpTemplateMu.Lock()
	defer pdpTemplateMu.Unlock()
	return pdpTemplate, pdpTemplate != nil
}

// ShouldSkipRequest reports whether the request is irrelevant for capture and
// parses the post data the handlers need.
func ShouldSkipRequest(request playwright.Request) ShouldProcess {
	url := request.URL()
	if url != "https://www.facebook.com/api/graphql/" {
		return ShouldProcess{skip: true}
	}
	postDataMap, err := GetPostDataMap(request)
	if err != nil {
		return ShouldProcess{skip: true, postDataMap: postDataMap}
	}
	friendlyName := postDataMap.GetDefault("fb_api_req_friendly_name", "unknown")
	if _, exists := friendlyNamesToSkipSet[friendlyName]; exists {
		return ShouldProcess{skip: true, postDataMap: postDataMap, friendlyName: friendlyName}
	}
	return ShouldProcess{skip: false, postDataMap: postDataMap, friendlyName: friendlyName}
}

// OnResponse schedules response handling on a tracked goroutine.
func (ceh *ContextEventHandlers) OnResponse(response playwright.Response) {
	ceh.run(func() {
		ceh.handleResponse(response)
	})
}

// run schedules work on a tracked goroutine. Once Wait is called, no new work
// is accepted, so late browser events cannot race with context shutdown.
func (ceh *ContextEventHandlers) run(fn func()) {
	ceh.mu.Lock()
	if ceh.closed {
		ceh.mu.Unlock()
		return
	}
	ceh.wg.Add(1)
	ceh.mu.Unlock()

	go func() {
		defer ceh.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				logging.LogError0("OnResponse", "panic while handling response", "panic", r)
			}
		}()

		fn()
	}()
}

// Wait stops accepting new response handlers and waits for the in-flight ones
// to finish. Call it before closing the browser context or reading the files
// those handlers write.
func (ceh *ContextEventHandlers) Wait() {
	ceh.mu.Lock()
	ceh.closed = true
	ceh.mu.Unlock()

	ceh.wg.Wait()
}

func (ceh *ContextEventHandlers) handleResponse(resp playwright.Response) {
	request := resp.Request()
	shouldProcess := ShouldSkipRequest(request)
	if shouldProcess.skip {
		return
	}

	jsonDatas, ok := readResponseBody(resp)
	if !ok {
		return
	}

	if ceh.sink == nil {
		logging.LogWarn0("OnResponse", "no response sink configured, discarding response")
		return
	}

	// MarketplacePDPContainerQuery responses carry the listing description
	// (redacted_description). A PDP request is only sent when a listing is opened
	// in the browser, so we capture this first request as a reusable template and
	// replay it for the other listings (see FetchPDP / PullDescriptions).
	// See docs/redacted_description.md.
	if shouldProcess.friendlyName != "MarketplacePDPContainerQuery" {
		ceh.saveResponse(jsonDatas, shouldProcess.friendlyName)
		return
	}

	ceh.handlePDPResponse(request, jsonDatas, shouldProcess)
}

// readResponseBody decodes the JSON payloads in a response body.
func readResponseBody(resp playwright.Response) ([]any, bool) {
	body, err := resp.Body()
	if err != nil {
		logging.LogError0("OnResponse", "Error resp.Body()", "error", err)
		return nil, false
	}

	jsonDatas, err := jsonutil.ExtractJSONFromBody(body)
	if err != nil {
		logging.LogError0("OnResponse", "Error ExtractJSONFromBody()", "error", err)
		return nil, false
	}
	return jsonDatas, true
}

// handlePDPResponse persists a PDP response and captures the replay template.
func (ceh *ContextEventHandlers) handlePDPResponse(request playwright.Request, jsonDatas []any, shouldProcess ShouldProcess) {
	if slices.ContainsFunc(jsonDatas, extract.IsErrorRateLimit) {
		logging.LogWarn0("OnResponse", "PDP response was rate limited, not saving")
	} else {
		ceh.savePDPResponse(jsonDatas, shouldProcess)
	}

	capturePDPTemplate(request, shouldProcess)
}

// savePDPResponse writes the PDP response idempotently by target id and merges
// the extracted products into their detail records.
func (ceh *ContextEventHandlers) savePDPResponse(jsonDatas []any, shouldProcess ShouldProcess) {
	targetID := GetPostDataTargetID(shouldProcess.postDataMap)
	if targetID != "" {
		if _, err := ceh.sink.SaveResponseByID(jsonDatas, shouldProcess.friendlyName, targetID); err != nil {
			logging.LogError0("OnResponse", "Error SaveResponseByID()", "error", err)
		}
	} else if _, err := ceh.sink.SaveResponse(jsonDatas, shouldProcess.friendlyName); err != nil {
		logging.LogError0("OnResponse", "Error SaveResponse()", "error", err)
	}

	// Merge the description into data/__Details immediately, so the listing
	// opened to capture the template is filled without waiting for ProcessData.
	ceh.sink.MergeProducts(jsonDatas)
}

// capturePDPTemplate stores the first PDP request so it can be replayed.
func capturePDPTemplate(request playwright.Request, shouldProcess ShouldProcess) {
	headers, err := GetHeaders(request, false)
	if err != nil {
		return
	}

	SetPDPTemplate(PDPTemplate{
		URL:         request.URL(),
		Method:      request.Method(),
		Headers:     headers,
		PostDataMap: shouldProcess.postDataMap.Clone(),
	})
}

// saveResponse writes a non-PDP response through the sink.
func (ceh *ContextEventHandlers) saveResponse(jsonDatas []any, friendlyName string) {
	if _, err := ceh.sink.SaveResponse(jsonDatas, friendlyName); err != nil {
		logging.LogError0("OnResponse", "Error SaveResponse()", "error", err)
	}
}

// GetExtension returns the file extension of path.
func GetExtension(path string) string {
	return filepath.Ext(path)
}

// Route aborts image and media requests when image blocking is enabled.
func (ceh *ContextEventHandlers) Route(r playwright.Route) {
	request := r.Request()
	if strings.ToLower(request.Method()) != "get" {
		continueRoute(r)
		return
	}

	parsedURL, err := url.Parse(request.URL())
	if err != nil {
		logging.LogError0("Route", "Error parsing URL", "error", err)
		continueRoute(r)
		return
	}

	if _, skip := ceh.extensionsToSkip[GetExtension(parsedURL.Path)]; skip {
		if err := r.Abort(); err != nil {
			logging.LogError0("Route", "could not abort request", "path", parsedURL.Path, "error", err)
		}
		return
	}
	continueRoute(r)
}

// continueRoute lets the request proceed, logging failures.
func continueRoute(r playwright.Route) {
	if err := r.Continue(); err != nil {
		logging.LogError0("Route", "could not continue request", "error", err)
	}
}

// SetContextEventHandlers registers the response and route handlers on ctx and
// returns the handlers so the caller can wait for in-flight work.
func SetContextEventHandlers(ctx driver.ContextWrapperInterface, blockImages bool, sink Sink) *ContextEventHandlers {
	contextEventHandlers := &ContextEventHandlers{
		ctx:  ctx,
		sink: sink,
		extensionsToSkip: map[string]struct{}{
			".jpg":  {},
			".webp": {},
			".mp3":  {},
			".mp4":  {},
			".svg":  {},
		},
	}

	if blockImages {
		// if contextEventHandlers.Route is enabled then the response
		// from graphqh is invalid because there are missing headers
		// but it does not matter on search
		if err := ctx.Route("**", contextEventHandlers.Route); err != nil {
			logging.LogError0("SetContextEventHandlers", "could not register route handler", "error", err)
		}
	}

	ctx.OnResponse(contextEventHandlers.OnResponse)

	return contextEventHandlers
}
