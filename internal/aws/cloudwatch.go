package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	logstypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

const (
	cwLogsNamespace      = "AWS/Logs"
	metricIncomingBytes  = "IncomingBytes"
	metricIncomingEvents = "IncomingLogEvents"
	secondsPerDay        = 86400
)

// LogGroupsAPI abstracts cloudwatchlogs.Client.DescribeLogGroups.
type LogGroupsAPI interface {
	DescribeLogGroups(ctx context.Context, params *cloudwatchlogs.DescribeLogGroupsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogGroupsOutput, error)
}

// MetricFiltersAPI abstracts cloudwatchlogs.Client.DescribeMetricFilters.
type MetricFiltersAPI interface {
	DescribeMetricFilters(ctx context.Context, params *cloudwatchlogs.DescribeMetricFiltersInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeMetricFiltersOutput, error)
}

// SubscriptionFiltersAPI abstracts cloudwatchlogs.Client.DescribeSubscriptionFilters.
type SubscriptionFiltersAPI interface {
	DescribeSubscriptionFilters(ctx context.Context, params *cloudwatchlogs.DescribeSubscriptionFiltersInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeSubscriptionFiltersOutput, error)
}

// MetricsAPI abstracts cloudwatch.Client.GetMetricStatistics.
type MetricsAPI interface {
	GetMetricStatistics(ctx context.Context, params *cloudwatch.GetMetricStatisticsInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricStatisticsOutput, error)
}

// LogGroupInfo holds enriched metadata about a single CloudWatch log group.
type LogGroupInfo struct {
	Name            string
	ARN             string
	StoredBytes     int64
	RetentionDays   int32 // 0 means never expires
	CreationTime    time.Time
	HasMetricFilter bool
	HasSubscription bool
	IncomingBytes   float64 // average daily incoming bytes over lookback period
	IncomingEvents  float64 // total incoming log events over lookback period
	IsStale         bool    // true if zero events in lookback period
}

// CloudWatchScanner fetches log group metadata from AWS CloudWatch Logs.
type CloudWatchScanner struct {
	logs          LogGroupsAPI
	metricFilters MetricFiltersAPI
	subscriptions SubscriptionFiltersAPI
	metrics       MetricsAPI
	lookbackDays  int
}

// NewCloudWatchScanner creates a scanner with the given API clients and lookback period.
func NewCloudWatchScanner(
	logs LogGroupsAPI,
	metricFilters MetricFiltersAPI,
	subscriptions SubscriptionFiltersAPI,
	metrics MetricsAPI,
	lookbackDays int,
) *CloudWatchScanner {
	return &CloudWatchScanner{
		logs:          logs,
		metricFilters: metricFilters,
		subscriptions: subscriptions,
		metrics:       metrics,
		lookbackDays:  lookbackDays,
	}
}

// Scan lists all CloudWatch log groups and enriches each with metric and filter data.
func (s *CloudWatchScanner) Scan(ctx context.Context) ([]LogGroupInfo, error) {
	groups, err := s.listAllLogGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("list log groups: %w", err)
	}

	results := make([]LogGroupInfo, 0, len(groups))
	for _, g := range groups {
		info, err := s.enrichLogGroup(ctx, g)
		if err != nil {
			return nil, fmt.Errorf("enrich log group %s: %w", aws.ToString(g.LogGroupName), err)
		}
		results = append(results, info)
	}
	return results, nil
}

func (s *CloudWatchScanner) listAllLogGroups(ctx context.Context) ([]logstypes.LogGroup, error) {
	var groups []logstypes.LogGroup
	paginator := cloudwatchlogs.NewDescribeLogGroupsPaginator(s.logs, &cloudwatchlogs.DescribeLogGroupsInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		groups = append(groups, page.LogGroups...)
	}
	return groups, nil
}

func (s *CloudWatchScanner) enrichLogGroup(ctx context.Context, g logstypes.LogGroup) (LogGroupInfo, error) {
	name := aws.ToString(g.LogGroupName)

	info := LogGroupInfo{
		Name:         name,
		ARN:          aws.ToString(g.Arn),
		StoredBytes:  aws.ToInt64(g.StoredBytes),
		CreationTime: time.UnixMilli(aws.ToInt64(g.CreationTime)),
	}

	if g.RetentionInDays != nil {
		info.RetentionDays = *g.RetentionInDays
	}

	hasMetric, err := s.hasMetricFilters(ctx, name)
	if err != nil {
		return LogGroupInfo{}, err
	}
	info.HasMetricFilter = hasMetric

	hasSub, err := s.hasSubscriptionFilters(ctx, name)
	if err != nil {
		return LogGroupInfo{}, err
	}
	info.HasSubscription = hasSub

	inBytes, inEvents, err := s.getIngestionMetrics(ctx, name)
	if err != nil {
		return LogGroupInfo{}, err
	}
	info.IncomingBytes = inBytes
	info.IncomingEvents = inEvents
	info.IsStale = inEvents == 0

	return info, nil
}

func (s *CloudWatchScanner) hasMetricFilters(ctx context.Context, logGroupName string) (bool, error) {
	out, err := s.metricFilters.DescribeMetricFilters(ctx, &cloudwatchlogs.DescribeMetricFiltersInput{
		LogGroupName: aws.String(logGroupName),
		Limit:        aws.Int32(1),
	})
	if err != nil {
		return false, fmt.Errorf("describe metric filters for %s: %w", logGroupName, err)
	}
	return len(out.MetricFilters) > 0, nil
}

func (s *CloudWatchScanner) hasSubscriptionFilters(ctx context.Context, logGroupName string) (bool, error) {
	out, err := s.subscriptions.DescribeSubscriptionFilters(ctx, &cloudwatchlogs.DescribeSubscriptionFiltersInput{
		LogGroupName: aws.String(logGroupName),
		Limit:        aws.Int32(1),
	})
	if err != nil {
		return false, fmt.Errorf("describe subscription filters for %s: %w", logGroupName, err)
	}
	return len(out.SubscriptionFilters) > 0, nil
}

func (s *CloudWatchScanner) getIngestionMetrics(ctx context.Context, logGroupName string) (incomingBytes float64, incomingEvents float64, err error) {
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -s.lookbackDays)
	period := int32(s.lookbackDays * secondsPerDay)

	dimension := cwtypes.Dimension{
		Name:  aws.String("LogGroupName"),
		Value: aws.String(logGroupName),
	}

	bytesOut, err := s.metrics.GetMetricStatistics(ctx, &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String(cwLogsNamespace),
		MetricName: aws.String(metricIncomingBytes),
		StartTime:  aws.Time(start),
		EndTime:    aws.Time(now),
		Period:     aws.Int32(period),
		Statistics: []cwtypes.Statistic{cwtypes.StatisticSum},
		Dimensions: []cwtypes.Dimension{dimension},
	})
	if err != nil {
		return 0, 0, fmt.Errorf("get %s for %s: %w", metricIncomingBytes, logGroupName, err)
	}
	if len(bytesOut.Datapoints) > 0 && bytesOut.Datapoints[0].Sum != nil {
		incomingBytes = *bytesOut.Datapoints[0].Sum / float64(s.lookbackDays)
	}

	eventsOut, err := s.metrics.GetMetricStatistics(ctx, &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String(cwLogsNamespace),
		MetricName: aws.String(metricIncomingEvents),
		StartTime:  aws.Time(start),
		EndTime:    aws.Time(now),
		Period:     aws.Int32(period),
		Statistics: []cwtypes.Statistic{cwtypes.StatisticSum},
		Dimensions: []cwtypes.Dimension{dimension},
	})
	if err != nil {
		return 0, 0, fmt.Errorf("get %s for %s: %w", metricIncomingEvents, logGroupName, err)
	}
	if len(eventsOut.Datapoints) > 0 && eventsOut.Datapoints[0].Sum != nil {
		incomingEvents = *eventsOut.Datapoints[0].Sum
	}

	return incomingBytes, incomingEvents, nil
}
