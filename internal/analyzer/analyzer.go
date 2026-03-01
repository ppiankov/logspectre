package analyzer

import (
	"fmt"

	"github.com/ppiankov/logspectre/internal/aws"
	"github.com/ppiankov/logspectre/internal/azure"
	"github.com/ppiankov/logspectre/internal/gcp"
)

// AnalyzeAWS produces findings from CloudWatch log group scan results.
func AnalyzeAWS(groups []aws.LogGroupInfo, cfg Config) []Finding {
	var findings []Finding
	for _, g := range groups {
		cost := calcAWSCost(g.StoredBytes, g.IncomingBytes)

		if g.IsStale {
			findings = append(findings, Finding{
				Provider:             ProviderAWS,
				Type:                 FindingStale,
				ResourceID:           g.Name,
				ResourceARN:          g.ARN,
				StoredBytes:          g.StoredBytes,
				DailyIngestionBytes:  g.IncomingBytes,
				EstimatedMonthlyCost: cost,
				Detail:               fmt.Sprintf("log group %s has had no events in the lookback period", g.Name),
			})
		}

		if g.RetentionDays == 0 {
			findings = append(findings, Finding{
				Provider:             ProviderAWS,
				Type:                 FindingNoRetention,
				ResourceID:           g.Name,
				ResourceARN:          g.ARN,
				StoredBytes:          g.StoredBytes,
				DailyIngestionBytes:  g.IncomingBytes,
				EstimatedMonthlyCost: cost,
				Detail:               fmt.Sprintf("log group %s has no retention policy; logs are stored indefinitely", g.Name),
			})
		}

		if g.StoredBytes == 0 {
			findings = append(findings, Finding{
				Provider:             ProviderAWS,
				Type:                 FindingEmpty,
				ResourceID:           g.Name,
				ResourceARN:          g.ARN,
				StoredBytes:          0,
				DailyIngestionBytes:  g.IncomingBytes,
				EstimatedMonthlyCost: 0,
				Detail:               fmt.Sprintf("log group %s stores zero bytes", g.Name),
			})
		}

		if cfg.HighIngestionBytesPerDay > 0 && g.IncomingBytes > cfg.HighIngestionBytesPerDay {
			findings = append(findings, Finding{
				Provider:             ProviderAWS,
				Type:                 FindingHighIngestion,
				ResourceID:           g.Name,
				ResourceARN:          g.ARN,
				StoredBytes:          g.StoredBytes,
				DailyIngestionBytes:  g.IncomingBytes,
				EstimatedMonthlyCost: cost,
				Detail:               fmt.Sprintf("log group %s ingests %.0f bytes/day, exceeding threshold", g.Name, g.IncomingBytes),
			})
		}

		if !g.HasMetricFilter && !g.HasSubscription {
			findings = append(findings, Finding{
				Provider:             ProviderAWS,
				Type:                 FindingUnread,
				ResourceID:           g.Name,
				ResourceARN:          g.ARN,
				StoredBytes:          g.StoredBytes,
				DailyIngestionBytes:  g.IncomingBytes,
				EstimatedMonthlyCost: cost,
				Detail:               fmt.Sprintf("log group %s has no metric filters or subscriptions", g.Name),
			})
		}
	}

	if findings == nil {
		findings = []Finding{}
	}
	return findings
}

// AnalyzeGCP produces findings from GCP log bucket and sink scan results.
func AnalyzeGCP(buckets []gcp.LogBucketInfo, sinks []gcp.SinkInfo, cfg Config) []Finding {
	var findings []Finding

	for _, b := range buckets {
		if b.RetentionDays == 0 && !b.IsDefault {
			findings = append(findings, Finding{
				Provider:             ProviderGCP,
				Type:                 FindingNoRetention,
				ResourceID:           b.Name,
				EstimatedMonthlyCost: 0,
				Detail:               fmt.Sprintf("bucket %s has no retention policy", b.Name),
			})
		}
	}

	lookback := cfg.LookbackDays
	if lookback <= 0 {
		lookback = 90
	}

	for _, s := range sinks {
		dailyBytes := s.ExportedBytes / float64(lookback)

		if s.IsStale {
			findings = append(findings, Finding{
				Provider:             ProviderGCP,
				Type:                 FindingStaleSink,
				ResourceID:           s.Name,
				DailyIngestionBytes:  0,
				EstimatedMonthlyCost: 0,
				Detail:               fmt.Sprintf("sink %s has exported zero bytes in the lookback period", s.Name),
			})
		}

		if cfg.HighIngestionBytesPerDay > 0 && dailyBytes > cfg.HighIngestionBytesPerDay {
			findings = append(findings, Finding{
				Provider:             ProviderGCP,
				Type:                 FindingHighIngestion,
				ResourceID:           s.Name,
				DailyIngestionBytes:  dailyBytes,
				EstimatedMonthlyCost: calcGCPCost(dailyBytes),
				Detail:               fmt.Sprintf("sink %s exports %.0f bytes/day, exceeding threshold", s.Name, dailyBytes),
			})
		}
	}

	if findings == nil {
		findings = []Finding{}
	}
	return findings
}

// AnalyzeAzure produces findings from Azure Monitor workspace scan results.
func AnalyzeAzure(workspaces []azure.WorkspaceInfo, cfg Config) []Finding {
	var findings []Finding

	for _, ws := range workspaces {
		dailyBytes, cost := azureWorkspaceCost(ws)

		if ws.IsStale {
			findings = append(findings, Finding{
				Provider:             ProviderAzure,
				Type:                 FindingStale,
				ResourceID:           ws.Name,
				ResourceARN:          ws.ID,
				DailyIngestionBytes:  dailyBytes,
				EstimatedMonthlyCost: cost,
				Detail:               fmt.Sprintf("workspace %s has had no heartbeats in the lookback period", ws.Name),
			})
		}

		if ws.RetentionDays == 0 {
			findings = append(findings, Finding{
				Provider:             ProviderAzure,
				Type:                 FindingNoRetention,
				ResourceID:           ws.Name,
				ResourceARN:          ws.ID,
				DailyIngestionBytes:  dailyBytes,
				EstimatedMonthlyCost: cost,
				Detail:               fmt.Sprintf("workspace %s has default retention; consider setting explicit policy", ws.Name),
			})
		}
	}

	if findings == nil {
		findings = []Finding{}
	}
	return findings
}

// azureWorkspaceCost returns the estimated daily ingestion bytes and monthly cost
// for an Azure workspace, using DailyQuotaGB as a proxy.
func azureWorkspaceCost(ws azure.WorkspaceInfo) (dailyBytes float64, monthlyCost float64) {
	if ws.DailyQuotaGB <= 0 {
		return 0, 0
	}
	dailyBytes = ws.DailyQuotaGB * bytesPerGB
	monthlyCost = calcAzureCost(dailyBytes)
	return dailyBytes, monthlyCost
}
