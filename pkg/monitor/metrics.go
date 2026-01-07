package monitor

import (
	"encoding/json"
	"fmt"
	"time"
)

// NetworkMetrics methods

// CalculateTotalBandwidth calculates total bandwidth from rx and tx
func (nm *NetworkMetrics) CalculateTotalBandwidth() float64 {
	return nm.AverageRxRateMbps + nm.AverageTxRateMbps
}

// GetEfficiency returns the efficiency ratio (average/peak)
func (nm *NetworkMetrics) GetEfficiency() float64 {
	if nm.PeakBandwidthMbps > 0 {
		return nm.AverageBandwidthMbps / nm.PeakBandwidthMbps
	}
	return 0
}

// Format returns a human-readable string representation
func (nm *NetworkMetrics) Format() string {
	return fmt.Sprintf("Avg: %.2f Mbps | Peak: %.2f Mbps | Total: %s",
		nm.AverageBandwidthMbps,
		nm.PeakBandwidthMbps,
		FormatBytesHuman(nm.TotalBytesTransferred))
}

// FormatJSON returns JSON representation
func (nm *NetworkMetrics) FormatJSON() (string, error) {
	data, err := json.MarshalIndent(nm, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ResourceMetrics methods

// CalculateTotalMemoryMB returns total memory usage in MB
func (rm *ResourceMetrics) CalculateTotalMemoryMB() float64 {
	return rm.MemoryAvgMB
}

// GetCPUUtilization returns CPU utilization as a ratio (0-1)
func (rm *ResourceMetrics) GetCPUUtilization() float64 {
	return rm.CPUAvgPercent / 100.0
}

// GetMemoryUtilization returns memory utilization as a ratio (0-1)
func (rm *ResourceMetrics) GetMemoryUtilization() float64 {
	if rm.MemoryPeakMB > 0 {
		return rm.MemoryAvgMB / rm.MemoryPeakMB
	}
	return 0
}

// Format returns a human-readable string representation
func (rm *ResourceMetrics) Format() string {
	return fmt.Sprintf("CPU: Avg %.2f%% | Peak %.2f%% | Memory: Avg %.2f MB | Peak %.2f MB",
		rm.CPUAvgPercent,
		rm.CPUPeakPercent,
		rm.MemoryAvgMB,
		rm.MemoryPeakMB)
}

// FormatJSON returns JSON representation
func (rm *ResourceMetrics) FormatJSON() (string, error) {
	data, err := json.MarshalIndent(rm, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// PrintSummary prints a formatted summary of resource metrics
func (rm *ResourceMetrics) PrintSummary() {
	fmt.Printf("  │ ─── Resource Usage ───────────────────────────────────────────\n")
	fmt.Printf("  │   CPU Avg: %.2f%% | Peak: %.2f%%\n", rm.CPUAvgPercent, rm.CPUPeakPercent)
	fmt.Printf("  │   Memory Avg: %.2f MB | Peak: %.2f MB\n", rm.MemoryAvgMB, rm.MemoryPeakMB)
	fmt.Printf("  │   Goroutines Avg: %.0f | Peak: %d\n", rm.AvgGoroutines, rm.PeakGoroutines)
	fmt.Printf("  │   Threads Avg: %.0f | Peak: %d\n", rm.AvgThreads, rm.PeakThreads)
}

// DownloadMetrics methods

// CalculateEfficiency returns download efficiency (average/peak)
func (dm *DownloadMetrics) CalculateEfficiency() float64 {
	if dm.PeakSpeedMBs > 0 {
		return dm.AverageSpeedMBs / dm.PeakSpeedMBs
	}
	return 0
}

// GetTotalTime returns the duration as a formatted string
func (dm *DownloadMetrics) GetTotalTime() string {
	return dm.Duration.String()
}

// Format returns a human-readable string representation
func (dm *DownloadMetrics) Format() string {
	return fmt.Sprintf("Total: %s | Avg Speed: %.2f MB/s | Peak: %.2f MB/s | Files: %d",
		FormatBytesHuman(dm.TotalBytesDownloaded),
		dm.AverageSpeedMBs,
		dm.PeakSpeedMBs,
		dm.TotalFiles)
}

// FormatJSON returns JSON representation
func (dm *DownloadMetrics) FormatJSON() (string, error) {
	data, err := json.MarshalIndent(dm, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// OutputMetrics methods

// GetAverageFileSize returns average file size in bytes
func (om *OutputMetrics) GetAverageFileSize() int64 {
	if om.TotalFiles > 0 {
		return om.TotalSize / int64(om.TotalFiles)
	}
	return 0
}

// GetSizePerDirectory returns average size per directory
func (om *OutputMetrics) GetSizePerDirectory() int64 {
	if om.TotalDirs > 0 {
		return om.TotalSize / int64(om.TotalDirs)
	}
	return 0
}

// Format returns a human-readable string representation
func (om *OutputMetrics) Format() string {
	return fmt.Sprintf("Size: %s | Files: %d | Dirs: %d | Layers: %d | Manifests: %d",
		FormatBytesHuman(om.TotalSize),
		om.TotalFiles,
		om.TotalDirs,
		om.LayerCount,
		om.ManifestCount)
}

// FormatJSON returns JSON representation
func (om *OutputMetrics) FormatJSON() (string, error) {
	data, err := json.MarshalIndent(om, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// FormatDuration formats a duration in a human-readable way
func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
	if d < time.Hour {
		minutes := d / time.Minute
		seconds := (d % time.Minute) / time.Second
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	hours := d / time.Hour
	minutes := (d % time.Hour) / time.Minute
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

