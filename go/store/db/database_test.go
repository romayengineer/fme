package db

import (
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/stretchr/testify/require"

	"github.com/romayengineer/fme/go/marketplace/model"
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

func TestGetProductIDNotFound(t *testing.T) {
	database := newTestDB(t)

	_, err := database.GetProductID(model.MarketplaceItemDetails{ID: "missing"})
	require.ErrorIs(t, err, ErrNotFound)
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

func TestToFloat(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		want    float64
		wantErr bool
	}{
		{name: "nil", value: nil, want: 0},
		{name: "float64", value: 1.5, want: 1.5},
		{name: "float32", value: float32(2.5), want: 2.5},
		{name: "int", value: 3, want: 3},
		{name: "int64", value: int64(4), want: 4},
		{name: "string", value: "5.5", want: 5.5},
		{name: "bad string", value: "nope", wantErr: true},
		{name: "bool", value: true, wantErr: true},
		{name: "list", value: []any{}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toFloat(tt.value)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.InDelta(t, tt.want, got, 1e-9)
		})
	}
}

func TestToInt64(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		want    int64
		wantErr bool
	}{
		{name: "nil", value: nil, want: 0},
		{name: "int64", value: int64(1), want: 1},
		{name: "int", value: 2, want: 2},
		{name: "float64", value: 3.9, want: 3},
		{name: "string", value: "42", want: 42},
		{name: "bad string", value: "4.2", wantErr: true},
		{name: "bool", value: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toInt64(tt.value)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestUpsertProductRejectsInvalidNumbers(t *testing.T) {
	database := newTestDB(t)

	tests := []struct {
		name    string
		product model.MarketplaceItemDetails
	}{
		{name: "price amount", product: model.MarketplaceItemDetails{ID: "1", PriceAmount: "nope"}},
		{name: "creation time", product: model.MarketplaceItemDetails{ID: "2", CreationTime: "nope"}},
		{name: "latitude", product: model.MarketplaceItemDetails{ID: "3", LocationLatitud: true}},
		{name: "longitude", product: model.MarketplaceItemDetails{ID: "4", LocationLongitude: []any{}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := database.UpsertProduct(tt.product)
			require.Error(t, err)
		})
	}
}
