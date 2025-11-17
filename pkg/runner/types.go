package runner

import (
	"time"

	"github.com/telco-core/ngc-495/pkg/monitor"
)

// TestResult represents the results of a single test iteration
type TestResult struct {
	Iteration      int                    `json:"iteration"`
	IsCleanRun     bool                   `json:"is_clean_run"`
	Version        string                 `json:"version"` // "v1" or "v2"
	DownloadPhase  PhaseMetrics           `json:"download_phase"`
	UploadPhase    PhaseMetrics           `json:"upload_phase"`
	NetworkMetrics monitor.NetworkMetrics `json:"network_metrics"`
	Summary        string                 `json:"summary"`
}

// PhaseMetrics represents metrics for a single phase (download or upload)
type PhaseMetrics struct {
	WallTime      time.Duration `json:"wall_time_seconds"`
	BytesUploaded int64         `json:"bytes_uploaded"`
	Logs          []string      `json:"logs"`
	ImagesSkipped int           `json:"images_skipped"`
	CacheHits     int           `json:"cache_hits"`
}

// ComparisonResult represents comparison between v1 and v2 or clean vs cached
type ComparisonResult struct {
	Type              string        `json:"type"` // "v1_v2" or "clean_cached"
	DownloadTimeDiff  time.Duration `json:"download_time_diff"`
	UploadTimeDiff    time.Duration `json:"upload_time_diff"`
	DownloadTimeDiffPct float64     `json:"download_time_diff_percent"`
	UploadTimeDiffPct   float64     `json:"upload_time_diff_percent"`
	BytesDiff         int64         `json:"bytes_diff"`
	CacheHitsDiff     int           `json:"cache_hits_diff"`
	NetworkDiff       NetworkComparison `json:"network_diff"`
}

// NetworkComparison compares network metrics
type NetworkComparison struct {
	AvgBandwidthDiff float64 `json:"avg_bandwidth_diff_mbps"`
	PeakBandwidthDiff float64 `json:"peak_bandwidth_diff_mbps"`
	BytesTransferredDiff int64 `json:"bytes_transferred_diff"`
}
