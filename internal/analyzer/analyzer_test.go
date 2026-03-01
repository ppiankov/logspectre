package analyzer

import (
	"math"
	"testing"

	"github.com/ppiankov/logspectre/internal/aws"
	"github.com/ppiankov/logspectre/internal/azure"
	"github.com/ppiankov/logspectre/internal/gcp"
)

const (
	oneGB         = 1_000_000_000
	oneGiB        = 1024 * 1024 * 1024
	costTolerance = 0.000001
	testLookback  = 90
	testThreshold = float64(oneGiB) // 1 GiB/day
)

func defaultCfg() Config {
	return Config{
		LookbackDays:             testLookback,
		HighIngestionBytesPerDay: testThreshold,
	}
}

func assertClose(t *testing.T, got, want float64, label string) {
	t.Helper()
	if math.Abs(got-want) > costTolerance {
		t.Errorf("%s: got %f, want %f", label, got, want)
	}
}

func findByType(findings []Finding, ft FindingType) *Finding {
	for i := range findings {
		if findings[i].Type == ft {
			return &findings[i]
		}
	}
	return nil
}

// --- Cost function tests ---

func TestCalcAWSCost_StorageOnly(t *testing.T) {
	cost := calcAWSCost(oneGB, 0)
	assertClose(t, cost, 0.03, "storage-only")
}

func TestCalcAWSCost_IngestionOnly(t *testing.T) {
	cost := calcAWSCost(0, oneGB)
	assertClose(t, cost, 15.0, "ingestion-only")
}

func TestCalcAWSCost_Combined(t *testing.T) {
	cost := calcAWSCost(oneGB, oneGB)
	assertClose(t, cost, 15.03, "combined")
}

func TestCalcAWSCost_KnownValues(t *testing.T) {
	// 10 GB stored + 5 GB/day ingestion
	cost := calcAWSCost(10*oneGB, 5*oneGB)
	// storage: 10 * 0.03 = 0.30, ingestion: 5 * 30 * 0.50 = 75.00
	assertClose(t, cost, 75.30, "known-values")
}

func TestCalcGCPCost_ZeroIngestion(t *testing.T) {
	cost := calcGCPCost(0)
	assertClose(t, cost, 0, "zero-ingestion")
}

func TestCalcGCPCost_KnownValues(t *testing.T) {
	// 1 GiB/day
	cost := calcGCPCost(oneGiB)
	assertClose(t, cost, 15.0, "gcp-1gib")
}

func TestCalcAzureCost_KnownValues(t *testing.T) {
	// 1 GB/day
	cost := calcAzureCost(oneGB)
	assertClose(t, cost, 82.80, "azure-1gb")
}

// --- AWS analyzer tests ---

func TestAnalyzeAWS_StaleGroup(t *testing.T) {
	groups := []aws.LogGroupInfo{
		{Name: "/app/stale", ARN: "arn:stale", StoredBytes: 100, RetentionDays: 30, IsStale: true, HasMetricFilter: true, HasSubscription: true},
	}
	findings := AnalyzeAWS(groups, defaultCfg())

	f := findByType(findings, FindingStale)
	if f == nil {
		t.Fatal("expected FindingStale")
	}
	if f.ResourceID != "/app/stale" {
		t.Errorf("expected ResourceID /app/stale, got %s", f.ResourceID)
	}
}

func TestAnalyzeAWS_NoRetention(t *testing.T) {
	groups := []aws.LogGroupInfo{
		{Name: "/app/no-ret", ARN: "arn:no-ret", StoredBytes: 100, RetentionDays: 0, HasMetricFilter: true, HasSubscription: true},
	}
	findings := AnalyzeAWS(groups, defaultCfg())

	f := findByType(findings, FindingNoRetention)
	if f == nil {
		t.Fatal("expected FindingNoRetention")
	}
}

func TestAnalyzeAWS_EmptyGroup(t *testing.T) {
	groups := []aws.LogGroupInfo{
		{Name: "/app/empty", ARN: "arn:empty", StoredBytes: 0, RetentionDays: 30, HasMetricFilter: true, HasSubscription: true},
	}
	findings := AnalyzeAWS(groups, defaultCfg())

	f := findByType(findings, FindingEmpty)
	if f == nil {
		t.Fatal("expected FindingEmpty")
	}
	if f.EstimatedMonthlyCost != 0 {
		t.Errorf("expected cost=0 for empty group, got %f", f.EstimatedMonthlyCost)
	}
}

func TestAnalyzeAWS_HighIngestion(t *testing.T) {
	groups := []aws.LogGroupInfo{
		{Name: "/app/heavy", ARN: "arn:heavy", StoredBytes: 100, RetentionDays: 30, IncomingBytes: 2 * oneGiB, HasMetricFilter: true, HasSubscription: true},
	}
	findings := AnalyzeAWS(groups, defaultCfg())

	f := findByType(findings, FindingHighIngestion)
	if f == nil {
		t.Fatal("expected FindingHighIngestion")
	}
}

func TestAnalyzeAWS_Unread(t *testing.T) {
	groups := []aws.LogGroupInfo{
		{Name: "/app/unread", ARN: "arn:unread", StoredBytes: 100, RetentionDays: 30, HasMetricFilter: false, HasSubscription: false},
	}
	findings := AnalyzeAWS(groups, defaultCfg())

	f := findByType(findings, FindingUnread)
	if f == nil {
		t.Fatal("expected FindingUnread")
	}
}

func TestAnalyzeAWS_MultipleFindings(t *testing.T) {
	groups := []aws.LogGroupInfo{
		{Name: "/app/bad", ARN: "arn:bad", StoredBytes: 100, RetentionDays: 0, IsStale: true, HasMetricFilter: true, HasSubscription: true},
	}
	findings := AnalyzeAWS(groups, defaultCfg())

	stale := findByType(findings, FindingStale)
	noRet := findByType(findings, FindingNoRetention)
	if stale == nil {
		t.Error("expected FindingStale")
	}
	if noRet == nil {
		t.Error("expected FindingNoRetention")
	}
	if len(findings) < 2 {
		t.Errorf("expected at least 2 findings, got %d", len(findings))
	}
}

func TestAnalyzeAWS_EmptyInput(t *testing.T) {
	findings := AnalyzeAWS(nil, defaultCfg())
	if findings == nil {
		t.Fatal("expected empty slice, got nil")
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(findings))
	}
}

// --- GCP analyzer tests ---

func TestAnalyzeGCP_StaleSink(t *testing.T) {
	sinks := []gcp.SinkInfo{
		{Name: "dead-sink", Destination: "storage.googleapis.com/bucket", IsStale: true, ExportedBytes: 0},
	}
	findings := AnalyzeGCP(nil, sinks, defaultCfg())

	f := findByType(findings, FindingStaleSink)
	if f == nil {
		t.Fatal("expected FindingStaleSink")
	}
	if f.EstimatedMonthlyCost != 0 {
		t.Errorf("expected cost=0 for stale sink, got %f", f.EstimatedMonthlyCost)
	}
}

func TestAnalyzeGCP_HighIngestionSink(t *testing.T) {
	// Total exported over 90 days: 2 GiB/day * 90 = 180 GiB
	totalBytes := float64(2*oneGiB) * testLookback
	sinks := []gcp.SinkInfo{
		{Name: "heavy-sink", ExportedBytes: totalBytes, IsStale: false},
	}
	findings := AnalyzeGCP(nil, sinks, defaultCfg())

	f := findByType(findings, FindingHighIngestion)
	if f == nil {
		t.Fatal("expected FindingHighIngestion")
	}
	if f.EstimatedMonthlyCost <= 0 {
		t.Error("expected positive cost for high ingestion sink")
	}
}

func TestAnalyzeGCP_NoRetentionBucket(t *testing.T) {
	buckets := []gcp.LogBucketInfo{
		{Name: "projects/p/locations/global/buckets/custom", RetentionDays: 0, IsDefault: false},
	}
	findings := AnalyzeGCP(buckets, nil, defaultCfg())

	f := findByType(findings, FindingNoRetention)
	if f == nil {
		t.Fatal("expected FindingNoRetention for custom bucket with no retention")
	}
}

func TestAnalyzeGCP_DefaultBucketSkipped(t *testing.T) {
	buckets := []gcp.LogBucketInfo{
		{Name: "projects/p/locations/global/buckets/_Default", RetentionDays: 0, IsDefault: true},
	}
	findings := AnalyzeGCP(buckets, nil, defaultCfg())

	f := findByType(findings, FindingNoRetention)
	if f != nil {
		t.Error("expected default bucket to be skipped for no_retention")
	}
}

func TestAnalyzeGCP_EmptyInput(t *testing.T) {
	findings := AnalyzeGCP(nil, nil, defaultCfg())
	if findings == nil {
		t.Fatal("expected empty slice, got nil")
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(findings))
	}
}

// --- Azure analyzer tests ---

func TestAnalyzeAzure_StaleWorkspace(t *testing.T) {
	workspaces := []azure.WorkspaceInfo{
		{Name: "stale-ws", ID: "/subscriptions/sub/resourceGroups/rg/providers/.../stale-ws", IsStale: true, RetentionDays: 30, DailyQuotaGB: 5},
	}
	findings := AnalyzeAzure(workspaces, defaultCfg())

	f := findByType(findings, FindingStale)
	if f == nil {
		t.Fatal("expected FindingStale")
	}
	if f.ResourceARN != workspaces[0].ID {
		t.Errorf("expected ResourceARN=%s, got %s", workspaces[0].ID, f.ResourceARN)
	}
}

func TestAnalyzeAzure_NoRetention(t *testing.T) {
	workspaces := []azure.WorkspaceInfo{
		{Name: "default-ret-ws", RetentionDays: 0, DailyQuotaGB: 5},
	}
	findings := AnalyzeAzure(workspaces, defaultCfg())

	f := findByType(findings, FindingNoRetention)
	if f == nil {
		t.Fatal("expected FindingNoRetention")
	}
}

func TestAnalyzeAzure_CostViaQuota(t *testing.T) {
	workspaces := []azure.WorkspaceInfo{
		{Name: "quota-ws", RetentionDays: 30, DailyQuotaGB: 1, IsStale: true},
	}
	findings := AnalyzeAzure(workspaces, defaultCfg())

	f := findByType(findings, FindingStale)
	if f == nil {
		t.Fatal("expected FindingStale")
	}
	// 1 GB/day * 30 days * $2.76/GB = $82.80
	assertClose(t, f.EstimatedMonthlyCost, 82.80, "azure-cost-via-quota")
}

func TestAnalyzeAzure_UnlimitedQuotaNoCost(t *testing.T) {
	workspaces := []azure.WorkspaceInfo{
		{Name: "unlimited-ws", RetentionDays: 0, DailyQuotaGB: -1, IsStale: true},
	}
	findings := AnalyzeAzure(workspaces, defaultCfg())

	for _, f := range findings {
		if f.EstimatedMonthlyCost != 0 {
			t.Errorf("expected cost=0 for unlimited quota, got %f (type=%s)", f.EstimatedMonthlyCost, f.Type)
		}
	}
}

func TestAnalyzeAzure_EmptyInput(t *testing.T) {
	findings := AnalyzeAzure(nil, defaultCfg())
	if findings == nil {
		t.Fatal("expected empty slice, got nil")
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(findings))
	}
}
