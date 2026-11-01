package filestore

import (
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/romayengineer/fme/go/marketplace/model"
)

// newTestStore returns a product store rooted at dir.
func newTestStore(dir, productID string) FileStore[model.MarketplaceItemDetails] {
	store := NewProductFileStore(productID)
	store.SetDir(dir)
	return store
}

func TestFileStoreGet(t *testing.T) {
	tmpDir := t.TempDir()
	productID := uuid.New().String()

	testData := model.MarketplaceItemDetails{
		ID:            "123",
		Title:         "Test Product",
		Description:   "Test Description",
		PriceAmount:   99.99,
		PriceCurrency: "USD",
		SellerID:      "seller-456",
		SellerName:    "Test Seller",
	}

	_, err := newTestStore(tmpDir, productID).Save(testData)
	require.NoError(t, err)

	gotData, err := newTestStore(tmpDir, productID).Get()
	require.NoError(t, err)
	require.NotNil(t, gotData)

	assert.Equal(t, testData.ID, gotData.ID)
	assert.Equal(t, testData.Title, gotData.Title)
	assert.Equal(t, testData.Description, gotData.Description)
	assert.Equal(t, testData.PriceAmount, gotData.PriceAmount)
	assert.Equal(t, testData.PriceCurrency, gotData.PriceCurrency)
	assert.Equal(t, testData.SellerID, gotData.SellerID)
	assert.Equal(t, testData.SellerName, gotData.SellerName)
}

func TestFileStoreSaveMissingFile(t *testing.T) {
	data, err := newTestStore(t.TempDir(), uuid.New().String()).Get()
	require.NoError(t, err)
	require.NotNil(t, data)

	assert.Nil(t, data.ID)
	assert.Nil(t, data.Title)
	assert.Nil(t, data.Description)
}

func TestFileStoreSavePersistsFields(t *testing.T) {
	tmpDir := t.TempDir()
	productID := uuid.New().String()

	testData := model.MarketplaceItemDetails{
		ID:          "123",
		Title:       "Test Product",
		Description: "Test Description",
	}

	_, err := newTestStore(tmpDir, productID).Save(testData)
	require.NoError(t, err)

	data, err := newTestStore(tmpDir, productID).Get()
	require.NoError(t, err)
	require.NotNil(t, data)

	assert.Equal(t, testData.ID, data.ID)
	assert.Equal(t, testData.Title, data.Title)
	assert.Equal(t, testData.Description, data.Description)
}

func TestFileStoreSaveUpdateOverwritesID(t *testing.T) {
	tmpDir := t.TempDir()
	productID := uuid.New().String()
	store := newTestStore(tmpDir, productID)
	newProductID := uuid.New().String()

	_, err := store.Save(model.MarketplaceItemDetails{ID: newProductID})
	require.NoError(t, err)

	data, err := store.Get()
	require.NoError(t, err)
	assert.Equal(t, newProductID, data.ID)
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

	require.Empty(t, concurrentSaveErrors(tmpDir, productID, patches), "concurrent Save errors")

	got, err := newTestStore(tmpDir, productID).Get()
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, "title", got.Title)
	assert.Equal(t, "description", got.Description)
	assert.Equal(t, "USD", got.PriceCurrency)
	assert.Equal(t, "seller", got.SellerName)
	assert.Equal(t, "city", got.LocationGeocodeCityName1)
}

// concurrentSaveErrors saves every patch concurrently and returns the errors.
func concurrentSaveErrors(dir, productID string, patches []model.MarketplaceItemDetails) []error {
	start := make(chan struct{})
	errCh := make(chan error, len(patches))
	var wg sync.WaitGroup

	for _, patch := range patches {
		wg.Go(func() {
			<-start
			if err := savePatch(dir, productID, patch); err != nil {
				errCh <- err
			}
		})
	}

	close(start)
	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	return errs
}

// savePatch saves one concurrent patch.
func savePatch(dir, productID string, patch model.MarketplaceItemDetails) error {
	_, err := newTestStore(dir, productID).Save(patch)
	return err
}
