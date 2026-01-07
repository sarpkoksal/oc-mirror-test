package monitor

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RegistryMonitor monitors bytes sent to a specific registry endpoint
// This daemon runs in the background and tracks upload traffic to the registry
type RegistryMonitor struct {
	registryHost   string
	registryPort   string
	startTime      time.Time
	stopTime       time.Time
	monitoring     bool
	samples        []RegistrySample
	mu             sync.RWMutex
	pollInterval   time.Duration
	initialTxBytes int64
	interfaceName  string
}

// RegistrySample represents a single measurement of bytes sent to registry
type RegistrySample struct {
	Timestamp    time.Time `json:"Timestamp"`
	TotalTxBytes int64     `json:"TotalTxBytes"`
	BytesDelta   int64     `json:"BytesDelta"`     // Bytes sent since last sample
	UploadRateMB float64   `json:"UploadRateMB"`   // Upload rate in MB/s
	Connections  int       `json:"Connections"`    // Number of active connections
}

// RegistryMetrics represents aggregated registry upload metrics
type RegistryMetrics struct {
	TotalBytesUploaded  int64              `json:"TotalBytesUploaded"`
	Duration            time.Duration      `json:"Duration"`
	AverageUploadRateMB float64            `json:"AverageUploadRateMB"`
	PeakUploadRateMB    float64            `json:"PeakUploadRateMB"`
	MinUploadRateMB     float64            `json:"MinUploadRateMB"`
	Samples             []RegistrySample   `json:"Samples"`
	StartTime           time.Time          `json:"StartTime"`
	EndTime             time.Time          `json:"EndTime"`
	ConnectionCount     int                `json:"ConnectionCount"`
}

// NewRegistryMonitor creates a new registry monitor for the specified registry
// registryAddr should be in format "host:port" or just "host" (defaults to port 5000)
func NewRegistryMonitor(registryAddr string) *RegistryMonitor {
	// Parse registry address
	parts := strings.Split(registryAddr, ":")
	host := parts[0]
	port := "5000" // Default port
	if len(parts) > 1 {
		port = parts[1]
	}

	return &RegistryMonitor{
		registryHost:  host,
		registryPort:  port,
		samples:       make([]RegistrySample, 0),
		pollInterval:  1 * time.Second,
		interfaceName: getDefaultInterface(),
	}
}

// SetPollInterval sets the polling interval for monitoring
func (rm *RegistryMonitor) SetPollInterval(interval time.Duration) {
	rm.pollInterval = interval
}

// Start begins monitoring registry uploads
func (rm *RegistryMonitor) Start() error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.monitoring {
		return fmt.Errorf("registry monitoring already started")
	}

	rm.startTime = time.Now()
	rm.monitoring = true
	rm.samples = make([]RegistrySample, 0)
	
	// Get initial TX bytes for the interface
	rm.initialTxBytes = rm.getInterfaceTxBytes()

	// Start background monitoring goroutine
	go rm.monitorLoop()

	return nil
}

// Stop stops monitoring and returns metrics
func (rm *RegistryMonitor) Stop() RegistryMetrics {
	rm.mu.Lock()
	rm.monitoring = false
	rm.stopTime = time.Now()
	rm.mu.Unlock()

	// Use context timeout instead of blocking sleep
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	<-ctx.Done()
	cancel()

	return rm.calculateMetrics()
}

// StopInterface implements Monitor interface
func (rm *RegistryMonitor) StopInterface() interface{} {
	return rm.Stop()
}

// IsMonitoring implements Monitor interface
func (rm *RegistryMonitor) IsMonitoring() bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.monitoring
}

// GetDuration implements Monitor interface
func (rm *RegistryMonitor) GetDuration() time.Duration {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	if !rm.monitoring {
		return rm.stopTime.Sub(rm.startTime)
	}
	return time.Since(rm.startTime)
}

// GetPollInterval implements PollingMonitor interface
func (rm *RegistryMonitor) GetPollInterval() time.Duration {
	return rm.pollInterval
}

// GetCurrentMetrics returns current metrics without stopping
func (rm *RegistryMonitor) GetCurrentMetrics() RegistryMetrics {
	return rm.getCurrentMetrics()
}

// getCurrentMetrics is the internal implementation
func (rm *RegistryMonitor) getCurrentMetrics() RegistryMetrics {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	// Calculate metrics from current samples
	metrics := RegistryMetrics{
		StartTime: rm.startTime,
		EndTime:   time.Now(),
		Duration:  time.Since(rm.startTime),
		Samples:   make([]RegistrySample, len(rm.samples)),
	}
	
	copy(metrics.Samples, rm.samples)
	
	if len(rm.samples) > 0 {
		lastSample := rm.samples[len(rm.samples)-1]
		metrics.TotalBytesUploaded = lastSample.TotalTxBytes - rm.initialTxBytes
		
		// Calculate rates
		var totalRate float64
		var peakRate float64
		var minRate float64 = -1
		validSamples := 0
		
		for _, sample := range rm.samples {
			if sample.UploadRateMB >= 0 {
				totalRate += sample.UploadRateMB
				validSamples++
				if sample.UploadRateMB > peakRate {
					peakRate = sample.UploadRateMB
				}
				if minRate < 0 || (sample.UploadRateMB < minRate && sample.UploadRateMB > 0) {
					minRate = sample.UploadRateMB
				}
			}
		}
		
		if validSamples > 0 {
			metrics.AverageUploadRateMB = totalRate / float64(validSamples)
		}
		metrics.PeakUploadRateMB = peakRate
		if minRate < 0 {
			minRate = 0
		}
		metrics.MinUploadRateMB = minRate
	}
	
	return metrics
}

func (rm *RegistryMonitor) monitorLoop() {
	ticker := time.NewTicker(rm.pollInterval)
	defer ticker.Stop()

	var lastTxBytes int64 = rm.initialTxBytes
	lastSampleTime := rm.startTime

	for {
		rm.mu.RLock()
		monitoring := rm.monitoring
		rm.mu.RUnlock()

		if !monitoring {
			break
		}

		select {
		case <-ticker.C:
			currentTxBytes := rm.getInterfaceTxBytes()
			currentTime := time.Now()
			
			// Also try to get registry-specific stats using netstat/ss
			connections := rm.getRegistryConnections()

			bytesDelta := currentTxBytes - lastTxBytes
			elapsed := currentTime.Sub(lastSampleTime).Seconds()

			var uploadRate float64
			if elapsed > 0 {
				uploadRate = float64(bytesDelta) / elapsed / (1024 * 1024) // MB/s
			}

			sample := RegistrySample{
				Timestamp:    currentTime,
				TotalTxBytes: currentTxBytes - rm.initialTxBytes,
				BytesDelta:   bytesDelta,
				UploadRateMB: uploadRate,
				Connections:  connections,
			}

			rm.mu.Lock()
			rm.samples = append(rm.samples, sample)
			rm.mu.Unlock()

			lastTxBytes = currentTxBytes
			lastSampleTime = currentTime
		}
	}
}

// getInterfaceTxBytes gets total TX bytes from the network interface
func (rm *RegistryMonitor) getInterfaceTxBytes() int64 {
	txPath := fmt.Sprintf("/sys/class/net/%s/statistics/tx_bytes", rm.interfaceName)
	
	cmd := exec.Command("cat", txPath)
	output, err := cmd.Output()
	if err != nil {
		// Fallback to /proc/net/dev
		return rm.getTxBytesFromProc()
	}

	if txBytes, err := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64); err == nil {
		return txBytes
	}

	return rm.getTxBytesFromProc()
}

// getTxBytesFromProc gets TX bytes from /proc/net/dev
func (rm *RegistryMonitor) getTxBytesFromProc() int64 {
	cmd := exec.Command("cat", "/proc/net/dev")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, rm.interfaceName+":") {
			parts := strings.Fields(line)
			if len(parts) >= 10 {
				// Format: interface: rx_bytes rx_packets ... tx_bytes tx_packets ...
				if txBytes, err := strconv.ParseInt(parts[9], 10, 64); err == nil {
					return txBytes
				}
			}
			break
		}
	}

	return 0
}

// getRegistryConnections gets the number of active connections to the registry
func (rm *RegistryMonitor) getRegistryConnections() int {
	// Try using 'ss' command first (more modern)
	cmd := exec.Command("sh", "-c", fmt.Sprintf("ss -tn state established 2>/dev/null | grep %s:%s", rm.registryHost, rm.registryPort))
	output, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		count := 0
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && strings.Contains(line, rm.registryHost+":"+rm.registryPort) {
				count++
			}
		}
		return count
	}

	// Fallback to netstat
	cmd = exec.Command("sh", "-c", fmt.Sprintf("netstat -tn 2>/dev/null | grep %s:%s", rm.registryHost, rm.registryPort))
	output, err = cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		count := 0
		for _, line := range lines {
			if strings.Contains(line, rm.registryHost+":"+rm.registryPort) {
				count++
			}
		}
		return count
	}

	return 0
}

func (rm *RegistryMonitor) calculateMetrics() RegistryMetrics {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	metrics := RegistryMetrics{
		Duration:  rm.stopTime.Sub(rm.startTime),
		Samples:   make([]RegistrySample, len(rm.samples)),
		StartTime: rm.startTime,
		EndTime:   rm.stopTime,
	}

	copy(metrics.Samples, rm.samples)

	if len(rm.samples) == 0 {
		return metrics
	}

	lastSample := rm.samples[len(rm.samples)-1]
	metrics.TotalBytesUploaded = lastSample.TotalTxBytes
	metrics.ConnectionCount = lastSample.Connections

	// Calculate average, peak, and min rates
	var totalRate float64
	var peakRate float64
	var minRate float64 = -1
	validSamples := 0

	for _, sample := range rm.samples {
		if sample.UploadRateMB >= 0 {
			totalRate += sample.UploadRateMB
			validSamples++

			if sample.UploadRateMB > peakRate {
				peakRate = sample.UploadRateMB
			}
			if minRate < 0 || (sample.UploadRateMB < minRate && sample.UploadRateMB > 0) {
				minRate = sample.UploadRateMB
			}
		}
	}

	if validSamples > 0 {
		metrics.AverageUploadRateMB = totalRate / float64(validSamples)
	} else if metrics.Duration.Seconds() > 0 {
		// Fallback: calculate from total bytes and duration
		metrics.AverageUploadRateMB = float64(metrics.TotalBytesUploaded) / metrics.Duration.Seconds() / (1024 * 1024)
	}

	metrics.PeakUploadRateMB = peakRate
	if minRate < 0 {
		minRate = 0
	}
	metrics.MinUploadRateMB = minRate

	return metrics
}

// Format returns a human-readable string representation
func (rm *RegistryMetrics) Format() string {
	return fmt.Sprintf("Registry Upload: %s | Avg: %.2f MB/s | Peak: %.2f MB/s | Connections: %d",
		FormatBytesHuman(rm.TotalBytesUploaded),
		rm.AverageUploadRateMB,
		rm.PeakUploadRateMB,
		rm.ConnectionCount)
}

// FormatJSON returns JSON representation
func (rm *RegistryMetrics) FormatJSON() (string, error) {
	// Implementation would use encoding/json
	return "", fmt.Errorf("not implemented")
}

