package monitor

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// NetworkMonitor monitors network interface statistics
type NetworkMonitor struct {
	startTime     time.Time
	stopTime      time.Time
	monitoring    bool
	interfaceName string
	samples       []BandwidthSample
}

// BandwidthSample represents a single bandwidth measurement
type BandwidthSample struct {
	Timestamp time.Time
	RxBytes   int64
	TxBytes   int64
	RxRate    float64 // Mbps
	TxRate    float64 // Mbps
}

// NetworkMetrics represents aggregated network metrics
type NetworkMetrics struct {
	AverageBandwidthMbps    float64
	PeakBandwidthMbps       float64
	TotalBytesTransferred   int64
	Duration                time.Duration
	AverageRxRateMbps       float64
	AverageTxRateMbps       float64
}

// NewNetworkMonitor creates a new network monitor
func NewNetworkMonitor() *NetworkMonitor {
	return &NetworkMonitor{
		interfaceName: getDefaultInterface(),
		samples:       make([]BandwidthSample, 0),
	}
}

func getDefaultInterface() string {
	// Try to detect default network interface
	cmd := exec.Command("ip", "route", "show", "default")
	output, err := cmd.Output()
	if err == nil {
		// Parse output to find interface name
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "dev") {
				parts := strings.Fields(line)
				for i, part := range parts {
					if part == "dev" && i+1 < len(parts) {
						return parts[i+1]
					}
				}
			}
		}
	}
	
	// Try detectNetworkInterface as fallback
	iface := detectNetworkInterface()
	if iface != "" {
		return iface
	}
	
	// Ultimate fallback to common interface names
	return "eth0"
}

// Start begins network monitoring
func (nm *NetworkMonitor) Start() error {
	if nm.monitoring {
		return fmt.Errorf("network monitoring already started")
	}

	nm.startTime = time.Now()
	nm.monitoring = true
	nm.samples = make([]BandwidthSample, 0)

	// Start background monitoring goroutine
	go nm.monitorLoop()

	return nil
}

// Stop stops network monitoring and returns metrics
func (nm *NetworkMonitor) Stop() NetworkMetrics {
	if !nm.monitoring {
		return NetworkMetrics{}
	}

	nm.stopTime = time.Now()
	nm.monitoring = false

	// Wait a bit for last sample
	time.Sleep(500 * time.Millisecond)

	return nm.calculateMetrics()
}

func (nm *NetworkMonitor) monitorLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var lastRxBytes, lastTxBytes int64
	lastSampleTime := nm.startTime
	firstSample := true

	for nm.monitoring {
		select {
		case <-ticker.C:
			sample := nm.collectSample()
			if sample.RxBytes > 0 || sample.TxBytes > 0 || firstSample {
				// Calculate rate if we have previous sample
				if !firstSample && lastSampleTime.Before(sample.Timestamp) {
					elapsed := sample.Timestamp.Sub(lastSampleTime).Seconds()
					if elapsed > 0 {
						sample.RxRate = float64(sample.RxBytes-lastRxBytes) * 8 / elapsed / 1000000 // Mbps
						sample.TxRate = float64(sample.TxBytes-lastTxBytes) * 8 / elapsed / 1000000 // Mbps
					}
				}
				nm.samples = append(nm.samples, sample)
				lastRxBytes = sample.RxBytes
				lastTxBytes = sample.TxBytes
				lastSampleTime = sample.Timestamp
				firstSample = false
			}
		}
	}
}

func (nm *NetworkMonitor) collectSample() BandwidthSample {
	sample := BandwidthSample{
		Timestamp: time.Now(),
	}

	// Try to read from /sys/class/net/<interface>/statistics/
	rxPath := fmt.Sprintf("/sys/class/net/%s/statistics/rx_bytes", nm.interfaceName)
	txPath := fmt.Sprintf("/sys/class/net/%s/statistics/tx_bytes", nm.interfaceName)

	if rxData, err := exec.Command("cat", rxPath).Output(); err == nil {
		if rxBytes, err := strconv.ParseInt(strings.TrimSpace(string(rxData)), 10, 64); err == nil {
			sample.RxBytes = rxBytes
		}
	}

	if txData, err := exec.Command("cat", txPath).Output(); err == nil {
		if txBytes, err := strconv.ParseInt(strings.TrimSpace(string(txData)), 10, 64); err == nil {
			sample.TxBytes = txBytes
		}
	}

	// Fallback: try using iftop or other tools if sysfs not available
	if sample.RxBytes == 0 && sample.TxBytes == 0 {
		sample = nm.collectSampleFromIftop()
	}

	return sample
}

func (nm *NetworkMonitor) collectSampleFromIftop() BandwidthSample {
	sample := BandwidthSample{
		Timestamp: time.Now(),
	}

	// Try using iftop if available (requires sudo typically)
	// For now, we'll use a simpler approach with /proc/net/dev
	cmd := exec.Command("cat", "/proc/net/dev")
	output, err := cmd.Output()
	if err != nil {
		return sample
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, nm.interfaceName+":") {
			parts := strings.Fields(line)
			if len(parts) >= 10 {
				// Format: interface: rx_bytes rx_packets ... tx_bytes tx_packets ...
				if rxBytes, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
					sample.RxBytes = rxBytes
				}
				if txBytes, err := strconv.ParseInt(parts[9], 10, 64); err == nil {
					sample.TxBytes = txBytes
				}
			}
			break
		}
	}

	return sample
}

func (nm *NetworkMonitor) calculateMetrics() NetworkMetrics {
	if len(nm.samples) == 0 {
		return NetworkMetrics{
			Duration: nm.stopTime.Sub(nm.startTime),
		}
	}

	metrics := NetworkMetrics{
		Duration: nm.stopTime.Sub(nm.startTime),
	}

	var totalRxRate, totalTxRate float64
	var peakRate float64
	var firstRxBytes, lastRxBytes int64
	var firstTxBytes, lastTxBytes int64

	if len(nm.samples) > 0 {
		firstRxBytes = nm.samples[0].RxBytes
		firstTxBytes = nm.samples[0].TxBytes
		lastRxBytes = nm.samples[len(nm.samples)-1].RxBytes
		lastTxBytes = nm.samples[len(nm.samples)-1].TxBytes
	}

	validSamples := 0
	for _, sample := range nm.samples {
		if sample.RxRate > 0 || sample.TxRate > 0 {
			totalRxRate += sample.RxRate
			totalTxRate += sample.TxRate
			validSamples++

			totalRate := sample.RxRate + sample.TxRate
			if totalRate > peakRate {
				peakRate = totalRate
			}
		}
	}

	if validSamples > 0 {
		metrics.AverageRxRateMbps = totalRxRate / float64(validSamples)
		metrics.AverageTxRateMbps = totalTxRate / float64(validSamples)
		metrics.AverageBandwidthMbps = metrics.AverageRxRateMbps + metrics.AverageTxRateMbps
	}

	metrics.PeakBandwidthMbps = peakRate
	metrics.TotalBytesTransferred = (lastRxBytes - firstRxBytes) + (lastTxBytes - firstTxBytes)

	return metrics
}

// Try to detect network interface automatically
func detectNetworkInterface() string {
	// Try common methods
	interfaces := []string{"eth0", "ens33", "enp0s3", "wlan0"}

	cmd := exec.Command("ip", "link", "show")
	output, err := cmd.Output()
	if err == nil {
		// Parse to find active interface
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			for _, iface := range interfaces {
				if strings.Contains(line, iface+":") && strings.Contains(line, "state UP") {
					return iface
				}
			}
		}
	}

	// Fallback: try to read from /proc/net/route
	cmd = exec.Command("cat", "/proc/net/route")
	output, err = cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		if len(lines) > 1 {
			// First non-header line usually has default route interface
			parts := strings.Fields(lines[1])
			if len(parts) > 0 {
				return parts[0]
			}
		}
	}

	return "eth0" // Ultimate fallback
}
