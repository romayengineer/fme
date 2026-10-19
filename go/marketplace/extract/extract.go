// Package extract maps Facebook GraphQL payloads to Marketplace products.
package extract

import (
	"fmt"
	"strings"

	"github.com/romayengineer/fme/go/core/jsonutil"
	"github.com/romayengineer/fme/go/marketplace/model"
)

type ProductExtractor struct {
	validator func(data any) bool
	extractor func(data any) ([]model.MarketplaceItemDetails, error)
}

type ProductExtractors struct {
	extractors []ProductExtractor
}

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
				validator: ProducFromDataValid,
				extractor: ProducFromDataGet,
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

func IsErrorRateLimit(data any) bool {
	errors := jsonutil.GetKey(data, "errors")

	errorsList, ok := errors.([]any)
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

func GetTaxonomiPathJoined(detail any) any {
	taxonomyPath := jsonutil.GetKey(detail, "marketplace_listing_renderable_target.seo_virtual_category.taxonomy_path")

	taxonomiNames := []string{}
	taxonomyPathList, ok := taxonomyPath.([]any)
	if ok {
		for _, taxonomi := range taxonomyPathList {
			taxonomiName, ok := jsonutil.GetKey(taxonomi, "seo_info.seo_url").(string)
			if !ok || taxonomiName == "" {
				continue
			}
			taxonomiNames = append(taxonomiNames, taxonomiName)
		}
	}
	var taxonomiPathJoined any
	if len(taxonomiNames) == 0 {
		taxonomiPathJoined = nil
	} else {
		taxonomiPathJoined = strings.Join(taxonomiNames, ",")
	}
	return taxonomiPathJoined
}

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
		return nil, fmt.Errorf("detail not found")
	}

	productID, ok := jsonutil.GetKey(detail, "target.id").(string)
	if !ok || productID == "" {
		return nil, fmt.Errorf("detail does not have a valid id")
	}

	var products []model.MarketplaceItemDetails

	productIdLong := jsonutil.GetKey(detail, "target.product_item.id")

	productURL := jsonutil.GetKey(detail, "target.story.url")
	if productURL == nil {
		productURL = model.ProductIDTolink(productID)
	}
	productTitle := jsonutil.GetKey(detail, "target.marketplace_listing_title")
	productDescription := jsonutil.GetKey(detail, "target.redacted_description.text")
	productPriceAmount := jsonutil.GetKey(detail, "target.listing_price.amount")
	productPriceCurrency := jsonutil.GetKey(detail, "target.listing_price.currency")
	productAttributeData := jsonutil.GetKey(detail, "target.attribute_data")
	productCreation := jsonutil.GetKey(detail, "target.creation_time")

	location := GetLocationAttrs(jsonutil.GetKey(detail, "marketplace_listing_renderable_target.location"))

	taxonomiPathJoined := GetTaxonomiPathJoined(detail)

	detailSellerId := jsonutil.GetKey(detail, "target.marketplace_listing_seller.id")
	detailSellerName := jsonutil.GetKey(detail, "target.marketplace_listing_seller.name")
	detailPhotos := GetPhotosURI(jsonutil.GetKey(detail, "target.listing_photos"))

	marketplaceItemDetails := model.MarketplaceItemDetails{
		ID:                       productID,
		TaxonomiPathJoined:       taxonomiPathJoined,
		IDLong:                   productIdLong,
		URL:                      productURL,
		Title:                    productTitle,
		Description:              productDescription,
		PriceAmount:              productPriceAmount,
		PriceCurrency:            productPriceCurrency,
		AttributeData:            productAttributeData,
		CreationTime:             productCreation,
		SellerID:                 detailSellerId,
		SellerName:               detailSellerName,
		Photos:                   detailPhotos,
		LocationLatitud:          location.LocationLatitud,
		LocationLongitude:        location.LocationLongitude,
		LocationGeocodeCityName1: location.LocationGeocodeCityName1,
		LocationGeocodeCityName2: location.LocationGeocodeCityName2,
		LocationGeocodeStateCode: location.LocationGeocodeStateCode,
		LocationGeocodeCityID:    location.LocationGeocodeCityID,
	}

	products = append(products, marketplaceItemDetails)

	return products, nil
}

func ProductsFromSearchValid(data any) bool {
	edges := jsonutil.GetKey(data, "data.marketplace_search.feed_units.edges")
	if edges == nil {
		return false
	}

	edgesList, ok := edges.([]any)
	if !ok {
		return false
	}

	for _, edge := range edgesList {
		listing := jsonutil.GetKey(edge, "node.listing")
		if listing == nil {
			continue
		}

		if productID, ok := jsonutil.GetKey(listing, "id").(string); ok && productID != "" {
			return true
		}
	}

	return false
}

func GetPhotoURI(photo any) any {
	uri := jsonutil.GetKey(photo, "image.uri")
	return uri
	// DO NOT UNSCAPE BECAUSE Marshall scapes it back
	// if uri == nil {
	// 	return nil
	// }
	// uriStr, ok := uri.(string)
	// if !ok {
	// 	return nil
	// }
	// unquoted, err := strconv.Unquote(uriStr)
	// if err != nil {
	// 	return uriStr
	// }
	// return unquoted
}

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

func ProductsFromSearchGet(data any) ([]model.MarketplaceItemDetails, error) {
	edges := jsonutil.GetKey(data, "data.marketplace_search.feed_units.edges")
	if edges == nil {
		return nil, fmt.Errorf("no marketplace search found")
	}

	edgesList, ok := edges.([]any)
	if !ok {
		return nil, fmt.Errorf("edges is not a list")
	}

	var products []model.MarketplaceItemDetails

	for _, edge := range edgesList {
		listing := jsonutil.GetKey(edge, "node.listing")
		if listing == nil {
			continue
		}

		productID, ok := jsonutil.GetKey(listing, "id").(string)
		if !ok || productID == "" {
			continue
		}

		productURL := model.ProductIDTolink(productID)

		title := jsonutil.GetKey(listing, "marketplace_listing_title")
		price := jsonutil.GetKey(listing, "listing_price.amount")
		time := jsonutil.GetKey(listing, "if_gk_just_listed_tag_on_search_feed.creation_time")

		isHidden := jsonutil.GetKey(listing, "is_hidden")
		isLive := jsonutil.GetKey(listing, "is_live")
		isPending := jsonutil.GetKey(listing, "is_pending")
		isSold := jsonutil.GetKey(listing, "is_sold")

		location := GetLocationAttrs(jsonutil.GetKey(listing, "location"))

		sellerId := jsonutil.GetKey(listing, "marketplace_listing_seller.id")
		sellerName := jsonutil.GetKey(listing, "marketplace_listing_seller.name")

		photoPrimary := GetPhotoURI(jsonutil.GetKey(listing, "primary_listing_photo"))

		productDeliveryTypes := jsonutil.GetKey(listing, "delivery_types")

		marketplaceItemDetails := model.MarketplaceItemDetails{
			ID:                       productID,
			URL:                      productURL,
			Title:                    title,
			CreationTime:             time,
			PriceAmount:              price,
			LocationLatitud:          location.LocationLatitud,
			LocationLongitude:        location.LocationLongitude,
			LocationGeocodeCityName1: location.LocationGeocodeCityName1,
			LocationGeocodeCityName2: location.LocationGeocodeCityName2,
			LocationGeocodeStateCode: location.LocationGeocodeStateCode,
			LocationGeocodeCityID:    location.LocationGeocodeCityID,
			SellerID:                 sellerId,
			SellerName:               sellerName,
			PhotoPrimary:             photoPrimary,
			DeliveryTypes:            productDeliveryTypes,
			IsHidden:                 isHidden,
			IsLive:                   isLive,
			IsPending:                isPending,
			IsSold:                   isSold,
		}

		products = append(products, marketplaceItemDetails)
	}

	return products, nil
}

func ProducFromDataValid(data any) bool {
	id, ok := jsonutil.GetKey(data, "data.node.entity_id").(string)
	return ok && id != ""
}

func ProducFromDataGet(data any) ([]model.MarketplaceItemDetails, error) {
	node := jsonutil.GetKey(data, "data.node")
	if node == nil {
		return nil, fmt.Errorf("data node not found")
	}

	productID, ok := jsonutil.GetKey(node, "entity_id").(string)
	if !ok || productID == "" {
		return nil, fmt.Errorf("product does not have a valid id")
	}

	var products []model.MarketplaceItemDetails

	productIdLong := jsonutil.GetKey(node, "data.product_item_id")

	productTitle := jsonutil.GetKey(node, "data.title")

	productCategory := jsonutil.GetKey(node, "data.upsell_type")

	// do not save this caterogy
	if productCategory != nil && productCategory == "CATEGORY_MISCELLANEOUS_UPSELL" {
		productCategory = nil
	}

	productURL := model.ProductIDTolink(productID)

	productPriceCurrency := jsonutil.GetKey(node, "data.price.currency")

	location := GetLocationAttrs(jsonutil.GetKey(node, "entity.location"))

	productCreation := jsonutil.GetKey(node, "listing.creation_time")

	marketplaceItemDetails := model.MarketplaceItemDetails{
		ID:                       productID,
		URL:                      productURL,
		IDLong:                   productIdLong,
		Category:                 productCategory,
		Title:                    productTitle,
		PriceCurrency:            productPriceCurrency,
		LocationLatitud:          location.LocationLatitud,
		LocationLongitude:        location.LocationLongitude,
		LocationGeocodeCityName1: location.LocationGeocodeCityName1,
		LocationGeocodeCityName2: location.LocationGeocodeCityName2,
		LocationGeocodeStateCode: location.LocationGeocodeStateCode,
		LocationGeocodeCityID:    location.LocationGeocodeCityID,
		CreationTime:             productCreation,
	}

	products = append(products, marketplaceItemDetails)

	return products, nil
}
