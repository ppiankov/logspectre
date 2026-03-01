package report

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ppiankov/logspectre/internal/analyzer"
)

var update = flag.Bool("update", false, "update golden files")

var fixedTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func goldenTest(t *testing.T, goldenFile string, reporter *SpectrehubReporter, findings []analyzer.Finding) {
	t.Helper()
	var buf bytes.Buffer
	if err := reporter.Write(&buf, findings); err != nil {
		t.Fatalf("Write: %v", err)
	}

	goldenPath := filepath.Join("testdata", goldenFile)
	if *update {
		if err := os.WriteFile(goldenPath, buf.Bytes(), 0644); err != nil {
			t.Fatalf("update golden: %v", err)
		}
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden file %s: %v (run with -update to create)", goldenPath, err)
	}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("output mismatch for %s:\ngot:\n%s\nwant:\n%s", goldenFile, buf.String(), string(want))
	}
}

func TestSpectrehubReporter_Empty(t *testing.T) {
	reporter := NewSpectrehubReporter(SpectrehubOptions{
		TargetType:  "aws-account",
		TargetID:    "123456789012",
		GeneratedAt: fixedTime,
	})
	goldenTest(t, "spectrehub_empty.golden", reporter, nil)
}

func TestSpectrehubReporter_AWSFindings(t *testing.T) {
	findings := []analyzer.Finding{
		{
			Provider:             analyzer.ProviderAWS,
			Type:                 analyzer.FindingNoRetention,
			ResourceID:           "/app/logs",
			ResourceARN:          "arn:aws:logs:us-east-1:123456789012:log-group:/app/logs",
			StoredBytes:          5_000_000_000,
			DailyIngestionBytes:  500_000_000,
			EstimatedMonthlyCost: 7.65,
			Detail:               "log group /app/logs has no retention policy; logs are stored indefinitely",
		},
		{
			Provider:             analyzer.ProviderAWS,
			Type:                 analyzer.FindingStale,
			ResourceID:           "/app/stale",
			ResourceARN:          "arn:aws:logs:us-east-1:123456789012:log-group:/app/stale",
			StoredBytes:          1_000_000_000,
			DailyIngestionBytes:  0,
			EstimatedMonthlyCost: 0.03,
			Detail:               "log group /app/stale has had no events in the lookback period",
		},
	}
	reporter := NewSpectrehubReporter(SpectrehubOptions{
		TargetType:  "aws-account",
		TargetID:    "123456789012",
		GeneratedAt: fixedTime,
	})
	goldenTest(t, "spectrehub_aws_single.golden", reporter, findings)
}

func TestSpectrehubReporter_MultiProvider(t *testing.T) {
	findings := []analyzer.Finding{
		{
			Provider:             analyzer.ProviderAWS,
			Type:                 analyzer.FindingUnread,
			ResourceID:           "/app/unread",
			ResourceARN:          "arn:aws:logs:us-east-1:123456789012:log-group:/app/unread",
			StoredBytes:          2_000_000_000,
			DailyIngestionBytes:  100_000_000,
			EstimatedMonthlyCost: 1.56,
			Detail:               "log group /app/unread has no metric filters or subscriptions",
		},
		{
			Provider:             analyzer.ProviderGCP,
			Type:                 analyzer.FindingStaleSink,
			ResourceID:           "dead-sink",
			StoredBytes:          0,
			DailyIngestionBytes:  0,
			EstimatedMonthlyCost: 0,
			Detail:               "sink dead-sink has exported zero bytes in the lookback period",
		},
		{
			Provider:             analyzer.ProviderAzure,
			Type:                 analyzer.FindingStale,
			ResourceID:           "stale-ws",
			ResourceARN:          "/subscriptions/sub-123/resourceGroups/rg/providers/Microsoft.OperationalInsights/workspaces/stale-ws",
			StoredBytes:          0,
			DailyIngestionBytes:  0,
			EstimatedMonthlyCost: 0,
			Detail:               "workspace stale-ws has had no heartbeats in the lookback period",
		},
	}
	reporter := NewSpectrehubReporter(SpectrehubOptions{
		TargetType:  "aws-account",
		TargetID:    "multi-scan",
		GeneratedAt: fixedTime,
	})
	goldenTest(t, "spectrehub_multi_provider.golden", reporter, findings)
}
