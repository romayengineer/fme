package db

import (
	"fmt"

	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/models/schema"

	"github.com/romayengineer/fme/go/core/logging"
)

// productField describes one column of the products collection.
type productField struct {
	name string
	typ  string
}

// ensureProductsCollection creates the products collection when it is missing.
func (db *PocketBaseDB) ensureProductsCollection() error {
	if _, err := db.app.Dao().FindCollectionByNameOrId("products"); err == nil {
		logging.LogDebug0("ensureProductsCollection", "products collection already exists")
		return nil
	}

	logging.LogInfo0("ensureProductsCollection", "creating products collection")
	if err := db.app.Dao().SaveCollection(productsCollection()); err != nil {
		return fmt.Errorf("failed to create products collection: %w", err)
	}

	logging.LogInfo0("ensureProductsCollection", "products collection created successfully")
	return nil
}

// productsCollection builds the products collection, its schema, and indexes.
func productsCollection() *models.Collection {
	collection := &models.Collection{
		Name:   "products",
		Type:   "base",
		Schema: productsSchema(),
	}
	// A unique index on facebook_id keeps upserts fast and duplicate-free.
	collection.Indexes = append(collection.Indexes, "CREATE UNIQUE INDEX IF NOT EXISTS `idx_facebook_id` ON `products` (facebook_id)")
	return collection
}

// productsSchema builds the schema from the field list.
func productsSchema() schema.Schema {
	s := schema.Schema{}
	for _, f := range productFields() {
		s.AddField(&schema.SchemaField{Name: f.name, Type: f.typ})
	}
	return s
}

// productFields lists every column persisted for a product.
func productFields() []productField {
	return []productField{
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
}
