// Package model defines the persisted Marketplace listing types shared by the
// extractors, stores, and CLI.
package model

import "fmt"

// AttributeData is one key/value pair from a listing's attribute block.
type AttributeData struct {
	AttributeName string
	Label         string
	Value         string
}

// ItemLocation is a plain latitude/longitude pair.
type ItemLocation struct {
	Latitude  float64
	Longitude float64
}

// Location holds the raw location fields extracted from a listing payload.
type Location struct {
	LocationLatitud          any
	LocationLongitude        any
	LocationGeocodeCityID    any
	LocationGeocodeCityName1 any
	LocationGeocodeCityName2 any
	LocationGeocodeStateCode any
}

// MarketplaceItemDetails is one listing as extracted from Facebook.
//
// The field names are also the persisted JSON keys (the type has no json tags),
// so renaming a field changes the on-disk format. Update fixtures and add a
// migration before renaming any field.
type MarketplaceItemDetails struct {
	ID                       any
	TaxonomiPathJoined       any
	IDLong                   any
	Category                 any
	URL                      any
	Title                    any
	Description              any
	PriceAmount              any
	PriceCurrency            any
	AttributeData            any
	CreationTime             any
	LocationLatitud          any
	LocationLongitude        any
	LocationGeocodeCityID    any
	LocationGeocodeCityName1 any
	LocationGeocodeCityName2 any
	LocationGeocodeStateCode any
	SellerID                 any
	SellerName               any
	Photos                   any
	PhotoPrimary             any
	DeliveryTypes            any
	IsHidden                 any
	IsLive                   any
	IsPending                any
	IsSold                   any
}

// ToMarketplaceItemDetails converts a persisted detail map back to a listing.
func ToMarketplaceItemDetails(data map[string]any) MarketplaceItemDetails {
	return MarketplaceItemDetails{
		ID:                       data["ID"],
		TaxonomiPathJoined:       data["TaxonomiPathJoined"],
		IDLong:                   data["IDLong"],
		Category:                 data["Category"],
		URL:                      data["URL"],
		Title:                    data["Title"],
		Description:              data["Description"],
		PriceAmount:              data["PriceAmount"],
		PriceCurrency:            data["PriceCurrency"],
		AttributeData:            data["AttributeData"],
		CreationTime:             data["CreationTime"],
		SellerID:                 data["SellerID"],
		SellerName:               data["SellerName"],
		Photos:                   data["Photos"],
		PhotoPrimary:             data["PhotoPrimary"],
		DeliveryTypes:            data["DeliveryTypes"],
		IsHidden:                 data["IsHidden"],
		IsLive:                   data["IsLive"],
		IsPending:                data["IsPending"],
		IsSold:                   data["IsSold"],
		LocationLatitud:          data["LocationLatitud"],
		LocationLongitude:        data["LocationLongitude"],
		LocationGeocodeCityID:    data["LocationGeocodeCityID"],
		LocationGeocodeCityName1: data["LocationGeocodeCityName1"],
		LocationGeocodeCityName2: data["LocationGeocodeCityName2"],
		LocationGeocodeStateCode: data["LocationGeocodeStateCode"],
	}
}

// ProductIDToLink returns the public Marketplace URL for a listing id.
func ProductIDToLink(productID string) string {
	return fmt.Sprintf("https://www.facebook.com/marketplace/item/%s/", productID)
}
