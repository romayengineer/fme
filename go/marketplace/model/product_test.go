package model

import (
	"reflect"
	"testing"
)

func TestToMarketplaceItemDetailsMapsEveryField(t *testing.T) {
	data := map[string]any{
		"ID":                       "1",
		"TaxonomiPathJoined":       "vehicles,cars",
		"IDLong":                   "long-1",
		"Category":                 "cars",
		"URL":                      "https://example.test/1",
		"Title":                    "Café",
		"Description":              "Añadido",
		"PriceAmount":              100,
		"PriceCurrency":            "USD",
		"AttributeData":            []any{"a"},
		"CreationTime":             1700000000,
		"SellerID":                 "seller",
		"SellerName":               "Ana",
		"Photos":                   []any{"p1"},
		"PhotoPrimary":             "p1",
		"DeliveryTypes":            []any{"pickup"},
		"IsHidden":                 false,
		"IsLive":                   true,
		"IsPending":                false,
		"IsSold":                   false,
		"LocationLatitud":          -34.6,
		"LocationLongitude":        -58.4,
		"LocationGeocodeCityID":    "city",
		"LocationGeocodeCityName1": "Buenos Aires",
		"LocationGeocodeCityName2": "CABA",
		"LocationGeocodeStateCode": "CF",
	}

	got := ToMarketplaceItemDetails(data)
	want := wantProductDetails()

	if !reflect.DeepEqual(got, want) {
		t.Errorf("ToMarketplaceItemDetails() = %+v, want %+v", got, want)
	}
}

// wantProductDetails is the expected mapping of the test fixture.
func wantProductDetails() MarketplaceItemDetails {
	return MarketplaceItemDetails{
		ID:                       "1",
		TaxonomiPathJoined:       "vehicles,cars",
		IDLong:                   "long-1",
		Category:                 "cars",
		URL:                      "https://example.test/1",
		Title:                    "Café",
		Description:              "Añadido",
		PriceAmount:              100,
		PriceCurrency:            "USD",
		AttributeData:            []any{"a"},
		CreationTime:             1700000000,
		SellerID:                 "seller",
		SellerName:               "Ana",
		Photos:                   []any{"p1"},
		PhotoPrimary:             "p1",
		DeliveryTypes:            []any{"pickup"},
		IsHidden:                 false,
		IsLive:                   true,
		IsPending:                false,
		IsSold:                   false,
		LocationLatitud:          -34.6,
		LocationLongitude:        -58.4,
		LocationGeocodeCityID:    "city",
		LocationGeocodeCityName1: "Buenos Aires",
		LocationGeocodeCityName2: "CABA",
		LocationGeocodeStateCode: "CF",
	}
}

func TestToMarketplaceItemDetailsMissingKeysAreNil(t *testing.T) {
	got := ToMarketplaceItemDetails(map[string]any{})

	if got.ID != nil || got.Title != nil || got.LocationLatitud != nil {
		t.Errorf("missing keys mapped to non-nil values: %+v", got)
	}
}

func TestProductIDToLink(t *testing.T) {
	if got, want := ProductIDToLink("123"), "https://www.facebook.com/marketplace/item/123/"; got != want {
		t.Errorf("ProductIDToLink = %q, want %q", got, want)
	}
}
