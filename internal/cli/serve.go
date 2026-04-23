package cli

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"multi_model_router/internal/config"
	"multi_model_router/internal/core"

	"github.com/spf13/cobra"
)

func newServeCmd() *cobra.Command {
	var port int
	var mode string
	var apiKey string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the proxy server in headless mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Default()

			c := core.New(cfg)
			if err := c.Init(); err != nil {
				return fmt.Errorf("init: %w", err)
			}
			defer c.Close()

			// Persist mode if provided via flag
			if mode != "" {
				if err := c.SetProxyMode(mode); err != nil {
					return fmt.Errorf("set mode: %w", err)
				}
			}

			// Persist API key if provided via flag
			if apiKey != "" {
				if err := c.SetConfig("proxy_api_key", apiKey); err != nil {
					return fmt.Errorf("set api key: %w", err)
				}
			}

			// Determine port: flag > DB config > default
			servePort := port
			if servePort <= 0 {
				if v := c.GetConfig("proxy_port"); v != "" {
					fmt.Sscanf(v, "%d", &servePort)
				}
				if servePort <= 0 {
					servePort = cfg.ProxyPort
				}
			}

			if err := c.StartProxy(servePort); err != nil {
				return fmt.Errorf("start proxy: %w", err)
			}

			currentMode := c.GetProxyMode()
			currentKey := c.GetConfig("proxy_api_key")
			fmt.Printf("Multi-Model Router proxy listening on :%d (mode: %s)\n", servePort, currentMode)
			if currentKey != "" {
				fmt.Println("API key authentication: enabled")
			} else {
				fmt.Println("API key authentication: disabled")
			}
			fmt.Println("Use model=\"auto\" for auto routing, or pass a model name for manual selection.")
			fmt.Println("Press Ctrl+C to stop.")

			// Block until signal
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			sig := <-sigCh
			fmt.Printf("\nReceived %s, shutting down...\n", sig)
			return nil
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 0, "Proxy port (default: 9680)")
	cmd.Flags().StringVarP(&mode, "mode", "m", "", "Proxy mode: auto, manual, race (default: auto)")
	cmd.Flags().StringVarP(&apiKey, "api-key", "k", "", "API key for proxy authentication")
	return cmd
}
