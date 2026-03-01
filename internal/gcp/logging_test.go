package gcp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/api/logging/v2"
	"google.golang.org/api/monitoring/v3"
)

// --- Mock implementations ---

type mockBucketsAPI struct {
	responses []*logging.ListBucketsResponse
	calls     int
	err       error
}

func (m *mockBucketsAPI) ListBuckets(_ context.Context, _ string, _ string) (*logging.ListBucketsResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.calls >= len(m.responses) {
		return &logging.ListBucketsResponse{}, nil
	}
	resp := m.responses[m.calls]
	m.calls++
	return resp, nil
}

type mockSinksAPI struct {
	responses []*logging.ListSinksResponse
	calls     int
	err       error
}

func (m *mockSinksAPI) ListSinks(_ context.Context, _ string, _ string) (*logging.ListSinksResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.calls >= len(m.responses) {
		return &logging.ListSinksResponse{}, nil
	}
	resp := m.responses[m.calls]
	m.calls++
	return resp, nil
}

type mockMonitoringAPI struct {
	responses map[string]*monitoring.ListTimeSeriesResponse
	err       error
}

func (m *mockMonitoringAPI) ListTimeSeries(_ context.Context, _ string, filter string, _ string, _ string, _ string) (*monitoring.ListTimeSeriesResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	for key, resp := range m.responses {
		if strings.Contains(filter, key) {
			return resp, nil
		}
	}
	return &monitoring.ListTimeSeriesResponse{}, nil
}

// --- Helpers ---

func newTestScanner(
	buckets *mockBucketsAPI,
	sinks *mockSinksAPI,
	metrics *mockMonitoringAPI,
) *LoggingScanner {
	if buckets == nil {
		buckets = &mockBucketsAPI{responses: []*logging.ListBucketsResponse{{}}}
	}
	if sinks == nil {
		sinks = &mockSinksAPI{responses: []*logging.ListSinksResponse{{}}}
	}
	if metrics == nil {
		metrics = &mockMonitoringAPI{responses: map[string]*monitoring.ListTimeSeriesResponse{}}
	}
	return NewLoggingScanner(buckets, sinks, metrics, "test-project", 90)
}

func makeLogBucket(name string, retentionDays int64, locked bool) *logging.LogBucket {
	return &logging.LogBucket{
		Name:          name,
		RetentionDays: retentionDays,
		Locked:        locked,
		CreateTime:    "2024-01-01T00:00:00Z",
		UpdateTime:    "2024-06-01T00:00:00Z",
	}
}

func makeLogSink(name string, destination string, disabled bool) *logging.LogSink {
	return &logging.LogSink{
		Name:        name,
		Destination: destination,
		Disabled:    disabled,
	}
}

func int64Ptr(v int64) *int64       { return &v }
func float64Ptr(v float64) *float64 { return &v }

// --- Bucket tests ---

func TestScan_ListsAllBucketsWithPagination(t *testing.T) {
	buckets := &mockBucketsAPI{
		responses: []*logging.ListBucketsResponse{
			{
				Buckets:       []*logging.LogBucket{makeLogBucket("projects/p/locations/us-east1/buckets/app-logs", 90, false)},
				NextPageToken: "page2",
			},
			{
				Buckets: []*logging.LogBucket{makeLogBucket("projects/p/locations/eu-west1/buckets/audit-logs", 365, true)},
			},
		},
	}
	scanner := newTestScanner(buckets, nil, nil)

	result, _, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(result))
	}
	if result[0].Name != "projects/p/locations/us-east1/buckets/app-logs" {
		t.Errorf("unexpected first bucket: %s", result[0].Name)
	}
	if result[1].Name != "projects/p/locations/eu-west1/buckets/audit-logs" {
		t.Errorf("unexpected second bucket: %s", result[1].Name)
	}
}

func TestScan_EmptyProject(t *testing.T) {
	buckets := &mockBucketsAPI{responses: []*logging.ListBucketsResponse{{}}}
	sinks := &mockSinksAPI{responses: []*logging.ListSinksResponse{{}}}
	scanner := newTestScanner(buckets, sinks, nil)

	result, sinkResult, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected empty slice, got nil")
	}
	if len(result) != 0 {
		t.Errorf("expected 0 buckets, got %d", len(result))
	}
	if len(sinkResult) != 0 {
		t.Errorf("expected 0 sinks, got %d", len(sinkResult))
	}
}

func TestScan_BucketRetentionNotSet(t *testing.T) {
	buckets := &mockBucketsAPI{
		responses: []*logging.ListBucketsResponse{
			{Buckets: []*logging.LogBucket{makeLogBucket("projects/p/locations/global/buckets/no-retention", 0, false)}},
		},
	}
	scanner := newTestScanner(buckets, nil, nil)

	result, _, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].RetentionDays != 0 {
		t.Errorf("expected RetentionDays=0, got %d", result[0].RetentionDays)
	}
}

func TestScan_BucketWithRetention(t *testing.T) {
	buckets := &mockBucketsAPI{
		responses: []*logging.ListBucketsResponse{
			{Buckets: []*logging.LogBucket{makeLogBucket("projects/p/locations/global/buckets/retained", 365, false)}},
		},
	}
	scanner := newTestScanner(buckets, nil, nil)

	result, _, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].RetentionDays != 365 {
		t.Errorf("expected RetentionDays=365, got %d", result[0].RetentionDays)
	}
}

func TestScan_DefaultBuckets(t *testing.T) {
	buckets := &mockBucketsAPI{
		responses: []*logging.ListBucketsResponse{
			{Buckets: []*logging.LogBucket{
				makeLogBucket("projects/p/locations/global/buckets/_Default", 30, false),
				makeLogBucket("projects/p/locations/global/buckets/_Required", 400, true),
				makeLogBucket("projects/p/locations/global/buckets/custom", 90, false),
			}},
		},
	}
	scanner := newTestScanner(buckets, nil, nil)

	result, _, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result[0].IsDefault {
		t.Error("expected _Default to be flagged as default")
	}
	if !result[1].IsDefault {
		t.Error("expected _Required to be flagged as default")
	}
	if result[2].IsDefault {
		t.Error("expected custom bucket NOT to be flagged as default")
	}
}

func TestScan_LockedBucket(t *testing.T) {
	buckets := &mockBucketsAPI{
		responses: []*logging.ListBucketsResponse{
			{Buckets: []*logging.LogBucket{makeLogBucket("projects/p/locations/global/buckets/locked", 90, true)}},
		},
	}
	scanner := newTestScanner(buckets, nil, nil)

	result, _, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result[0].Locked {
		t.Error("expected Locked=true")
	}
}

func TestScan_BucketLocation(t *testing.T) {
	buckets := &mockBucketsAPI{
		responses: []*logging.ListBucketsResponse{
			{Buckets: []*logging.LogBucket{makeLogBucket("projects/p/locations/us-central1/buckets/regional", 90, false)}},
		},
	}
	scanner := newTestScanner(buckets, nil, nil)

	result, _, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].Location != "us-central1" {
		t.Errorf("expected location us-central1, got %s", result[0].Location)
	}
}

// --- Sink tests ---

func TestScan_SinkWithExportData(t *testing.T) {
	sinks := &mockSinksAPI{
		responses: []*logging.ListSinksResponse{
			{Sinks: []*logging.LogSink{makeLogSink("projects/p/sinks/bq-export", "bigquery.googleapis.com/projects/p/datasets/logs", false)}},
		},
	}
	metrics := &mockMonitoringAPI{
		responses: map[string]*monitoring.ListTimeSeriesResponse{
			"bq-export": {
				TimeSeries: []*monitoring.TimeSeries{
					{Points: []*monitoring.Point{{Value: &monitoring.TypedValue{Int64Value: int64Ptr(5000)}}}},
				},
			},
		},
	}
	scanner := newTestScanner(nil, sinks, metrics)

	_, sinkResult, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sinkResult) != 1 {
		t.Fatalf("expected 1 sink, got %d", len(sinkResult))
	}
	if sinkResult[0].ExportedBytes != 5000 {
		t.Errorf("expected ExportedBytes=5000, got %f", sinkResult[0].ExportedBytes)
	}
	if sinkResult[0].IsStale {
		t.Error("expected IsStale=false for active sink")
	}
}

func TestScan_StaleSink(t *testing.T) {
	sinks := &mockSinksAPI{
		responses: []*logging.ListSinksResponse{
			{Sinks: []*logging.LogSink{makeLogSink("projects/p/sinks/dead-sink", "storage.googleapis.com/dead-bucket", false)}},
		},
	}
	metrics := &mockMonitoringAPI{
		responses: map[string]*monitoring.ListTimeSeriesResponse{},
	}
	scanner := newTestScanner(nil, sinks, metrics)

	_, sinkResult, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sinkResult[0].IsStale {
		t.Error("expected IsStale=true for sink with no export data")
	}
	if sinkResult[0].ExportedBytes != 0 {
		t.Errorf("expected ExportedBytes=0, got %f", sinkResult[0].ExportedBytes)
	}
}

func TestScan_DisabledSink(t *testing.T) {
	sinks := &mockSinksAPI{
		responses: []*logging.ListSinksResponse{
			{Sinks: []*logging.LogSink{makeLogSink("projects/p/sinks/disabled", "storage.googleapis.com/bucket", true)}},
		},
	}
	scanner := newTestScanner(nil, sinks, nil)

	_, sinkResult, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sinkResult[0].Disabled {
		t.Error("expected Disabled=true")
	}
}

func TestScan_SinkPagination(t *testing.T) {
	sinks := &mockSinksAPI{
		responses: []*logging.ListSinksResponse{
			{
				Sinks:         []*logging.LogSink{makeLogSink("projects/p/sinks/sink-1", "storage.googleapis.com/b1", false)},
				NextPageToken: "page2",
			},
			{
				Sinks: []*logging.LogSink{makeLogSink("projects/p/sinks/sink-2", "storage.googleapis.com/b2", false)},
			},
		},
	}
	scanner := newTestScanner(nil, sinks, nil)

	_, sinkResult, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sinkResult) != 2 {
		t.Fatalf("expected 2 sinks, got %d", len(sinkResult))
	}
}

func TestScan_NoSinks(t *testing.T) {
	sinks := &mockSinksAPI{responses: []*logging.ListSinksResponse{{}}}
	scanner := newTestScanner(nil, sinks, nil)

	_, sinkResult, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sinkResult) != 0 {
		t.Errorf("expected 0 sinks, got %d", len(sinkResult))
	}
}

func TestScan_SinkWithDoubleValue(t *testing.T) {
	sinks := &mockSinksAPI{
		responses: []*logging.ListSinksResponse{
			{Sinks: []*logging.LogSink{makeLogSink("projects/p/sinks/dbl-sink", "storage.googleapis.com/b", false)}},
		},
	}
	metrics := &mockMonitoringAPI{
		responses: map[string]*monitoring.ListTimeSeriesResponse{
			"dbl-sink": {
				TimeSeries: []*monitoring.TimeSeries{
					{Points: []*monitoring.Point{{Value: &monitoring.TypedValue{DoubleValue: float64Ptr(1234.56)}}}},
				},
			},
		},
	}
	scanner := newTestScanner(nil, sinks, metrics)

	_, sinkResult, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sinkResult[0].ExportedBytes != 1234.56 {
		t.Errorf("expected ExportedBytes=1234.56, got %f", sinkResult[0].ExportedBytes)
	}
}

// --- Error propagation tests ---

func TestScan_ErrorOnListBuckets(t *testing.T) {
	buckets := &mockBucketsAPI{err: errors.New("permission denied")}
	scanner := newTestScanner(buckets, nil, nil)

	_, _, err := scanner.Scan(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestScan_ErrorOnListSinks(t *testing.T) {
	sinks := &mockSinksAPI{err: errors.New("quota exceeded")}
	scanner := newTestScanner(nil, sinks, nil)

	_, _, err := scanner.Scan(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestScan_ErrorOnListTimeSeries(t *testing.T) {
	sinks := &mockSinksAPI{
		responses: []*logging.ListSinksResponse{
			{Sinks: []*logging.LogSink{makeLogSink("projects/p/sinks/fail", "storage.googleapis.com/b", false)}},
		},
	}
	metrics := &mockMonitoringAPI{err: errors.New("service unavailable")}
	scanner := newTestScanner(nil, sinks, metrics)

	_, _, err := scanner.Scan(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- Helper function tests ---

func TestExtractLocation(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"projects/p/locations/us-east1/buckets/b", "us-east1"},
		{"projects/p/locations/global/buckets/_Default", "global"},
		{"invalid-path", ""},
	}
	for _, tt := range tests {
		got := extractLocation(tt.input)
		if got != tt.want {
			t.Errorf("extractLocation(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractSinkShortName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"projects/p/sinks/my-sink", "my-sink"},
		{"my-sink", "my-sink"},
	}
	for _, tt := range tests {
		got := extractSinkShortName(tt.input)
		if got != tt.want {
			t.Errorf("extractSinkShortName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsDefaultBucket(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"projects/p/locations/global/buckets/_Default", true},
		{"projects/p/locations/global/buckets/_Required", true},
		{"projects/p/locations/global/buckets/custom", false},
	}
	for _, tt := range tests {
		got := isDefaultBucket(tt.input)
		if got != tt.want {
			t.Errorf("isDefaultBucket(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
