package core

import (
	"context"
	"fmt"
	"log"
	"time"

	"multi_model_router/internal/crypto"
	"multi_model_router/internal/proxy"
	"multi_model_router/internal/provider"
	"multi_model_router/internal/router"
	"multi_model_router/internal/stats"
)

// loadModels loads all models from the database into the engine.
func (c *Core) loadModels() {
	if c.db == nil {
		return
	}

	rows, err := c.db.Query(
		`SELECT id, name, provider, base_url, api_key, model_id,
		        reasoning, coding, creativity, speed, cost_efficiency,
		        max_rpm, max_tpm, is_active
		 FROM models`,
	)
	if err != nil {
		fmt.Printf("load models error: %v\n", err)
		return
	}
	defer rows.Close()

	var models []*router.ModelConfig
	for rows.Next() {
		var m router.ModelConfig
		var isActive int
		if err := rows.Scan(
			&m.ID, &m.Name, &m.Provider, &m.BaseURL, &m.APIKey, &m.ModelID,
			&m.Reasoning, &m.Coding, &m.Creativity, &m.Speed, &m.CostEfficiency,
			&m.MaxRPM, &m.MaxTPM, &isActive,
		); err != nil {
			fmt.Printf("scan model error: %v\n", err)
			continue
		}
		m.IsActive = isActive == 1

		// Decrypt API key
		if m.APIKey != "" {
			decrypted, err := crypto.Decrypt(m.APIKey)
			if err != nil {
				fmt.Printf("decrypt key error for %s: %v\n", m.ID, err)
				continue
			}
			m.APIKey = decrypted
		}

		c.setProviderInstance(&m)
		models = append(models, &m)
	}

	if len(models) > 0 {
		c.engine.SetModels(models)
	}
}

// ReloadModels clears and reloads all models from the database.
func (c *Core) ReloadModels() {
	c.engine.SetModels(nil)
	c.loadModels()
}

// setProviderInstance creates and assigns a provider instance to the model.
func (c *Core) setProviderInstance(m *router.ModelConfig) {
	switch m.Provider {
	case "openai":
		m.ProviderInstance = provider.NewOpenAI(m.BaseURL, m.APIKey)
	case "anthropic":
		m.ProviderInstance = provider.NewAnthropic(m.BaseURL, m.APIKey)
	}
}

// GetModels returns all models from the database for display.
func (c *Core) GetModels() []ModelJSON {
	if c.db == nil {
		return []ModelJSON{}
	}

	rows, err := c.db.Query(
		`SELECT id, name, provider, base_url, api_key, model_id,
		        reasoning, coding, creativity, speed, cost_efficiency,
		        max_rpm, max_tpm, is_active
		 FROM models
		 ORDER BY name`,
	)
	if err != nil {
		fmt.Printf("GetModels error: %v\n", err)
		return nil
	}
	defer rows.Close()

	var models []ModelJSON
	for rows.Next() {
		var m ModelJSON
		var isActive int
		if err := rows.Scan(
			&m.ID, &m.Name, &m.Provider, &m.BaseURL, &m.APIKey, &m.ModelID,
			&m.Reasoning, &m.Coding, &m.Creativity, &m.Speed, &m.CostEfficiency,
			&m.MaxRPM, &m.MaxTPM, &isActive,
		); err != nil {
			continue
		}
		m.IsActive = isActive == 1
		// Mask the API key for display
		if len(m.APIKey) > 8 {
			m.APIKey = m.APIKey[:4] + "..." + m.APIKey[len(m.APIKey)-4:]
		}
		models = append(models, m)
	}

	return models
}

// SaveModel inserts or updates a model and reloads the engine.
// If proxy is running, restart it to pick up the new configuration.
func (c *Core) SaveModel(m ModelJSON) error {
	if c.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Generate ID for new models
	if m.ID == "" {
		m.ID = router.NewUUID()
	}

	// Determine the API key to store
	encryptedKey := m.APIKey
	if m.APIKey == "" {
		// Empty key — clear it
		encryptedKey = ""
	} else if len(m.APIKey) > 8 && m.APIKey[4:7] == "..." {
		// Masked key (user didn't change it) — preserve existing encrypted key from DB
		var existingKey string
		err := c.db.QueryRow("SELECT api_key FROM models WHERE id = ?", m.ID).Scan(&existingKey)
		if err == nil && existingKey != "" {
			encryptedKey = existingKey
		}
	} else {
		// New key — encrypt it
		enc, err := crypto.Encrypt(m.APIKey)
		if err != nil {
			return fmt.Errorf("encrypt api key: %w", err)
		}
		encryptedKey = enc
	}

	activeInt := 0
	if m.IsActive {
		activeInt = 1
	}

	_, err := c.db.Exec(
		`INSERT OR REPLACE INTO models
		 (id, name, provider, base_url, api_key, model_id,
		  reasoning, coding, creativity, speed, cost_efficiency,
		  max_rpm, max_tpm, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.Name, m.Provider, m.BaseURL, encryptedKey, m.ModelID,
		m.Reasoning, m.Coding, m.Creativity, m.Speed, m.CostEfficiency,
		m.MaxRPM, m.MaxTPM, activeInt,
	)
	if err != nil {
		return fmt.Errorf("save model: %w", err)
	}

	c.ReloadModels()

	// If proxy is running, restart it to pick up the new configuration
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.proxy != nil {
		port := c.proxy.Port()
		c.proxy.Stop()
		modeStr := c.getProxyModeLocked()
		routeMode := router.RouteModeFromString(modeStr)
		proxyAPIKey := ""
		if c.db != nil {
			proxyAPIKey, _ = c.db.GetConfig("proxy_api_key")
		}
		c.proxy = proxy.New(port, c.engine, routeMode, proxyAPIKey)
		if err := c.proxy.Start(); err != nil {
			log.Printf("failed to restart proxy after model save: %v", err)
		}
	}

	return nil
}

// DeleteModel removes a model and reloads the engine.
// If proxy is running, restart it to pick up the new configuration.
func (c *Core) DeleteModel(id string) error {
	if c.db == nil {
		return fmt.Errorf("database not initialized")
	}

	_, err := c.db.Exec("DELETE FROM models WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete model: %w", err)
	}

	c.ReloadModels()

	// If proxy is running, restart it to pick up the new configuration
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.proxy != nil {
		port := c.proxy.Port()
		c.proxy.Stop()
		modeStr := c.getProxyModeLocked()
		routeMode := router.RouteModeFromString(modeStr)
		proxyAPIKey := ""
		if c.db != nil {
			proxyAPIKey, _ = c.db.GetConfig("proxy_api_key")
		}
		c.proxy = proxy.New(port, c.engine, routeMode, proxyAPIKey)
		if err := c.proxy.Start(); err != nil {
			log.Printf("failed to restart proxy after model delete: %v", err)
		}
	}

	return nil
}

// TestModel creates a temporary provider and checks its health.
func (c *Core) TestModel(ctx context.Context, m ModelJSON) (string, error) {
	// Read the real encrypted API key from DB and decrypt — the key from
	// GetModels() is masked (e.g. "sk-a...bcd") and won't work for API calls.
	apiKey := m.APIKey
	if c.db != nil && m.ID != "" {
		var encKey string
		if err := c.db.QueryRow("SELECT api_key FROM models WHERE id = ?", m.ID).Scan(&encKey); err == nil && encKey != "" {
			if dec, err := crypto.Decrypt(encKey); err == nil {
				apiKey = dec
			}
		}
	}

	var p provider.Provider
	switch m.Provider {
	case "openai":
		p = provider.NewOpenAI(m.BaseURL, apiKey)
	case "anthropic":
		p = provider.NewAnthropic(m.BaseURL, apiKey)
	default:
		return "FAIL: unknown provider " + m.Provider, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := p.HealthCheck(ctx, m.ModelID); err != nil {
		return "FAIL: " + err.Error(), nil
	}
	return "OK", nil
}

// LogRequest logs a request to the stats collector.
func (c *Core) LogRequest(log *stats.RequestLog) error {
	if c.collector == nil {
		return nil
	}
	return c.collector.LogRequest(log)
}
