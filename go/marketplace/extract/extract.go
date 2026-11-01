// Package extract maps Facebook GraphQL payloads to Marketplace products.
package extract

import (
	"errors"
	"slices"
	"strings"

	"github.com/romayengineer/fme/go/core/jsonutil"
	"github.com/romayengineer/fme/go/marketplace/model"
)

// ProductExtractor pairs a payload validator with the extractor that knows how
// to read that payload shape.
type ProductExtractor struct {
	validator func(data any) bool
	extractor func(data any) ([]model.MarketplaceItemDetails, error)
}

// ProductExtractors is the ordered set of payload shapes ProcessData can read.
type ProductExtractors struct {
	extractors []ProductExtractor
}

// NewProductExtractors returns the extractors for every payload shape the
// scraper persists (PDP, search feed, and entity data).
func NewProductExtractors() ProductExtractors {
	return ProductExtractors{
		extractors: []ProductExtractor{
			{
				validator: ProductDetailsValid,
				extractor: ProductDetailsGet,
			},
			{
				validator: ProductsFromSearchValid,
				extractor: ProductsFromSearchGet,
			},
			{
				validator: ProductFromDataValid,
				extractor: ProductFromDataGet,
			},
		},
	}
}

// ExtractAll runs the registered extractors in order and returns the products from
// the first one that yields any, mirroring the original inline loop in ProcessData.
func (pe ProductExtractors) ExtractAll(data any) []model.MarketplaceItemDetails {
	for _, e := range pe.extractors {
		products, err := e.extractor(data)
		if err != nil || len(products) == 0 {
			continue
		}
		return products
	}
	return nil
}

// IsErrorRateLimit reports whether the payload is Facebook's rate-limit error
// response, which must not be persisted as if it were a listing.
func IsErrorRateLimit(data any) bool {
	errorsList, ok := jsonutil.GetKey(data, "errors").([]any)
	if !ok {
		return false
	}

	for _, err := range errorsList {
		message, ok := jsonutil.GetKey(err, "message").(string)
		if !ok {
			continue
		}

		if strings.EqualFold(message, "rate limit exceeded") {
			return true
		}
	}

	return false
}

// ProductGone reports whether a Marketplace PDP payload proves the listing no
// longer exists. Facebook returns the PDP container
// (data.viewer.marketplace_product_details_page) but with no listing inside it when
// the item has been removed. Detection is deliberately narrow so a login wall, a
// rate-limit reply, or any other non-PDP payload is never treated as gone.
//
// The returned reason is a short tag for logging ("null_container" or "no_target").
func ProductGone(data any) (reason string, ok bool) {
	container, present := jsonutil.Lookup(data, "data.viewer.marketplace_product_details_page")
	if !present {
		return "", false
	}
	if container == nil {
		return "null_container", true
	}

	id, hasID := jsonutil.Lookup(data, "data.viewer.marketplace_product_details_page.target.id")
	if !hasID || id == nil {
		return "no_target", true
	}
	return "", false
}

// ProductDetailsValid reports whether the GraphQL payload is a Marketplace PDP
// (product detail page) response. See ProductDetailsGet for the requirements to
// obtain a PDP response containing `redacted_description`.
func ProductDetailsValid(data any) bool {
	id, ok := jsonutil.GetKey(data, "data.viewer.marketplace_product_details_page.target.id").(string)
	return ok && id != ""
}

// GetTaxonomyPathJoined joins the listing's SEO category URLs into one
// comma-separated value, or returns nil when the payload has no taxonomy path.
func GetTaxonomyPathJoined(detail any) any {
	taxonomyPathList, ok := jsonutil.GetKey(detail, "marketplace_listing_renderable_target.seo_virtual_category.taxonomy_path").([]any)
	if !ok {
		return nil
	}

	names := taxonomyNames(taxonomyPathList)
	if len(names) == 0 {
		return nil
	}
	return strings.Join(names, ",")
}

// taxonomyNames collects the non-empty SEO URLs from taxonomy path entries.
func taxonomyNames(entries []any) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if name, ok := taxonomyName(entry); ok {
			names = append(names, name)
		}
	}
	return names
}

// taxonomyName returns one taxonomy entry's SEO URL when it is a non-empty string.
func taxonomyName(entry any) (string, bool) {
	name, ok := jsonutil.GetKey(entry, "seo_info.seo_url").(string)
	return name, ok && name != ""
}

// GetLocationAttrs reads the latitude, longitude, and reverse-geocoded city
// attributes from a listing location payload.
func GetLocationAttrs(location any) model.Location {
	return model.Location{
		LocationLatitud:          jsonutil.GetKey(location, "latitude"),
		LocationLongitude:        jsonutil.GetKey(location, "longitude"),
		LocationGeocodeCityName1: jsonutil.GetKey(location, "reverse_geocode.city"),
		LocationGeocodeCityName2: jsonutil.GetKey(location, "reverse_geocode.city_page.display_name"),
		LocationGeocodeCityID:    jsonutil.GetKey(location, "reverse_geocode.city_page.id"),
		LocationGeocodeStateCode: jsonutil.GetKey(location, "reverse_geocode.state"),
	}
}

// ProductDetailsGet maps a Facebook Marketplace PDP (product detail page) GraphQL
// response to MarketplaceItemDetails.
//
// The field we care about most is Description, which Facebook returns as
// `redacted_description` (the lead text shown at the top of a listing). Obtaining it
// has hard requirements:
//
//  1. Authenticated session. The request must run against a logged-in Facebook
//     session; otherwise the PDP response is a login wall with no description.
//     In practice this means launching with a Chrome profile that already has the
//     session (FME_USER_DATA_DIR) - see docs/redacted_description.md.
//
//  2. A real PDP request must be triggered. `MarketplacePDPContainerQuery` is only
//     sent when a listing is actually opened in the browser via a client-side click
//     on a search result. A plain URL navigation (Goto) does NOT reliably trigger it,
//     which is why Pages.OpenFirstProduct clicks the first item link after the search.
//
//  3. The response must be intercepted. ContextEventHandlers.OnResponse captures
//     requests to https://www.facebook.com/api/graphql/ and writes the body to
//     data/MarketplacePDPContainerQuery/*.json (the friendly name is read from the
//     request post-data field `fb_api_req_friendly_name`).
//
//  4. The description is read from the dotted path:
//     data.viewer.marketplace_product_details_page.target.redacted_description.text
//
//  5. ProcessData merges non-nil fields into data/__Details/detail_<id>.json, so the
//     description is added to the listing record already created from the search feed.
//
// The same intercepted request also provides the reusable headers + post-data that
// FetchPDP replays (with variables.targetId replaced) to fetch details for the other
// listings in the pull_description flow.
func ProductDetailsGet(data any) ([]model.MarketplaceItemDetails, error) {
	detail := jsonutil.GetKey(data, "data.viewer.marketplace_product_details_page")
	if detail == nil {
		return nil, errors.New("detail not found")
	}

	productID, ok := jsonutil.GetKey(detail, "target.id").(string)
	if !ok || productID == "" {
		return nil, errors.New("detail does not have a valid id")
	}

	return []model.MarketplaceItemDetails{productDetailsFrom(detail, productID)}, nil
}

// productDetailsFrom maps one PDP detail object to the persisted listing.
func productDetailsFrom(detail any, productID string) model.MarketplaceItemDetails {
	productURL := jsonutil.GetKey(detail, "target.story.url")
	if productURL == nil {
		productURL = model.ProductIDToLink(productID)
	}

	location := GetLocationAttrs(jsonutil.GetKey(detail, "marketplace_listing_renderable_target.location"))

	return model.MarketplaceItemDetails{
		ID:                       productID,
		TaxonomiPathJoined:       GetTaxonomyPathJoined(detail),
		IDLong:                   jsonutil.GetKey(detail, "target.product_item.id"),
		URL:                      productURL,
		Title:                    jsonutil.GetKey(detail, "target.marketplace_listing_title"),
		Description:              jsonutil.GetKey(detail, "target.redacted_description.text"),
		PriceAmount:              jsonutil.GetKey(detail, "target.listing_price.amount"),
		PriceCurrency:            jsonutil.GetKey(detail, "target.listing_price.currency"),
		AttributeData:            jsonutil.GetKey(detail, "target.attribute_data"),
		CreationTime:             jsonutil.GetKey(detail, "target.creation_time"),
		SellerID:                 jsonutil.GetKey(detail, "target.marketplace_listing_seller.id"),
		SellerName:               jsonutil.GetKey(detail, "target.marketplace_listing_seller.name"),
		Photos:                   GetPhotosURI(jsonutil.GetKey(detail, "target.listing_photos")),
		LocationLatitud:          location.LocationLatitud,
		LocationLongitude:        location.LocationLongitude,
		LocationGeocodeCityName1: location.LocationGeocodeCityName1,
		LocationGeocodeCityName2: location.LocationGeocodeCityName2,
		LocationGeocodeStateCode: location.LocationGeocodeStateCode,
		LocationGeocodeCityID:    location.LocationGeocodeCityID,
	}
}

// ProductsFromSearchValid reports whether the payload is a Marketplace search
// feed that contains at least one listing with an id.
func ProductsFromSearchValid(data any) bool {
	edgesList, ok := jsonutil.GetKey(data, "data.marketplace_search.feed_units.edges").([]any)
	if !ok {
		return false
	}

	return slices.ContainsFunc(edgesList, searchEdgeHasID)
}

// searchEdgeHasID reports whether a search edge contains a listing id.
func searchEdgeHasID(edge any) bool {
	listing := jsonutil.GetKey(edge, "node.listing")
	if listing == nil {
		return false
	}

	productID, ok := jsonutil.GetKey(listing, "id").(string)
	return ok && productID != ""
}

// GetPhotoURI returns the encoded image URI from one listing photo. The value
// is left escaped because re-marshaling the listing escapes it again.
func GetPhotoURI(photo any) any {
	return jsonutil.GetKey(photo, "image.uri")
}

// GetPhotosURI returns the image URI of every photo, or nil for a non-list.
func GetPhotosURI(photos any) any {
	photosList, ok := photos.([]any)
	if !ok {
		return nil
	}
	photosListURIs := []any{}
	for _, photo := range photosList {
		photosListURIs = append(photosListURIs, GetPhotoURI(photo))
	}
	return photosListURIs
}

// ProductsFromSearchGet maps a Marketplace search feed payload to the listings
// it contains.
func ProductsFromSearchGet(data any) ([]model.MarketplaceItemDetails, error) {
	edges := jsonutil.GetKey(data, "data.marketplace_search.feed_units.edges")
	if edges == nil {
		return nil, errors.New("no marketplace search found")
	}

	edgesList, ok := edges.([]any)
	if !ok {
		return nil, errors.New("edges is not a list")
	}

	products := make([]model.MarketplaceItemDetails, 0, len(edgesList))
	for _, edge := range edgesList {
		if listing, ok := searchListingFromEdge(edge); ok {
			products = append(products, listing)
		}
	}

	return products, nil
}

// searchListingFromEdge maps one search feed edge to a listing, dropping edges
// without a valid listing id.
func searchListingFromEdge(edge any) (model.MarketplaceItemDetails, bool) {
	listing := jsonutil.GetKey(edge, "node.listing")
	if listing == nil {
		return model.MarketplaceItemDetails{}, false
	}

	productID, ok := jsonutil.GetKey(listing, "id").(string)
	if !ok || productID == "" {
		return model.MarketplaceItemDetails{}, false
	}

	location := GetLocationAttrs(jsonutil.GetKey(listing, "location"))

	return model.MarketplaceItemDetails{
		ID:                       productID,
		URL:                      model.ProductIDToLink(productID),
		Title:                    jsonutil.GetKey(listing, "marketplace_listing_title"),
		CreationTime:             jsonutil.GetKey(listing, "if_gk_just_listed_tag_on_search_feed.creation_time"),
		PriceAmount:              jsonutil.GetKey(listing, "listing_price.amount"),
		LocationLatitud:          location.LocationLatitud,
		LocationLongitude:        location.LocationLongitude,
		LocationGeocodeCityName1: location.LocationGeocodeCityName1,
		LocationGeocodeCityName2: location.LocationGeocodeCityName2,
		LocationGeocodeStateCode: location.LocationGeocodeStateCode,
		LocationGeocodeCityID:    location.LocationGeocodeCityID,
		SellerID:                 jsonutil.GetKey(listing, "marketplace_listing_seller.id"),
		SellerName:               jsonutil.GetKey(listing, "marketplace_listing_seller.name"),
		PhotoPrimary:             GetPhotoURI(jsonutil.GetKey(listing, "primary_listing_photo")),
		DeliveryTypes:            jsonutil.GetKey(listing, "delivery_types"),
		IsHidden:                 jsonutil.GetKey(listing, "is_hidden"),
		IsLive:                   jsonutil.GetKey(listing, "is_live"),
		IsPending:                jsonutil.GetKey(listing, "is_pending"),
		IsSold:                   jsonutil.GetKey(listing, "is_sold"),
	}, true
}

// ProductFromDataValid reports whether the payload is an entity-data response
// that contains an entity id.
func ProductFromDataValid(data any) bool {
	id, ok := jsonutil.GetKey(data, "data.node.entity_id").(string)
	return ok && id != ""
}

// ProductFromDataGet maps an entity-data payload to the listing it describes.
func ProductFromDataGet(data any) ([]model.MarketplaceItemDetails, error) {
	node := jsonutil.GetKey(data, "data.node")
	if node == nil {
		return nil, errors.New("data node not found")
	}

	productID, ok := jsonutil.GetKey(node, "entity_id").(string)
	if !ok || productID == "" {
		return nil, errors.New("product does not have a valid id")
	}

	productCategory := jsonutil.GetKey(node, "data.upsell_type")

	// The miscellaneous upsell category is noise, not a real product category.
	if productCategory == "CATEGORY_MISCELLANEOUS_UPSELL" {
		productCategory = nil
	}

	location := GetLocationAttrs(jsonutil.GetKey(node, "entity.location"))

	listing := model.MarketplaceItemDetails{
		ID:                       productID,
		URL:                      model.ProductIDToLink(productID),
		IDLong:                   jsonutil.GetKey(node, "data.product_item_id"),
		Category:                 productCategory,
		Title:                    jsonutil.GetKey(node, "data.title"),
		PriceCurrency:            jsonutil.GetKey(node, "data.price.currency"),
		LocationLatitud:          location.LocationLatitud,
		LocationLongitude:        location.LocationLongitude,
		LocationGeocodeCityName1: location.LocationGeocodeCityName1,
		LocationGeocodeCityName2: location.LocationGeocodeCityName2,
		LocationGeocodeStateCode: location.LocationGeocodeStateCode,
		LocationGeocodeCityID:    location.LocationGeocodeCityID,
		CreationTime:             jsonutil.GetKey(node, "listing.creation_time"),
	}

	return []model.MarketplaceItemDetails{listing}, nil
}
