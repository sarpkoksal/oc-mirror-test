package monitor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DownloadMonitor monitors the download progress by tracking data written to the mirror directory
type DownloadMonitor struct {
	targetDir      string
	startTime      time.Time
	stopTime       time.Time
	monitoring     bool
	samples        []DownloadSample
	mu             sync.RWMutex
	pollInterval   time.Duration
	initialBytes   int64
	progressChan   chan DownloadProgress
	showProgress   bool
}

// DownloadSample represents a single download measurement
type DownloadSample struct {
	Timestamp      time.Time `json:"Timestamp"`
	TotalBytes     int64     `json:"TotalBytes"`
	BytesDelta     int64     `json:"BytesDelta"`     // Bytes downloaded since last sample
	DownloadRateMB float64   `json:"DownloadRateMB"` // Download rate in MB/s
	FileCount      int       `json:"FileCount"`
}

// DownloadProgress represents real-time progress for display
type DownloadProgress struct {
	ElapsedTime    time.Duration `json:"ElapsedTime"`
	TotalBytes     int64         `json:"TotalBytes"`
	CurrentRateMBs float64        `json:"CurrentRateMBs"`
	AverageRateMBs float64       `json:"AverageRateMBs"`
	FileCount      int           `json:"FileCount"`
}

// DownloadMetrics represents the final download metrics
type DownloadMetrics struct {
	TotalBytesDownloaded int64            `json:"TotalBytesDownloaded"`
	TotalFiles           int               `json:"TotalFiles"`
	Duration             time.Duration     `json:"Duration"`
	AverageSpeedMBs      float64           `json:"AverageSpeedMBs"`
	PeakSpeedMBs         float64           `json:"PeakSpeedMBs"`
	MinSpeedMBs          float64           `json:"MinSpeedMBs"`
	Samples              []DownloadSample  `json:"Samples"`
	StartTime            time.Time         `json:"StartTime"`
	EndTime              time.Time         `json:"EndTime"`
}

// NewDownloadMonitor creates a new download monitor for the specified directory
func NewDownloadMonitor(targetDir string) *DownloadMonitor {
	return &DownloadMonitor{
		targetDir:    targetDir,
		samples:      make([]DownloadSample, 0),
		pollInterval: 1 * time.Second,
		showProgress: true,
	}
}

// SetPollInterval sets the polling interval for monitoring
func (dm *DownloadMonitor) SetPollInterval(interval time.Duration) {
	dm.pollInterval = interval
}

// SetShowProgress enables or disables real-time progress display
func (dm *DownloadMonitor) SetShowProgress(show bool) {
	dm.showProgress = show
}

// GetProgressChannel returns a channel for receiving progress updates
func (dm *DownloadMonitor) GetProgressChannel() <-chan DownloadProgress {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	if dm.progressChan == nil {
		dm.progressChan = make(chan DownloadProgress, 100)
	}
	return dm.progressChan
}

// Start begins monitoring the download directory
func (dm *DownloadMonitor) Start() error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.monitoring {
		return nil
	}

	// Get initial size of directory (in case it already has some data)
	dm.initialBytes = dm.getDirectorySize()

	dm.startTime = time.Now()
	dm.monitoring = true
	dm.samples = make([]DownloadSample, 0)

	if dm.progressChan == nil {
		dm.progressChan = make(chan DownloadProgress, 100)
	}

	// Start background monitoring goroutine
	go dm.monitorLoop()

	return nil
}

// Stop stops monitoring and returns the collected metrics
func (dm *DownloadMonitor) Stop() DownloadMetrics {
	dm.mu.Lock()
	dm.monitoring = false
	dm.stopTime = time.Now()
	if dm.progressChan != nil {
		close(dm.progressChan)
		dm.progressChan = nil
	}
	dm.mu.Unlock()

	// Wait a bit for last sample (use context with timeout for better control)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	<-ctx.Done()
	cancel()

	return dm.calculateMetrics()
}

// StopInterface implements Monitor interface
func (dm *DownloadMonitor) StopInterface() interface{} {
	return dm.Stop()
}

// IsMonitoring implements Monitor interface
func (dm *DownloadMonitor) IsMonitoring() bool {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.monitoring
}

// GetDuration implements Monitor interface
func (dm *DownloadMonitor) GetDuration() time.Duration {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	if !dm.monitoring {
		return dm.stopTime.Sub(dm.startTime)
	}
	return time.Since(dm.startTime)
}

// GetPollInterval implements PollingMonitor interface
func (dm *DownloadMonitor) GetPollInterval() time.Duration {
	return dm.pollInterval
}

func (dm *DownloadMonitor) monitorLoop() {
	ticker := time.NewTicker(dm.pollInterval)
	defer ticker.Stop()

	var lastBytes int64 = dm.initialBytes
	lastSampleTime := dm.startTime

	for {
		dm.mu.RLock()
		monitoring := dm.monitoring
		dm.mu.RUnlock()

		if !monitoring {
			break
		}

		select {
		case <-ticker.C:
			currentBytes, fileCount := dm.getDirectoryStats()
			currentTime := time.Now()

			bytesDelta := currentBytes - lastBytes
			elapsed := currentTime.Sub(lastSampleTime).Seconds()

			var downloadRate float64
			if elapsed > 0 {
				downloadRate = float64(bytesDelta) / elapsed / (1024 * 1024) // MB/s
			}

			sample := DownloadSample{
				Timestamp:      currentTime,
				TotalBytes:     currentBytes - dm.initialBytes, // Only count new bytes
				BytesDelta:     bytesDelta,
				DownloadRateMB: downloadRate,
				FileCount:      fileCount,
			}

			dm.mu.Lock()
			dm.samples = append(dm.samples, sample)
			dm.mu.Unlock()

			// Send progress update
			if dm.showProgress {
				dm.mu.RLock()
				progressChan := dm.progressChan
				dm.mu.RUnlock()

				if progressChan != nil {
					avgRate := dm.calculateCurrentAverageRate()
					progress := DownloadProgress{
						ElapsedTime:    currentTime.Sub(dm.startTime),
						TotalBytes:     currentBytes - dm.initialBytes,
						CurrentRateMBs: downloadRate,
						AverageRateMBs: avgRate,
						FileCount:      fileCount,
					}
					select {
					case progressChan <- progress:
					default:
						// Channel full, skip this update
					}
				}
			}

			lastBytes = currentBytes
			lastSampleTime = currentTime
		}
	}
}

// getDirectoryStats efficiently gets both size and count in a single walk
func (dm *DownloadMonitor) getDirectoryStats() (size int64, count int) {
	filepath.Walk(dm.targetDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
			count++
		}
		return nil
	})
	return size, count
}

func (dm *DownloadMonitor) getDirectorySize() int64 {
	size, _ := dm.getDirectoryStats()
	return size
}

func (dm *DownloadMonitor) getFileCount() int {
	_, count := dm.getDirectoryStats()
	return count
}

func (dm *DownloadMonitor) calculateCurrentAverageRate() float64 {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	if len(dm.samples) == 0 {
		return 0
	}

	elapsed := time.Since(dm.startTime).Seconds()
	if elapsed <= 0 {
		return 0
	}

	lastSample := dm.samples[len(dm.samples)-1]
	return float64(lastSample.TotalBytes) / elapsed / (1024 * 1024)
}

func (dm *DownloadMonitor) calculateMetrics() DownloadMetrics {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	metrics := DownloadMetrics{
		Duration:  dm.stopTime.Sub(dm.startTime),
		Samples:   make([]DownloadSample, len(dm.samples)),
		StartTime: dm.startTime,
		EndTime:   dm.stopTime,
	}

	copy(metrics.Samples, dm.samples)

	if len(dm.samples) == 0 {
		// Get final size even if no samples
		metrics.TotalBytesDownloaded = dm.getDirectorySize() - dm.initialBytes
		metrics.TotalFiles = dm.getFileCount()
		if metrics.Duration.Seconds() > 0 {
			metrics.AverageSpeedMBs = float64(metrics.TotalBytesDownloaded) / metrics.Duration.Seconds() / (1024 * 1024)
		}
		return metrics
	}

	// Get final totals from last sample
	lastSample := dm.samples[len(dm.samples)-1]
	metrics.TotalBytesDownloaded = lastSample.TotalBytes
	metrics.TotalFiles = lastSample.FileCount

	// Calculate average, peak, and min speeds
	var totalRate float64
	var peakRate float64 = 0
	var minRate float64 = -1
	validSamples := 0

	for _, sample := range dm.samples {
		if sample.DownloadRateMB >= 0 {
			totalRate += sample.DownloadRateMB
			validSamples++

			if sample.DownloadRateMB > peakRate {
				peakRate = sample.DownloadRateMB
			}
			if minRate < 0 || (sample.DownloadRateMB < minRate && sample.DownloadRateMB > 0) {
				minRate = sample.DownloadRateMB
			}
		}
	}

	if validSamples > 0 {
		metrics.AverageSpeedMBs = totalRate / float64(validSamples)
	} else if metrics.Duration.Seconds() > 0 {
		// Fallback: calculate from total bytes and duration
		metrics.AverageSpeedMBs = float64(metrics.TotalBytesDownloaded) / metrics.Duration.Seconds() / (1024 * 1024)
	}

	metrics.PeakSpeedMBs = peakRate
	if minRate < 0 {
		minRate = 0
	}
	metrics.MinSpeedMBs = minRate

	return metrics
}

// PrintSummary prints a formatted summary of the download metrics
func (m *DownloadMetrics) PrintSummary() {
	fmt.Printf("  │ ═══════════════════════════════════════════════════════════\n")
	fmt.Printf("  │ Download Summary:\n")
	fmt.Printf("  │   Total Downloaded: %s (%d bytes)\n", FormatBytesHuman(m.TotalBytesDownloaded), m.TotalBytesDownloaded)
	fmt.Printf("  │   Total Files: %d\n", m.TotalFiles)
	fmt.Printf("  │   Duration: %v\n", m.Duration.Round(time.Second))
	fmt.Printf("  │   Average Speed: %.2f MB/s\n", m.AverageSpeedMBs)
	fmt.Printf("  │   Peak Speed: %.2f MB/s\n", m.PeakSpeedMBs)
	fmt.Printf("  │   Min Speed: %.2f MB/s\n", m.MinSpeedMBs)
	fmt.Printf("  │ ═══════════════════════════════════════════════════════════\n")
}

// FormatBytesHuman formats bytes to a human-readable string with proper units
func FormatBytesHuman(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.2f TB", float64(bytes)/float64(TB))
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

