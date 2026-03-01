package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

const (
	defaultIdleDays = 90
	defaultMinCost  = 0.0
	defaultFormat   = "text"
	defaultPlatform = "all"
)

var validPlatforms = map[string]bool{
	"aws":   true,
	"gcp":   true,
	"azure": true,
	"all":   true,
}

var validFormats = map[string]bool{
	"text":  true,
	"json":  true,
	"sarif": true,
}

func newScanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan cloud log storage for waste",
		Long:  "Scan cloud log storage across AWS, GCP, and Azure to identify idle log groups, excessive retention, and cost optimization opportunities.",
		RunE:  runScan,
	}

	cmd.Flags().StringP("platform", "p", defaultPlatform, "cloud platform to scan (aws, gcp, azure, all)")
	cmd.Flags().StringP("format", "f", defaultFormat, "output format (text, json, sarif)")
	cmd.Flags().StringSliceP("region", "r", nil, "regions to scan (default: all configured regions)")
	cmd.Flags().IntP("idle-days", "d", defaultIdleDays, "minimum days since last log event to consider idle")
	cmd.Flags().Float64P("min-cost", "c", defaultMinCost, "minimum monthly cost threshold in USD")

	return cmd
}

func runScan(cmd *cobra.Command, _ []string) error {
	platform, _ := cmd.Flags().GetString("platform")
	if !validPlatforms[platform] {
		return fmt.Errorf("invalid platform %q: must be one of aws, gcp, azure, all", platform)
	}

	format, _ := cmd.Flags().GetString("format")
	if !validFormats[format] {
		return fmt.Errorf("invalid format %q: must be one of text, json, sarif", format)
	}

	idleDays, _ := cmd.Flags().GetInt("idle-days")
	if idleDays < 1 {
		return fmt.Errorf("idle-days must be at least 1, got %d", idleDays)
	}

	minCost, _ := cmd.Flags().GetFloat64("min-cost")
	if minCost < 0 {
		return fmt.Errorf("min-cost must be non-negative, got %.2f", minCost)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "scan not yet implemented (platform=%s, format=%s, idle-days=%d, min-cost=%.2f)\n",
		platform, format, idleDays, minCost)
	return nil
}
