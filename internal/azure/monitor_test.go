package azure

import (
	"context"
	"errors"
	"testing"
)

// --- Mock implementations ---

type mockWorkspacesAPI struct {
	workspaces []WorkspaceData
	err        error
}

func (m *mockWorkspacesAPI) ListWorkspaces(_ context.Context) ([]WorkspaceData, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.workspaces, nil
}

type mockMetricsAPI struct {
	responses map[string][]MetricResult
	err       error
}

func (m *mockMetricsAPI) GetMetrics(_ context.Context, resourceURI string, _ string, _ string, _ string) ([]MetricResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	if results, ok := m.responses[resourceURI]; ok {
		return results, nil
	}
	return []MetricResult{}, nil
}

// --- Helpers ---

func newTestScanner(
	workspaces *mockWorkspacesAPI,
	metrics *mockMetricsAPI,
) *MonitorScanner {
	if workspaces == nil {
		workspaces = &mockWorkspacesAPI{workspaces: []WorkspaceData{}}
	}
	if metrics == nil {
		metrics = &mockMetricsAPI{responses: map[string][]MetricResult{}}
	}
	return NewMonitorScanner(workspaces, metrics, "test-subscription", 90)
}

func makeWorkspace(name string, retentionDays int32, dailyQuotaGB float64) WorkspaceData {
	return WorkspaceData{
		Name:          name,
		ID:            "/subscriptions/sub-123/resourceGroups/rg-logs/providers/Microsoft.OperationalInsights/workspaces/" + name,
		ResourceGroup: "rg-logs",
		Location:      "eastus",
		RetentionDays: retentionDays,
		DailyQuotaGB:  dailyQuotaGB,
	}
}

// --- Workspace listing tests ---

func TestScan_MultipleWorkspaces(t *testing.T) {
	ws := &mockWorkspacesAPI{
		workspaces: []WorkspaceData{
			makeWorkspace("prod-logs", 90, -1),
			makeWorkspace("dev-logs", 30, 5),
		},
	}
	scanner := newTestScanner(ws, nil)

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 workspaces, got %d", len(results))
	}
	if results[0].Name != "prod-logs" {
		t.Errorf("expected first workspace prod-logs, got %s", results[0].Name)
	}
	if results[1].Name != "dev-logs" {
		t.Errorf("expected second workspace dev-logs, got %s", results[1].Name)
	}
}

func TestScan_EmptySubscription(t *testing.T) {
	ws := &mockWorkspacesAPI{workspaces: []WorkspaceData{}}
	scanner := newTestScanner(ws, nil)

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results == nil {
		t.Fatal("expected empty slice, got nil")
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// --- Retention tests ---

func TestScan_DefaultRetention(t *testing.T) {
	ws := &mockWorkspacesAPI{
		workspaces: []WorkspaceData{makeWorkspace("default-ret", 0, -1)},
	}
	scanner := newTestScanner(ws, nil)

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].RetentionDays != 0 {
		t.Errorf("expected RetentionDays=0 (default), got %d", results[0].RetentionDays)
	}
}

func TestScan_CustomRetention(t *testing.T) {
	ws := &mockWorkspacesAPI{
		workspaces: []WorkspaceData{makeWorkspace("long-ret", 365, -1)},
	}
	scanner := newTestScanner(ws, nil)

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].RetentionDays != 365 {
		t.Errorf("expected RetentionDays=365, got %d", results[0].RetentionDays)
	}
}

// --- Quota tests ---

func TestScan_UnlimitedQuota(t *testing.T) {
	ws := &mockWorkspacesAPI{
		workspaces: []WorkspaceData{makeWorkspace("unlimited", 30, -1)},
	}
	scanner := newTestScanner(ws, nil)

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].DailyQuotaGB != -1 {
		t.Errorf("expected DailyQuotaGB=-1 (unlimited), got %f", results[0].DailyQuotaGB)
	}
}

func TestScan_CustomQuota(t *testing.T) {
	ws := &mockWorkspacesAPI{
		workspaces: []WorkspaceData{makeWorkspace("limited", 30, 10)},
	}
	scanner := newTestScanner(ws, nil)

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].DailyQuotaGB != 10 {
		t.Errorf("expected DailyQuotaGB=10, got %f", results[0].DailyQuotaGB)
	}
}

// --- Stale detection tests ---

func TestScan_StaleWorkspace(t *testing.T) {
	ws := &mockWorkspacesAPI{
		workspaces: []WorkspaceData{makeWorkspace("stale-ws", 30, -1)},
	}
	scanner := newTestScanner(ws, nil)

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !results[0].IsStale {
		t.Error("expected IsStale=true for workspace with no heartbeat data")
	}
	if results[0].HeartbeatCount != 0 {
		t.Errorf("expected HeartbeatCount=0, got %f", results[0].HeartbeatCount)
	}
}

func TestScan_ActiveWorkspace(t *testing.T) {
	data := makeWorkspace("active-ws", 30, -1)
	ws := &mockWorkspacesAPI{
		workspaces: []WorkspaceData{data},
	}
	metrics := &mockMetricsAPI{
		responses: map[string][]MetricResult{
			data.ID: {
				{
					Name: metricHeartbeat,
					TimeSeries: []TimeSeriesData{
						{Values: []MetricPoint{{Total: 1500}}},
					},
				},
			},
		},
	}
	scanner := newTestScanner(ws, metrics)

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].IsStale {
		t.Error("expected IsStale=false for active workspace")
	}
	if results[0].HeartbeatCount != 1500 {
		t.Errorf("expected HeartbeatCount=1500, got %f", results[0].HeartbeatCount)
	}
}

func TestScan_NoMetricData(t *testing.T) {
	data := makeWorkspace("no-metrics", 30, -1)
	ws := &mockWorkspacesAPI{
		workspaces: []WorkspaceData{data},
	}
	metrics := &mockMetricsAPI{
		responses: map[string][]MetricResult{
			data.ID: {},
		},
	}
	scanner := newTestScanner(ws, metrics)

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !results[0].IsStale {
		t.Error("expected IsStale=true for empty metric results")
	}
}

func TestScan_MultipleTimeSeriesSummed(t *testing.T) {
	data := makeWorkspace("multi-ts", 30, -1)
	ws := &mockWorkspacesAPI{
		workspaces: []WorkspaceData{data},
	}
	metrics := &mockMetricsAPI{
		responses: map[string][]MetricResult{
			data.ID: {
				{
					Name: metricHeartbeat,
					TimeSeries: []TimeSeriesData{
						{Values: []MetricPoint{{Total: 100}, {Total: 200}}},
						{Values: []MetricPoint{{Total: 300}}},
					},
				},
			},
		},
	}
	scanner := newTestScanner(ws, metrics)

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].HeartbeatCount != 600 {
		t.Errorf("expected HeartbeatCount=600, got %f", results[0].HeartbeatCount)
	}
	if results[0].IsStale {
		t.Error("expected IsStale=false for summed heartbeats")
	}
}

// --- Error propagation tests ---

func TestScan_ErrorOnListWorkspaces(t *testing.T) {
	ws := &mockWorkspacesAPI{err: errors.New("access denied")}
	scanner := newTestScanner(ws, nil)

	_, err := scanner.Scan(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestScan_ErrorOnGetMetrics(t *testing.T) {
	ws := &mockWorkspacesAPI{
		workspaces: []WorkspaceData{makeWorkspace("fail-ws", 30, -1)},
	}
	metrics := &mockMetricsAPI{err: errors.New("service unavailable")}
	scanner := newTestScanner(ws, metrics)

	_, err := scanner.Scan(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- Helper function tests ---

func TestExtractResourceGroup(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/subscriptions/sub-123/resourceGroups/my-rg/providers/Microsoft.OperationalInsights/workspaces/ws", "my-rg"},
		{"/subscriptions/sub-123/resourcegroups/my-rg/providers/Microsoft.OperationalInsights/workspaces/ws", "my-rg"},
		{"invalid-path", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := extractResourceGroup(tt.input)
		if got != tt.want {
			t.Errorf("extractResourceGroup(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
