package analyzer

// Provider identifies which cloud platform a finding originates from.
type Provider string

const (
	ProviderAWS   Provider = "aws"
	ProviderGCP   Provider = "gcp"
	ProviderAzure Provider = "azure"
)

// FindingType classifies the nature of a waste finding.
type FindingType string

const (
	// FindingStale: resource had no log activity in the lookback period.
	FindingStale FindingType = "stale"

	// FindingNoRetention: log storage has no expiration policy (infinite cost accrual).
	FindingNoRetention FindingType = "no_retention"

	// FindingEmpty: resource stores zero bytes.
	FindingEmpty FindingType = "empty"

	// FindingHighIngestion: daily ingestion exceeds the configured threshold.
	FindingHighIngestion FindingType = "high_ingestion"

	// FindingUnread: logs are written but never consumed (no metric filters or subscriptions).
	FindingUnread FindingType = "unread"

	// FindingStaleSink: GCP sink has exported zero bytes in the lookback period.
	FindingStaleSink FindingType = "stale_sink"
)

// Finding represents a single waste or cost anomaly detected for a cloud resource.
type Finding struct {
	Provider             Provider
	Type                 FindingType
	ResourceID           string // log group name, bucket name, workspace name, or sink name
	ResourceARN          string // ARN or full resource path; empty when unavailable
	StoredBytes          int64
	DailyIngestionBytes  float64
	EstimatedMonthlyCost float64 // USD
	Detail               string  // human-readable explanation
}

// Config controls analyzer behavior and threshold values.
type Config struct {
	// LookbackDays is the scanning window used by the scanners. Required for
	// normalizing GCP sink totals (ExportedBytes is a total, not a daily average).
	LookbackDays int

	// HighIngestionBytesPerDay is the daily ingestion threshold above which
	// a FindingHighIngestion is raised.
	HighIngestionBytesPerDay float64
}
