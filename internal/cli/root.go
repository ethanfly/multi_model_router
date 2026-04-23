package cli

import (
	"github.com/spf13/cobra"
)

// NewRootCommand creates the root CLI command with all subcommands.
func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "multi_model_router",
		Short:         "Multi-Model Router — AI model routing proxy",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newServeCmd())
	cmd.AddCommand(newTUICmd())
	cmd.AddCommand(newVersionCmd())
	return cmd
}
