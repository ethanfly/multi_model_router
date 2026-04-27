package cli

import (
	"fmt"
	"strings"

	"multi_model_router/internal/agentconfig"

	"github.com/spf13/cobra"
)

func newAgentsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agents",
		Short: "Configure AI agent CLIs to use the local proxy",
	}
	cmd.AddCommand(newAgentsConfigureCmd())
	return cmd
}

func newAgentsConfigureCmd() *cobra.Command {
	var apps string
	var home string
	var port int
	var baseURL string
	var apiKey string
	var model string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "configure",
		Short: "One-click configure Claude Code, Codex, Gemini CLI, OpenCode, and OpenClaw",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := agentconfig.Configure(agentconfig.Options{
				HomeDir: home,
				Apps:    strings.Split(apps, ","),
				Port:    port,
				BaseURL: baseURL,
				APIKey:  apiKey,
				Model:   model,
				DryRun:  dryRun,
			})
			if err != nil {
				return err
			}

			status := "APPLY"
			if dryRun {
				status = "DRY-RUN"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s multi-agent proxy configuration\n", status)
			for _, change := range result.Changes {
				line := fmt.Sprintf("- %s: %s %s", change.App, change.Action, change.Path)
				if change.BackupPath != "" {
					line += " backup=" + change.BackupPath
				}
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			for _, warning := range result.Warnings {
				if warning.Path != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "warning[%s]: %s (%s)\n", warning.App, warning.Message, warning.Path)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "warning[%s]: %s\n", warning.App, warning.Message)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&apps, "apps", "all", "Comma-separated apps: claude,codex,gemini,opencode,openclaw,all")
	cmd.Flags().StringVar(&home, "home", "", "Home directory override for testing or portable profiles")
	cmd.Flags().IntVarP(&port, "port", "p", 9680, "Local proxy port")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Override proxy base URL, e.g. http://127.0.0.1:9680")
	cmd.Flags().StringVarP(&apiKey, "api-key", "k", "", "Proxy API key; defaults to a local placeholder when empty")
	cmd.Flags().StringVarP(&model, "model", "m", "auto", "Proxy model name, usually auto or race")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview file changes without writing")
	return cmd
}
