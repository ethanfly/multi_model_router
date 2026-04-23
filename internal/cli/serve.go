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

			fmt.Printf("Multi-Model Router proxy listening on :%d\n", servePort)
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
	return cmd
}
