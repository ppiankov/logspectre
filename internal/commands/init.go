package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const configFileName = ".logspectre.yaml"

const configTemplate = `# logspectre configuration
# Documentation: https://github.com/ppiankov/logspectre

# Cloud platforms to scan
platforms:
  aws:
    enabled: true
    # regions: ["us-east-1", "us-west-2"]  # default: all regions
    # profile: "default"                    # AWS CLI profile name
  gcp:
    enabled: false
    # project_id: "my-project"
  azure:
    enabled: false
    # subscription_id: "00000000-0000-0000-0000-000000000000"

# Scan thresholds
scan:
  idle_days: 90       # log groups with no events in this many days are flagged
  min_cost: 0.00      # minimum monthly cost (USD) to include in results

# Output settings
output:
  format: text        # text, json, sarif
  # file: ""          # write output to file instead of stdout

# Exclusion patterns (regex matched against log group names)
# exclude:
#   - "^/aws/lambda/"
#   - "^/aws/rds/"
`

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Generate a sample .logspectre.yaml configuration file",
		Args:  cobra.NoArgs,
		RunE:  runInit,
	}
}

func runInit(cmd *cobra.Command, _ []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	path := filepath.Join(dir, configFileName)

	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", configFileName)
	}

	if err := os.WriteFile(path, []byte(configTemplate), 0644); err != nil {
		return fmt.Errorf("write %s: %w", configFileName, err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Created %s\n", path)
	return nil
}
