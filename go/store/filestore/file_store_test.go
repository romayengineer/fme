package filestore

import (
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/romayengineer/fme/go/marketplace/model"
)

func TestFileStoreGet(t *testing.T) {
	tmpDir := t.TempDir()

	productID := uuid.New().String()
	store := NewProductFileStore(productID)
	store.SetDir(tmpDir)

	// Create test data
	testData := model.MarketplaceItemDetails{
		ID:            "123",
		Title:         "Test Product",
		Description:   "Test Description",
		PriceAmount:   99.99,
		PriceCurrency: "USD",
		SellerID:      "seller-456",
		SellerName:    "Test Seller",
	}

	_, err := store.Save(testData)
	require.NoError(t, err)

	store2 := NewProductFileStore(productID)
	store2.SetDir(tmpDir)

	gotData, err := store2.Get()
	require.NoError(t, err)

	assert.NotNil(t, gotData)

	assert.Equal(t, testData.ID, gotData.ID)
	assert.Equal(t, testData.Title, gotData.Title)
	assert.Equal(t, testData.Description, gotData.Description)
	assert.Equal(t, testData.PriceAmount, gotData.PriceAmount)
	assert.Equal(t, testData.PriceCurrency, gotData.PriceCurrency)
	assert.Equal(t, testData.SellerID, gotData.SellerID)
	assert.Equal(t, testData.SellerName, gotData.SellerName)
}

func TestFileStoreSave(t *testing.T) {
	tmpDir := t.TempDir()

	productID := uuid.New().String()

	store := NewProductFileStore(productID)
	store.SetDir(tmpDir)

	data, err := store.Get()

	assert.NoError(t, err)
	assert.NotNil(t, data)

	assert.Nil(t, data.ID)
	assert.Nil(t, data.Title)
	assert.Nil(t, data.Description)

	testData := model.MarketplaceItemDetails{
		ID:          "123",
		Title:       "Test Product",
		Description: "Test Description",
	}

	_, err = store.Save(testData)
	assert.NoError(t, err)

	store2 := NewProductFileStore(productID)
	store2.SetDir(tmpDir)

	data2, err := store2.Get()

	assert.NoError(t, err)
	assert.NotNil(t, data2)

	assert.Equal(t, testData.ID, data2.ID)
	assert.Equal(t, testData.Title, data2.Title)
	assert.Equal(t, testData.Description, data2.Description)

	// update
	newProductID := uuid.New().String()
	testData.ID = newProductID

	_, err = store2.Save(testData)
	assert.NoError(t, err)

	data3, err := store2.Get()
	assert.NoError(t, err)
	assert.Equal(t, newProductID, data3.ID)
}

func TestFileStoreConcurrentSavesPreserveFields(t *testing.T) {
	tmpDir := t.TempDir()
	productID := uuid.New().String()

	patches := []model.MarketplaceItemDetails{
		{ID: "123", Title: "title"},
		{ID: "123", Description: "description"},
		{ID: "123", PriceCurrency: "USD"},
		{ID: "123", SellerName: "seller"},
		{ID: "123", LocationGeocodeCityName1: "city"},
	}

	start := make(chan struct{})
	errCh := make(chan error, len(patches))
	var wg sync.WaitGroup

	for _, patch := range patches {
		wg.Add(1)
		go func(patch model.MarketplaceItemDetails) {
			defer wg.Done()
			<-start

			store := NewProductFileStore(productID)
			store.SetDir(tmpDir)
			if _, err := store.Save(patch); err != nil {
				errCh <- err
			}
		}(patch)
	}

	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent Save: %v", err)
	}

	store := NewProductFileStore(productID)
	store.SetDir(tmpDir)
	got, err := store.Get()
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, "title", got.Title)
	assert.Equal(t, "description", got.Description)
	assert.Equal(t, "USD", got.PriceCurrency)
	assert.Equal(t, "seller", got.SellerName)
	assert.Equal(t, "city", got.LocationGeocodeCityName1)
}
