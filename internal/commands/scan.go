package commands

import (
	"context"
	"fmt"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/spf13/cobra"

	"github.com/ppiankov/logspectre/internal/analyzer"
	awsscanner "github.com/ppiankov/logspectre/internal/aws"
	azurescanner "github.com/ppiankov/logspectre/internal/azure"
	gcpscanner "github.com/ppiankov/logspectre/internal/gcp"
	"github.com/ppiankov/logspectre/internal/report"
)

const (
	defaultIdleDays = 90
	defaultMinCost  = 0.0
	defaultFormat   = "text"
	defaultPlatform = "all"

	defaultHighIngestionBytesPerDay = 1024 * 1024 * 1024 // 1 GiB
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

var targetTypeMap = map[string]string{
	"aws":   "aws-account",
	"gcp":   "gcp-project",
	"azure": "azure-subscription",
	"all":   "multi-platform",
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
	cmd.Flags().String("project", "", "GCP project ID (required for --platform gcp)")
	cmd.Flags().String("subscription", "", "Azure subscription ID (required for --platform azure)")

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

	project, _ := cmd.Flags().GetString("project")
	subscription, _ := cmd.Flags().GetString("subscription")

	if platform == "gcp" && project == "" {
		return fmt.Errorf("--project is required when --platform is gcp")
	}
	if platform == "azure" && subscription == "" {
		return fmt.Errorf("--subscription is required when --platform is azure")
	}

	ctx := cmd.Context()
	cfg := analyzer.Config{
		LookbackDays:             idleDays,
		HighIngestionBytesPerDay: defaultHighIngestionBytesPerDay,
	}

	var allFindings []analyzer.Finding
	scanPlatform := func(p string) error {
		findings, err := dispatchScan(ctx, p, project, subscription, idleDays, cfg)
		if err != nil {
			return err
		}
		allFindings = append(allFindings, findings...)
		return nil
	}

	if platform == "all" {
		platforms := determinePlatforms(project, subscription)
		for _, p := range platforms {
			if err := scanPlatform(p); err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s scan failed: %v\n", p, err)
			}
		}
	} else {
		if err := scanPlatform(platform); err != nil {
			return fmt.Errorf("%s scan failed: %w", platform, err)
		}
	}

	allFindings = filterByMinCost(allFindings, minCost)

	targetType := targetTypeMap[platform]
	targetID := targetIDForPlatform(platform, project, subscription)
	reporter := selectReporter(format, targetType, targetID)

	return reporter.Write(cmd.OutOrStdout(), allFindings)
}

func dispatchScan(ctx context.Context, platform, project, subscription string, idleDays int, cfg analyzer.Config) ([]analyzer.Finding, error) {
	switch platform {
	case "aws":
		return scanAWS(ctx, idleDays, cfg)
	case "gcp":
		return scanGCP(ctx, project, idleDays, cfg)
	case "azure":
		return scanAzure(ctx, subscription, idleDays, cfg)
	default:
		return nil, fmt.Errorf("unknown platform %q", platform)
	}
}

func scanAWS(ctx context.Context, idleDays int, cfg analyzer.Config) ([]analyzer.Finding, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}
	scanner := awsscanner.NewCloudWatchScannerFromConfig(awsCfg, idleDays)
	groups, err := scanner.Scan(ctx)
	if err != nil {
		return nil, err
	}
	return analyzer.AnalyzeAWS(groups, cfg), nil
}

func scanGCP(ctx context.Context, projectID string, idleDays int, cfg analyzer.Config) ([]analyzer.Finding, error) {
	scanner, err := gcpscanner.NewLoggingScannerFromCredentials(ctx, projectID, idleDays)
	if err != nil {
		return nil, fmt.Errorf("create GCP scanner: %w", err)
	}
	buckets, sinks, err := scanner.Scan(ctx)
	if err != nil {
		return nil, err
	}
	return analyzer.AnalyzeGCP(buckets, sinks, cfg), nil
}

func scanAzure(_ context.Context, subscriptionID string, idleDays int, cfg analyzer.Config) ([]analyzer.Finding, error) {
	scanner, err := azurescanner.NewMonitorScannerFromCredentials(subscriptionID, idleDays)
	if err != nil {
		return nil, fmt.Errorf("create Azure scanner: %w", err)
	}
	ctx := context.Background()
	workspaces, err := scanner.Scan(ctx)
	if err != nil {
		return nil, err
	}
	return analyzer.AnalyzeAzure(workspaces, cfg), nil
}

func determinePlatforms(project, subscription string) []string {
	var platforms []string
	// Always attempt AWS (credentials are auto-detected from environment)
	platforms = append(platforms, "aws")
	if project != "" {
		platforms = append(platforms, "gcp")
	}
	if subscription != "" {
		platforms = append(platforms, "azure")
	}
	return platforms
}

func targetIDForPlatform(platform, project, subscription string) string {
	switch platform {
	case "gcp":
		return project
	case "azure":
		return subscription
	default:
		return ""
	}
}

func selectReporter(format, targetType, targetID string) report.Reporter {
	now := time.Now().UTC()
	switch format {
	case "json":
		return report.NewSpectrehubReporter(report.SpectrehubOptions{
			TargetType:  targetType,
			TargetID:    targetID,
			GeneratedAt: now,
		})
	default:
		return report.NewTextReporter(report.TextOptions{
			GeneratedAt: now,
		})
	}
}

func filterByMinCost(findings []analyzer.Finding, minCost float64) []analyzer.Finding {
	if minCost <= 0 {
		return findings
	}
	var filtered []analyzer.Finding
	for _, f := range findings {
		if f.EstimatedMonthlyCost >= minCost {
			filtered = append(filtered, f)
		}
	}
	if filtered == nil {
		filtered = []analyzer.Finding{}
	}
	return filtered
}
