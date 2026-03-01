package aws

import (
	"context"
	"errors"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	logstypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

// --- Mock implementations ---

type mockLogGroupsAPI struct {
	pages [][]logstypes.LogGroup
	calls int
	err   error
}

func (m *mockLogGroupsAPI) DescribeLogGroups(_ context.Context, _ *cloudwatchlogs.DescribeLogGroupsInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogGroupsOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.calls >= len(m.pages) {
		return &cloudwatchlogs.DescribeLogGroupsOutput{}, nil
	}
	out := &cloudwatchlogs.DescribeLogGroupsOutput{
		LogGroups: m.pages[m.calls],
	}
	m.calls++
	if m.calls < len(m.pages) {
		out.NextToken = awssdk.String("next")
	}
	return out, nil
}

type mockMetricFiltersAPI struct {
	filters []logstypes.MetricFilter
	err     error
}

func (m *mockMetricFiltersAPI) DescribeMetricFilters(_ context.Context, _ *cloudwatchlogs.DescribeMetricFiltersInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeMetricFiltersOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &cloudwatchlogs.DescribeMetricFiltersOutput{
		MetricFilters: m.filters,
	}, nil
}

type mockSubscriptionFiltersAPI struct {
	filters []logstypes.SubscriptionFilter
	err     error
}

func (m *mockSubscriptionFiltersAPI) DescribeSubscriptionFilters(_ context.Context, _ *cloudwatchlogs.DescribeSubscriptionFiltersInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeSubscriptionFiltersOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &cloudwatchlogs.DescribeSubscriptionFiltersOutput{
		SubscriptionFilters: m.filters,
	}, nil
}

type mockMetricsAPI struct {
	responses map[string]*cloudwatch.GetMetricStatisticsOutput
	err       error
}

func (m *mockMetricsAPI) GetMetricStatistics(_ context.Context, params *cloudwatch.GetMetricStatisticsInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricStatisticsOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	key := awssdk.ToString(params.MetricName)
	if out, ok := m.responses[key]; ok {
		return out, nil
	}
	return &cloudwatch.GetMetricStatisticsOutput{}, nil
}

// --- Helper to build a scanner with defaults ---

func newTestScanner(
	logs *mockLogGroupsAPI,
	mf *mockMetricFiltersAPI,
	sf *mockSubscriptionFiltersAPI,
	metrics *mockMetricsAPI,
) *CloudWatchScanner {
	if logs == nil {
		logs = &mockLogGroupsAPI{}
	}
	if mf == nil {
		mf = &mockMetricFiltersAPI{}
	}
	if sf == nil {
		sf = &mockSubscriptionFiltersAPI{}
	}
	if metrics == nil {
		metrics = &mockMetricsAPI{responses: map[string]*cloudwatch.GetMetricStatisticsOutput{}}
	}
	return NewCloudWatchScanner(logs, mf, sf, metrics, 90)
}

func makeLogGroup(name string, storedBytes *int64, retentionDays *int32) logstypes.LogGroup {
	return logstypes.LogGroup{
		LogGroupName:    awssdk.String(name),
		Arn:             awssdk.String("arn:aws:logs:us-east-1:123456789012:log-group:" + name),
		StoredBytes:     storedBytes,
		RetentionInDays: retentionDays,
		CreationTime:    awssdk.Int64(1700000000000),
	}
}

// --- Tests ---

func TestScan_ListsAllLogGroupsWithPagination(t *testing.T) {
	logs := &mockLogGroupsAPI{
		pages: [][]logstypes.LogGroup{
			{makeLogGroup("/app/page1-a", awssdk.Int64(100), awssdk.Int32(30)), makeLogGroup("/app/page1-b", awssdk.Int64(200), awssdk.Int32(30))},
			{makeLogGroup("/app/page2-a", awssdk.Int64(300), awssdk.Int32(30)), makeLogGroup("/app/page2-b", awssdk.Int64(400), awssdk.Int32(30))},
		},
	}
	scanner := newTestScanner(logs, nil, nil, nil)

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 log groups, got %d", len(results))
	}
	if results[0].Name != "/app/page1-a" {
		t.Errorf("expected first group /app/page1-a, got %s", results[0].Name)
	}
	if results[3].Name != "/app/page2-b" {
		t.Errorf("expected last group /app/page2-b, got %s", results[3].Name)
	}
}

func TestScan_EmptyLogGroup(t *testing.T) {
	logs := &mockLogGroupsAPI{
		pages: [][]logstypes.LogGroup{
			{makeLogGroup("/app/empty", awssdk.Int64(0), awssdk.Int32(30))},
		},
	}
	scanner := newTestScanner(logs, nil, nil, nil)

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].StoredBytes != 0 {
		t.Errorf("expected StoredBytes=0, got %d", results[0].StoredBytes)
	}
}

func TestScan_NilStoredBytes(t *testing.T) {
	logs := &mockLogGroupsAPI{
		pages: [][]logstypes.LogGroup{
			{makeLogGroup("/app/nil-bytes", nil, awssdk.Int32(30))},
		},
	}
	scanner := newTestScanner(logs, nil, nil, nil)

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].StoredBytes != 0 {
		t.Errorf("expected StoredBytes=0 for nil, got %d", results[0].StoredBytes)
	}
}

func TestScan_NoRetention(t *testing.T) {
	logs := &mockLogGroupsAPI{
		pages: [][]logstypes.LogGroup{
			{makeLogGroup("/app/no-retention", awssdk.Int64(100), nil)},
		},
	}
	scanner := newTestScanner(logs, nil, nil, nil)

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].RetentionDays != 0 {
		t.Errorf("expected RetentionDays=0 for nil, got %d", results[0].RetentionDays)
	}
}

func TestScan_WithRetention(t *testing.T) {
	logs := &mockLogGroupsAPI{
		pages: [][]logstypes.LogGroup{
			{makeLogGroup("/app/retained", awssdk.Int64(100), awssdk.Int32(30))},
		},
	}
	scanner := newTestScanner(logs, nil, nil, nil)

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].RetentionDays != 30 {
		t.Errorf("expected RetentionDays=30, got %d", results[0].RetentionDays)
	}
}

func TestScan_Stale(t *testing.T) {
	logs := &mockLogGroupsAPI{
		pages: [][]logstypes.LogGroup{
			{makeLogGroup("/app/stale", awssdk.Int64(100), awssdk.Int32(30))},
		},
	}
	metrics := &mockMetricsAPI{
		responses: map[string]*cloudwatch.GetMetricStatisticsOutput{
			metricIncomingBytes:  {Datapoints: []cwtypes.Datapoint{{Sum: awssdk.Float64(0)}}},
			metricIncomingEvents: {Datapoints: []cwtypes.Datapoint{{Sum: awssdk.Float64(0)}}},
		},
	}
	scanner := newTestScanner(logs, nil, nil, metrics)

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !results[0].IsStale {
		t.Error("expected IsStale=true for zero events")
	}
	if results[0].IncomingEvents != 0 {
		t.Errorf("expected IncomingEvents=0, got %f", results[0].IncomingEvents)
	}
}

func TestScan_Active(t *testing.T) {
	logs := &mockLogGroupsAPI{
		pages: [][]logstypes.LogGroup{
			{makeLogGroup("/app/active", awssdk.Int64(100), awssdk.Int32(30))},
		},
	}
	metrics := &mockMetricsAPI{
		responses: map[string]*cloudwatch.GetMetricStatisticsOutput{
			metricIncomingBytes:  {Datapoints: []cwtypes.Datapoint{{Sum: awssdk.Float64(1000)}}},
			metricIncomingEvents: {Datapoints: []cwtypes.Datapoint{{Sum: awssdk.Float64(500)}}},
		},
	}
	scanner := newTestScanner(logs, nil, nil, metrics)

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].IsStale {
		t.Error("expected IsStale=false for active group")
	}
	if results[0].IncomingEvents != 500 {
		t.Errorf("expected IncomingEvents=500, got %f", results[0].IncomingEvents)
	}
}

func TestScan_HighIngestion(t *testing.T) {
	// 90GB total over 90 days = 1GB/day average
	totalBytes := float64(90) * 1073741824
	logs := &mockLogGroupsAPI{
		pages: [][]logstypes.LogGroup{
			{makeLogGroup("/app/heavy", awssdk.Int64(100), awssdk.Int32(30))},
		},
	}
	metrics := &mockMetricsAPI{
		responses: map[string]*cloudwatch.GetMetricStatisticsOutput{
			metricIncomingBytes:  {Datapoints: []cwtypes.Datapoint{{Sum: awssdk.Float64(totalBytes)}}},
			metricIncomingEvents: {Datapoints: []cwtypes.Datapoint{{Sum: awssdk.Float64(1000000)}}},
		},
	}
	scanner := newTestScanner(logs, nil, nil, metrics)

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedDaily := 1073741824.0 // 1GB
	if results[0].IncomingBytes != expectedDaily {
		t.Errorf("expected IncomingBytes=%.0f, got %.0f", expectedDaily, results[0].IncomingBytes)
	}
}

func TestScan_Unread_NoFilters(t *testing.T) {
	logs := &mockLogGroupsAPI{
		pages: [][]logstypes.LogGroup{
			{makeLogGroup("/app/unread", awssdk.Int64(100), awssdk.Int32(30))},
		},
	}
	scanner := newTestScanner(logs, nil, nil, nil)

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].HasMetricFilter {
		t.Error("expected HasMetricFilter=false")
	}
	if results[0].HasSubscription {
		t.Error("expected HasSubscription=false")
	}
}

func TestScan_HasMetricFilter(t *testing.T) {
	logs := &mockLogGroupsAPI{
		pages: [][]logstypes.LogGroup{
			{makeLogGroup("/app/filtered", awssdk.Int64(100), awssdk.Int32(30))},
		},
	}
	mf := &mockMetricFiltersAPI{
		filters: []logstypes.MetricFilter{{}},
	}
	scanner := newTestScanner(logs, mf, nil, nil)

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !results[0].HasMetricFilter {
		t.Error("expected HasMetricFilter=true")
	}
}

func TestScan_HasSubscription(t *testing.T) {
	logs := &mockLogGroupsAPI{
		pages: [][]logstypes.LogGroup{
			{makeLogGroup("/app/subscribed", awssdk.Int64(100), awssdk.Int32(30))},
		},
	}
	sf := &mockSubscriptionFiltersAPI{
		filters: []logstypes.SubscriptionFilter{{}},
	}
	scanner := newTestScanner(logs, nil, sf, nil)

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !results[0].HasSubscription {
		t.Error("expected HasSubscription=true")
	}
}

func TestScan_EmptyAccount(t *testing.T) {
	logs := &mockLogGroupsAPI{
		pages: [][]logstypes.LogGroup{},
	}
	scanner := newTestScanner(logs, nil, nil, nil)

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

func TestScan_NoDatapoints(t *testing.T) {
	logs := &mockLogGroupsAPI{
		pages: [][]logstypes.LogGroup{
			{makeLogGroup("/app/no-data", awssdk.Int64(100), awssdk.Int32(30))},
		},
	}
	metrics := &mockMetricsAPI{
		responses: map[string]*cloudwatch.GetMetricStatisticsOutput{
			metricIncomingBytes:  {Datapoints: []cwtypes.Datapoint{}},
			metricIncomingEvents: {Datapoints: []cwtypes.Datapoint{}},
		},
	}
	scanner := newTestScanner(logs, nil, nil, metrics)

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].IncomingBytes != 0 {
		t.Errorf("expected IncomingBytes=0 for no datapoints, got %f", results[0].IncomingBytes)
	}
	if results[0].IncomingEvents != 0 {
		t.Errorf("expected IncomingEvents=0 for no datapoints, got %f", results[0].IncomingEvents)
	}
	if !results[0].IsStale {
		t.Error("expected IsStale=true for no datapoints")
	}
}

func TestScan_ErrorOnDescribeLogGroups(t *testing.T) {
	logs := &mockLogGroupsAPI{err: errors.New("access denied")}
	scanner := newTestScanner(logs, nil, nil, nil)

	_, err := scanner.Scan(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestScan_ErrorOnMetricFilters(t *testing.T) {
	logs := &mockLogGroupsAPI{
		pages: [][]logstypes.LogGroup{
			{makeLogGroup("/app/fail", awssdk.Int64(100), awssdk.Int32(30))},
		},
	}
	mf := &mockMetricFiltersAPI{err: errors.New("throttled")}
	scanner := newTestScanner(logs, mf, nil, nil)

	_, err := scanner.Scan(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestScan_ErrorOnSubscriptionFilters(t *testing.T) {
	logs := &mockLogGroupsAPI{
		pages: [][]logstypes.LogGroup{
			{makeLogGroup("/app/fail", awssdk.Int64(100), awssdk.Int32(30))},
		},
	}
	mf := &mockMetricFiltersAPI{}
	sf := &mockSubscriptionFiltersAPI{err: errors.New("throttled")}
	scanner := newTestScanner(logs, mf, sf, nil)

	_, err := scanner.Scan(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestScan_ErrorOnGetMetricStatistics(t *testing.T) {
	logs := &mockLogGroupsAPI{
		pages: [][]logstypes.LogGroup{
			{makeLogGroup("/app/fail", awssdk.Int64(100), awssdk.Int32(30))},
		},
	}
	metrics := &mockMetricsAPI{err: errors.New("service unavailable")}
	scanner := newTestScanner(logs, nil, nil, metrics)

	_, err := scanner.Scan(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
