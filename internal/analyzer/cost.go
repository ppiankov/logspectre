package analyzer

const (
	daysPerMonth = 30

	bytesPerGB  = 1_000_000_000      // decimal gigabyte (AWS, Azure)
	bytesPerGiB = 1024 * 1024 * 1024 // binary gibibyte (GCP)

	// AWS CloudWatch Logs: $0.50/GB ingest + $0.03/GB/month storage
	awsIngestRatePerByte  = 0.50 / bytesPerGB
	awsStorageRatePerByte = 0.03 / bytesPerGB

	// GCP Cloud Logging: $0.50/GiB ingestion (no separate storage rate)
	gcpIngestRatePerByte = 0.50 / bytesPerGiB

	// Azure Monitor Log Analytics: $2.76/GB ingestion
	azureIngestRatePerByte = 2.76 / bytesPerGB
)

// calcAWSCost returns the estimated monthly cost in USD for a CloudWatch log group.
func calcAWSCost(storedBytes int64, dailyIngestionBytes float64) float64 {
	storage := float64(storedBytes) * awsStorageRatePerByte
	ingestion := dailyIngestionBytes * daysPerMonth * awsIngestRatePerByte
	return storage + ingestion
}

// calcGCPCost returns the estimated monthly cost in USD for a GCP sink.
func calcGCPCost(dailyIngestionBytes float64) float64 {
	return dailyIngestionBytes * daysPerMonth * gcpIngestRatePerByte
}

// calcAzureCost returns the estimated monthly cost in USD for an Azure workspace.
func calcAzureCost(dailyIngestionBytes float64) float64 {
	return dailyIngestionBytes * daysPerMonth * azureIngestRatePerByte
}
