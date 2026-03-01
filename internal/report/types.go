package report

import (
	"io"
	"sort"
	"time"

	"github.com/ppiankov/logspectre/internal/analyzer"
)

// Reporter writes a formatted report of findings to w.
type Reporter interface {
	Write(w io.Writer, findings []analyzer.Finding) error
}

// SpectrehubOptions controls the spectre/v1 envelope metadata.
type SpectrehubOptions struct {
	// TargetType must be one of "aws-account", "gcp-project", "azure-subscription".
	TargetType string
	// TargetID is the account, project, or subscription identifier.
	TargetID string
	// GeneratedAt is the scan timestamp. Injected for deterministic tests.
	GeneratedAt time.Time
}

// TextOptions controls text report rendering.
type TextOptions struct {
	// GeneratedAt is the scan timestamp shown in the header.
	GeneratedAt time.Time
}

// sortFindings returns a sorted copy of findings (cost descending, ResourceID ascending as tiebreaker).
func sortFindings(findings []analyzer.Finding) []analyzer.Finding {
	sorted := make([]analyzer.Finding, len(findings))
	copy(sorted, findings)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].EstimatedMonthlyCost != sorted[j].EstimatedMonthlyCost {
			return sorted[i].EstimatedMonthlyCost > sorted[j].EstimatedMonthlyCost
		}
		return sorted[i].ResourceID < sorted[j].ResourceID
	})
	return sorted
}
