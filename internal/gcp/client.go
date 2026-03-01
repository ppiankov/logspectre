package gcp

import (
	"context"
	"fmt"

	"google.golang.org/api/logging/v2"
	"google.golang.org/api/monitoring/v3"
	"google.golang.org/api/option"
)

// NewLoggingScannerFromCredentials creates a LoggingScanner using default credentials
// or GOOGLE_APPLICATION_CREDENTIALS.
func NewLoggingScannerFromCredentials(ctx context.Context, projectID string, lookbackDays int, opts ...option.ClientOption) (*LoggingScanner, error) {
	loggingSvc, err := logging.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create logging service: %w", err)
	}

	monitoringSvc, err := monitoring.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create monitoring service: %w", err)
	}

	return NewLoggingScanner(
		&loggingBucketsAdapter{svc: loggingSvc},
		&loggingSinksAdapter{svc: loggingSvc},
		&monitoringAdapter{svc: monitoringSvc},
		projectID,
		lookbackDays,
	), nil
}
