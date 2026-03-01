package report

import (
	"encoding/json"
	"io"
	"time"

	"github.com/ppiankov/logspectre/internal/analyzer"
)

const spectreSchema = "spectre/v1"

type spectreEnvelope struct {
	Schema      string           `json:"schema"`
	GeneratedAt string           `json:"generated_at"`
	Target      spectreTarget    `json:"target"`
	Summary     spectreSummary   `json:"summary"`
	Findings    []spectreFinding `json:"findings"`
}

type spectreTarget struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type spectreSummary struct {
	FindingCount            int     `json:"finding_count"`
	EstimatedMonthlyCostUSD float64 `json:"estimated_monthly_cost_usd"`
}

type spectreFinding struct {
	Provider                string  `json:"provider"`
	Type                    string  `json:"type"`
	ResourceID              string  `json:"resource_id"`
	ResourceARN             string  `json:"resource_arn,omitempty"`
	StoredBytes             int64   `json:"stored_bytes"`
	DailyIngestionBytes     float64 `json:"daily_ingestion_bytes"`
	EstimatedMonthlyCostUSD float64 `json:"estimated_monthly_cost_usd"`
	Detail                  string  `json:"detail"`
}

// SpectrehubReporter writes spectre/v1 JSON envelopes.
type SpectrehubReporter struct {
	opts SpectrehubOptions
}

// NewSpectrehubReporter creates a reporter with the given options.
func NewSpectrehubReporter(opts SpectrehubOptions) *SpectrehubReporter {
	return &SpectrehubReporter{opts: opts}
}

// Write implements Reporter.
func (r *SpectrehubReporter) Write(w io.Writer, findings []analyzer.Finding) error {
	sorted := sortFindings(findings)

	jsonFindings := make([]spectreFinding, 0, len(sorted))
	var totalCost float64
	for _, f := range sorted {
		totalCost += f.EstimatedMonthlyCost
		jsonFindings = append(jsonFindings, spectreFinding{
			Provider:                string(f.Provider),
			Type:                    string(f.Type),
			ResourceID:              f.ResourceID,
			ResourceARN:             f.ResourceARN,
			StoredBytes:             f.StoredBytes,
			DailyIngestionBytes:     f.DailyIngestionBytes,
			EstimatedMonthlyCostUSD: f.EstimatedMonthlyCost,
			Detail:                  f.Detail,
		})
	}

	generatedAt := r.opts.GeneratedAt
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}

	envelope := spectreEnvelope{
		Schema:      spectreSchema,
		GeneratedAt: generatedAt.Format(time.RFC3339),
		Target: spectreTarget{
			Type: r.opts.TargetType,
			ID:   r.opts.TargetID,
		},
		Summary: spectreSummary{
			FindingCount:            len(jsonFindings),
			EstimatedMonthlyCostUSD: totalCost,
		},
		Findings: jsonFindings,
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(envelope)
}
