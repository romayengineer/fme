package db

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/migrations/logs"
	"github.com/pocketbase/pocketbase/tools/migrate"

	"github.com/romayengineer/fme/go/core/logging"
)

type PocketBaseDB struct {
	app *pocketbase.PocketBase
	mu  sync.Mutex
}

var (
	dbInstance *PocketBaseDB
	dbOnce     sync.Once
)

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
		return errors.New("pocketbase app not initialized")
	}

	// Bootstrap creates the database tables if they don't exist
	if err := db.app.Bootstrap(); err != nil {
		logging.LogError0("Start", "bootstrap failed", "error", err)
		return fmt.Errorf("failed to bootstrap pocketbase: %w", err)
	}

	// Bootstrap only opens the databases; the built-in migrations (which create
	// `_collections` and friends) are applied by the serve command, not by Bootstrap.
	// Running them here makes `just save` work on a fresh clone without needing
	// `just serve` first. Runner.Up is idempotent, so this is safe on every start.
	if err := db.runMigrations(); err != nil {
		logging.LogError0("Start", "migrations failed", "error", err)
		return fmt.Errorf("failed to run pocketbase migrations: %w", err)
	}

	// Ensure products collection exists
	if err := db.ensureProductsCollection(); err != nil {
		logging.LogError0("Start", "failed to ensure products collection", "error", err)
		return fmt.Errorf("failed to ensure products collection: %w", err)
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

func (db *PocketBaseDB) GetApp() *pocketbase.PocketBase {
	return db.app
}

func (db *PocketBaseDB) Close() error {
	return nil
}
