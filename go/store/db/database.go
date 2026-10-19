package db

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/mozillazg/go-unidecode"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/migrations/logs"
	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/models/schema"
	"github.com/pocketbase/pocketbase/tools/migrate"
	"github.com/romayengineer/fme/go/core/logging"
	"github.com/romayengineer/fme/go/marketplace/model"
	"github.com/romayengineer/fme/go/store/filestore"
)

type PocketBaseDB struct {
	app *pocketbase.PocketBase
	mu  sync.Mutex
}

var dbInstance *PocketBaseDB
var dbOnce sync.Once

func GetPocketBaseDB() (*PocketBaseDB, error) {
	var err error
	dbOnce.Do(func() {
		pb := pocketbase.NewWithConfig(pocketbase.Config{
			DefaultDataDir: dataDir(),
		})
		dbInstance = &PocketBaseDB{
			app: pb,
		}
	})
	return dbInstance, err
}

// dataDir resolves the PocketBase data directory. PocketBase defaults it to a
// path next to the executable, which under `go run` is the Go build cache — so
// `just save`/`just serve` would scatter the database outside the project.
// Anchoring it to the working directory (the repo root under just) keeps pb_data
// where .gitignore expects it. Override with FME_PB_DATA_DIR.
func dataDir() string {
	if dir := os.Getenv("FME_PB_DATA_DIR"); dir != "" {
		return dir
	}
	wd, err := os.Getwd()
	if err != nil {
		return "pb_data"
	}
	return filepath.Join(wd, "pb_data")
}

func (db *PocketBaseDB) Start() error {
	if db.app == nil {
		return fmt.Errorf("pocketbase app not initialized")
	}

	// Bootstrap creates the database tables if they don't exist
	if err := db.app.Bootstrap(); err != nil {
		logging.LogError0("Start", "bootstrap failed", "error", err)
		return fmt.Errorf("failed to bootstrap pocketbase: %v", err)
	}

	// Bootstrap only opens the databases; the built-in migrations (which create
	// `_collections` and friends) are applied by the serve command, not by Bootstrap.
	// Running them here makes `just save` work on a fresh clone without needing
	// `just serve` first. Runner.Up is idempotent, so this is safe on every start.
	if err := db.runMigrations(); err != nil {
		logging.LogError0("Start", "migrations failed", "error", err)
		return fmt.Errorf("failed to run pocketbase migrations: %v", err)
	}

	// Ensure products collection exists
	if err := db.ensureProductsCollection(); err != nil {
		logging.LogError0("Start", "failed to ensure products collection", "error", err)
		return fmt.Errorf("failed to ensure products collection: %v", err)
	}

	logging.LogInfo0("Start", "database initialized successfully")
	return nil
}

// runMigrations applies PocketBase's app and logs migrations, mirroring what
// apis.Serve does before starting the HTTP server.
func (db *PocketBaseDB) runMigrations() error {
	connections := []struct {
		db   *dbx.DB
		list migrate.MigrationsList
	}{
		{db.app.DB(), migrations.AppMigrations},
		{db.app.LogsDB(), logs.LogsMigrations},
	}

	for _, c := range connections {
		runner, err := migrate.NewRunner(c.db, c.list)
		if err != nil {
			return err
		}
		if _, err := runner.Up(); err != nil {
			return err
		}
	}

	return nil
}

func (db *PocketBaseDB) ensureProductsCollection() error {
	// Try to find existing collection
	collection, err := db.app.Dao().FindCollectionByNameOrId("products")
	if err == nil {
		logging.LogDebug0("ensureProductsCollection", "products collection already exists")
		return nil
	}

	logging.LogInfo0("ensureProductsCollection", "creating products collection")

	// Create new collection
	collection = &models.Collection{
		Name: "products",
		Type: "base",
	}

	// Create schema with all fields
	s := schema.Schema{}

	fields := []struct {
		name string
		typ  string
	}{
		{"facebook_id", "text"},
		{"facebook_id_long", "text"},
		{"title", "text"},
		{"description", "text"},
		{"category", "text"},
		{"url", "url"},
		{"price_amount", "number"},
		{"price_currency", "text"},
		{"creation_time", "number"},
		{"location_latitude", "number"},
		{"location_longitude", "number"},
		{"location_city_id", "text"},
		{"location_city_name1", "text"},
		{"location_city_name2", "text"},
		{"location_state_code", "text"},
		{"seller_id", "text"},
		{"seller_name", "text"},
		{"is_hidden", "bool"},
		{"is_live", "bool"},
		{"is_pending", "bool"},
		{"is_sold", "bool"},
	}

	for _, f := range fields {
		s.AddField(&schema.SchemaField{
			Name: f.name,
			Type: f.typ,
		})
	}

	collection.Schema = s

	// Add Unique index on facebook_id for fast lookups
	collection.Indexes = append(collection.Indexes, "CREATE UNIQUE INDEX IF NOT EXISTS `idx_facebook_id` ON `products` (facebook_id)")

	// Save the collection to database
	if err := db.app.Dao().SaveCollection(collection); err != nil {
		return fmt.Errorf("failed to create products collection: %v", err)
	}

	logging.LogInfo0("ensureProductsCollection", "products collection created successfully")
	return nil
}

func SetProductFields(record *models.Record, product model.MarketplaceItemDetails) error {
	if record == nil {
		return fmt.Errorf("record is nil")
	}

	priceAmount, err := toFloat(product.PriceAmount)
	if err != nil {
		return fmt.Errorf("price_amount: %w", err)
	}
	creationTime, err := toInt64(product.CreationTime)
	if err != nil {
		return fmt.Errorf("creation_time: %w", err)
	}
	latitude, err := toFloat(product.LocationLatitud)
	if err != nil {
		return fmt.Errorf("location_latitude: %w", err)
	}
	longitude, err := toFloat(product.LocationLongitude)
	if err != nil {
		return fmt.Errorf("location_longitude: %w", err)
	}

	record.Set("facebook_id_long", toString(product.IDLong))
	record.Set("title", toStringClean(product.Title))
	record.Set("description", toStringClean(product.Description))
	record.Set("category", toString(product.Category))
	record.Set("url", toString(product.URL))
	record.Set("price_amount", priceAmount)
	record.Set("price_currency", toString(product.PriceCurrency))
	record.Set("creation_time", creationTime)
	record.Set("location_latitude", latitude)
	record.Set("location_longitude", longitude)
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

	return nil
}

func (db *PocketBaseDB) UpsertProduct(product model.MarketplaceItemDetails) (sql.Result, error) {
	facebookID := toString(product.ID)

	priceAmount, err := toFloat(product.PriceAmount)
	if err != nil {
		return nil, fmt.Errorf("price_amount: %w", err)
	}
	creationTime, err := toInt64(product.CreationTime)
	if err != nil {
		return nil, fmt.Errorf("creation_time: %w", err)
	}
	latitude, err := toFloat(product.LocationLatitud)
	if err != nil {
		return nil, fmt.Errorf("location_latitude: %w", err)
	}
	longitude, err := toFloat(product.LocationLongitude)
	if err != nil {
		return nil, fmt.Errorf("location_longitude: %w", err)
	}

	sql := `INSERT INTO products (
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

	result, err := db.app.Dao().DB().NewQuery(sql).Bind(map[string]interface{}{
		"facebook_id":         facebookID,
		"facebook_id_long":    toString(product.IDLong),
		"title":               toStringClean(product.Title),
		"description":         toStringClean(product.Description),
		"category":            toString(product.Category),
		"url":                 toString(product.URL),
		"price_amount":        priceAmount,
		"price_currency":      toString(product.PriceCurrency),
		"creation_time":       creationTime,
		"location_latitude":   latitude,
		"location_longitude":  longitude,
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
	}).Execute()

	if err != nil {
		logging.LogError0("SaveProduct", "failed to upsert product", "facebook_id", facebookID, "error", err)
		return result, err
	}

	return result, nil
}

func (db *PocketBaseDB) GetProductID(product model.MarketplaceItemDetails) (string, error) {

	facebookID := toString(product.ID)

	var result struct {
		ID string
	}

	err := db.app.Dao().DB().NewQuery("SELECT id FROM products WHERE facebook_id = {:facebook_id} LIMIT 1").Bind(map[string]interface{}{
		"facebook_id": facebookID,
	}).One(&result)

	if err != nil {
		logging.LogError0("SaveProduct", "failed to retrieve product id", "facebook_id", facebookID, "error", err)
		return "", err
	}

	return result.ID, nil
}

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

func (db *PocketBaseDB) GetApp() *pocketbase.PocketBase {
	return db.app
}

func (db *PocketBaseDB) Close() error {
	return nil
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

func toFloat(v any) (float64, error) {
	if v == nil {
		return 0, nil
	}

	switch val := v.(type) {
	case float64:
		return val, nil
	case float32:
		return float64(val), nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case string:
		num, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return 0, fmt.Errorf("parse %q as float: %w", val, err)
		}
		return num, nil
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", v)
	}
}

func toInt64(v any) (int64, error) {
	if v == nil {
		return 0, nil
	}

	switch val := v.(type) {
	case int64:
		return val, nil
	case int:
		return int64(val), nil
	case float64:
		return int64(val), nil
	case string:
		num, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse %q as int64: %w", val, err)
		}
		return num, nil
	default:
		return 0, fmt.Errorf("cannot convert %T to int64", v)
	}
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

func Serve() (*PocketBaseDB, error) {

	// need to override the flags because pocketbase reads them
	os.Args = os.Args[:1]
	os.Args = append(os.Args, "serve")

	dbInstance, err := GetPocketBaseDB()
	if err != nil {
		return dbInstance, err
	}
	err = dbInstance.app.Start()
	if err != nil {
		return dbInstance, err
	}
	return dbInstance, err
}

func ProcessDataInDB() (int64, error) {
	var filesProcessedCounter int64
	var firstErr error

	dbInstance, err := GetPocketBaseDB()
	if err != nil {
		return filesProcessedCounter, err
	}
	err = dbInstance.Start()
	if err != nil {
		return filesProcessedCounter, err
	}

	filestore.ForEachDetail(func(filePath string, jsonData map[string]any) bool {
		product := model.ToMarketplaceItemDetails(jsonData)
		productIdDb, err := dbInstance.SaveProduct(product)

		if err != nil {
			logging.LogError0("ProcessDataInDB", "error saving product", "file", filePath, "facebook_id", product.ID, "error", err)
			if firstErr == nil {
				firstErr = err
			}
			return false
		}

		logging.LogInfo0("ProcessDataInDB", "saved product in db", "productIdDb", productIdDb, "product.ID", product.ID)

		filesProcessedCounter += 1

		return true

	}, true)

	logging.LogInfo0("ProcessDataInDB", "all files processed", "filesProcessedCounter", filesProcessedCounter)

	return filesProcessedCounter, firstErr
}
