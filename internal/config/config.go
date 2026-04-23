package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	AppDataDir string
	ProxyPort  int
}

func Default() *Config {
	home, _ := os.UserHomeDir()
	appData := filepath.Join(home, ".multi_model_router")
	return &Config{
		AppDataDir: appData,
		ProxyPort:  9680,
	}
}

func (c *Config) DBPath() string {
	return filepath.Join(c.AppDataDir, "multi_model_router.db")
}
