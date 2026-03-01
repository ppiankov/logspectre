package commands

import (
	"github.com/spf13/cobra"
)

// NewRootCmd builds the root command with all subcommands attached.
func NewRootCmd(version, commit, date string) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "logspectre",
		Short:         "Cloud log storage waste auditor",
		Long:          "logspectre audits cloud log storage across AWS, GCP, and Azure to identify waste, idle log groups, and cost optimization opportunities.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.AddCommand(newVersionCmd(version, commit, date))
	rootCmd.AddCommand(newScanCmd())
	rootCmd.AddCommand(newInitCmd())

	return rootCmd
}

// Execute builds the CLI command tree and runs it.
func Execute(version, commit, date string) error {
	return NewRootCmd(version, commit, date).Execute()
}
