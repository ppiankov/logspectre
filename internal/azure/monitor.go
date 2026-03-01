package azure

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/monitor/armmonitor"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/operationalinsights/armoperationalinsights"
)

const (
	metricHeartbeat = "Heartbeat"
)

// WorkspaceData is an intermediate type that decouples interface consumers from the SDK.
type WorkspaceData struct {
	Name          string
	ID            string // full Azure resource ID
	ResourceGroup string
	Location      string
	RetentionDays int32   // 0 means service default (30 days)
	DailyQuotaGB  float64 // -1 means unlimited
}

// MetricResult holds a single metric's time series data.
type MetricResult struct {
	Name       string
	TimeSeries []TimeSeriesData
}

// TimeSeriesData holds an ordered set of metric data points.
type TimeSeriesData struct {
	Values []MetricPoint
}

// MetricPoint holds a single aggregated metric data point.
type MetricPoint struct {
	Total   float64
	Average float64
	Count   float64
}

// WorkspaceInfo holds enriched metadata about a single Log Analytics workspace.
type WorkspaceInfo struct {
	Name           string
	ID             string
	ResourceGroup  string
	Location       string
	RetentionDays  int32
	DailyQuotaGB   float64 // -1 means unlimited
	HeartbeatCount float64 // total heartbeat count over lookback period
	IsStale        bool    // true if zero heartbeats in lookback period
}

// WorkspacesAPI abstracts listing Log Analytics workspaces in a subscription.
type WorkspacesAPI interface {
	ListWorkspaces(ctx context.Context) ([]WorkspaceData, error)
}

// MetricsAPI abstracts querying Azure Monitor metrics for a resource.
type MetricsAPI interface {
	GetMetrics(ctx context.Context, resourceURI string, metricNames string, timespan string, aggregation string) ([]MetricResult, error)
}

// MonitorScanner fetches Log Analytics workspace metadata from Azure Monitor.
type MonitorScanner struct {
	workspaces     WorkspacesAPI
	metrics        MetricsAPI
	subscriptionID string
	lookbackDays   int
}

// NewMonitorScanner creates a scanner with the given API clients.
func NewMonitorScanner(
	workspaces WorkspacesAPI,
	metrics MetricsAPI,
	subscriptionID string,
	lookbackDays int,
) *MonitorScanner {
	return &MonitorScanner{
		workspaces:     workspaces,
		metrics:        metrics,
		subscriptionID: subscriptionID,
		lookbackDays:   lookbackDays,
	}
}

// Scan lists all Log Analytics workspaces and enriches each with heartbeat metrics.
func (s *MonitorScanner) Scan(ctx context.Context) ([]WorkspaceInfo, error) {
	workspaces, err := s.listAllWorkspaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}

	results := make([]WorkspaceInfo, 0, len(workspaces))
	for _, ws := range workspaces {
		info, err := s.enrichWorkspace(ctx, ws)
		if err != nil {
			return nil, fmt.Errorf("enrich workspace %s: %w", ws.Name, err)
		}
		results = append(results, info)
	}
	return results, nil
}

func (s *MonitorScanner) listAllWorkspaces(ctx context.Context) ([]WorkspaceData, error) {
	return s.workspaces.ListWorkspaces(ctx)
}

func (s *MonitorScanner) enrichWorkspace(ctx context.Context, ws WorkspaceData) (WorkspaceInfo, error) {
	info := WorkspaceInfo{
		Name:          ws.Name,
		ID:            ws.ID,
		ResourceGroup: ws.ResourceGroup,
		Location:      ws.Location,
		RetentionDays: ws.RetentionDays,
		DailyQuotaGB:  ws.DailyQuotaGB,
	}

	heartbeatCount, err := s.getHeartbeatCount(ctx, ws.ID)
	if err != nil {
		return WorkspaceInfo{}, err
	}
	info.HeartbeatCount = heartbeatCount
	info.IsStale = heartbeatCount == 0

	return info, nil
}

func (s *MonitorScanner) getHeartbeatCount(ctx context.Context, resourceID string) (float64, error) {
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -s.lookbackDays)

	timespan := fmt.Sprintf("%s/%s", start.Format(time.RFC3339), now.Format(time.RFC3339))

	results, err := s.metrics.GetMetrics(ctx, resourceID, metricHeartbeat, timespan, "Total")
	if err != nil {
		return 0, fmt.Errorf("get heartbeat metrics for %s: %w", resourceID, err)
	}

	var totalCount float64
	for _, metric := range results {
		for _, ts := range metric.TimeSeries {
			for _, point := range ts.Values {
				totalCount += point.Total
			}
		}
	}

	return totalCount, nil
}

// extractResourceGroup extracts the resource group name from an Azure resource ID.
// Format: /subscriptions/{sub}/resourceGroups/{rg}/providers/...
func extractResourceGroup(resourceID string) string {
	parts := strings.Split(resourceID, "/")
	for i, p := range parts {
		if strings.EqualFold(p, "resourceGroups") && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// toWorkspaceData converts an SDK Workspace to our intermediate type.
func toWorkspaceData(ws *armoperationalinsights.Workspace) WorkspaceData {
	data := WorkspaceData{
		DailyQuotaGB: -1, // default: unlimited
	}

	if ws.Name != nil {
		data.Name = *ws.Name
	}
	if ws.ID != nil {
		data.ID = *ws.ID
		data.ResourceGroup = extractResourceGroup(*ws.ID)
	}
	if ws.Location != nil {
		data.Location = *ws.Location
	}

	if ws.Properties != nil {
		if ws.Properties.RetentionInDays != nil {
			data.RetentionDays = *ws.Properties.RetentionInDays
		}
		if ws.Properties.WorkspaceCapping != nil && ws.Properties.WorkspaceCapping.DailyQuotaGb != nil {
			data.DailyQuotaGB = *ws.Properties.WorkspaceCapping.DailyQuotaGb
		}
	}

	return data
}

// toMetricResults converts SDK Response metrics to our intermediate types.
func toMetricResults(resp armmonitor.MetricsClientListResponse) []MetricResult {
	results := make([]MetricResult, 0, len(resp.Value))
	for _, m := range resp.Value {
		result := MetricResult{}
		if m.Name != nil && m.Name.Value != nil {
			result.Name = *m.Name.Value
		}
		for _, ts := range m.Timeseries {
			tsData := TimeSeriesData{}
			for _, mv := range ts.Data {
				point := MetricPoint{}
				if mv.Total != nil {
					point.Total = *mv.Total
				}
				if mv.Average != nil {
					point.Average = *mv.Average
				}
				if mv.Count != nil {
					point.Count = *mv.Count
				}
				tsData.Values = append(tsData.Values, point)
			}
			result.TimeSeries = append(result.TimeSeries, tsData)
		}
		results = append(results, result)
	}
	return results
}

// --- Adapter structs bridging real SDK to interfaces ---

type workspacesAdapter struct {
	client         *armoperationalinsights.WorkspacesClient
	subscriptionID string
}

func (a *workspacesAdapter) ListWorkspaces(ctx context.Context) ([]WorkspaceData, error) {
	pager := a.client.NewListPager(nil)
	var workspaces []WorkspaceData
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, ws := range page.Value {
			workspaces = append(workspaces, toWorkspaceData(ws))
		}
	}
	if workspaces == nil {
		workspaces = []WorkspaceData{}
	}
	return workspaces, nil
}

type metricsAdapter struct {
	client *armmonitor.MetricsClient
}

func (a *metricsAdapter) GetMetrics(ctx context.Context, resourceURI string, metricNames string, timespan string, aggregation string) ([]MetricResult, error) {
	resp, err := a.client.List(ctx, resourceURI, &armmonitor.MetricsClientListOptions{
		Metricnames: &metricNames,
		Timespan:    &timespan,
		Aggregation: &aggregation,
	})
	if err != nil {
		return nil, err
	}
	return toMetricResults(resp), nil
}
