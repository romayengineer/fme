# Getting `redacted_description`

`redacted_description` is the lead text Facebook shows at the top of a Marketplace
listing. It is the most important field the extractor collects, and it is **not**
available from the search feed — it only comes from a separate Product Detail Page
(PDP) GraphQL request.

This document lists the requirements to obtain it. The same requirements are also
documented in code:

- `src/product.go` — `ProductDetailsGet` (extraction + full requirements)
- `src/pages.go` — `OpenFirstProduct` (why we must click a listing)
- `src/context_manager.go` — `OnResponse` (where the response is intercepted)

## Where it lives

In the PDP GraphQL response:

```
data.viewer.marketplace_product_details_page.target.redacted_description.text
```

Extracted in `ProductDetailsGet` (`src/product.go`) into `MarketplaceItemDetails.Description`.

## Requirements

1. **An authenticated Facebook session.**
   The PDP request is only meaningful when logged in; otherwise Facebook returns a
   login wall. The scraper launches Chrome with an existing logged-in profile via
   `FME_USER_DATA_DIR` / `FME_PROFILE_DIRECTORY` (see `.env`). A brand new,
   cookie-less browser profile triggers a captcha/security check instead.

2. **A real PDP request must be triggered by opening a listing.**
   Facebook only sends the `MarketplacePDPContainerQuery` request when a listing is
   opened through a **client-side click** on a search result. A plain URL navigation
   (`Page.Goto`) does **not** reliably trigger it. That is why, after the search,
   `Pages.OpenFirstProduct` clicks the first listing link.

3. **The response must be intercepted.**
   `ContextEventHandlers.OnResponse` (`src/context_manager.go`) listens for requests
   to `https://www.facebook.com/api/graphql/` whose friendly name
   (`fb_api_req_friendly_name` in the request post-data) is not in the skip set, and
   writes the decoded body to:

   ```
   data/MarketplacePDPContainerQuery/response_0_<id>.json
   ```

4. **The field is extracted and merged.**
   `ProcessData` runs the extractors over the captured responses. `ProductDetailsGet`
   reads `target.redacted_description.text`; the result is merged into the listing
   record at `data/__Details/detail_<id>.json` (the `FileStore` only overwrites
   non-nil fields, so it adds the description to the entry created from the search feed).

## Flow

Two commands, normally run in order:

### `just search "keyword"` — collect listings
1. Log in with the seeded Chrome profile (`FME_USER_DATA_DIR`).
2. `MarketpaceSearch` navigates to the Marketplace search URL and scrolls
   (bounded by `-scroll-seconds`).
3. Search responses are captured as `CometMarketplaceSearchContentPaginationQuery`.
4. `ProcessData` runs at the end of the search and extracts them into
   `data/__Details/detail_<id>.json` (without descriptions), so searching alone
   materializes new listings.

### `just pull-description` — fetch missing descriptions
1. `ProcessData` runs first, materializing any unprocessed search responses so newly
   searched listings are included in this pass.
2. Log in, open the Marketplace home, and `Pages.OpenFirstProduct` clicks the first
   listing → triggers `MarketplacePDPContainerQuery`. Any listing works; it only needs
   to exist so we can capture the request.
3. `ContextEventHandlers.OnResponse` captures that first PDP request as a reusable
   template (`URL`, `method`, `headers`, post-data with `variables.targetId`) and
   merges that listing's description immediately (`SaveProductsFrom`).
4. For every `data/__Details/detail_<id>.json` **missing** a description,
   `FetchPDP` clones the template, sets `variables.targetId` to that listing's id,
   and replays the request **inside the page** (a `fetch(...)` executed via
   `page.Evaluate` with `credentials: "include"`). It must run in the page: Facebook
   rejects the replayed GraphQL request (error `1357054`) when it is issued outside
   the page context, even with an identical body/headers.
5. `FetchPDP` writes each response to `data/MarketplacePDPContainerQuery/`, stopping
   early if Facebook returns the rate-limit error. As soon as a response arrives its
   description is merged into the matching `data/__Details/detail_<id>.json` (via
   `SaveProductsFrom`), so descriptions are filled incrementally during the run.
6. `ProcessData` runs once at the end as a safety net (it is no longer required for the
   descriptions to appear).

Run it with `just pull-description` (optionally `-delay-ms=N`).

`just process-data` runs the extraction step (`ProcessData`) on its own, without a
browser, if you ever need to materialize responses manually.

Responses are written **once per product id** at
`data/MarketplacePDPContainerQuery/response_0_<id>.json` (idempotent overwrite), and
ids that already have a response on disk are skipped. Re-running therefore never
creates duplicates. If duplicates ever appear, `just dedupe-pdp` removes them
(keeping the newest per id).

## Notes

- `ReplayHeaders` drops `content-length`/`content-encoding`/`host` from the captured
  headers (they belong to the original body); `GetPostData` rebuilds a correctly
  form-encoded body.
- Pulling stops (without failing the run) on the rate-limit error, so a very large
  batch may need more than one run. Use `-delay-ms` to slow it down if needed.

## Removed listings

Some listings can never be filled because they no longer exist on Facebook. Their
PDP request succeeds but the reply has no listing inside it, so `FetchPDP` cannot
extract a product and the detail record stays without a description. Without help
these would be retried on every run.

`pull-description` now remembers them:

- A confirmed dead listing (PDP container present but no target) writes a tombstone
  `data/MarketplacePDPContainerQuery/unavailable_0_<id>.json` and is skipped on later
  runs.
- An **ambiguous** reply (fetched, no listing, not a rate limit) increments
  `data/MarketplacePDPContainerQuery/miss_0_<id>.json`; after `DefaultMissThreshold`
  (3) misses it becomes a tombstone too.
- The raw body of any non-product PDP reply is saved to
  `data/MarketplacePDPContainerQuery/debug/miss_<id>.json` so the payload can be
  inspected.
- The run summary reports how many listings were skipped as `unavailable`.
- `just description-stats` reports them separately as `unavailable`, so they no
  longer show up as `empty (null)`.
- `pull-description` treats a present `Description` (including an empty string) as
  done. When every remaining null description is already fetched or tombstoned it
  prints `nothing to pull: ...` and exits **before launching the browser**.

To retry them (for example after a listing is re-posted), run
`just run -action=pull_description -retry-unavailable` or delete the tombstone
files. A successful fetch clears the failure state and tombstone.


## Facebook throttling (`1675004`)

Facebook sometimes answers the PDP GraphQL request with HTTP 200 and
`{"errors":[{"message":"Rate limit exceeded","severity":"CRITICAL","code":1675004}]}`.
While this is active, Facebook also serves its error page ("This page isn't available
right now") for listing pages, so no PDP request is issued at all.

This is an account/session-level throttle (not something the browser can bypass):

- Requests are **paced** by default (`-delay-ms`, default 1000 ms) to avoid tripping it.
- On a rate-limited reply the run **waits and retries** with exponential backoff
  (`-rate-limit-wait`, default 300 s; `-max-rate-limit-wait`, default 1800 s) instead
  of aborting, so it self-recovers from short throttles.
- If the limit does not clear within the budget the run stops cleanly (exit 0) and all
  progress is saved; re-run later to continue. Do not hammer it — according to Meta's
  behaviour, repeated retries can extend the block.
- Rate-limited responses are not written to disk, and if one listing's page errors
  during template capture the tool tries the next listing.

If a run keeps yielding `pulled=0`, the account is under a longer block: wait (minutes
to hours) before retrying, and avoid running back-to-back.



