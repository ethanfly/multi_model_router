package core

import (
	"fmt"
	"log"
	"sync"

	"multi_model_router/internal/config"
	"multi_model_router/internal/db"
	"multi_model_router/internal/proxy"
	"multi_model_router/internal/router"
	"multi_model_router/internal/stats"
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

	// Load classifier config from DB
	classifierConfig := c.loadClassifierConfig()
	c.classifier = router.NewClassifier(classifierConfig, nil)
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

// loadClassifierConfig reads classifier config from DB.
func (c *Core) loadClassifierConfig() *router.ClassifierConfig {
	if c.db == nil {
		return nil
	}
	data, err := c.db.GetConfig("classifier_config")
	if err != nil || data == "" {
		return nil
	}
	return router.ParseClassifierConfig(data)
}

// GetClassifierConfig returns the current classifier configuration.
func (c *Core) GetClassifierConfig() *router.ClassifierConfig {
	if c.classifier == nil {
		return router.DefaultClassifierConfig()
	}
	// Re-read from DB for freshness
	if c.db != nil {
		data, err := c.db.GetConfig("classifier_config")
		if err == nil && data != "" {
			return router.ParseClassifierConfig(data)
		}
	}
	return router.DefaultClassifierConfig()
}

// SetClassifierConfig persists classifier config to DB and updates the running classifier.
// If proxy is running, restart it to pick up the new configuration.
func (c *Core) SetClassifierConfig(cfg *router.ClassifierConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.db != nil {
		if err := c.db.SetConfig("classifier_config", cfg.ToJSON()); err != nil {
			return err
		}
	}

	// Rebuild classifier and engine with new config
	c.classifier = router.NewClassifier(cfg, nil)
	c.engine = router.NewEngine(c.classifier)
	c.loadModels()

	// If proxy is running, restart it to use the new engine
	if c.proxy != nil {
		port := c.proxy.Port()
		c.proxy.Stop()
		modeStr := c.getProxyModeLocked()
		routeMode := router.RouteModeFromString(modeStr)
		proxyAPIKey := ""
		manualModelID := ""
		if c.db != nil {
			proxyAPIKey, _ = c.db.GetConfig("proxy_api_key")
			manualModelID, _ = c.db.GetConfig("manual_model_id")
		}
		c.proxy = proxy.NewWithManualModel(port, c.engine, routeMode, manualModelID, proxyAPIKey)
		c.wireProxyLogger()
		if err := c.proxy.Start(); err != nil {
			log.Printf("failed to restart proxy after classifier config change: %v", err)
		}
	}

	return nil
}
