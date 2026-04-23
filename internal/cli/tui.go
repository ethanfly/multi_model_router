package cli

import (
	"fmt"

	"multi_model_router/internal/config"
	"multi_model_router/internal/core"
	"multi_model_router/internal/tui"

	"github.com/spf13/cobra"
)

func newTUICmd() *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Start interactive terminal UI for managing the router",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Default()

			c := core.New(cfg)
			if err := c.Init(); err != nil {
				return fmt.Errorf("init: %w", err)
			}
			defer c.Close()

			return tui.Run(c, port)
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 0, "Default proxy port (default: 9680)")
	return cmd
}
