package core

import (
	"fmt"
	"time"

	"multi_model_router/internal/proxy"
	"multi_model_router/internal/router"
	"multi_model_router/internal/stats"
)

// StartProxy starts the proxy server on the given port.
func (c *Core) StartProxy(port int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Stop existing proxy if running
	if c.proxy != nil {
		c.proxy.Stop()
		c.proxy = nil
	}

	modeStr := c.getProxyModeLocked()
	routeMode := router.RouteModeFromString(modeStr)
	proxyAPIKey := ""
	if c.db != nil {
		proxyAPIKey, _ = c.db.GetConfig("proxy_api_key")
	}
	c.proxy = proxy.New(port, c.engine, routeMode, proxyAPIKey)
	c.wireProxyLogger()
	if err := c.proxy.Start(); err != nil {
		return fmt.Errorf("start proxy: %w", err)
	}

	// Save proxy config
	if c.db != nil {
		c.db.SetConfig("proxy_enabled", "true")
		c.db.SetConfig("proxy_port", fmt.Sprintf("%d", port))
	}

	return nil
}

// StopProxy stops the proxy server.
func (c *Core) StopProxy() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.proxy != nil {
		c.proxy.Stop()
		c.proxy = nil
	}
	if c.db != nil {
		c.db.SetConfig("proxy_enabled", "false")
	}
	return nil
}

// GetProxyStatus returns the current proxy status.
func (c *Core) GetProxyStatus() ProxyStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	mode := c.getProxyModeLocked()
	if c.proxy != nil {
		return ProxyStatus{Running: true, Port: c.proxy.Port(), Mode: mode}
	}
	return ProxyStatus{Running: false, Port: 0, Mode: mode}
}

// GetProxyMode returns the persisted proxy route mode (default "auto").
func (c *Core) GetProxyMode() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.getProxyModeLocked()
}

func (c *Core) getProxyModeLocked() string {
	if c.db == nil {
		return "auto"
	}
	val, err := c.db.GetConfig("proxy_mode")
	if err != nil || val == "" {
		return "auto"
	}
	return val
}

// SetProxyMode persists the proxy route mode and updates the running proxy if active.
func (c *Core) SetProxyMode(mode string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.db != nil {
		if err := c.db.SetConfig("proxy_mode", mode); err != nil {
			return err
		}
	}

	// If proxy is running, restart it with the new mode
	if c.proxy != nil {
		port := c.proxy.Port()
		c.proxy.Stop()
		routeMode := router.RouteModeFromString(mode)
		proxyAPIKey := ""
		if c.db != nil {
			proxyAPIKey, _ = c.db.GetConfig("proxy_api_key")
		}
		c.proxy = proxy.New(port, c.engine, routeMode, proxyAPIKey)
		c.wireProxyLogger()
		if err := c.proxy.Start(); err != nil {
			return fmt.Errorf("restart proxy with new mode: %w", err)
		}
	}

	return nil
}

// wireProxyLogger sets the OnRequestLog callback to record proxy stats.
func (c *Core) wireProxyLogger() {
	if c.proxy == nil {
		return
	}
	c.proxy.OnRequestLog = func(entry *proxy.RequestLogEntry) {
		if c.collector == nil {
			return
		}
		_ = c.collector.LogRequest(&stats.RequestLog{
			ModelID:    entry.ModelName,
			Source:     entry.Source,
			Complexity: router.Complexity(entry.Complexity).String(),
			RouteMode:  entry.RouteMode,
			Status:     entry.Status,
			TokensIn:   entry.TokensIn,
			TokensOut:  entry.TokensOut,
			LatencyMs:  entry.LatencyMs,
			ErrorMsg:   entry.ErrorMsg,
			CreatedAt:  time.Now(),
		})
	}
}
