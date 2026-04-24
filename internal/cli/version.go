package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is set via -ldflags during build, defaults to "dev".
var Version = "dev"

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Multi-Model Router %s\n", Version)
		},
	}
}
