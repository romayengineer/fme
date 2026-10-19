package db

import (
	"fmt"

	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/models/schema"
	"github.com/romayengineer/fme/go/core/logging"
)

func (db *PocketBaseDB) ensureProductsCollection() error {
	// Try to find existing collection
	if _, err := db.app.Dao().FindCollectionByNameOrId("products"); err == nil {
		logging.LogDebug0("ensureProductsCollection", "products collection already exists")
		return nil
	}

	logging.LogInfo0("ensureProductsCollection", "creating products collection")

	// Create new collection
	collection := &models.Collection{
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
