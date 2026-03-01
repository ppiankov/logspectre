package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/ppiankov/logspectre/internal/analyzer"
)

func TestTextReporter_Empty(t *testing.T) {
	reporter := NewTextReporter(TextOptions{GeneratedAt: fixedTime})
	var buf bytes.Buffer
	if err := reporter.Write(&buf, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "no findings") {
		t.Errorf("expected 'no findings' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "2026-01-01T00:00:00Z") {
		t.Errorf("expected timestamp in output, got:\n%s", out)
	}
}

func TestTextReporter_SingleFinding(t *testing.T) {
	findings := []analyzer.Finding{
		{
			Provider:             analyzer.ProviderAWS,
			Type:                 analyzer.FindingStale,
			ResourceID:           "/app/stale",
			StoredBytes:          5_000_000_000,
			DailyIngestionBytes:  0,
			EstimatedMonthlyCost: 0.15,
			Detail:               "log group /app/stale has had no events in the lookback period",
		},
	}
	reporter := NewTextReporter(TextOptions{GeneratedAt: fixedTime})
	var buf bytes.Buffer
	if err := reporter.Write(&buf, findings); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out := buf.String()

	checks := []string{
		"logspectre",
		"1 findings",
		"AWS",
		"RESOURCE",
		"TYPE",
		"EST. COST/MO",
		"/app/stale",
		"stale",
		"5.0 GB",
		"$0.15",
		"Total: $0.15",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestTextReporter_MultiProvider(t *testing.T) {
	findings := []analyzer.Finding{
		{
			Provider:             analyzer.ProviderGCP,
			Type:                 analyzer.FindingStaleSink,
			ResourceID:           "dead-sink",
			EstimatedMonthlyCost: 0,
		},
		{
			Provider:             analyzer.ProviderAWS,
			Type:                 analyzer.FindingNoRetention,
			ResourceID:           "/app/logs",
			StoredBytes:          1_000_000_000,
			EstimatedMonthlyCost: 10.00,
		},
		{
			Provider:             analyzer.ProviderAzure,
			Type:                 analyzer.FindingStale,
			ResourceID:           "stale-ws",
			EstimatedMonthlyCost: 5.00,
		},
	}
	reporter := NewTextReporter(TextOptions{GeneratedAt: fixedTime})
	var buf bytes.Buffer
	if err := reporter.Write(&buf, findings); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out := buf.String()

	// All three providers should appear
	for _, p := range []string{"AWS", "GCP", "Azure"} {
		if !strings.Contains(out, p) {
			t.Errorf("expected provider %q in output", p)
		}
	}

	// AWS should appear before GCP, GCP before Azure (provider order)
	awsIdx := strings.Index(out, "AWS")
	gcpIdx := strings.Index(out, "GCP")
	azureIdx := strings.Index(out, "Azure")
	if awsIdx >= gcpIdx || gcpIdx >= azureIdx {
		t.Errorf("expected provider order AWS < GCP < Azure, got AWS=%d GCP=%d Azure=%d", awsIdx, gcpIdx, azureIdx)
	}

	if !strings.Contains(out, "Total: $15.00") {
		t.Errorf("expected Total: $15.00 in output, got:\n%s", out)
	}
}

func TestTextReporter_SortedByCost(t *testing.T) {
	findings := []analyzer.Finding{
		{
			Provider:             analyzer.ProviderAWS,
			Type:                 analyzer.FindingStale,
			ResourceID:           "/app/cheap",
			EstimatedMonthlyCost: 1.00,
		},
		{
			Provider:             analyzer.ProviderAWS,
			Type:                 analyzer.FindingNoRetention,
			ResourceID:           "/app/expensive",
			EstimatedMonthlyCost: 50.00,
		},
	}
	reporter := NewTextReporter(TextOptions{GeneratedAt: fixedTime})
	var buf bytes.Buffer
	if err := reporter.Write(&buf, findings); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out := buf.String()

	expIdx := strings.Index(out, "/app/expensive")
	cheapIdx := strings.Index(out, "/app/cheap")
	if expIdx >= cheapIdx {
		t.Errorf("expected /app/expensive before /app/cheap (cost descending)")
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{999, "999 B"},
		{1_000, "1.0 KB"},
		{1_500_000, "1.5 MB"},
		{1_000_000_000, "1.0 GB"},
		{5_500_000_000, "5.5 GB"},
	}
	for _, tt := range tests {
		got := formatBytes(tt.input)
		if got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatBytesFloat(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{0, "0 B"},
		{-1, "0 B"},
		{1_000_000_000, "1.0 GB"},
	}
	for _, tt := range tests {
		got := formatBytesFloat(tt.input)
		if got != tt.want {
			t.Errorf("formatBytesFloat(%f) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSortFindings(t *testing.T) {
	findings := []analyzer.Finding{
		{ResourceID: "b", EstimatedMonthlyCost: 5},
		{ResourceID: "a", EstimatedMonthlyCost: 10},
		{ResourceID: "c", EstimatedMonthlyCost: 5},
	}
	sorted := sortFindings(findings)

	// Original should not be mutated
	if findings[0].ResourceID != "b" {
		t.Error("original slice was mutated")
	}

	// Sorted: a (10), b (5), c (5) — cost desc, then ResourceID asc
	if sorted[0].ResourceID != "a" {
		t.Errorf("expected first=a, got %s", sorted[0].ResourceID)
	}
	if sorted[1].ResourceID != "b" {
		t.Errorf("expected second=b, got %s", sorted[1].ResourceID)
	}
	if sorted[2].ResourceID != "c" {
		t.Errorf("expected third=c, got %s", sorted[2].ResourceID)
	}
}

func TestTextReporter_DefaultTime(t *testing.T) {
	reporter := NewTextReporter(TextOptions{})
	var buf bytes.Buffer
	if err := reporter.Write(&buf, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out := buf.String()
	// Should contain current year, not zero time
	if strings.Contains(out, "0001-01-01") {
		t.Error("expected current time, got zero time")
	}
	now := time.Now().UTC()
	if !strings.Contains(out, now.Format("2006")) {
		t.Errorf("expected current year in output, got:\n%s", out)
	}
}
