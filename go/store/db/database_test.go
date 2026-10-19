package db

import (
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/romayengineer/fme/go/marketplace/model"
	"github.com/stretchr/testify/require"
)

func newTestDB(t *testing.T) *PocketBaseDB {
	t.Helper()

	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	database := &PocketBaseDB{app: app}
	require.NoError(t, database.Start())

	t.Cleanup(func() {
		if app.DB() != nil {
			_ = app.DB().Close()
		}
	})

	return database
}

func TestSaveProductAndProductExists(t *testing.T) {
	database := newTestDB(t)

	product := model.MarketplaceItemDetails{
		ID:          "123",
		Title:       "Test Product",
		PriceAmount: 10.5,
	}

	productID, err := database.SaveProduct(product)
	require.NoError(t, err)
	require.NotEmpty(t, productID)

	exists, err := database.ProductExists("123")
	require.NoError(t, err)
	require.True(t, exists)

	exists, err = database.ProductExists("missing")
	require.NoError(t, err)
	require.False(t, exists)
}

func TestSaveProductUpdatesExistingRow(t *testing.T) {
	database := newTestDB(t)

	product := model.MarketplaceItemDetails{ID: "123", Title: "First Title"}
	_, err := database.SaveProduct(product)
	require.NoError(t, err)

	product.Title = "Second Title"
	_, err = database.SaveProduct(product)
	require.NoError(t, err)

	var got struct {
		Title string `db:"title"`
	}
	err = database.app.Dao().DB().
		NewQuery("SELECT title FROM products WHERE facebook_id = {:id}").
		Bind(map[string]any{"id": "123"}).
		One(&got)
	require.NoError(t, err)
	require.Equal(t, "second title", got.Title)
}

func TestSaveProductPropagatesDatabaseErrors(t *testing.T) {
	database := newTestDB(t)
	require.NoError(t, database.app.DB().Close())

	_, err := database.SaveProduct(model.MarketplaceItemDetails{ID: "123"})
	require.Error(t, err)
}

func TestSaveProductsPropagatesDatabaseErrors(t *testing.T) {
	database := newTestDB(t)
	require.NoError(t, database.app.DB().Close())

	count, err := database.SaveProducts([]model.MarketplaceItemDetails{
		{ID: "1"},
		{ID: "2"},
	})
	require.Error(t, err)
	require.Zero(t, count)
}

func TestProductExistsPropagatesDatabaseErrors(t *testing.T) {
	database := newTestDB(t)
	require.NoError(t, database.app.DB().Close())

	exists, err := database.ProductExists("123")
	require.Error(t, err)
	require.False(t, exists)
}
