package monitor

import (
	"context"
	"time"
)

// Monitor defines the common interface for all monitoring types
// This enables polymorphism and makes the code more flexible and testable
type Monitor interface {
	// Start begins monitoring
	Start() error
	
	// Stop stops monitoring and returns aggregated metrics as interface{}
	// Each monitor type should implement Stop() returning its specific metrics type,
	// and also implement StopInterface() for the interface
	StopInterface() interface{}
	
	// IsMonitoring returns whether monitoring is currently active
	IsMonitoring() bool
	
	// GetDuration returns the duration of monitoring
	GetDuration() time.Duration
}

// StartableMonitor extends Monitor with context-aware start
type StartableMonitor interface {
	Monitor
	StartWithContext(ctx context.Context) error
}

// PollingMonitor extends Monitor with polling interval control
type PollingMonitor interface {
	Monitor
	SetPollInterval(interval time.Duration)
	GetPollInterval() time.Duration
}

// MetricsCalculator defines interface for types that can calculate metrics
type MetricsCalculator interface {
	// CalculateMetrics computes and returns aggregated metrics
	CalculateMetrics() interface{}
	
	// GetSampleCount returns the number of samples collected
	GetSampleCount() int
}

// Formatter defines interface for types that can format their output
type Formatter interface {
	// Format returns a human-readable string representation
	Format() string
	
	// FormatJSON returns a JSON string representation
	FormatJSON() (string, error)
}

// Ensure all monitors implement the Monitor interface
var (
	_ Monitor = (*NetworkMonitor)(nil)
	_ Monitor = (*ResourceMonitor)(nil)
	_ Monitor = (*DownloadMonitor)(nil)
	_ Monitor = (*DiskWriteMonitor)(nil)
	_ Monitor = (*RegistryMonitor)(nil)
)

// Ensure monitors implement PollingMonitor where applicable
var (
	_ PollingMonitor = (*ResourceMonitor)(nil)
	_ PollingMonitor = (*DownloadMonitor)(nil)
	_ PollingMonitor = (*DiskWriteMonitor)(nil)
	_ PollingMonitor = (*RegistryMonitor)(nil)
)

