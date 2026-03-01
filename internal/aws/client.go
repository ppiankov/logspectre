package aws

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
)

// NewCloudWatchScannerFromConfig creates a CloudWatchScanner using real AWS SDK clients.
func NewCloudWatchScannerFromConfig(cfg aws.Config, lookbackDays int) *CloudWatchScanner {
	logsClient := cloudwatchlogs.NewFromConfig(cfg)
	cwClient := cloudwatch.NewFromConfig(cfg)
	return NewCloudWatchScanner(logsClient, logsClient, logsClient, cwClient, lookbackDays)
}
