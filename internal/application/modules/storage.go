package modules

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mshaulsky/domovoi/internal/container"
	"github.com/mshaulsky/domovoi/internal/storage/sqlite"
	"github.com/mshaulsky/domovoi/internal/storage/store"
)

// Storage provides the database and the store. It is a pointer in the
// module list because Stop must close the connection Register opened.
type Storage struct {
	db *sql.DB
}

// openTimeout bounds opening and migrating the database at start.
const openTimeout = time.Minute

// Name returns the module name.
func (*Storage) Name() string {
	return "storage"
}

// Register provides the DB and Store singletons; the file is opened on
// first use. Under -check the file is left alone: its directory must exist,
// and the schema is migrated into an in-memory database, so the wiring and
// the migrations are exercised without leaving a file behind — a -check run
// as root would otherwise create one the service user cannot write.
func (m *Storage) Register(c *container.Container) error {
	c.DB.Provide(func() (*sql.DB, error) {
		cfg, err := c.Config.Get()
		if err != nil {
			return nil, err
		}
		log, err := c.Logger.Get()
		if err != nil {
			return nil, err
		}
		path := cfg.Storage.Path
		if c.Check {
			if err := checkDir(path); err != nil {
				return nil, fmt.Errorf("storage: %w", err)
			}
			path = sqlite.Memory
		}
		ctx, cancel := context.WithTimeout(context.Background(), openTimeout)
		defer cancel()
		db, err := sqlite.Open(ctx, path, log)
		if err != nil {
			return nil, fmt.Errorf("storage: %w", err)
		}
		m.db = db
		log.Info("storage opened", "path", path)
		return db, nil
	})
	c.Store.Provide(func() (*store.Store, error) {
		db, err := c.DB.Get()
		if err != nil {
			return nil, err
		}
		return store.New(db), nil
	})
	return nil
}

// Start does nothing: the database opens on first use.
func (*Storage) Start(context.Context) error {
	return nil
}

// Stop closes the database after everything that wrote to it has stopped.
func (m *Storage) Stop(context.Context) error {
	if m.db == nil {
		return nil
	}
	if err := m.db.Close(); err != nil {
		return fmt.Errorf("storage: close: %w", err)
	}
	return nil
}

// checkDir verifies that the directory the database file would live in
// exists, which is all -check can know without creating the file.
func checkDir(path string) error {
	dir := filepath.Dir(path)
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("database directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("database directory %s is not a directory", dir)
	}
	return nil
}
