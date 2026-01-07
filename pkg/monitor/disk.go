package monitor

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DiskWriteMonitor monitors data being written to a directory
type DiskWriteMonitor struct {
	targetDir    string
	startTime    time.Time
	stopTime     time.Time
	monitoring   bool
	samples      []DiskWriteSample
	mu           sync.RWMutex
	pollInterval time.Duration
}

// DiskWriteSample represents a single disk write measurement
type DiskWriteSample struct {
	Timestamp  time.Time `json:"Timestamp"`
	TotalBytes int64     `json:"TotalBytes"`
	FileCount  int       `json:"FileCount"`
	WriteRate  float64   `json:"WriteRate"` // MB/s
}

// DiskWriteMetrics represents aggregated disk write metrics
type DiskWriteMetrics struct {
	TotalBytesWritten   int64              `json:"TotalBytesWritten"`
	TotalFiles          int                `json:"TotalFiles"`
	Duration            time.Duration      `json:"Duration"`
	AverageWriteRateMBs float64            `json:"AverageWriteRateMBs"`
	PeakWriteRateMBs    float64            `json:"PeakWriteRateMBs"`
	Samples             []DiskWriteSample  `json:"Samples"`
}

// NewDiskWriteMonitor creates a new disk write monitor for the specified directory
func NewDiskWriteMonitor(targetDir string) *DiskWriteMonitor {
	return &DiskWriteMonitor{
		targetDir:    targetDir,
		samples:      make([]DiskWriteSample, 0),
		pollInterval: 1 * time.Second,
	}
}

// SetPollInterval sets the polling interval for monitoring
func (dm *DiskWriteMonitor) SetPollInterval(interval time.Duration) {
	dm.pollInterval = interval
}

// Start begins monitoring the directory
func (dm *DiskWriteMonitor) Start() error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.monitoring {
		return nil
	}

	dm.startTime = time.Now()
	dm.monitoring = true
	dm.samples = make([]DiskWriteSample, 0)

	// Start background monitoring goroutine
	go dm.monitorLoop()

	return nil
}

// Stop stops monitoring and returns the collected metrics
func (dm *DiskWriteMonitor) Stop() DiskWriteMetrics {
	dm.mu.Lock()
	dm.monitoring = false
	dm.stopTime = time.Now()
	dm.mu.Unlock()

	// Use context timeout instead of blocking sleep
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	<-ctx.Done()
	cancel()

	return dm.calculateMetrics()
}

// IsMonitoring returns whether the monitor is currently active
func (dm *DiskWriteMonitor) IsMonitoring() bool {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.monitoring
}

// StopInterface implements Monitor interface
func (dm *DiskWriteMonitor) StopInterface() interface{} {
	return dm.Stop()
}

// GetDuration implements Monitor interface
func (dm *DiskWriteMonitor) GetDuration() time.Duration {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	if !dm.monitoring {
		return dm.stopTime.Sub(dm.startTime)
	}
	return time.Since(dm.startTime)
}

// GetPollInterval implements PollingMonitor interface
func (dm *DiskWriteMonitor) GetPollInterval() time.Duration {
	return dm.pollInterval
}

// GetCurrentStats returns the current statistics without stopping the monitor
func (dm *DiskWriteMonitor) GetCurrentStats() DiskWriteSample {
	return dm.collectSample()
}

func (dm *DiskWriteMonitor) monitorLoop() {
	ticker := time.NewTicker(dm.pollInterval)
	defer ticker.Stop()

	var lastBytes int64
	lastSampleTime := dm.startTime
	firstSample := true

	for {
		dm.mu.RLock()
		monitoring := dm.monitoring
		dm.mu.RUnlock()

		if !monitoring {
			break
		}

		select {
		case <-ticker.C:
			sample := dm.collectSample()

			// Calculate write rate if we have a previous sample
			if !firstSample && lastSampleTime.Before(sample.Timestamp) {
				elapsed := sample.Timestamp.Sub(lastSampleTime).Seconds()
				if elapsed > 0 {
					bytesWritten := sample.TotalBytes - lastBytes
					sample.WriteRate = float64(bytesWritten) / elapsed / (1024 * 1024) // MB/s
				}
			}

			dm.mu.Lock()
			dm.samples = append(dm.samples, sample)
			dm.mu.Unlock()

			lastBytes = sample.TotalBytes
			lastSampleTime = sample.Timestamp
			firstSample = false
		}
	}
}

func (dm *DiskWriteMonitor) collectSample() DiskWriteSample {
	sample := DiskWriteSample{
		Timestamp: time.Now(),
	}

	var totalBytes int64
	var fileCount int

	err := filepath.Walk(dm.targetDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Directory might not exist yet, that's OK
			return nil
		}
		if !info.IsDir() {
			totalBytes += info.Size()
			fileCount++
		}
		return nil
	})

	if err == nil {
		sample.TotalBytes = totalBytes
		sample.FileCount = fileCount
	}

	return sample
}

func (dm *DiskWriteMonitor) calculateMetrics() DiskWriteMetrics {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	metrics := DiskWriteMetrics{
		Duration: dm.stopTime.Sub(dm.startTime),
		Samples:  make([]DiskWriteSample, len(dm.samples)),
	}

	copy(metrics.Samples, dm.samples)

	if len(dm.samples) == 0 {
		return metrics
	}

	// Get final totals from last sample
	lastSample := dm.samples[len(dm.samples)-1]
	metrics.TotalBytesWritten = lastSample.TotalBytes
	metrics.TotalFiles = lastSample.FileCount

	// Calculate average and peak write rates
	var totalRate float64
	var peakRate float64
	validSamples := 0

	for _, sample := range dm.samples {
		if sample.WriteRate > 0 {
			totalRate += sample.WriteRate
			validSamples++

			if sample.WriteRate > peakRate {
				peakRate = sample.WriteRate
			}
		}
	}

	if validSamples > 0 {
		metrics.AverageWriteRateMBs = totalRate / float64(validSamples)
	} else if metrics.Duration.Seconds() > 0 {
		// Fallback: calculate from total bytes and duration
		metrics.AverageWriteRateMBs = float64(metrics.TotalBytesWritten) / metrics.Duration.Seconds() / (1024 * 1024)
	}

	metrics.PeakWriteRateMBs = peakRate

	return metrics
}

// FormatBytes formats bytes to a human-readable string
func FormatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return formatFloat(float64(bytes)/float64(GB)) + " GB"
	case bytes >= MB:
		return formatFloat(float64(bytes)/float64(MB)) + " MB"
	case bytes >= KB:
		return formatFloat(float64(bytes)/float64(KB)) + " KB"
	default:
		return formatFloat(float64(bytes)) + " B"
	}
}

func formatFloat(f float64) string {
	if f == float64(int64(f)) {
		return string(rune(int64(f)))
	}
	// Simple formatting without fmt to avoid import cycle
	intPart := int64(f)
	decPart := int64((f - float64(intPart)) * 100)
	if decPart < 0 {
		decPart = -decPart
	}

	result := itoa(intPart) + "."
	if decPart < 10 {
		result += "0"
	}
	result += itoa(decPart)
	return result
}

func itoa(i int64) string {
	if i == 0 {
		return "0"
	}

	negative := i < 0
	if negative {
		i = -i
	}

	var result []byte
	for i > 0 {
		result = append([]byte{byte('0' + i%10)}, result...)
		i /= 10
	}

	if negative {
		result = append([]byte{'-'}, result...)
	}

	return string(result)
}

