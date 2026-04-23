package core

import (
	"fmt"
	"sync"

	"multi_model_router/internal/config"
	"multi_model_router/internal/db"
	"multi_model_router/internal/router"
	"multi_model_router/internal/stats"
	"multi_model_router/internal/proxy"
)

// Core holds all headless-capable business logic, independent of Wails.
type Core struct {
	config     *config.Config
	db         *db.DB
	engine     *router.Engine
	classifier *router.Classifier
	collector  *stats.Collector
	proxy      *proxy.Server
	mu         sync.RWMutex
}

// New creates a new Core instance with the given config.
func New(cfg *config.Config) *Core {
	return &Core{config: cfg}
}

// Init initializes DB, collector, classifier, engine, and loads models.
func (c *Core) Init() error {
	var err error
	c.db, err = db.New(c.config.AppDataDir)
	if err != nil {
		return fmt.Errorf("db init: %w", err)
	}

	c.collector = stats.NewCollector(c.db)
	c.classifier = router.NewClassifier(nil)
	c.engine = router.NewEngine(c.classifier)
	c.loadModels()

	return nil
}

// Close shuts down proxy and closes DB.
func (c *Core) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.proxy != nil {
		c.proxy.Stop()
		c.proxy = nil
	}
	if c.db != nil {
		c.db.Close()
	}
}

// Config returns the app config.
func (c *Core) Config() *config.Config {
	return c.config
}

// DB returns the database instance.
func (c *Core) DB() *db.DB {
	return c.db
}

// Engine returns the router engine.
func (c *Core) Engine() *router.Engine {
	return c.engine
}

// Collector returns the stats collector.
func (c *Core) Collector() *stats.Collector {
	return c.collector
}
