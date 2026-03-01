package azure

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/monitor/armmonitor"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/operationalinsights/armoperationalinsights"
)

// NewMonitorScannerFromCredentials creates a MonitorScanner using default Azure credentials
// (environment variables, managed identity, or Azure CLI).
func NewMonitorScannerFromCredentials(subscriptionID string, lookbackDays int) (*MonitorScanner, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("create azure credential: %w", err)
	}

	wsClient, err := armoperationalinsights.NewWorkspacesClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create workspaces client: %w", err)
	}

	metricsClient, err := armmonitor.NewMetricsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create metrics client: %w", err)
	}

	return NewMonitorScanner(
		&workspacesAdapter{client: wsClient, subscriptionID: subscriptionID},
		&metricsAdapter{client: metricsClient},
		subscriptionID,
		lookbackDays,
	), nil
}
