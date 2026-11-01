package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/mozillazg/go-unidecode"
	"github.com/pocketbase/pocketbase/models"

	"github.com/romayengineer/fme/go/core/logging"
	"github.com/romayengineer/fme/go/marketplace/model"
)

// ErrNotFound reports that a product row does not exist.
var ErrNotFound = errors.New("product not found")

// productNumbers holds the converted numeric fields of a listing.
type productNumbers struct {
	priceAmount  float64
	creationTime int64
	latitude     float64
	longitude    float64
}

// productNumericFields converts the listing's numeric JSON scalars.
func productNumericFields(product model.MarketplaceItemDetails) (productNumbers, error) {
	var nums productNumbers
	var err error

	if nums.priceAmount, err = toFloat(product.PriceAmount); err != nil {
		return nums, fmt.Errorf("price_amount: %w", err)
	}
	if nums.creationTime, err = toInt64(product.CreationTime); err != nil {
		return nums, fmt.Errorf("creation_time: %w", err)
	}
	if nums.latitude, err = toFloat(product.LocationLatitud); err != nil {
		return nums, fmt.Errorf("location_latitude: %w", err)
	}
	if nums.longitude, err = toFloat(product.LocationLongitude); err != nil {
		return nums, fmt.Errorf("location_longitude: %w", err)
	}

	return nums, nil
}

// SetProductFields maps a listing onto a PocketBase record.
func SetProductFields(record *models.Record, product model.MarketplaceItemDetails) error {
	if record == nil {
		return errors.New("record is nil")
	}

	nums, err := productNumericFields(product)
	if err != nil {
		return err
	}

	setProductRecord(record, product, nums)
	return nil
}

// setProductRecord writes the listing fields onto a PocketBase record.
func setProductRecord(record *models.Record, product model.MarketplaceItemDetails, nums productNumbers) {
	record.Set("facebook_id_long", toString(product.IDLong))
	record.Set("title", toStringClean(product.Title))
	record.Set("description", toStringClean(product.Description))
	record.Set("category", toString(product.Category))
	record.Set("url", toString(product.URL))
	record.Set("price_amount", nums.priceAmount)
	record.Set("price_currency", toString(product.PriceCurrency))
	record.Set("creation_time", nums.creationTime)
	record.Set("location_latitude", nums.latitude)
	record.Set("location_longitude", nums.longitude)
	record.Set("location_city_id", toString(product.LocationGeocodeCityID))
	record.Set("location_city_name1", toString(product.LocationGeocodeCityName1))
	record.Set("location_city_name2", toString(product.LocationGeocodeCityName2))
	record.Set("location_state_code", toString(product.LocationGeocodeStateCode))
	record.Set("seller_id", toString(product.SellerID))
	record.Set("seller_name", toString(product.SellerName))
	record.Set("is_hidden", toBool(product.IsHidden))
	record.Set("is_live", toBool(product.IsLive))
	record.Set("is_pending", toBool(product.IsPending))
	record.Set("is_sold", toBool(product.IsSold))
}

// upsertProductSQL inserts a product or updates the row with the same facebook_id.
const upsertProductSQL = `INSERT INTO products (
		facebook_id, facebook_id_long, title, description, category, url,
		price_amount, price_currency, creation_time, location_latitude,
		location_longitude, location_city_id, location_city_name1,
		location_city_name2, location_state_code, seller_id, seller_name,
		is_hidden, is_live, is_pending, is_sold
	) VALUES ({:facebook_id}, {:facebook_id_long}, {:title}, {:description}, {:category}, {:url},
		{:price_amount}, {:price_currency}, {:creation_time}, {:location_latitude},
		{:location_longitude}, {:location_city_id}, {:location_city_name1},
		{:location_city_name2}, {:location_state_code}, {:seller_id}, {:seller_name},
		{:is_hidden}, {:is_live}, {:is_pending}, {:is_sold})
	ON CONFLICT(facebook_id) DO UPDATE SET
		facebook_id_long = excluded.facebook_id_long,
		title = excluded.title,
		description = excluded.description,
		category = excluded.category,
		url = excluded.url,
		price_amount = excluded.price_amount,
		price_currency = excluded.price_currency,
		creation_time = excluded.creation_time,
		location_latitude = excluded.location_latitude,
		location_longitude = excluded.location_longitude,
		location_city_id = excluded.location_city_id,
		location_city_name1 = excluded.location_city_name1,
		location_city_name2 = excluded.location_city_name2,
		location_state_code = excluded.location_state_code,
		seller_id = excluded.seller_id,
		seller_name = excluded.seller_name,
		is_hidden = excluded.is_hidden,
		is_live = excluded.is_live,
		is_pending = excluded.is_pending,
		is_sold = excluded.is_sold`

// UpsertProduct inserts or updates the product row keyed by facebook_id.
func (db *PocketBaseDB) UpsertProduct(product model.MarketplaceItemDetails) (sql.Result, error) {
	facebookID := toString(product.ID)

	nums, err := productNumericFields(product)
	if err != nil {
		return nil, err
	}

	result, err := db.app.Dao().DB().NewQuery(upsertProductSQL).
		Bind(productBindValues(product, facebookID, nums)).Execute()
	if err != nil {
		logging.LogError0("SaveProduct", "failed to upsert product", "facebook_id", facebookID, "error", err)
		return result, err
	}

	return result, nil
}

// productBindValues builds the upsert query parameters for one product.
func productBindValues(product model.MarketplaceItemDetails, facebookID string, nums productNumbers) map[string]any {
	return map[string]any{
		"facebook_id":         facebookID,
		"facebook_id_long":    toString(product.IDLong),
		"title":               toStringClean(product.Title),
		"description":         toStringClean(product.Description),
		"category":            toString(product.Category),
		"url":                 toString(product.URL),
		"price_amount":        nums.priceAmount,
		"price_currency":      toString(product.PriceCurrency),
		"creation_time":       nums.creationTime,
		"location_latitude":   nums.latitude,
		"location_longitude":  nums.longitude,
		"location_city_id":    toString(product.LocationGeocodeCityID),
		"location_city_name1": toString(product.LocationGeocodeCityName1),
		"location_city_name2": toString(product.LocationGeocodeCityName2),
		"location_state_code": toString(product.LocationGeocodeStateCode),
		"seller_id":           toString(product.SellerID),
		"seller_name":         toString(product.SellerName),
		"is_hidden":           toBool(product.IsHidden),
		"is_live":             toBool(product.IsLive),
		"is_pending":          toBool(product.IsPending),
		"is_sold":             toBool(product.IsSold),
	}
}

// GetProductID returns the PocketBase row id for the product's facebook_id.
func (db *PocketBaseDB) GetProductID(product model.MarketplaceItemDetails) (string, error) {
	facebookID := toString(product.ID)

	var result struct {
		ID string
	}

	err := db.app.Dao().DB().NewQuery("SELECT id FROM products WHERE facebook_id = {:facebook_id} LIMIT 1").Bind(map[string]any{
		"facebook_id": facebookID,
	}).One(&result)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		logging.LogError0("SaveProduct", "failed to retrieve product id", "facebook_id", facebookID, "error", err)
		return "", fmt.Errorf("get product id for facebook_id %s: %w", facebookID, err)
	}

	return result.ID, nil
}

// SaveProduct upserts one product and returns its PocketBase row id.
func (db *PocketBaseDB) SaveProduct(product model.MarketplaceItemDetails) (string, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	if _, err := db.UpsertProduct(product); err != nil {
		return "", fmt.Errorf("save product %v: %w", product.ID, err)
	}

	productID, err := db.GetProductID(product)
	if err != nil {
		return "", fmt.Errorf("get saved product id for %v: %w", product.ID, err)
	}

	logging.LogDebug0("SaveProduct", "product saved", "productID", productID, "facebook_id", product.ID)
	return productID, nil
}

// SaveProducts saves every product and returns how many succeeded, joined with
// the errors of the ones that failed.
func (db *PocketBaseDB) SaveProducts(products []model.MarketplaceItemDetails) (int, error) {
	count := 0
	var errs []error
	for _, product := range products {
		if _, err := db.SaveProduct(product); err != nil {
			logging.LogError0("SaveProducts", "error saving product", "facebook_id", product.ID, "error", err)
			errs = append(errs, err)
			continue
		}
		count++
	}
	logging.LogInfo0("SaveProducts", "batch save completed", "count", count, "total", len(products))
	return count, errors.Join(errs...)
}

// ProductExists reports whether a product with facebookID is already stored.
func (db *PocketBaseDB) ProductExists(facebookID string) (bool, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	collection, err := db.app.Dao().FindCollectionByNameOrId("products")
	if err != nil {
		return false, err
	}

	record, err := db.app.Dao().FindFirstRecordByData(collection.Id, "facebook_id", toString(facebookID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("find product by facebook_id %s: %w", facebookID, err)
	}

	return record != nil, nil
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func toStringClean(v any) string {
	if v == nil {
		return ""
	}
	value := fmt.Sprintf("%v", v)
	// Convert UTF-8 to ASCII using unidecode
	value = unidecode.Unidecode(value)
	// Convert to lowercase
	value = strings.ToLower(value)
	return value
}

// toFloat converts a JSON scalar to float64, parsing numeric strings.
func toFloat(v any) (float64, error) {
	if v == nil {
		return 0, nil
	}
	if str, ok := v.(string); ok {
		return parseFloat(str)
	}
	return floatValue(v)
}

// floatValue converts a non-string numeric value to float64.
func floatValue(v any) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case float32:
		return float64(val), nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	}
	return 0, fmt.Errorf("cannot convert %T to float64", v)
}

// parseFloat parses a decimal string as float64.
func parseFloat(str string) (float64, error) {
	num, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %q as float: %w", str, err)
	}
	return num, nil
}

// toInt64 converts a JSON scalar to int64, parsing numeric strings.
func toInt64(v any) (int64, error) {
	if v == nil {
		return 0, nil
	}
	if str, ok := v.(string); ok {
		return parseInt64(str)
	}
	return intValue(v)
}

// intValue converts a non-string numeric value to int64.
func intValue(v any) (int64, error) {
	switch val := v.(type) {
	case int64:
		return val, nil
	case int:
		return int64(val), nil
	case float64:
		return int64(val), nil
	}
	return 0, fmt.Errorf("cannot convert %T to int64", v)
}

// parseInt64 parses a decimal string as int64.
func parseInt64(str string) (int64, error) {
	num, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %q as int64: %w", str, err)
	}
	return num, nil
}

func toBool(v any) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case int:
		return val != 0
	default:
		return false
	}
}
