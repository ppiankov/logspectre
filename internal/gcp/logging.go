package gcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/logging/v2"
	"google.golang.org/api/monitoring/v3"
)

const (
	metricExportByteCount = "logging.googleapis.com/exports/byte_count"
)

// BucketsAPI abstracts listing log buckets for a project.
type BucketsAPI interface {
	ListBuckets(ctx context.Context, parent string, pageToken string) (*logging.ListBucketsResponse, error)
}

// SinksAPI abstracts listing log sinks for a project.
type SinksAPI interface {
	ListSinks(ctx context.Context, parent string, pageToken string) (*logging.ListSinksResponse, error)
}

// MonitoringAPI abstracts querying time series metrics.
type MonitoringAPI interface {
	ListTimeSeries(ctx context.Context, project string, filter string, startTime string, endTime string, pageToken string) (*monitoring.ListTimeSeriesResponse, error)
}

// LogBucketInfo holds metadata about a single GCP log bucket.
type LogBucketInfo struct {
	Name          string
	Location      string
	RetentionDays int64 // 0 means GCP default (30 days)
	Locked        bool
	CreateTime    string
	UpdateTime    string
	IsDefault     bool // true for _Default and _Required buckets
}

// SinkInfo holds metadata about a log sink with export metrics.
type SinkInfo struct {
	Name          string
	Destination   string
	Disabled      bool
	Filter        string
	ExportedBytes float64 // total bytes exported over lookback period
	IsStale       bool    // true if zero bytes exported in lookback period
}

// LoggingScanner fetches log bucket and sink metadata from GCP Cloud Logging.
type LoggingScanner struct {
	buckets      BucketsAPI
	sinks        SinksAPI
	metrics      MonitoringAPI
	projectID    string
	lookbackDays int
}

// NewLoggingScanner creates a scanner with the given API clients.
func NewLoggingScanner(
	buckets BucketsAPI,
	sinks SinksAPI,
	metrics MonitoringAPI,
	projectID string,
	lookbackDays int,
) *LoggingScanner {
	return &LoggingScanner{
		buckets:      buckets,
		sinks:        sinks,
		metrics:      metrics,
		projectID:    projectID,
		lookbackDays: lookbackDays,
	}
}

// Scan lists all log buckets and sinks, enriching sinks with export metrics.
func (s *LoggingScanner) Scan(ctx context.Context) ([]LogBucketInfo, []SinkInfo, error) {
	buckets, err := s.listAllBuckets(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list buckets: %w", err)
	}

	rawSinks, err := s.listAllSinks(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list sinks: %w", err)
	}

	sinks := make([]SinkInfo, 0, len(rawSinks))
	for _, sink := range rawSinks {
		info, err := s.enrichSink(ctx, sink)
		if err != nil {
			return nil, nil, fmt.Errorf("enrich sink %s: %w", sink.Name, err)
		}
		sinks = append(sinks, info)
	}

	return buckets, sinks, nil
}

func (s *LoggingScanner) listAllBuckets(ctx context.Context) ([]LogBucketInfo, error) {
	parent := fmt.Sprintf("projects/%s/locations/-", s.projectID)
	var buckets []LogBucketInfo
	pageToken := ""

	for {
		resp, err := s.buckets.ListBuckets(ctx, parent, pageToken)
		if err != nil {
			return nil, err
		}
		for _, b := range resp.Buckets {
			buckets = append(buckets, LogBucketInfo{
				Name:          b.Name,
				Location:      extractLocation(b.Name),
				RetentionDays: b.RetentionDays,
				Locked:        b.Locked,
				CreateTime:    b.CreateTime,
				UpdateTime:    b.UpdateTime,
				IsDefault:     isDefaultBucket(b.Name),
			})
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	if buckets == nil {
		buckets = []LogBucketInfo{}
	}
	return buckets, nil
}

func (s *LoggingScanner) listAllSinks(ctx context.Context) ([]*logging.LogSink, error) {
	parent := fmt.Sprintf("projects/%s", s.projectID)
	var sinks []*logging.LogSink
	pageToken := ""

	for {
		resp, err := s.sinks.ListSinks(ctx, parent, pageToken)
		if err != nil {
			return nil, err
		}
		sinks = append(sinks, resp.Sinks...)
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	return sinks, nil
}

func (s *LoggingScanner) enrichSink(ctx context.Context, sink *logging.LogSink) (SinkInfo, error) {
	info := SinkInfo{
		Name:        sink.Name,
		Destination: sink.Destination,
		Disabled:    sink.Disabled,
		Filter:      sink.Filter,
	}

	exportedBytes, err := s.getSinkExportBytes(ctx, sink.Name)
	if err != nil {
		return SinkInfo{}, err
	}
	info.ExportedBytes = exportedBytes
	info.IsStale = exportedBytes == 0

	return info, nil
}

func (s *LoggingScanner) getSinkExportBytes(ctx context.Context, sinkName string) (float64, error) {
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -s.lookbackDays)

	filter := fmt.Sprintf(
		`metric.type="%s" AND resource.labels.name="%s"`,
		metricExportByteCount,
		extractSinkShortName(sinkName),
	)

	project := fmt.Sprintf("projects/%s", s.projectID)
	startTime := start.Format(time.RFC3339)
	endTime := now.Format(time.RFC3339)

	var totalBytes float64
	pageToken := ""

	for {
		resp, err := s.metrics.ListTimeSeries(ctx, project, filter, startTime, endTime, pageToken)
		if err != nil {
			return 0, fmt.Errorf("list time series for sink %s: %w", sinkName, err)
		}
		for _, ts := range resp.TimeSeries {
			for _, point := range ts.Points {
				if point.Value != nil && point.Value.Int64Value != nil {
					totalBytes += float64(*point.Value.Int64Value)
				} else if point.Value != nil && point.Value.DoubleValue != nil {
					totalBytes += *point.Value.DoubleValue
				}
			}
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	return totalBytes, nil
}

// extractLocation extracts the location from a bucket resource name.
// Format: projects/{project}/locations/{location}/buckets/{bucket}
func extractLocation(bucketName string) string {
	parts := strings.Split(bucketName, "/")
	for i, p := range parts {
		if p == "locations" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// extractSinkShortName extracts the short name from a sink resource name.
func extractSinkShortName(sinkName string) string {
	parts := strings.Split(sinkName, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return sinkName
}

// isDefaultBucket returns true if the bucket is _Default or _Required.
func isDefaultBucket(bucketName string) bool {
	return strings.HasSuffix(bucketName, "/buckets/_Default") ||
		strings.HasSuffix(bucketName, "/buckets/_Required")
}

// --- Adapter structs bridging real SDK to interfaces ---

type loggingBucketsAdapter struct {
	svc *logging.Service
}

func (a *loggingBucketsAdapter) ListBuckets(ctx context.Context, parent string, pageToken string) (*logging.ListBucketsResponse, error) {
	call := a.svc.Projects.Locations.Buckets.List(parent).Context(ctx)
	if pageToken != "" {
		call = call.PageToken(pageToken)
	}
	return call.Do()
}

type loggingSinksAdapter struct {
	svc *logging.Service
}

func (a *loggingSinksAdapter) ListSinks(ctx context.Context, parent string, pageToken string) (*logging.ListSinksResponse, error) {
	call := a.svc.Projects.Sinks.List(parent).Context(ctx)
	if pageToken != "" {
		call = call.PageToken(pageToken)
	}
	return call.Do()
}

type monitoringAdapter struct {
	svc *monitoring.Service
}

func (a *monitoringAdapter) ListTimeSeries(ctx context.Context, project string, filter string, startTime string, endTime string, pageToken string) (*monitoring.ListTimeSeriesResponse, error) {
	call := a.svc.Projects.TimeSeries.List(project).
		Context(ctx).
		Filter(filter).
		IntervalStartTime(startTime).
		IntervalEndTime(endTime)
	if pageToken != "" {
		call = call.PageToken(pageToken)
	}
	return call.Do()
}
