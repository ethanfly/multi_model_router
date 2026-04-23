package core

import (
	"fmt"

	"multi_model_router/internal/proxy"
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

	c.proxy = proxy.New(port, c.engine)
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

	if c.proxy != nil {
		return ProxyStatus{Running: true, Port: c.proxy.Port()}
	}
	return ProxyStatus{Running: false, Port: 0}
}
