package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"multi_model_router/internal/crypto"
	"multi_model_router/internal/provider"
	"multi_model_router/internal/proxy"
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
		manualModelID := ""
		if c.db != nil {
			proxyAPIKey, _ = c.db.GetConfig("proxy_api_key")
			manualModelID, _ = c.db.GetConfig("manual_model_id")
		}
		c.proxy = proxy.NewWithManualModel(port, c.engine, routeMode, manualModelID, proxyAPIKey)
		c.wireProxyLogger()
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
		manualModelID := ""
		if c.db != nil {
			proxyAPIKey, _ = c.db.GetConfig("proxy_api_key")
			manualModelID, _ = c.db.GetConfig("manual_model_id")
		}
		c.proxy = proxy.NewWithManualModel(port, c.engine, routeMode, manualModelID, proxyAPIKey)
		c.wireProxyLogger()
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

// exportFile is the JSON structure for model export/import.
type exportFile struct {
	Version    int           `json:"version"`
	ExportedAt string        `json:"exported_at"`
	Models     []exportModel `json:"models"`
}

type exportModel struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Provider       string `json:"provider"`
	BaseURL        string `json:"base_url"`
	APIKey         string `json:"api_key"`
	ModelID        string `json:"model_id"`
	Reasoning      int    `json:"reasoning"`
	Coding         int    `json:"coding"`
	Creativity     int    `json:"creativity"`
	Speed          int    `json:"speed"`
	CostEfficiency int    `json:"cost_efficiency"`
	MaxRPM         int    `json:"max_rpm"`
	MaxTPM         int    `json:"max_tpm"`
	IsActive       bool   `json:"is_active"`
}

// ExportModels exports all models as a JSON string with API keys encrypted by password.
func (c *Core) ExportModels(password string) (string, error) {
	if c.db == nil {
		return "", fmt.Errorf("database not initialized")
	}

	rows, err := c.db.Query(
		`SELECT id, name, provider, base_url, api_key, model_id,
		        reasoning, coding, creativity, speed, cost_efficiency,
		        max_rpm, max_tpm, is_active
		 FROM models ORDER BY name`,
	)
	if err != nil {
		return "", fmt.Errorf("query models: %w", err)
	}
	defer rows.Close()

	var models []exportModel
	for rows.Next() {
		var em exportModel
		var isActive int
		var encryptedKey string
		if err := rows.Scan(
			&em.ID, &em.Name, &em.Provider, &em.BaseURL, &encryptedKey, &em.ModelID,
			&em.Reasoning, &em.Coding, &em.Creativity, &em.Speed, &em.CostEfficiency,
			&em.MaxRPM, &em.MaxTPM, &isActive,
		); err != nil {
			continue
		}
		em.IsActive = isActive == 1

		// Decrypt with machine key, then re-encrypt with user password
		if encryptedKey != "" {
			plain, err := crypto.Decrypt(encryptedKey)
			if err != nil {
				continue
			}
			enc, err := crypto.EncryptWithPassword(plain, password)
			if err != nil {
				continue
			}
			em.APIKey = enc
		}

		models = append(models, em)
	}

	if len(models) == 0 {
		return "", fmt.Errorf("no models to export")
	}

	data := exportFile{
		Version:    1,
		ExportedAt: time.Now().Format(time.RFC3339),
		Models:     models,
	}

	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal export: %w", err)
	}
	return string(b), nil
}

// ImportModels imports models from a JSON string, decrypting API keys with password.
func (c *Core) ImportModels(jsonData, password string) (int, error) {
	if c.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	var data exportFile
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		return 0, fmt.Errorf("parse import file: %w", err)
	}
	if data.Version != 1 {
		return 0, fmt.Errorf("unsupported export version: %d", data.Version)
	}

	imported := 0
	for _, em := range data.Models {
		// Decrypt API key from export password
		apiKey := ""
		if em.APIKey != "" {
			plain, err := crypto.DecryptWithPassword(em.APIKey, password)
			if err != nil {
				return imported, fmt.Errorf("decrypt API key for %s: %w", em.Name, err)
			}
			apiKey = plain
		}

		// Generate new ID to avoid collisions
		newID := router.NewUUID()

		m := ModelJSON{
			ID:             newID,
			Name:           em.Name,
			Provider:       em.Provider,
			BaseURL:        em.BaseURL,
			APIKey:         apiKey,
			ModelID:        em.ModelID,
			Reasoning:      em.Reasoning,
			Coding:         em.Coding,
			Creativity:     em.Creativity,
			Speed:          em.Speed,
			CostEfficiency: em.CostEfficiency,
			MaxRPM:         em.MaxRPM,
			MaxTPM:         em.MaxTPM,
			IsActive:       em.IsActive,
		}

		if err := c.SaveModel(m); err != nil {
			return imported, fmt.Errorf("save imported model %s: %w", em.Name, err)
		}
		imported++
	}

	return imported, nil
}
