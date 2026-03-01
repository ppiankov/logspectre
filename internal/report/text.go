package report

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/ppiankov/logspectre/internal/analyzer"
)

const (
	bytesInKB = 1_000
	bytesInMB = 1_000_000
	bytesInGB = 1_000_000_000
)

// providerOrder defines the display order for provider groups.
var providerOrder = []analyzer.Provider{
	analyzer.ProviderAWS,
	analyzer.ProviderGCP,
	analyzer.ProviderAzure,
}

// providerLabel maps provider to display name.
var providerLabel = map[analyzer.Provider]string{
	analyzer.ProviderAWS:   "AWS",
	analyzer.ProviderGCP:   "GCP",
	analyzer.ProviderAzure: "Azure",
}

// TextReporter writes human-readable tabular reports.
type TextReporter struct {
	opts TextOptions
}

// NewTextReporter creates a reporter with the given options.
func NewTextReporter(opts TextOptions) *TextReporter {
	return &TextReporter{opts: opts}
}

// Write implements Reporter.
func (r *TextReporter) Write(w io.Writer, findings []analyzer.Finding) error {
	generatedAt := r.opts.GeneratedAt
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}

	sorted := sortFindings(findings)

	if len(sorted) == 0 {
		_, err := fmt.Fprintf(w, "logspectre — %s\nno findings\n", generatedAt.Format(time.RFC3339))
		return err
	}

	// Compute total cost and provider count.
	var totalCost float64
	providers := map[analyzer.Provider]bool{}
	for _, f := range sorted {
		totalCost += f.EstimatedMonthlyCost
		providers[f.Provider] = true
	}

	_, err := fmt.Fprintf(w, "logspectre — %s\n%d findings across %d provider(s)   estimated monthly waste: $%.2f\n",
		generatedAt.Format(time.RFC3339), len(sorted), len(providers), totalCost)
	if err != nil {
		return err
	}

	// Group findings by provider.
	grouped := map[analyzer.Provider][]analyzer.Finding{}
	for _, f := range sorted {
		grouped[f.Provider] = append(grouped[f.Provider], f)
	}

	for _, p := range providerOrder {
		group, ok := grouped[p]
		if !ok {
			continue
		}

		label := providerLabel[p]
		_, err = fmt.Fprintf(w, "\n%s\n", label)
		if err != nil {
			return err
		}

		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		_, err = fmt.Fprintf(tw, "  RESOURCE\tTYPE\tSTORED\tDAILY IN\tEST. COST/MO\n")
		if err != nil {
			return err
		}
		for _, f := range group {
			_, err = fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t$%.2f\n",
				f.ResourceID,
				string(f.Type),
				formatBytes(f.StoredBytes),
				formatBytesFloat(f.DailyIngestionBytes),
				f.EstimatedMonthlyCost,
			)
			if err != nil {
				return err
			}
		}
		if err = tw.Flush(); err != nil {
			return err
		}
	}

	_, err = fmt.Fprintf(w, "\nTotal: $%.2f\n", totalCost)
	return err
}

func formatBytes(n int64) string {
	switch {
	case n >= bytesInGB:
		return fmt.Sprintf("%.1f GB", float64(n)/bytesInGB)
	case n >= bytesInMB:
		return fmt.Sprintf("%.1f MB", float64(n)/bytesInMB)
	case n >= bytesInKB:
		return fmt.Sprintf("%.1f KB", float64(n)/bytesInKB)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func formatBytesFloat(f float64) string {
	if f < 0 {
		return "0 B"
	}
	return formatBytes(int64(f))
}
